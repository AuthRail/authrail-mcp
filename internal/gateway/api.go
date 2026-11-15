// Package gateway integrates private gateway transport with API-owned authority.
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxBody = 512 << 10
const ticketHeader = "X-Authrail-Gateway-Ticket"

var errBoundary = errors.New("gateway operation unavailable")
var errOversized = errors.New("gateway message exceeds its limit")

type Route struct {
	Target   string `json:"target"`
	Protocol string `json:"protocol"`
	Endpoint string `json:"endpoint"`
}
type Routes struct {
	Revision string  `json:"revision"`
	Routes   []Route `json:"routes"`
}
type Resolution struct {
	Route
	Method     string    `json:"method"`
	Params     string    `json:"params"`
	DeadlineAt time.Time `json:"deadlineAt"`
	ActionID   string    `json:"actionId"`
	AttemptID  string    `json:"attemptId"`
}
type RequestCheck struct {
	Ticket    string `json:"ticket"`
	Target    string `json:"target"`
	Protocol  string `json:"protocol"`
	Method    string `json:"method"`
	Params    string `json:"params"`
	SessionID string `json:"sessionId,omitempty"`
}
type RequestPermission struct {
	Allowed       bool      `json:"allowed"`
	AttemptToken  string    `json:"attemptToken"`
	Authorization *string   `json:"authorization"`
	DeadlineAt    time.Time `json:"deadlineAt"`
}
type ResponseCheck struct {
	AttemptToken string `json:"attemptToken"`
	Target       string `json:"target"`
	Protocol     string `json:"protocol"`
	Method       string `json:"method"`
	Result       string `json:"result"`
}
type Failure struct {
	Ticket string `json:"ticket"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
}
type retained struct {
	Retained bool `json:"retained"`
}

type API struct {
	origin string
	token  string
	client *http.Client
}

func NewAPI(origin, token string, allowPrivateHTTP bool) (*API, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Path != "" && u.Path != "/") || (u.Scheme != "https" && !(allowPrivateHTTP && u.Scheme == "http")) ||
		len(token) < 32 || strings.ContainsAny(token, "\r\n\x00 ") {
		return nil, errBoundary
	}
	return &API{origin: strings.TrimSuffix(origin, "/"), token: token, client: &http.Client{
		Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (a *API) call(ctx context.Context, path string, input, output interface{}) error {
	var body io.Reader
	method := http.MethodGet
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil || len(encoded) > maxBody {
			return errOversized
		}
		body = bytes.NewReader(encoded)
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, a.origin+path, body)
	if err != nil {
		return errBoundary
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Content-Type", "application/json")
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	response, err := a.client.Do(req)
	if err != nil {
		return errBoundary
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errBoundary
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return errBoundary
	}
	if len(data) > maxBody {
		return errOversized
	}
	if err := json.Unmarshal(data, output); err != nil {
		return errBoundary
	}
	return nil
}

func (a *API) resolve(ctx context.Context, ticket string) (Resolution, error) {
	var resolution Resolution
	err := a.call(ctx, "/internal/gateway/resolve", struct {
		Ticket string `json:"ticket"`
	}{ticket}, &resolution)
	if err == nil && (!validRoute(resolution.Route) || !json.Valid([]byte(resolution.Params)) ||
		len(resolution.Params) > maxBody/2 || resolution.Method == "" || len(resolution.Method) > 128 ||
		!resolution.DeadlineAt.After(time.Now()) || resolution.DeadlineAt.After(time.Now().Add(time.Minute))) {
		err = errBoundary
	}
	return resolution, err
}

func (a *API) request(ctx context.Context, check RequestCheck) (RequestPermission, error) {
	var permission RequestPermission
	err := a.call(ctx, "/internal/gateway/request", check, &permission)
	if err == nil && (!permission.Allowed || permission.Authorization != nil &&
		(len(*permission.Authorization) > 32768 || strings.ContainsAny(*permission.Authorization, "\r\n\x00"))) {
		err = errBoundary
	}
	return permission, err
}

func (a *API) response(ctx context.Context, check ResponseCheck) error {
	var result retained
	if err := a.call(ctx, "/internal/gateway/response", check, &result); err != nil {
		return err
	}
	if !result.Retained {
		return errBoundary
	}
	return nil
}

func (a *API) failure(parent context.Context, ticket, target, kind string) bool {
	// Reporting survives a cancelled caller; it never repeats the business call.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer cancel()
	var result retained
	return a.call(ctx, "/internal/gateway/failure", Failure{ticket, target, kind}, &result) == nil && result.Retained
}

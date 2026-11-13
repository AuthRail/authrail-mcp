package protectedmcp

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	authrail "github.com/AuthRail/authrail-sdk-go"
)

// SharedStateConfig is supplied only by the credential-owning application.
// The deployment token and executor key never come from tool arguments.
type SharedStateConfig struct {
	APIURL          string
	DeploymentToken string
}

type sharedLease struct {
	owner    string
	fence    int
	revision int
	expires  time.Time
	err      error
}

type sharedStore struct {
	endpoint     string
	token        string
	organisation string
	deployment   string
	key          ed25519.PrivateKey
	keySHA256    string
	client       *http.Client
	stripes      [64]sync.Mutex
	mu           sync.Mutex
	leases       map[string]*sharedLease
	closed       bool
}

type sharedResponse struct {
	OrganisationID     string     `json:"organisationId"`
	DeploymentID       string     `json:"deploymentId"`
	Fence              int        `json:"fence"`
	Revision           int        `json:"revision"`
	LeaseExpiresAt     time.Time  `json:"leaseExpiresAt"`
	Record             *record    `json:"record"`
	DispatchDeadlineAt *time.Time `json:"dispatchDeadlineAt"`
}

var errSharedState = errors.New("shared request state is unavailable; inspect the saved request before retrying")
var errSharedConflict = errors.New("shared request changed or is owned by another executor")

func openSharedStore(runtime authrail.Config, config SharedStateConfig) (*sharedStore, error) {
	address, err := url.Parse(config.APIURL)
	if err != nil || address.Host == "" || address.User != nil || address.Opaque != "" ||
		address.RawQuery != "" || address.ForceQuery || address.Fragment != "" ||
		(address.Path != "" && address.Path != "/") || address.RawPath != "" {
		return nil, errors.New("shared request state needs a fixed AuthRail API origin")
	}
	loopback := address.Hostname() == "localhost" || address.Hostname() == "127.0.0.1" || address.Hostname() == "::1"
	if address.Scheme != "https" && !(address.Scheme == "http" && loopback) {
		return nil, errors.New("shared request state requires HTTPS or HTTP loopback")
	}
	if len(config.DeploymentToken) < 32 || strings.ContainsAny(config.DeploymentToken, " \r\n\x00") ||
		!uuid.MatchString(runtime.OrganisationID) || !uuid.MatchString(runtime.DeploymentID) ||
		len(runtime.ExecutorPrivateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("shared request state needs private deployment authentication")
	}
	client := http.Client{Timeout: 5 * time.Second}
	if runtime.HTTPClient != nil {
		client = *runtime.HTTPClient
		client.Timeout = 5 * time.Second
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	key := append(ed25519.PrivateKey(nil), runtime.ExecutorPrivateKey...)
	fingerprint := sha256.Sum256(key.Public().(ed25519.PublicKey))
	return &sharedStore{endpoint: strings.TrimRight(config.APIURL, "/") + "/runtime/deployment/execution-requests",
		token: config.DeploymentToken, organisation: runtime.OrganisationID, deployment: runtime.DeploymentID,
		key: key, keySHA256: hex.EncodeToString(fingerprint[:]), client: &client, leases: make(map[string]*sharedLease)}, nil
}

func (s *sharedStore) request(command any) (sharedResponse, error) {
	var result sharedResponse
	commandBytes, err := json.Marshal(command)
	if err != nil || len(commandBytes) > 640*1024 {
		return result, errSharedState
	}
	issued := time.Now().Unix()
	expires := issued + 30
	digest := sha256.Sum256(commandBytes)
	statement := strings.Join([]string{"authrail.execution.request.v1", s.organisation, s.deployment,
		hex.EncodeToString(digest[:]), strconv.FormatInt(issued, 10), strconv.FormatInt(expires, 10)}, "\n")
	body, err := json.Marshal(struct {
		Command   string `json:"command"`
		KeySHA256 string `json:"keySha256"`
		Proof     struct {
			IssuedAt  int64  `json:"issuedAt"`
			ExpiresAt int64  `json:"expiresAt"`
			Signature string `json:"signature"`
		} `json:"proof"`
	}{Command: base64.StdEncoding.EncodeToString(commandBytes), KeySHA256: s.keySHA256,
		Proof: struct {
			IssuedAt  int64  `json:"issuedAt"`
			ExpiresAt int64  `json:"expiresAt"`
			Signature string `json:"signature"`
		}{issued, expires, base64.RawURLEncoding.EncodeToString(ed25519.Sign(s.key, []byte(statement)))}})
	if err != nil {
		return result, errSharedState
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return result, errSharedState
	}
	request.Header.Set("Authorization", "Bearer "+s.token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return result, errSharedState
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusConflict {
		return result, errSharedConflict
	}
	if response.StatusCode != http.StatusOK {
		return result, errSharedState
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 512*1024+1))
	if err != nil || len(raw) > 512*1024 || decodeExact(raw, &result) != nil ||
		result.OrganisationID != s.organisation || result.DeploymentID != s.deployment ||
		result.Fence < 0 || result.Revision < 0 || result.Record != nil && !validSharedRecord(result.Record) {
		return result, errSharedState
	}
	if result.Record != nil {
		result.Record.DispatchDeadline = result.DispatchDeadlineAt
	}
	return result, nil
}

func (s *sharedStore) acquire(key string) func() {
	digest := sha256.Sum256([]byte(key))
	stripe := &s.stripes[int(digest[0])%len(s.stripes)]
	stripe.Lock()
	lease := &sharedLease{}
	owner := make([]byte, 32)
	if _, err := rand.Read(owner); err != nil {
		lease.err = errSharedState
	} else {
		lease.owner = hex.EncodeToString(owner)
	}
	s.mu.Lock()
	if s.closed {
		lease.err = errSharedState
	}
	s.leases[key] = lease
	s.mu.Unlock()
	if lease.err == nil {
		response, err := s.request(struct {
			Kind       string `json:"kind"`
			Key        string `json:"key"`
			OwnerToken string `json:"ownerToken"`
		}{"claim", key, lease.owner})
		if err != nil || response.Fence < 1 || !response.LeaseExpiresAt.After(time.Now()) {
			lease.err = errSharedState
		} else {
			lease.fence, lease.revision, lease.expires = response.Fence, response.Revision, response.LeaseExpiresAt
		}
	}
	return func() {
		if lease.fence > 0 {
			_, _ = s.request(struct {
				Kind       string `json:"kind"`
				Key        string `json:"key"`
				OwnerToken string `json:"ownerToken"`
				Fence      int    `json:"fence"`
			}{"release", key, lease.owner, lease.fence})
		}
		s.mu.Lock()
		delete(s.leases, key)
		s.mu.Unlock()
		stripe.Unlock()
	}
}

func (s *sharedStore) ownershipError(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lease := s.leases[key]
	if s.closed || lease == nil || lease.err != nil || lease.fence < 1 || !lease.expires.After(time.Now()) {
		return errSharedState
	}
	return nil
}

func (s *sharedStore) load(key string) (*record, error) {
	response, err := s.request(struct {
		Kind string `json:"kind"`
		Key  string `json:"key"`
	}{"read", key})
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if lease := s.leases[key]; lease != nil && lease.err == nil {
		if response.Fence != lease.fence {
			lease.err = errSharedConflict
		} else {
			lease.revision = response.Revision
		}
	}
	s.mu.Unlock()
	return response.Record, nil
}

func (s *sharedStore) save(key string, value *record) error {
	if err := s.ownershipError(key); err != nil {
		return err
	}
	if !validSharedRecord(value) {
		return errors.New("shared recovery requires the exact request and authenticated identity")
	}
	body, err := json.Marshal(value)
	if err != nil || len(body) > 256*1024 {
		return errors.New("recovery state exceeds its bound")
	}
	s.mu.Lock()
	lease := s.leases[key]
	s.mu.Unlock()
	command := struct {
		Kind       string `json:"kind"`
		Key        string `json:"key"`
		OwnerToken string `json:"ownerToken"`
		Fence      int    `json:"fence"`
		Revision   int    `json:"revision"`
		Record     string `json:"record"`
	}{"save", key, lease.owner, lease.fence, lease.revision, string(body)}
	response, err := s.request(command)
	// A worker may have recorded an overdue result. Never retry a dispatch marker.
	if errors.Is(err, errSharedConflict) && value.State != "started" {
		current, readErr := s.request(struct {
			Kind string `json:"kind"`
			Key  string `json:"key"`
		}{"read", key})
		if readErr == nil && current.Fence == lease.fence && s.ownershipError(key) == nil {
			command.Revision = current.Revision
			response, err = s.request(command)
		}
	}
	if err == nil && (response.Fence != lease.fence || response.Revision <= command.Revision ||
		value.State == "started" && (response.Record == nil || response.Record.State != "started" ||
			response.DispatchDeadlineAt == nil || !response.DispatchDeadlineAt.After(time.Now()))) {
		err = errSharedState
	}
	if err != nil {
		s.mu.Lock()
		lease.err = err
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	lease.revision = response.Revision
	s.mu.Unlock()
	value.DispatchDeadline = response.DispatchDeadlineAt
	return nil
}

func validSharedRecord(value *record) bool {
	if value == nil || len(value.Arguments) == 0 || len(value.Arguments) > 65536 ||
		!uuid.MatchString(value.OperationVersionID) || !requestKey.MatchString(value.PrincipalID) ||
		!requestKey.MatchString(value.SessionID) || !digest.MatchString(value.Binding) ||
		!digest.MatchString(value.Intent) || value.ActionID != "" && !uuid.MatchString(value.ActionID) ||
		value.ApprovalID != "" && !uuid.MatchString(value.ApprovalID) ||
		value.ResourceID != "" && !requestKey.MatchString(value.ResourceID) ||
		value.ResourceReference != "" && !requestKey.MatchString(value.ResourceReference) {
		return false
	}
	intent := sha256.Sum256([]byte(value.Arguments))
	var arguments map[string]json.RawMessage
	if hex.EncodeToString(intent[:]) != value.Intent || json.Unmarshal([]byte(value.Arguments), &arguments) != nil || arguments == nil {
		return false
	}
	switch value.State {
	case "checking", "awaiting_approval", "claim_unknown", "started", "completed", "blocked", "result_withheld",
		"outcome_unknown", "rejected", "cancelled", "expired", "succeeded", "failed", "reconciled_succeeded", "reconciled_failed":
		return true
	default:
		return false
	}
}

func (s *sharedStore) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

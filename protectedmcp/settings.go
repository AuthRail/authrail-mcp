package protectedmcp

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	authrail "github.com/AuthRail/authrail-sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/sys/unix"
)

// Settings contains reviewed non-secret configuration and private file references.
type Settings struct {
	AuthRailDirectory   string                  `json:"authrailDirectory"`
	SidecarURL          string                  `json:"sidecarUrl"`
	PrincipalID         string                  `json:"principalId"`
	SessionID           string                  `json:"sessionId"`
	StateDirectory      string                  `json:"stateDirectory"`
	DownstreamURL       string                  `json:"downstreamUrl"`
	DownstreamTokenFile string                  `json:"downstreamTokenFile"`
	AllowLoopback       bool                    `json:"allowLoopback,omitempty"`
	Bindings            []Binding               `json:"bindings"`
	Sources             []SourceSettings        `json:"dataSources,omitempty"`
	ResourceLookup      *ResourceLookupSettings `json:"resourceLookup,omitempty"`
}

// SourceSettings binds supported automatic readers to operator-private credentials.
type SourceSettings struct {
	SourceVersionID   string `json:"sourceVersionId"`
	Kind              string `json:"kind"`
	ProviderAccountID string `json:"providerAccountId,omitempty"`
	SourceIdentity    string `json:"sourceIdentity,omitempty"`
	AccountBoundary   string `json:"accountBoundary,omitempty"`
	Lookup            string `json:"lookup,omitempty"`
	Endpoint          string `json:"endpoint,omitempty"`
	TokenFile         string `json:"tokenFile"`
	ConnectedAccount  bool   `json:"connectedAccount,omitempty"`
	Sandbox           bool   `json:"sandbox,omitempty"`
}

// ResourceLookupSettings reads authenticated resource ownership through an existing application API.
type ResourceLookupSettings struct {
	Endpoint        string `json:"endpoint"`
	TokenFile       string `json:"tokenFile"`
	AccountBoundary string `json:"accountBoundary"`
	MaxAgeSeconds   int    `json:"maxAgeSeconds"`
}

func readPrivate(path string, limit int64) ([]byte, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("private file paths must be absolute")
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, errors.New("private file cannot be opened")
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > limit {
		return nil, errors.New("private file must be a bounded owner-only regular file")
	}
	body, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, errors.New("private file exceeds its bound")
	}
	return body, nil
}

// LoadSettings reads a private configuration file without consulting .env files.
func LoadSettings(path string) (Settings, error) {
	var settings Settings
	body, err := readPrivate(path, 256*1024)
	if err != nil {
		return settings, err
	}
	if decodeExact(body, &settings) != nil {
		return settings, errors.New("connector configuration is invalid")
	}
	return settings, nil
}
func privateToken(path string) (string, error) {
	body, err := readPrivate(path, 8192)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(body))
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return "", errors.New("private credential is invalid")
	}
	return token, nil
}

type fixedTransport struct {
	origin, path, token string
	base                http.RoundTripper
}

func (t fixedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme+"://"+request.URL.Host != t.origin || request.URL.EscapedPath() != t.path || request.URL.User != nil {
		return nil, errors.New("downstream destination changed")
	}
	clone := request.Clone(request.Context())
	clone.Header = clone.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+t.token)
	clone.Header.Del("Cookie")
	response, err := t.base.RoundTrip(clone)
	if err != nil {
		return nil, err
	}
	response.Body = &boundedBody{ReadCloser: response.Body, left: 1 << 20}
	return response, nil
}

type boundedBody struct {
	io.ReadCloser
	left int64
}

func (b *boundedBody) Read(target []byte) (int, error) {
	if b.left <= 0 {
		return 0, errors.New("downstream response exceeds its bound")
	}
	if int64(len(target)) > b.left {
		target = target[:b.left]
	}
	n, err := b.ReadCloser.Read(target)
	b.left -= int64(n)
	return n, err
}

// ConnectDownstream fixes one authenticated HTTP MCP origin; tool calls are never automatically retried.
func ConnectDownstream(ctx context.Context, settings Settings) (*mcp.ClientSession, error) {
	endpoint, err := url.Parse(settings.DownstreamURL)
	if err != nil || endpoint.User != nil || endpoint.Hostname() == "" || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.Opaque != "" || endpoint.RawPath != "" {
		return nil, errors.New("downstream MCP URL is invalid")
	}
	if endpoint.Path == "" {
		endpoint.Path = "/"
	}
	local := endpoint.Hostname() == "127.0.0.1" || endpoint.Hostname() == "localhost" || endpoint.Hostname() == "::1"
	if endpoint.Scheme != "https" && !(settings.AllowLoopback && endpoint.Scheme == "http" && local) {
		return nil, errors.New("downstream MCP requires HTTPS")
	}
	token, err := privateToken(settings.DownstreamTokenFile)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Transport: fixedTransport{endpoint.Scheme + "://" + endpoint.Host, endpoint.EscapedPath(), token, http.DefaultTransport}, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return mcp.NewClient(&mcp.Implementation{Name: "authrail-protected-mcp", Version: "0.1.0"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint.String(), HTTPClient: client, MaxRetries: -1, DisableStandaloneSSE: true}, nil)
}

// Open reuses the existing sidecar setup identity and keys; it never obtains a deployment token.
func Open(ctx context.Context, settings Settings, downstream ToolClient) (*Connector, error) {
	if !filepath.IsAbs(settings.AuthRailDirectory) || !filepath.IsAbs(settings.StateDirectory) {
		return nil, errors.New("private AuthRail and state directories must be absolute")
	}
	body, err := readPrivate(filepath.Join(settings.AuthRailDirectory, "connection.json"), 16384)
	if err != nil {
		return nil, err
	}
	var connection struct {
		OrganisationID string `json:"organisationId"`
		DeploymentID   string `json:"deploymentId"`
	}
	if json.Unmarshal(body, &connection) != nil {
		return nil, errors.New("sidecar setup connection is invalid")
	}
	token, err := privateToken(filepath.Join(settings.AuthRailDirectory, "adapter-token"))
	if err != nil {
		return nil, err
	}
	encoded, err := readPrivate(filepath.Join(settings.AuthRailDirectory, "executor-private.pem"), 8192)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(encoded)
	if block == nil {
		return nil, errors.New("application signing key is invalid")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("application signing key is invalid")
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("application requires an Ed25519 signing key")
	}
	if err = os.MkdirAll(settings.StateDirectory, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(settings.StateDirectory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("connector state directory must be private")
	}
	keyPath := filepath.Join(settings.StateDirectory, "journal-key")
	journalKey, err := readPrivate(keyPath, 32)
	if err != nil {
		// Exclusive creation never replaces a retained recovery key.
		if _, statErr := os.Lstat(keyPath); !os.IsNotExist(statErr) {
			return nil, errors.New("retained journal key is unavailable")
		}
		journalKey = make([]byte, 32)
		if _, err = io.ReadFull(rand.Reader, journalKey); err != nil {
			return nil, err
		}
		file, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return nil, err
		}
		_, err = file.Write(journalKey)
		if err == nil {
			err = file.Sync()
		}
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			return nil, errors.New("cannot retain journal key")
		}
		directory, syncErr := os.Open(settings.StateDirectory)
		if syncErr != nil {
			return nil, errors.New("cannot retain journal key directory")
		}
		syncErr = directory.Sync()
		directory.Close()
		if syncErr != nil {
			return nil, errors.New("cannot retain journal key directory")
		}
	}
	config := Config{Runtime: authrail.Config{SidecarURL: settings.SidecarURL, AdapterToken: token, OrganisationID: connection.OrganisationID, DeploymentID: connection.DeploymentID, ExecutorPrivateKey: key}, PrincipalID: settings.PrincipalID, SessionID: settings.SessionID, StateDirectory: settings.StateDirectory, JournalKey: journalKey, Bindings: settings.Bindings, Downstream: downstream}
	// An explicit empty registry still checks the deployment's Data requirements.
	config.Runtime.DataSources = []authrail.DataSourceBinding{}
	for _, source := range settings.Sources {
		spec := source
		binding := authrail.DataSourceBinding{SourceVersionID: spec.SourceVersionID, Kind: spec.Kind, ProviderAccountID: spec.ProviderAccountID, SourceIdentity: spec.SourceIdentity, AccountBoundary: spec.AccountBoundary, Lookup: spec.Lookup, ConnectedAccount: spec.ConnectedAccount, Sandbox: spec.Sandbox, ResolveResource: func(_ context.Context, identity authrail.DataResourceContext) (string, error) {
			if identity.ResourceID == "" {
				return "", errors.New("trusted resource is unavailable")
			}
			return identity.ResourceID, nil
		}}
		getter := func(context.Context) (string, error) { return privateToken(spec.TokenFile) }
		switch spec.Kind {
		case "stripe":
			binding.SecretKeyProvider = getter
		case "quickbooks":
			binding.AccessTokenProvider = getter
		case "customer_api":
			credential, err := getter(ctx)
			if err != nil {
				return nil, err
			}
			binding.Read, err = authrail.NewCustomerAPIReader(authrail.CustomerAPIConfig{Endpoint: spec.Endpoint, BearerToken: credential, SourceIdentity: spec.SourceIdentity, AccountBoundary: spec.AccountBoundary, Lookup: spec.Lookup})
			if err != nil {
				return nil, err
			}
		default:
			return nil, errors.New("this command supports provider/API readers; embed a named application reader for other source kinds")
		}
		config.Runtime.DataSources = append(config.Runtime.DataSources, binding)
	}
	if settings.ResourceLookup != nil {
		spec := settings.ResourceLookup
		if spec.MaxAgeSeconds < 1 || spec.MaxAgeSeconds > 3600 {
			return nil, errors.New("resource lookup requires a reviewed freshness limit of 1–3600 seconds")
		}
		credential, err := privateToken(spec.TokenFile)
		if err != nil {
			return nil, err
		}
		reader, err := authrail.NewCustomerAPIReader(authrail.CustomerAPIConfig{Endpoint: spec.Endpoint, BearerToken: credential, SourceIdentity: "protected.resources", AccountBoundary: spec.AccountBoundary, Lookup: "resource.read"})
		if err != nil {
			return nil, err
		}
		config.ResolveResource = func(ctx context.Context, binding Binding, args map[string]any) (string, error) {
			reference, ok := args[binding.ResourceField].(string)
			if !ok {
				return "", errors.New("resource reference is missing")
			}
			result, err := reader(ctx, reference)
			if err != nil || result.AccountBoundary != spec.AccountBoundary || result.LookupKey != reference || result.ObservedAt.IsZero() || time.Since(result.ObservedAt) > time.Duration(spec.MaxAgeSeconds)*time.Second || result.ObservedAt.After(time.Now().Add(5*time.Second)) {
				return "", errors.New("resource lookup binding is invalid")
			}
			principal, ok := result.Value["principalId"].(string)
			resource, valid := result.Value["resourceId"].(string)
			if !ok || !valid || principal != settings.PrincipalID {
				return "", errors.New("resource is not bound to this authenticated principal")
			}
			return resource, nil
		}
	}
	return New(ctx, config)
}

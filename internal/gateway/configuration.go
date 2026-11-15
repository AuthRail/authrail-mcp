package gateway

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var targetPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,127}$`)

func validTarget(value string) bool { return targetPattern.MatchString(value) }
func validTicket(value string) bool {
	return len(value) >= 32 && len(value) <= 16384 && !strings.ContainsAny(value, "\r\n\x00 \t")
}
func validRoute(route Route) bool {
	u, err := url.Parse(route.Endpoint)
	return validTarget(route.Target) && (route.Protocol == "mcp" || route.Protocol == "a2a") && err == nil &&
		u.Host != "" && u.User == nil && u.Fragment == "" && u.RawQuery == "" && u.Scheme == "https"
}

type Configuration struct {
	mu       sync.RWMutex
	api      *API
	path     string
	hookHost string
	key      string
	revision string
	routes   map[string]Route
	updated  time.Time
	digest   [32]byte
}

func NewConfiguration(api *API, path, hookHost string) (*Configuration, error) {
	if !filepath.IsAbs(path) || hookHost == "" || strings.ContainsAny(hookHost, "/\r\n") {
		return nil, errBoundary
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(filepath.Dir(path), "session.key")
	key, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		data := make([]byte, 32)
		if _, err = rand.Read(data); err != nil {
			return nil, err
		}
		key = []byte(hex.EncodeToString(data))
		file, e := os.OpenFile(keyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return nil, e
		}
		_, err = file.Write(key)
		closeErr := file.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
	} else if err != nil {
		return nil, err
	}
	info, err := os.Stat(keyPath)
	if err != nil || info.Mode().Perm()&0077 != 0 || len(key) != 64 {
		return nil, errBoundary
	}
	decoded, err := hex.DecodeString(string(key))
	if err != nil || len(decoded) != 32 {
		return nil, errBoundary
	}
	configuration := &Configuration{api: api, path: path, hookHost: hookHost, key: string(key), routes: map[string]Route{}}
	_, _ = otel.Meter("authrail.gateway").Float64ObservableGauge("authrail.gateway.configuration.age", metric.WithUnit("s"),
		metric.WithFloat64Callback(func(_ context.Context, observer metric.Float64Observer) error {
			configuration.mu.RLock()
			defer configuration.mu.RUnlock()
			if !configuration.updated.IsZero() {
				observer.Observe(time.Since(configuration.updated).Seconds())
			}
			return nil
		}))
	return configuration, nil
}

func (c *Configuration) Sync(ctx context.Context) error {
	var snapshot Routes
	if err := c.api.call(ctx, "/internal/gateway/routes", nil, &snapshot); err != nil {
		return err
	}
	if snapshot.Revision == "" || len(snapshot.Revision) > 128 || len(snapshot.Routes) > 512 {
		return errBoundary
	}
	routes := make(map[string]Route, len(snapshot.Routes))
	pinned := make(map[string]string, len(snapshot.Routes))
	for _, route := range snapshot.Routes {
		if !validRoute(route) {
			return errBoundary
		}
		if _, exists := routes[route.Target]; exists {
			return errBoundary
		}
		routes[route.Target] = route
		endpoint, _ := url.Parse(route.Endpoint)
		addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", endpoint.Hostname())
		if err != nil || len(addresses) == 0 || len(addresses) > 32 {
			return errBoundary
		}
		for _, address := range addresses {
			if !publicAddress(address) {
				return errBoundary
			}
		}
		sort.Slice(addresses, func(i, j int) bool { return addresses[i].Compare(addresses[j]) < 0 })
		pinned[route.Target] = addresses[0].String()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	data, err := configurationJSON(snapshot.Routes, c.hookHost, c.key, c.transportToken(), pinned)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	if digest != c.digest {
		temporary, err := os.CreateTemp(filepath.Dir(c.path), ".gateway-*.json")
		if err != nil {
			return err
		}
		name := temporary.Name()
		defer os.Remove(name)
		if err = temporary.Chmod(0600); err == nil {
			_, err = temporary.Write(data)
		}
		if err == nil {
			err = temporary.Sync()
		}
		closeErr := temporary.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if err = os.Rename(name, c.path); err != nil {
			return err
		}
		c.digest = digest
	}
	c.revision = snapshot.Revision
	c.routes = routes
	c.updated = time.Now()
	return nil
}

func (c *Configuration) Ready() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.revision != "" && time.Since(c.updated) < 30*time.Second
}
func (c *Configuration) Matches(route Route) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	current, ok := c.routes[route.Target]
	return ok && current == route && time.Since(c.updated) < 30*time.Second
}

const dispatchHeader = "X-Authrail-Dispatch-Authorization"

func (c *Configuration) transportToken() string {
	// The gateway's private volume owns this transport key. Holding the API's
	// service credential alone must not permit bypassing the A2A dispatcher.
	mac := hmac.New(sha256.New, []byte(c.key))
	_, _ = mac.Write([]byte("authrail.gateway.transport.v1"))
	return hex.EncodeToString(mac.Sum(nil))
}

func publicAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
		return false
	}
	if address.Is6() && !netip.MustParsePrefix("2000::/3").Contains(address) {
		return false
	}
	for _, prefix := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001::/23", "2001:db8::/32", "2002::/16"} {
		if netip.MustParsePrefix(prefix).Contains(address) {
			return false
		}
	}
	return true
}

func configurationJSON(routes []Route, hookHost, key, transportToken string, pinned map[string]string) ([]byte, error) {
	configured := make([]interface{}, 0, len(routes))
	for _, route := range routes {
		endpoint, err := url.Parse(route.Endpoint)
		if err != nil {
			return nil, err
		}
		policies := map[string]interface{}{"retry": map[string]interface{}{"attempts": 0, "codes": []int{}},
			"timeout": map[string]interface{}{"requestTimeout": "60s", "backendRequestTimeout": "60s", "responseIdleTimeout": "60s"}}
		policies["authorization"] = map[string]interface{}{"rules": []interface{}{map[string]string{
			"allow": "'x-authrail-dispatch-authorization' in request.headers && request.headers['x-authrail-dispatch-authorization'] == '" + transportToken + "'"}}}
		transportPolicies := map[string]interface{}{"requestHeaderModifier": map[string]interface{}{"remove": []string{dispatchHeader, ticketHeader, "Cookie"}}}
		address, ok := pinned[route.Target]
		if !ok {
			return nil, errBoundary
		}
		port := endpoint.Port()
		if port == "" {
			port = "443"
		}
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber < 1 || portNumber > 65535 {
			return nil, errBoundary
		}
		path := endpoint.EscapedPath()
		if path == "" {
			path = "/"
		}
		transportPolicies["backendTLS"] = map[string]string{"hostname": endpoint.Hostname()}
		transportPolicies["requestHeaderModifier"] = map[string]interface{}{"remove": []string{dispatchHeader, ticketHeader, "Cookie"}, "set": map[string]string{"Host": endpoint.Host}}
		backend := map[string]interface{}{}
		if route.Protocol == "mcp" {
			policies["mcpAuthorization"] = map[string]interface{}{"rules": []string{
				"mcp.methodName in ['server/discover','initialize','notifications/initialized','tools/list','tools/call']",
			}}
			policies["mcpGuardrails"] = map[string]interface{}{"processors": []interface{}{map[string]interface{}{
				"kind": "remote", "host": hookHost, "failureMode": "failClosed",
				"requestHeaders": map[string]interface{}{"allowed": []string{ticketHeader, "traceparent", "tracestate"}},
				"metadata": map[string]string{"attemptToken": "has(mcpGuardrails.attemptToken) ? mcpGuardrails.attemptToken : ''", "sessionId": "has(mcp.sessionId) ? mcp.sessionId : ''",
					"traceparent": "has(mcpGuardrails.traceparent) ? mcpGuardrails.traceparent : ''", "tracestate": "has(mcpGuardrails.tracestate) ? mcpGuardrails.tracestate : ''"},
				"methods": map[string]string{"server/discover": "request", "initialize": "request", "notifications/initialized": "request", "tools/list": "request", "tools/call": "full"},
			}}}
			backend["mcp"] = map[string]interface{}{"statefulMode": "stateful", "prefixMode": "never", "targets": []interface{}{map[string]interface{}{
				"name": route.Target, "mcp": map[string]interface{}{"host": address, "port": portNumber, "path": path}, "policies": transportPolicies,
			}}}
		} else {
			policies["a2a"] = map[string]interface{}{}
			policies["urlRewrite"] = map[string]interface{}{"path": map[string]string{"full": path}}
			backend["host"] = net.JoinHostPort(address, port)
			backend["policies"] = transportPolicies
		}
		configured = append(configured, map[string]interface{}{"name": route.Target,
			"matches":  []interface{}{map[string]interface{}{"path": map[string]string{"exact": "/" + route.Protocol + "/" + route.Target}}},
			"policies": policies, "backends": []interface{}{backend}})
	}
	return json.MarshalIndent(map[string]interface{}{"config": map[string]interface{}{
		"session": map[string]string{"key": key}, "logging": map[string]interface{}{"format": "json"},
		"sensitiveHeaders": []string{ticketHeader, dispatchHeader, "Authorization", "Cookie"},
	},
		"frontendPolicies": map[string]interface{}{
			"accessLog": map[string]string{"filter": "false"},
			"tracing": map[string]interface{}{"host": "otel-collector:4317", "protocol": "grpc", "randomSampling": true,
				"resources": map[string]string{"service.name": "'authrail-agentgateway'"},
				"remove":    []string{"mcp.session.id", "http.request.header.authorization", "http.request.header.cookie", "url.query"}},
		},
		"binds": []interface{}{map[string]interface{}{"port": 3018, "listeners": []interface{}{map[string]interface{}{"routes": configured}}}}}, "", "  ")
}

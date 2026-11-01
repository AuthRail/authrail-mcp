package mcpserver

import (
	"context"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

type noArguments struct{}
type connection struct {
	ClientID     string        `json:"clientId"`
	Organisation NamedIdentity `json:"organisation"`
	User         NamedIdentity `json:"user"`
	Scopes       []string      `json:"scopes"`
}

// We expose only the tool groups consented to by this request’s identity.
func toolServer(backend *Backend, identity Identity) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "authrail-mcp", Version: "0.1.0"}, nil)
	annotations := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_workspace_summary", Description: "Read the organisation and account authorised for this AuthRail connection.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ noArguments) (*mcp.CallToolResult, Workspace, error) {
			token, identity := requestIdentity(ctx)
			result, err := backend.Workspace(ctx, token, "authrail_get_workspace_summary", identity)
			return nil, result, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_connection", Description: "Inspect this assistant connection's account, workspace and consented access.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ noArguments) (*mcp.CallToolResult, connection, error) {
			token, identity := requestIdentity(ctx)
			workspace, err := backend.Workspace(ctx, token, "authrail_get_connection", identity)
			if err != nil {
				return nil, connection{}, err
			}
			return nil, connection{ClientID: identity.ClientID, Organisation: workspace.Organisation, User: workspace.User, Scopes: identity.Scopes}, nil
		})
	if slices.Contains(identity.Scopes, GovernanceScope) {
		addFrameworkTools(server, backend)
		addControlTools(server, backend)
	}
	if slices.Contains(identity.Scopes, ConnectionsScope) {
		addConnectionTools(server, backend)
	}
	return server
}

// Handler serves MCP requests and checks each connection with AuthRail.
func Handler(backend *Backend) http.Handler {
	resource, _ := url.Parse(backend.resource)
	metadataURL := resource.Scheme + "://" + resource.Host + "/.well-known/oauth-protected-resource/mcp"
	transport := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		_, identity := requestIdentity(r.Context())
		return toolServer(backend, identity)
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	protected := auth.RequireBearerToken(func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		identity, err := backend.Introspect(ctx, token)
		if err != nil {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{UserID: identity.UserID + ":" + identity.OrganisationID + ":" + identity.ClientID, Scopes: identity.Scopes, Expiration: identity.ExpiresAt,
			Extra: map[string]any{"identity": identity, "token": token}}, nil
	}, &auth.RequireBearerTokenOptions{ResourceMetadataURL: metadataURL, Scopes: []string{Scope}})(transport)
	mux := http.NewServeMux()
	mux.Handle("/mcp", protected)
	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"resource": backend.resource, "authorization_servers": []string{backend.issuer}, "scopes_supported": []string{Scope, GovernanceScope, ConnectionsScope}, "bearer_methods_supported": []string{"header"}, "resource_name": "AuthRail"})
	})
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// We check Host and Origin so an unrelated website cannot reach this listener.
		// Native clients can omit Origin.
		if !strings.EqualFold(r.Host, resource.Host) {
			http.Error(w, "Unrecognised host", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != resource.Scheme+"://"+resource.Host {
			http.Error(w, "Unrecognised origin", http.StatusForbidden)
			return
		}
		if parent := r.Header.Get("traceparent"); tracePattern.MatchString(parent) && parent[3:35] != strings.Repeat("0", 32) && parent[36:52] != strings.Repeat("0", 16) {
			r = r.WithContext(context.WithValue(r.Context(), traceContextKey{}, parent))
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		mux.ServeHTTP(w, r)
	})
}
func requestIdentity(ctx context.Context) (string, Identity) {
	info := auth.TokenInfoFromContext(ctx)
	if info == nil || !slices.Contains(info.Scopes, Scope) {
		return "", Identity{}
	}
	token, _ := info.Extra["token"].(string)
	identity, _ := info.Extra["identity"].(Identity)
	return token, identity
}

package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type noArguments struct{}
type connection struct {
	MCPURL       string        `json:"mcpUrl,omitempty"`
	ClientID     string        `json:"clientId"`
	Organisation NamedIdentity `json:"organisation"`
	User         NamedIdentity `json:"user"`
	Scopes       []string      `json:"scopes"`
}

const serverInstructions = `AuthRail governs what AI agents may access, do and share in the connected workspace. Help the person in ordinary business language. Treat account setup, permission to use tools, Rail decisions and completed business effects as separate facts.

For a named provider such as GitHub, AWS, Cloudflare, Google Cloud or OpenAI, use the business-provider tools. Discover the supported providers, setup availability, account fields and permissions with authrail_list_business_providers. Prepare non-secret details with authrail_manage_business_account and let the person finish sign-in or private credential entry in AuthRail. Do not ask for secrets in chat. Use Custom MCP only when the person explicitly wants to connect their own MCP server; use the A2A tools for their own agent. Provider discovery is authoritative: do not mix in collection catalogues or invent extra providers. Report business setup availability and limitations as returned; do not apply old collection maturity labels to this workflow.

When the person describes a policy in ordinary language, help write its title and content using the Policy draft or authoring tools; do not require finished policy text or internal IDs. Find and reuse an existing matching draft before creating another. Ask only for missing business choices. Save the draft and provide its human review link.

When authorised to prepare Rails, continue through saved valid rules, saved examples and tests of the exact current revision; an unsaved proposal is optional assistance, not completion. Find and read existing Rails and drafts before creating another. Only Policy Rails need an exact published Policy source; Guard and Data Rails do not. Read the action contract and supported protections or source facts to express enforceable conditions. Operation identifiers and contract defaults describe the action; they are not missing Policy requirements. Do not invent unsupported fields, selectors or guarantees. If a requirement cannot be expressed, report the specific unsupported condition or missing business choice and continue independent supported work. A proposal error alone does not show that the Policy is insufficient. Return exact saved draft review links and test outcomes; publication, human approvals and deployment activation remain human decisions.

For requests covering every supported action, follow every catalogue page using its returned continuation until exhausted. A page size is not a capability limit. Account for every requested action, including later pages, and report any specific unavailable action instead of silently stopping at the first page.

For application integration, discover the supported Node.js, Go or Python SDK guidance with authrail_get_sdk_integrations. With authorised coding access, use that guidance to integrate the application's actual functions and sidecar; connecting this MCP server alone does not wrap another application's tools.

Discover and inspect the exact reviewed business tool before requesting an action. Check saved request status before retrying uncertain work. Prepare Policies and Rails as drafts, preserve exact versions and ask the person to review publication or approvals through the secure handoff. Do not claim that a draft, setup link, approval or reported execution proves a completed business effect. Existing compliance tools remain available under their own permissions.`

// We expose only the tool groups consented to by this request’s identity.
func toolServer(backend *Backend, identity Identity) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name: "authrail-mcp", Version: "0.1.0", Title: "AuthRail",
		Description: "Govern AI agent actions, account access and shared information with reviewed policies and Rails.",
		WebsiteURL:  "https://authrail.io",
		Icons:       []mcp.Icon{{Source: "https://res.cloudinary.com/thirtythree/image/upload/v1782793117/authrail-logo_mpzq8u.png", MIMEType: "image/png"}},
	}, &mcp.ServerOptions{Instructions: serverInstructions})
	annotations := &mcp.ToolAnnotations{ReadOnlyHint: true}
	addIntegrationTools(server, backend)
	addGuideTools(server, backend)
	if slices.Contains(identity.Scopes, InventoryReadScope) {
		addInventoryReadTools(server, backend)
	}
	if slices.Contains(identity.Scopes, InventoryManageScope) {
		addInventoryManageTools(server, backend)
	}
	if slices.Contains(identity.Scopes, ActivityReadScope) {
		addActivityReadTools(server, backend)
	}
	if slices.Contains(identity.Scopes, ActivityRecordScope) {
		addActivityRecordTools(server, backend)
	}
	addPolicyTools(server, backend, identity.Scopes)
	addComplianceConfigurationTools(server, backend, identity.Scopes)
	addComplianceVerificationTools(server, backend, identity.Scopes)
	if slices.Contains(identity.Scopes, OrganisationReadScope) {
		addOrganisationReadTools(server, backend)
		addExportListTool(server, backend, true)
	}
	if slices.Contains(identity.Scopes, OrganisationManageScope) {
		addOrganisationManageTools(server, backend)
		addExportHandoffTool(server, backend, true)
	}
	if slices.Contains(identity.Scopes, AccountReadScope) {
		addAccountReadTools(server, backend)
		addExportListTool(server, backend, false)
	}
	if slices.Contains(identity.Scopes, AccountManageScope) {
		addAccountManageTools(server, backend)
		addExportHandoffTool(server, backend, false)
	}
	if slices.Contains(identity.Scopes, NotificationsReadScope) {
		addNotificationReadTools(server, backend)
	}
	if slices.Contains(identity.Scopes, NotificationsManageScope) {
		addNotificationManageTools(server, backend)
	}
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
			return nil, connection{MCPURL: workspace.MCPURL, ClientID: identity.ClientID, Organisation: workspace.Organisation, User: workspace.User, Scopes: identity.Scopes}, nil
		})
	if slices.Contains(identity.Scopes, SupportScope) {
		addSupportTools(server, backend)
	}
	if slices.Contains(identity.Scopes, GovernanceScope) {
		addFrameworkTools(server, backend)
		addControlTools(server, backend)
		addHistoryTools(server, backend)
	}
	if slices.Contains(identity.Scopes, ConnectionsScope) {
		addCustomConnectionTools(server, backend)
		addBusinessConnectionTools(server, backend)
	}
	if slices.Contains(identity.Scopes, ConnectionsManageScope) {
		addCustomConnectionManagementTools(server, backend)
		addBusinessConnectionManagementTools(server, backend)
	}
	if slices.Contains(identity.Scopes, BusinessToolsReadScope) {
		addBusinessTools(server, backend)
	}
	if slices.Contains(identity.Scopes, BusinessExecuteScope) {
		addBusinessExecutionTools(server, backend)
	}
	if slices.Contains(identity.Scopes, RuntimeReadScope) {
		addRuntimeRecordTools(server, backend)
	}
	if slices.Contains(identity.Scopes, RuntimeDraftsScope) {
		addRuntimeDraftTools(server, backend)
		addPolicyRailTools(server, backend)
		addDataRailTools(server, backend)
		addGuardRailTools(server, backend)
	}
	if slices.Contains(identity.Scopes, RuntimeDeploymentsScope) {
		addRuntimeDeploymentTools(server, backend)
	}
	if slices.Contains(identity.Scopes, RuntimeApprovalsScope) {
		addRuntimeApprovalTools(server, backend)
	}
	return server
}

// Handler serves MCP requests and checks each connection with AuthRail.
func Handler(backend *Backend) http.Handler {
	resource, _ := url.Parse(backend.resource)
	metadataURL := resource.Scheme + "://" + resource.Host + "/.well-known/oauth-protected-resource"
	transport := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		_, identity := requestIdentity(r.Context())
		return toolServer(backend, identity)
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	protected := auth.RequireBearerToken(func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		identity, err := backend.Introspect(ctx, token)
		if err != nil {
			if errors.Is(err, errInvalidAccessToken) {
				return nil, auth.ErrInvalidToken
			}
			return nil, errors.New("AuthRail sign-in verification is temporarily unavailable; try again shortly")
		}
		return &auth.TokenInfo{UserID: identity.UserID + ":" + identity.OrganisationID + ":" + identity.ClientID, Scopes: identity.Scopes, Expiration: identity.ExpiresAt,
			Extra: map[string]any{"identity": identity, "token": token}}, nil
	}, &auth.RequireBearerTokenOptions{ResourceMetadataURL: metadataURL, Scopes: []string{Scope}})(transport)
	mux := http.NewServeMux()
	mux.Handle("/{$}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			// Request renewable sign-in without making renewal a permission required
			// from already-issued access tokens. The assistant must request it in OAuth.
			w.Header().Set("WWW-Authenticate", fmt.Sprintf("Bearer resource_metadata=%q, scope=%q", metadataURL, Scope+" offline_access"))
			http.Error(w, "no bearer token", http.StatusUnauthorized)
			return
		}
		protected.ServeHTTP(w, r)
	}))
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"resource": backend.resource, "authorization_servers": []string{backend.issuer}, "scopes_supported": []string{Scope, "offline_access", ComplianceConfigurationReadScope, ComplianceConfigurationManageScope, ComplianceVerificationReadScope, ComplianceVerificationManageScope, InventoryReadScope, InventoryManageScope, ActivityReadScope, ActivityRecordScope, PoliciesReadScope, PoliciesManageScope, OrganisationReadScope, OrganisationManageScope, AccountReadScope, AccountManageScope, NotificationsReadScope, NotificationsManageScope, SupportScope, GovernanceScope, ConnectionsScope, ConnectionsManageScope, BusinessToolsReadScope, BusinessExecuteScope, RuntimeDraftsScope, RuntimeReadScope, RuntimeDeploymentsScope, RuntimeApprovalsScope}, "bearer_methods_supported": []string{"header"}, "resource_name": "AuthRail"})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		document, err := backend.AuthorizationMetadata(r.Context())
		if err != nil {
			http.Error(w, "Sign-in discovery is temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(document)
	})
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		if !backend.Ready(r.Context()) {
			http.Error(w, "API unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reject unknown and noncanonical paths before ServeMux can redirect or
		// treat the root transport as a fallback for an old endpoint.
		switch r.URL.EscapedPath() {
		case "/", "/.well-known/oauth-protected-resource", "/.well-known/oauth-authorization-server", "/health/live", "/health/ready":
		default:
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet && r.Header.Get("Origin") == "" && localHealthRequest(r) && (r.URL.Path == "/health/live" || r.URL.Path == "/health/ready") {
			mux.ServeHTTP(w, r)
			return
		}
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
func localHealthRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
	}
	remote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	source := net.ParseIP(remote)
	destination := net.ParseIP(host)
	return source != nil && source.IsLoopback() && (host == "localhost" || destination != nil && destination.IsLoopback())
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

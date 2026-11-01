package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var frameworkID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var errReadAccess = errors.New("This connection needs governance read consent and your account’s permission for this information")

type frameworkListInput struct {
	Source string `json:"source" jsonschema:"Framework source: authrail for managed Frameworks or custom for your organisation’s Frameworks"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Page size from 1 to 20; defaults to 20"`
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque next cursor from the previous page"`
}
type frameworkInput struct {
	OrganisationFrameworkID string `json:"organisationFrameworkId" jsonschema:"Exact selected Framework workspace ID returned by authrail_list_organisation_frameworks"`
}
type readinessInput struct {
	OrganisationFrameworkID string `json:"organisationFrameworkId" jsonschema:"Exact selected Framework workspace ID"`
	Limit                   int    `json:"limit,omitempty" jsonschema:"Obligation page size from 1 to 10; defaults to 10"`
	Cursor                  string `json:"cursor,omitempty" jsonschema:"Opaque next cursor from the previous readiness page"`
}

func addFrameworkTools(server *mcp.Server, backend *Backend) {
	annotations := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_organisation_frameworks", Description: "List the exact Framework versions selected in your connected workspace. Choose authrail or custom and follow pageInfo.nextCursor for further pages.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, input frameworkListInput) (*mcp.CallToolResult, map[string]any, error) {
			if input.Source != "authrail" && input.Source != "custom" || input.Limit < 0 || input.Limit > 20 || len(input.Cursor) > 500 {
				return nil, nil, errors.New("Choose authrail or custom, a page size from 1 to 20, and the previous page’s cursor")
			}
			limit := input.Limit
			if limit == 0 {
				limit = 20
			}
			q := url.Values{"source": {input.Source}, "limit": {strconv.Itoa(limit)}}
			if input.Cursor != "" {
				q.Set("cursor", input.Cursor)
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.governance(ctx, token, "authrail_list_organisation_frameworks", "/organisation/mcp/frameworks?"+q.Encode(), identity)
			return nil, data, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_organisation_framework", Description: "Read one selected Framework’s exact version and lifecycle. Use its organisationFrameworkId, not the catalogue Framework ID.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, input frameworkInput) (*mcp.CallToolResult, map[string]any, error) {
			if !frameworkID.MatchString(input.OrganisationFrameworkID) {
				return nil, nil, errors.New("Use the selected Framework’s organisationFrameworkId")
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.governance(ctx, token, "authrail_get_organisation_framework", "/organisation/mcp/frameworks/"+input.OrganisationFrameworkID, identity)
			return nil, data, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_framework_readiness", Description: "Read stored readiness and a bounded page of applicable Obligations. Restricted, stale or unevaluated results are not passing assessments. Follow pageInfo.nextCursor to continue.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, input readinessInput) (*mcp.CallToolResult, map[string]any, error) {
			if !frameworkID.MatchString(input.OrganisationFrameworkID) || input.Limit < 0 || input.Limit > 10 || len(input.Cursor) > 512 {
				return nil, nil, errors.New("Use the selected Framework ID, a page size from 1 to 10, and the previous page’s cursor")
			}
			limit := input.Limit
			if limit == 0 {
				limit = 10
			}
			q := url.Values{"limit": {strconv.Itoa(limit)}}
			if input.Cursor != "" {
				q.Set("cursor", input.Cursor)
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.governance(ctx, token, "authrail_get_framework_readiness", "/organisation/mcp/frameworks/"+input.OrganisationFrameworkID+"/readiness?"+q.Encode(), identity)
			return nil, data, err
		})
}

// We accept only paths constructed by the named tools, never an agent-supplied API URL.
func (b *Backend) governance(ctx context.Context, token, tool, path string, identity Identity) (map[string]any, error) {
	return b.delegatedRead(ctx, token, tool, path, GovernanceScope, identity)
}

func (b *Backend) delegatedRead(ctx context.Context, token, tool, path, scope string, identity Identity) (map[string]any, error) {
	if !slices.Contains(identity.Scopes, scope) {
		return nil, errReadAccess
	}
	var delegation struct {
		AccessToken string    `json:"accessToken"`
		ExpiresAt   time.Time `json:"expiresAt"`
	}
	if err := b.call(ctx, http.MethodPost, "/auth/mcp/delegations/exchange", b.serviceToken, "", map[string]string{"token": token, "tool": tool}, &delegation); err != nil {
		return nil, err
	}
	if delegation.AccessToken == "" || !delegation.ExpiresAt.After(time.Now()) {
		return nil, errBackend
	}
	var result struct {
		OrganisationID string         `json:"organisationId"`
		UserID         string         `json:"userId"`
		Data           map[string]any `json:"data"`
	}
	if err := b.call(ctx, http.MethodGet, path, delegation.AccessToken, identity.OrganisationID, nil, &result); err != nil {
		return nil, err
	}
	if result.OrganisationID != identity.OrganisationID || result.UserID != identity.UserID || result.Data == nil {
		return nil, errBackend
	}
	return result.Data, nil
}

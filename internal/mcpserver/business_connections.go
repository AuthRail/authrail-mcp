package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Keep these public setup identifiers aligned with the server's business registry.
var businessProviderKeys = []string{"aws", "azure", "cloudflare", "github", "gitlab", "okta", "jamf", "crowdstrike", "snyk", "datadog", "splunk", "pagerduty", "snowflake", "knowbe4", "onetrust", "stripe", "openai", "azure-openai", "anthropic", "jira", "confluence", "notion", "linear", "asana", "gcp", "google-workspace"}

type businessAccountHandoffInput struct {
	Action       string `json:"action"`
	ProviderKey  string `json:"providerKey,omitempty"`
	ConnectionID string `json:"connectionId,omitempty"`
}

func addBusinessConnectionTools(server *mcp.Server, backend *Backend) {
	for _, listing := range []struct{ name, path, description string }{
		{"authrail_list_business_providers", "/organisation/mcp/connections/business/providers", "List business account providers and their saved setup availability. This does not connect an account or grant permission to run tools. Continue using pageInfo.nextCursor."},
		{"authrail_list_business_accounts", "/organisation/mcp/connections/business/accounts", "List the workspace's business accounts and saved review status. This does not contact providers or reveal credentials. Continue using pageInfo.nextCursor."},
	} {
		mcp.AddTool(server, &mcp.Tool{Name: listing.name, Description: listing.description, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}},
			func(ctx context.Context, _ *mcp.CallToolRequest, input connectionListInput) (*mcp.CallToolResult, map[string]any, error) {
				if input.Limit < 0 || input.Limit > 10 || len(input.Cursor) > 500 {
					return nil, nil, errors.New("Use a page size from 1 to 10 and the previous page's cursor")
				}
				limit := input.Limit
				if limit == 0 {
					limit = 10
				}
				query := url.Values{"limit": {strconv.Itoa(limit)}}
				if input.Cursor != "" {
					query.Set("cursor", input.Cursor)
				}
				token, identity := requestIdentity(ctx)
				data, err := backend.delegatedRead(ctx, token, listing.name, listing.path+"?"+query.Encode(), ConnectionsScope, identity)
				return nil, data, err
			})
	}
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_business_account", Description: "Read a business account's saved status and bounded tool-access summary. This does not reveal credentials or grant execution permission.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}},
		func(ctx context.Context, _ *mcp.CallToolRequest, input connectionStatusInput) (*mcp.CallToolResult, map[string]any, error) {
			if !frameworkID.MatchString(input.ConnectionID) {
				return nil, nil, errors.New("Choose the connectionId returned by authrail_list_business_accounts")
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.delegatedRead(ctx, token, "authrail_get_business_account", "/organisation/mcp/connections/business/accounts/"+input.ConnectionID, ConnectionsScope, identity)
			return nil, data, err
		})
}

func addBusinessConnectionManagementTools(server *mcp.Server, backend *Backend) {
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_manage_business_account", Description: "Open a secure AuthRail page to connect a business account, review its tools, change its access or disconnect it. Requires separate account-management consent. The person must sign in and finish the action. Never submit credentials, server addresses or tool arguments.", InputSchema: map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"action"},
		"properties": map[string]any{
			"action":       map[string]any{"type": "string", "enum": []string{"connect", "review", "access", "disconnect"}},
			"providerKey":  map[string]any{"type": "string", "enum": businessProviderKeys, "description": "Required only for connect"},
			"connectionId": map[string]any{"type": "string", "minLength": 36, "maxLength": 36, "description": "Required only for review, access or disconnect"},
		},
	}}, func(ctx context.Context, _ *mcp.CallToolRequest, input businessAccountHandoffInput) (*mcp.CallToolResult, map[string]any, error) {
		switch input.Action {
		case "connect":
			if !slices.Contains(businessProviderKeys, input.ProviderKey) || input.ConnectionID != "" {
				return nil, nil, errors.New("Choose a listed business provider to connect a new account")
			}
		case "review", "access", "disconnect":
			if !frameworkID.MatchString(input.ConnectionID) || input.ProviderKey != "" {
				return nil, nil, errors.New("Choose an existing business account")
			}
		default:
			return nil, nil, errors.New("Choose connect, review, access or disconnect")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.delegatedCall(ctx, token, "authrail_manage_business_account", http.MethodPost, "/organisation/mcp/connections/business/handoff", ConnectionsManageScope, input, identity)
		return nil, data, err
	})
}

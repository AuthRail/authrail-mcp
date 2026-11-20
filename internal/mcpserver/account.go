package mcpserver

import (
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/url"
	"unicode/utf8"
)

type accountProfileInput struct {
	IdempotencyKey string `json:"idempotencyKey" jsonschema:"Stable UUID; reuse unchanged after an uncertain response"`
	Name           string `json:"name" jsonschema:"New display name, 2 to 120 characters"`
	ExpectedName   string `json:"expectedName" jsonschema:"Exact current name from authrail_get_my_account"`
}
type accountPreferencesInput struct {
	IdempotencyKey             string  `json:"idempotencyKey"`
	ExpectedUpdatedAt          *string `json:"expectedUpdatedAt" jsonschema:"Exact preferences updatedAt from account read, or null when no preferences were saved"`
	Timezone                   string  `json:"timezone,omitempty" jsonschema:"IANA timezone to set"`
	ClearTimezone              bool    `json:"clearTimezone,omitempty" jsonschema:"Explicitly clear timezone; do not combine with timezone"`
	SecurityEmailAlertsEnabled *bool   `json:"securityEmailAlertsEnabled,omitempty"`
}
type accountHandoffInput struct {
	Action    string `json:"action" jsonschema:"One of profile_photo,password,sign_in_methods,mfa,backup_codes,sessions,revoke_session,logout,delete_account,restore_account"`
	SessionID string `json:"sessionId,omitempty" jsonschema:"Exact own active session UUID only for revoke_session"`
}

func addAccountReadTools(server *mcp.Server, backend *Backend) {
	addWorkspaceRead(server, backend, "authrail_get_my_account", "Read your authenticated account, preferences and current display name. Credentials and storage capabilities are excluded.", AccountReadScope, "/organisation/mcp/account")
	addWorkspaceRead(server, backend, "authrail_get_my_account_security", "Read your current security-method metadata. Passwords, MFA secrets and backup codes remain private.", AccountReadScope, "/organisation/mcp/account/security")
	addWorkspaceTool(server, backend, delegatedToolInput[workspacePageInput]{name: "authrail_list_my_sessions", description: "List a bounded page of your sign-in sessions without session tokens. Current identity and account.read consent apply.", scope: AccountReadScope, method: http.MethodGet, readonly: true, prepare: func(input workspacePageInput) (string, any, error) {
		q, err := workspacePage(url.Values{}, input)
		return "/organisation/mcp/account/sessions?" + q, nil, err
	}})
}
func addAccountManageTools(server *mcp.Server, backend *Backend) {
	addWorkspaceTool(server, backend, delegatedToolInput[accountProfileInput]{name: "authrail_update_my_account_profile", description: "Change your display name against its exact current value. Use a stable command UUID and retry unchanged after an uncertain response; changing input under that key conflicts.", scope: AccountManageScope, method: http.MethodPost, prepare: func(input accountProfileInput) (string, any, error) {
		if !frameworkID.MatchString(input.IdempotencyKey) || utf8.RuneCountInString(input.Name) < 2 || utf8.RuneCountInString(input.Name) > 120 || len(input.ExpectedName) == 0 {
			return "", nil, errors.New("Use a stable command UUID, current name and bounded new name")
		}
		return "/organisation/mcp/account/profile", input, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[accountPreferencesInput]{name: "authrail_update_my_account_preferences", description: "Change your timezone or optional security-email preference against the latest exact preference revision. Mandatory security channels remain governed by the account domain.", scope: AccountManageScope, method: http.MethodPost, prepare: func(input accountPreferencesInput) (string, any, error) {
		if !frameworkID.MatchString(input.IdempotencyKey) || input.ClearTimezone && input.Timezone != "" || input.Timezone == "" && !input.ClearTimezone && input.SecurityEmailAlertsEnabled == nil {
			return "", nil, errors.New("Use a stable command UUID and at least one explicit preference change")
		}
		body := map[string]any{"idempotencyKey": input.IdempotencyKey, "expectedUpdatedAt": input.ExpectedUpdatedAt}
		if input.Timezone != "" {
			body["timezone"] = input.Timezone
		}
		if input.ClearTimezone {
			body["timezone"] = nil
		}
		if input.SecurityEmailAlertsEnabled != nil {
			body["securityEmailAlertsEnabled"] = *input.SecurityEmailAlertsEnabled
		}
		return "/organisation/mcp/account/preferences", body, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[accountHandoffInput]{name: "authrail_manage_my_account", description: "Continue account security, photo, sessions, sign-in methods or lifecycle privately in AuthRail. This navigation handoff changes nothing and never returns credentials or backup codes.", scope: AccountManageScope, method: http.MethodPost, readonly: true, prepare: func(input accountHandoffInput) (string, any, error) {
		switch input.Action {
		case "profile_photo", "password", "sign_in_methods", "mfa", "backup_codes", "sessions", "revoke_session", "logout", "delete_account", "restore_account":
		default:
			return "", nil, errors.New("Select a named account workflow")
		}
		if input.Action == "revoke_session" && !frameworkID.MatchString(input.SessionID) || input.Action != "revoke_session" && input.SessionID != "" {
			return "", nil, errors.New("Select an exact own session only for revoke_session")
		}
		return "/organisation/mcp/account/handoff", input, nil
	}})
}

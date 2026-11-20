package mcpserver

import (
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/url"
	"time"
)

type notificationsPageInput struct {
	Inbox  string `json:"inbox" jsonschema:"account or organisation"`
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}
type notificationReadInput struct {
	IdempotencyKey string `json:"idempotencyKey" jsonschema:"Stable UUID for unchanged retries"`
	Inbox          string `json:"inbox" jsonschema:"account or organisation"`
	NotificationID string `json:"notificationId"`
}
type notificationsReadInput struct {
	IdempotencyKey   string `json:"idempotencyKey"`
	Inbox            string `json:"inbox" jsonschema:"account or organisation"`
	ThroughCreatedAt string `json:"throughCreatedAt" jsonschema:"Exact reviewed cutoff timestamp; later arrivals remain unread"`
}
type exportNotificationPreferencesInput struct {
	IdempotencyKey       string `json:"idempotencyKey"`
	InboxEnabled         bool   `json:"inboxEnabled"`
	EmailEnabled         bool   `json:"emailEnabled"`
	ExpectedInboxEnabled bool   `json:"expectedInboxEnabled"`
	ExpectedEmailEnabled bool   `json:"expectedEmailEnabled"`
}

func validInbox(value string) bool { return value == "account" || value == "organisation" }
func addNotificationReadTools(server *mcp.Server, backend *Backend) {
	addWorkspaceTool(server, backend, delegatedToolInput[notificationsPageInput]{name: "authrail_list_my_notifications", description: "Read only notifications addressed to you in the chosen account or workspace inbox. Internal Admin notifications are excluded.", scope: NotificationsReadScope, method: http.MethodGet, readonly: true, prepare: func(input notificationsPageInput) (string, any, error) {
		if !validInbox(input.Inbox) {
			return "", nil, errors.New("Choose account or organisation inbox")
		}
		q, err := workspacePage(url.Values{"inbox": {input.Inbox}}, workspacePageInput{Cursor: input.Cursor, Limit: input.Limit})
		return "/organisation/mcp/notifications?" + q, nil, err
	}})
	addWorkspaceRead(server, backend, "authrail_get_my_notification_counts", "Read separate unread counts for your own account and selected workspace inbox.", NotificationsReadScope, "/organisation/mcp/notifications/counts")
	addWorkspaceRead(server, backend, "authrail_get_my_export_notification_preferences", "Read your own export notification preference booleans. This does not expose internal staff preferences.", NotificationsReadScope, "/organisation/mcp/notifications/export-preferences")
}
func addNotificationManageTools(server *mcp.Server, backend *Backend) {
	addWorkspaceTool(server, backend, delegatedToolInput[notificationReadInput]{name: "authrail_mark_notification_read", description: "Mark one addressed notification read. Retain the same UUID and unchanged input after an uncertain response.", scope: NotificationsManageScope, method: http.MethodPost, prepare: func(input notificationReadInput) (string, any, error) {
		if !validInbox(input.Inbox) || !frameworkID.MatchString(input.IdempotencyKey) || !frameworkID.MatchString(input.NotificationID) {
			return "", nil, errors.New("Choose an own inbox notification and stable command UUID")
		}
		return "/organisation/mcp/notifications/read", input, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[notificationsReadInput]{name: "authrail_mark_notifications_read", description: "Mark up to500 notifications through an explicitly chosen creation-time cutoff read. Later arrivals remain unread; same-key retries retain the original changed-count. If reachedBatchLimit is true, check current counts before issuing a new command UUID.", scope: NotificationsManageScope, method: http.MethodPost, prepare: func(input notificationsReadInput) (string, any, error) {
		_, err := time.Parse(time.RFC3339Nano, input.ThroughCreatedAt)
		if !validInbox(input.Inbox) || !frameworkID.MatchString(input.IdempotencyKey) || err != nil {
			return "", nil, errors.New("Choose an inbox, stable command UUID and exact reviewed cutoff timestamp")
		}
		return "/organisation/mcp/notifications/read-through", input, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[exportNotificationPreferencesInput]{name: "authrail_update_my_export_notification_preferences", description: "Set own export inbox/email preferences against their exact current boolean values. Mandatory staff support channels are unaffected. Same-key retries retain the result.", scope: NotificationsManageScope, method: http.MethodPost, prepare: func(input exportNotificationPreferencesInput) (string, any, error) {
		if !frameworkID.MatchString(input.IdempotencyKey) {
			return "", nil, errors.New("Use a stable command UUID")
		}
		return "/organisation/mcp/notifications/export-preferences", input, nil
	}})
}

package mcpserver

import (
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/url"
)

const InventoryReadScope = "inventory.read"
const InventoryManageScope = "inventory.manage"

type inventoryProvider struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}
type inventoryFields struct {
	Name                string              `json:"name" jsonschema:"AI system business name, 2 to 180 characters"`
	Description         string              `json:"description,omitempty" jsonschema:"Optional description, up to 2000 characters"`
	Providers           []inventoryProvider `json:"providers" jsonschema:"Reported expected providers/models; configuration is not proof of execution identity"`
	Purpose             string              `json:"purpose" jsonschema:"Reviewed business purpose, 3 to 2000 characters"`
	DataClassifications []string            `json:"dataClassifications" jsonschema:"Up to20 data classifications"`
	HumanOversight      string              `json:"humanOversight" jsonschema:"Who oversees the system and how, 3 to 2000 characters"`
	DepartmentIDs       []string            `json:"departmentIds" jsonschema:"Exact active department UUIDs defining audience; empty means no department restriction"`
	TeamIDs             []string            `json:"teamIds" jsonschema:"Exact active team UUIDs defining audience; use only returned workspace IDs"`
}
type inventoryListInput struct {
	Query     string `json:"query,omitempty"`
	Lifecycle string `json:"lifecycle,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}
type inventoryItemInput struct {
	ItemID string `json:"itemId"`
}
type inventoryHistoryInput struct {
	ItemID string `json:"itemId"`
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}
type inventoryRevisionInput struct {
	ItemID     string `json:"itemId"`
	RevisionID string `json:"revisionId"`
	Section    string `json:"section,omitempty" jsonschema:"overview, providers or audience; default overview"`
	Cursor     string `json:"cursor,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}
type inventoryCreateInput struct {
	IdempotencyKey string          `json:"idempotencyKey" jsonschema:"Stable UUID reused only for identical retries"`
	Fields         inventoryFields `json:"fields"`
}
type inventoryUpdateInput struct {
	ItemID         string          `json:"itemId"`
	IdempotencyKey string          `json:"idempotencyKey"`
	Revision       int             `json:"revision" jsonschema:"Exact current revision read before editing"`
	Fields         inventoryFields `json:"fields"`
}
type inventoryLifecycleInput struct {
	ItemID           string `json:"itemId"`
	IdempotencyKey   string `json:"idempotencyKey"`
	Revision         int    `json:"revision"`
	CurrentLifecycle string `json:"currentLifecycle"`
	Lifecycle        string `json:"lifecycle"`
}
type inventoryReviewInput struct {
	ItemID   string `json:"itemId"`
	Revision int    `json:"revision"`
	Action   string `json:"action" jsonschema:"confirm, adopt_revision or discovery_setup; a person must decide in AuthRail"`
}

func validInventoryLifecycle(value string) bool {
	return value == "active" || value == "inactive" || value == "archived"
}
func addInventoryReadTools(server *mcp.Server, backend *Backend) {
	addWorkspaceTool(server, backend, delegatedToolInput[inventoryListInput]{name: "authrail_list_ai_inventory", description: "Find visible AI systems and discovered, unconfirmed integrations. Configuration and discovery do not prove execution identity or turn on protection. Continue with the returned cursor.", scope: InventoryReadScope, method: http.MethodGet, readonly: true, prepare: func(input inventoryListInput) (string, any, error) {
		if input.Limit > 20 || input.Lifecycle != "" && !validInventoryLifecycle(input.Lifecycle) {
			return "", nil, errors.New("Use a page up to20 and a valid lifecycle")
		}
		q := url.Values{}
		if input.Query != "" {
			q.Set("query", input.Query)
		}
		if input.Lifecycle != "" {
			q.Set("lifecycle", input.Lifecycle)
		}
		query, err := workspacePage(q, workspacePageInput{Cursor: input.Cursor, Limit: input.Limit})
		return "/organisation/mcp/inventory?" + query, nil, err
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[inventoryItemInput]{name: "authrail_get_ai_inventory_item", description: "Read current visible AI system identity and its revision overview. Follow exact revision sections for providers and audience, and the history tool for retained configurations.", scope: InventoryReadScope, method: http.MethodGet, readonly: true, prepare: func(input inventoryItemInput) (string, any, error) {
		if !frameworkID.MatchString(input.ItemID) {
			return "", nil, errors.New("Use an exact AI system UUID")
		}
		return "/organisation/mcp/inventory/" + input.ItemID, nil, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[inventoryHistoryInput]{name: "authrail_get_ai_inventory_history", description: "Read bounded retained configuration revision summaries for a visible AI system. Current and original revision audiences apply; use exact revision reads for content.", scope: InventoryReadScope, method: http.MethodGet, readonly: true, prepare: func(input inventoryHistoryInput) (string, any, error) {
		if !frameworkID.MatchString(input.ItemID) || input.Limit > 20 {
			return "", nil, errors.New("Use the exact AI system and page up to20")
		}
		q, err := workspacePage(url.Values{}, workspacePageInput{Cursor: input.Cursor, Limit: input.Limit})
		return "/organisation/mcp/inventory/" + input.ItemID + "/history?" + q, nil, err
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[inventoryRevisionInput]{name: "authrail_get_ai_inventory_revision", description: "Read one exact immutable AI configuration in bounded sections. Overview identifies purpose, oversight and classification; providers and audience return pages with explicit continuation. No hidden truncation.", scope: InventoryReadScope, method: http.MethodGet, readonly: true, prepare: func(input inventoryRevisionInput) (string, any, error) {
		if !frameworkID.MatchString(input.ItemID) || !frameworkID.MatchString(input.RevisionID) || input.Limit > 20 || input.Section != "" && input.Section != "overview" && input.Section != "providers" && input.Section != "audience" {
			return "", nil, errors.New("Use exact AI system/revision IDs and a supported section with page up to20")
		}
		q := url.Values{}
		if input.Section != "" {
			q.Set("section", input.Section)
		}
		query, err := workspacePage(q, workspacePageInput{Cursor: input.Cursor, Limit: input.Limit})
		return "/organisation/mcp/inventory/" + input.ItemID + "/revisions/" + input.RevisionID + "?" + query, nil, err
	}})
}

func addInventoryManageTools(server *mcp.Server, backend *Backend) {
	addWorkspaceTool(server, backend, delegatedToolInput[inventoryCreateInput]{name: "authrail_create_ai_inventory_item", description: "Create an AI system record from reviewed business details under current management permission. Reuse one UUID for unchanged retries; changed input conflicts. The record starts unconfirmed and needs a signed-in person to confirm it. This creates no deployment and grants no Runtime authority.", scope: InventoryManageScope, method: http.MethodPost, prepare: func(input inventoryCreateInput) (string, any, error) {
		if !frameworkID.MatchString(input.IdempotencyKey) {
			return "", nil, errors.New("Supply a stable command UUID")
		}
		body := struct {
			inventoryFields
			IdempotencyKey string `json:"idempotencyKey"`
		}{input.Fields, input.IdempotencyKey}
		return "/organisation/mcp/inventory", body, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[inventoryUpdateInput]{name: "authrail_update_ai_inventory_item", description: "Save a new AI configuration revision against the exact revision you read. Supply all reviewed fields and a stable UUID; identical retries retain the original saved revision, while current metadata may have advanced. Discovered systems still require human confirmation.", scope: InventoryManageScope, method: http.MethodPatch, prepare: func(input inventoryUpdateInput) (string, any, error) {
		if !frameworkID.MatchString(input.ItemID) || !frameworkID.MatchString(input.IdempotencyKey) || input.Revision < 1 {
			return "", nil, errors.New("Supply exact AI system, command UUID and current revision")
		}
		body := struct {
			inventoryFields
			IdempotencyKey string `json:"idempotencyKey"`
			Revision       int    `json:"revision"`
		}{input.Fields, input.IdempotencyKey, input.Revision}
		return "/organisation/mcp/inventory/" + input.ItemID, body, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[inventoryLifecycleInput]{name: "authrail_change_ai_inventory_lifecycle", description: "Change a visible AI system between active, inactive and archived using its exact revision and current lifecycle. Use a stable command UUID. This does not confirm a discovered identity, update deployment configuration or activate Rails.", scope: InventoryManageScope, method: http.MethodPost, prepare: func(input inventoryLifecycleInput) (string, any, error) {
		if !frameworkID.MatchString(input.ItemID) || !frameworkID.MatchString(input.IdempotencyKey) || input.Revision < 1 || !validInventoryLifecycle(input.CurrentLifecycle) || !validInventoryLifecycle(input.Lifecycle) {
			return "", nil, errors.New("Supply exact AI system, command UUID, revision and lifecycle values")
		}
		body := struct {
			IdempotencyKey   string `json:"idempotencyKey"`
			Revision         int    `json:"revision"`
			CurrentLifecycle string `json:"currentLifecycle"`
			Lifecycle        string `json:"lifecycle"`
		}{input.IdempotencyKey, input.Revision, input.CurrentLifecycle, input.Lifecycle}
		return "/organisation/mcp/inventory/" + input.ItemID + "/lifecycle", body, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[inventoryReviewInput]{name: "authrail_review_ai_inventory_item", description: "Open secure human confirmation of a discovered AI system, review before adopting its current revision on deployments, or manage private integration discovery/setup keys. Bind the exact revision you read. This navigation changes nothing; only the signed-in person can confirm or update deployments.", scope: InventoryManageScope, method: http.MethodPost, readonly: true, prepare: func(input inventoryReviewInput) (string, any, error) {
		if !frameworkID.MatchString(input.ItemID) || input.Revision < 1 || input.Action != "confirm" && input.Action != "adopt_revision" && input.Action != "discovery_setup" {
			return "", nil, errors.New("Choose confirm, adopt_revision or discovery_setup for an exact AI system revision")
		}
		return "/organisation/mcp/inventory/" + input.ItemID + "/review", struct {
			Action   string `json:"action"`
			Revision int    `json:"revision"`
		}{input.Action, input.Revision}, nil
	}})
}

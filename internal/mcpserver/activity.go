package mcpserver

import (
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/url"
)

const ActivityReadScope = "activity.read"
const ActivityRecordScope = "activity.record"

type activityEvidenceReference struct {
	EvidencePackageID string `json:"evidencePackageId"`
	ReferenceHash     string `json:"referenceHash,omitempty"`
}
type activityFields struct {
	InventoryItemID    string                      `json:"inventoryItemId" jsonschema:"Visible exact AI system UUID"`
	InventoryRevision  int                         `json:"inventoryRevision" jsonschema:"Exact reported Inventory revision read before recording; use the original pinned revision for corrections"`
	ActionCategory     string                      `json:"actionCategory" jsonschema:"Suggested or organisation-defined category, 2 to120 characters"`
	OccurredAt         string                      `json:"occurredAt" jsonschema:"ISO8601 occurrence time; future times are refused"`
	Actor              string                      `json:"actor,omitempty" jsonschema:"Optional reported actor label, not authenticated review identity"`
	RelatedWork        string                      `json:"relatedWork,omitempty"`
	Provider           string                      `json:"provider,omitempty" jsonschema:"Reported provider for this event, not proof of execution identity"`
	Model              string                      `json:"model,omitempty"`
	Action             string                      `json:"action" jsonschema:"Reported action, 3 to2000 characters"`
	DataClassification string                      `json:"dataClassification"`
	OutcomeSummary     string                      `json:"outcomeSummary" jsonschema:"Reported outcome, 3 to4000 characters; uncertainty must remain explicit"`
	PolicyVersionIDs   []string                    `json:"policyVersionIds" jsonschema:"Up to20 exact currently permitted published Policy version UUIDs"`
	EvidenceReferences []activityEvidenceReference `json:"evidenceReferences" jsonschema:"Up to20 permitted Evidence package references, never files"`
}
type activityListInput struct {
	Query           string `json:"query,omitempty"`
	InventoryItemID string `json:"inventoryItemId,omitempty"`
	RuntimeActionID string `json:"runtimeActionId,omitempty"`
	ActionCategory  string `json:"actionCategory,omitempty"`
	Limit           int    `json:"limit,omitempty"`
	Cursor          string `json:"cursor,omitempty"`
}
type activityIDInput struct {
	ActivityID string `json:"activityId"`
}
type activityRevisionInput struct {
	ActivityID string `json:"activityId"`
	RevisionID string `json:"revisionId" jsonschema:"Exact retained Activity revision UUID"`
}
type activitySectionInput struct {
	ActivityID string `json:"activityId"`
	RevisionID string `json:"revisionId"`
	Section    string `json:"section,omitempty" jsonschema:"overview, audience or references; default overview"`
	Cursor     string `json:"cursor,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}
type activityHistoryInput struct {
	ActivityID string `json:"activityId"`
	Limit      int    `json:"limit,omitempty"`
	Cursor     string `json:"cursor,omitempty"`
}
type activityReviewsInput struct {
	ActivityID string `json:"activityId"`
	RevisionID string `json:"revisionId"`
	Limit      int    `json:"limit,omitempty"`
	Cursor     string `json:"cursor,omitempty"`
}
type activityChoicesInput struct {
	Query  string `json:"query,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}
type activityCreateInput struct {
	IdempotencyKey string         `json:"idempotencyKey" jsonschema:"Stable UUID reused only for identical retries"`
	Fields         activityFields `json:"fields"`
}
type activityCorrectInput struct {
	ActivityID         string         `json:"activityId"`
	ExpectedRevisionID string         `json:"expectedRevisionId" jsonschema:"Exact current manual/imported revision being corrected"`
	IdempotencyKey     string         `json:"idempotencyKey"`
	Fields             activityFields `json:"fields"`
}
type activityReviewInput struct {
	ActivityID         string `json:"activityId"`
	ExpectedRevisionID string `json:"expectedRevisionId"`
}
type activityImportInput struct {
	ImportID string `json:"importId" jsonschema:"Exact import UUID returned by private workspace upload"`
}

func activityPage(q url.Values, cursor string, limit int) (string, error) {
	if limit > 20 {
		return "", errors.New("Use a bounded page up to20")
	}
	return workspacePage(q, workspacePageInput{Cursor: cursor, Limit: limit})
}
func addActivityReadTools(server *mcp.Server, backend *Backend) {
	addWorkspaceTool(server, backend, delegatedToolInput[activityListInput]{name: "authrail_list_activity_records", description: "Find visible manual, imported and Runtime Activity Records. Decisions, starts, reported outcomes and unknown effects remain distinct. This reads history; it neither executes nor retries business work.", scope: ActivityReadScope, method: http.MethodGet, readonly: true, prepare: func(input activityListInput) (string, any, error) {
		if input.Limit > 3 {
			return "", nil, errors.New("Use an Activity summary page up to3; full outcomes are read by exact detail")
		}
		if input.InventoryItemID != "" && !frameworkID.MatchString(input.InventoryItemID) || input.RuntimeActionID != "" && !frameworkID.MatchString(input.RuntimeActionID) {
			return "", nil, errors.New("Use exact returned filter UUIDs")
		}
		q := url.Values{}
		if input.Query != "" {
			q.Set("query", input.Query)
		}
		if input.InventoryItemID != "" {
			q.Set("inventoryItemId", input.InventoryItemID)
		}
		if input.RuntimeActionID != "" {
			q.Set("runtimeActionId", input.RuntimeActionID)
		}
		if input.ActionCategory != "" {
			q.Set("actionCategory", input.ActionCategory)
		}
		query, err := activityPage(q, input.Cursor, input.Limit)
		return "/organisation/mcp/activity?" + query, nil, err
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[activityIDInput]{name: "authrail_get_activity_record", description: "Read the exact current visible Activity revision. Follow separate bounded history and review tools; a reported outcome is not independent provider-effect proof.", scope: ActivityReadScope, method: http.MethodGet, readonly: true, prepare: func(input activityIDInput) (string, any, error) {
		if !frameworkID.MatchString(input.ActivityID) {
			return "", nil, errors.New("Use an exact Activity UUID")
		}
		return "/organisation/mcp/activity/" + input.ActivityID, nil, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[activitySectionInput]{name: "authrail_get_activity_revision", description: "Read an exact retained Activity revision within its original audience. Later corrections or reviews do not rewrite this content.", scope: ActivityReadScope, method: http.MethodGet, readonly: true, prepare: func(input activitySectionInput) (string, any, error) {
		if !frameworkID.MatchString(input.ActivityID) || !frameworkID.MatchString(input.RevisionID) {
			return "", nil, errors.New("Use exact Activity and revision UUIDs")
		}
		if input.Section != "" && input.Section != "overview" && input.Section != "audience" && input.Section != "references" {
			return "", nil, errors.New("Use a supported Activity content section")
		}
		q := url.Values{}
		if input.Section != "" {
			q.Set("section", input.Section)
		}
		query, err := activityPage(q, input.Cursor, input.Limit)
		return "/organisation/mcp/activity/" + input.ActivityID + "/revisions/" + input.RevisionID + "?" + query, nil, err
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[activityHistoryInput]{name: "authrail_get_activity_history", description: "Read bounded correction history for a visible Activity Record, newest first. Use returned exact revision IDs to inspect content; continuation remains bound to your workspace and filters.", scope: ActivityReadScope, method: http.MethodGet, readonly: true, prepare: func(input activityHistoryInput) (string, any, error) {
		if !frameworkID.MatchString(input.ActivityID) {
			return "", nil, errors.New("Use an exact Activity UUID")
		}
		q, err := activityPage(url.Values{}, input.Cursor, input.Limit)
		return "/organisation/mcp/activity/" + input.ActivityID + "/history?" + q, nil, err
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[activityReviewsInput]{name: "authrail_get_activity_reviews", description: "Read bounded human review history for one exact visible Activity revision. A review decision is distinct from a verified attestation, passing Test or execution proof.", scope: ActivityReadScope, method: http.MethodGet, readonly: true, prepare: func(input activityReviewsInput) (string, any, error) {
		if input.Limit > 4 {
			return "", nil, errors.New("Use a review page up to4; longer notes remain available through continuation")
		}
		if !frameworkID.MatchString(input.ActivityID) || !frameworkID.MatchString(input.RevisionID) {
			return "", nil, errors.New("Use exact Activity and revision UUIDs")
		}
		q, err := activityPage(url.Values{}, input.Cursor, input.Limit)
		return "/organisation/mcp/activity/" + input.ActivityID + "/revisions/" + input.RevisionID + "/reviews?" + q, nil, err
	}})
	for _, kind := range []string{"categories", "classifications"} {
		addWorkspaceTool(server, backend, delegatedToolInput[activityChoicesInput]{name: "authrail_list_activity_" + kind, description: "Find a bounded page of permitted Activity " + kind + " before preparing a manual record. This metadata grants no additional source access.", scope: ActivityReadScope, method: http.MethodGet, readonly: true, prepare: func(input activityChoicesInput) (string, any, error) {
			q := url.Values{}
			if input.Query != "" {
				q.Set("query", input.Query)
			}
			query, err := activityPage(q, input.Cursor, input.Limit)
			return "/organisation/mcp/activity/" + kind + "?" + query, nil, err
		}})
	}
	for _, operation := range []struct{ name, method, suffix, description string }{
		{"authrail_get_activity_attestation", http.MethodGet, "/attestation", "Read exact Activity attestation status and integrity proof metadata. It does not establish business execution, compliance or a passing Test."},
		{"authrail_verify_activity_attestation", http.MethodPost, "/attestation/verify", "Verify an exact visible Activity attestation hash/signature and inspect any correction reference. An unfinished proof has no verification result; integrity alone is not compliance."},
		{"authrail_download_activity_attestation", http.MethodPost, "/attestation/download", "Open private JSON/PDF attestation download in AuthRail for an exact visible revision. No signed storage URL or file bytes enter assistant results."},
	} {
		addWorkspaceTool(server, backend, delegatedToolInput[activityRevisionInput]{name: operation.name, description: operation.description, scope: ActivityReadScope, method: operation.method, readonly: true, prepare: func(input activityRevisionInput) (string, any, error) {
			if !frameworkID.MatchString(input.ActivityID) || !frameworkID.MatchString(input.RevisionID) {
				return "", nil, errors.New("Use exact Activity and revision UUIDs")
			}
			path := "/organisation/mcp/activity/" + input.ActivityID + operation.suffix
			if operation.method == http.MethodGet {
				return path + "?" + url.Values{"revisionId": {input.RevisionID}}.Encode(), nil, nil
			}
			return path, struct {
				RevisionID string `json:"revisionId"`
			}{input.RevisionID}, nil
		}})
	}
}

func addActivityRecordTools(server *mcp.Server, backend *Backend) {
	addWorkspaceTool(server, backend, delegatedToolInput[activityCreateInput]{name: "authrail_create_activity_record", description: "Append a reported manual Activity for the exact visible Inventory revision. Reuse one UUID only for identical retries. This record is not an independent human review, compliance proof or authorization to run the reported action.", scope: ActivityRecordScope, method: http.MethodPost, prepare: func(input activityCreateInput) (string, any, error) {
		if !frameworkID.MatchString(input.IdempotencyKey) || !frameworkID.MatchString(input.Fields.InventoryItemID) || input.Fields.InventoryRevision < 1 {
			return "", nil, errors.New("Use stable command and Inventory UUIDs with its exact revision")
		}
		return "/organisation/mcp/activity", struct {
			activityFields
			IdempotencyKey string `json:"idempotencyKey"`
		}{input.Fields, input.IdempotencyKey}, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[activityCorrectInput]{name: "authrail_correct_activity_record", description: "Append a correction to an exact current manual/imported Activity revision with a stable UUID. Retain the original pinned Inventory identity; stale revisions conflict. Runtime-origin entries cannot be rewritten or replaced here.", scope: ActivityRecordScope, method: http.MethodPost, prepare: func(input activityCorrectInput) (string, any, error) {
		if !frameworkID.MatchString(input.ActivityID) || !frameworkID.MatchString(input.ExpectedRevisionID) || !frameworkID.MatchString(input.IdempotencyKey) || !frameworkID.MatchString(input.Fields.InventoryItemID) || input.Fields.InventoryRevision < 1 {
			return "", nil, errors.New("Use exact Activity/Inventory identities and a stable correction UUID")
		}
		return "/organisation/mcp/activity/" + input.ActivityID + "/corrections", struct {
			activityFields
			IdempotencyKey     string `json:"idempotencyKey"`
			ExpectedRevisionID string `json:"expectedRevisionId"`
		}{input.Fields, input.IdempotencyKey, input.ExpectedRevisionID}, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[activityReviewInput]{name: "authrail_review_activity_record", description: "Ask an authorised person to review the exact current Activity revision in AuthRail. This secure navigation does not submit a decision, act as a human reviewer or change the record.", scope: ActivityRecordScope, method: http.MethodPost, readonly: true, prepare: func(input activityReviewInput) (string, any, error) {
		if !frameworkID.MatchString(input.ActivityID) || !frameworkID.MatchString(input.ExpectedRevisionID) {
			return "", nil, errors.New("Use the exact current Activity revision")
		}
		return "/organisation/mcp/activity/" + input.ActivityID + "/review", struct {
			ExpectedRevisionID string `json:"expectedRevisionId"`
		}{input.ExpectedRevisionID}, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[noArguments]{name: "authrail_import_activity_records", description: "Open private CSV/JSON Activity import in AuthRail. Upload bytes and presigned storage access stay in the authenticated workspace; this tool creates no import and returns no storage capability. Follow the returned import identity through status after upload.", scope: ActivityRecordScope, method: http.MethodPost, readonly: true, prepare: func(noArguments) (string, any, error) { return "/organisation/mcp/activity/imports/handoff", nil, nil }})
	addWorkspaceTool(server, backend, delegatedToolInput[activityImportInput]{name: "authrail_get_activity_import", description: "Follow a permitted retained Activity import by UUID. Read status and row counts without file bytes, object keys, signed URLs or raw parser diagnostics. Missing totals remain unknown.", scope: ActivityRecordScope, method: http.MethodGet, readonly: true, prepare: func(input activityImportInput) (string, any, error) {
		if !frameworkID.MatchString(input.ImportID) {
			return "", nil, errors.New("Use the retained import UUID")
		}
		return "/organisation/mcp/activity/imports/" + input.ImportID, nil, nil
	}})
}

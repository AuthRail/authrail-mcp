package mcpserver

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const PoliciesReadScope = "policies.read"
const PoliciesManageScope = "policies.manage"

type policiesListInput struct {
	Query         string   `json:"query,omitempty" jsonschema:"Policy title or category search, 2 to 120 characters"`
	Status        string   `json:"status,omitempty" jsonschema:"draft,published,archived"`
	CategoryIDs   []string `json:"categoryIds,omitempty"`
	DepartmentIDs []string `json:"departmentIds,omitempty"`
	TeamIDs       []string `json:"teamIds,omitempty"`
	Limit         int      `json:"limit,omitempty" jsonschema:"Page size 1 to 30"`
	Cursor        string   `json:"cursor,omitempty" jsonschema:"Exact opaque nextCursor returned for this list and filters"`
}
type policyIDInput struct {
	PolicyID string `json:"policyId"`
}
type policyPageInput struct {
	PolicyID string `json:"policyId"`
	Cursor   string `json:"cursor,omitempty"`
}
type policyContentInput struct {
	PolicyID  string `json:"policyId"`
	VersionID string `json:"versionId"`
	Cursor    string `json:"cursor,omitempty" jsonschema:"Continue exact content snapshot; concatenate JSON text chunks in offset order"`
}
type policyOptionsInput struct {
	Query  string `json:"query,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}
type policySection struct {
	Title        string `json:"title"`
	BodyMarkdown string `json:"bodyMarkdown"`
	Order        int    `json:"order"`
}
type policyContent struct {
	Sections []policySection `json:"sections"`
}
type policyAudience struct {
	DepartmentIDs []string `json:"departmentIds"`
	TeamIDs       []string `json:"teamIds"`
}
type policyCreateInput struct {
	IdempotencyKey string         `json:"idempotencyKey" jsonschema:"Stable command UUID; retain unchanged after uncertainty"`
	Title          string         `json:"title"`
	CategoryID     string         `json:"categoryId"`
	Content        policyContent  `json:"content"`
	Applicability  policyAudience `json:"applicability"`
}
type policyContinueInput struct {
	PolicyID         string `json:"policyId"`
	IdempotencyKey   string `json:"idempotencyKey"`
	PolicyRevision   int    `json:"policyRevision"`
	Preparation      string `json:"preparation" jsonschema:"empty or copy_version"`
	BasedOnVersionID string `json:"basedOnVersionId,omitempty"`
}
type policySaveInput struct {
	PolicyID       string          `json:"policyId"`
	VersionID      string          `json:"versionId"`
	IdempotencyKey string          `json:"idempotencyKey"`
	Revision       int             `json:"revision"`
	Content        policyContent   `json:"content"`
	Applicability  *policyAudience `json:"applicability,omitempty"`
	Title          string          `json:"title,omitempty"`
	CategoryID     string          `json:"categoryId,omitempty"`
	OwnerStaffID   string          `json:"ownerStaffId,omitempty"`
	ReviewCadence  string          `json:"reviewCadence,omitempty" jsonschema:"quarterly,semi_annual,annual"`
}
type policyAuthoringListInput struct {
	Query  string `json:"query,omitempty"`
	Status string `json:"status,omitempty" jsonschema:"open,active,ready,completed,cancelled"`
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}
type policyAuthoringReadInput struct {
	SessionID string `json:"sessionId"`
	Part      string `json:"part,omitempty" jsonschema:"summary lists ten turn references; proposal or turn reads lossless JSON chunks"`
	TurnID    string `json:"turnId,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
}
type policyAuthoringInput struct {
	IdempotencyKey      string         `json:"idempotencyKey"`
	PolicyID            string         `json:"policyId,omitempty"`
	BasedOnVersionID    string         `json:"basedOnVersionId,omitempty"`
	Title               string         `json:"title"`
	CategoryID          string         `json:"categoryId"`
	Goal                string         `json:"goal"`
	FrameworkVersionIDs []string       `json:"frameworkVersionIds" jsonschema:"Explicit selected versions; empty is allowed and no Framework is required"`
	Applicability       policyAudience `json:"applicability"`
}
type policyAuthoringUpdateInput struct {
	SessionID           string         `json:"sessionId"`
	IdempotencyKey      string         `json:"idempotencyKey"`
	Revision            int            `json:"revision"`
	PolicyID            string         `json:"policyId,omitempty"`
	BasedOnVersionID    string         `json:"basedOnVersionId,omitempty"`
	Title               string         `json:"title"`
	CategoryID          string         `json:"categoryId"`
	Goal                string         `json:"goal"`
	FrameworkVersionIDs []string       `json:"frameworkVersionIds"`
	Applicability       policyAudience `json:"applicability"`
}
type policyAuthoringAnswer struct {
	QuestionID string   `json:"questionId"`
	Text       *string  `json:"text,omitempty" jsonschema:"Supply exactly one of text,boolean,choices"`
	Boolean    *bool    `json:"boolean,omitempty"`
	Choices    []string `json:"choices,omitempty"`
}
type policyAuthoringTurnInput struct {
	SessionID      string                  `json:"sessionId"`
	IdempotencyKey string                  `json:"idempotencyKey"`
	Revision       int                     `json:"revision"`
	Message        string                  `json:"message"`
	Answers        []policyAuthoringAnswer `json:"answers"`
}
type policyAuthoringRetryInput struct {
	SessionID      string `json:"sessionId"`
	TurnID         string `json:"turnId"`
	Revision       int    `json:"revision"`
	IdempotencyKey string `json:"idempotencyKey"`
}
type policyAuthoringProposalInput struct {
	SessionID      string        `json:"sessionId"`
	Revision       int           `json:"revision"`
	IdempotencyKey string        `json:"idempotencyKey"`
	Content        policyContent `json:"content"`
}
type policyAuthoringRevisionInput struct {
	SessionID      string `json:"sessionId"`
	Revision       int    `json:"revision"`
	IdempotencyKey string `json:"idempotencyKey"`
}
type policyLibraryListInput struct {
	Query    string `json:"query,omitempty"`
	Category string `json:"category,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Cursor   string `json:"cursor,omitempty"`
}
type policyLibraryEntryInput struct {
	EntryID string `json:"entryId"`
	Cursor  string `json:"cursor,omitempty"`
}
type policyLibraryAdoptInput struct {
	EntryID        string         `json:"entryId"`
	IdempotencyKey string         `json:"idempotencyKey"`
	ContentHash    string         `json:"contentHash" jsonschema:"Exact entry contentHash from reviewed Library content"`
	Title          string         `json:"title"`
	CategoryID     string         `json:"categoryId"`
	Applicability  policyAudience `json:"applicability"`
}
type policyLibrarySubmissionsInput struct {
	PolicyID string `json:"policyId,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Cursor   string `json:"cursor,omitempty"`
}
type policyHandoffInput struct {
	Action    string `json:"action" jsonschema:"publish,review,discard,archive,restore,categories,share_library,unpublish_library,sources"`
	PolicyID  string `json:"policyId,omitempty"`
	VersionID string `json:"versionId,omitempty"`
	Revision  int    `json:"revision,omitempty"`
	EntryID   string `json:"entryId,omitempty"`
}

func addPolicyTools(server *mcp.Server, backend *Backend, scopes []string) {
	if slices.Contains(scopes, PoliciesReadScope) {
		addPolicyReadTools(server, backend)
	}
	if slices.Contains(scopes, PoliciesManageScope) {
		addPolicyManageTools(server, backend)
	}
}
func policyUUIDs(values ...string) bool {
	for _, value := range values {
		if !frameworkID.MatchString(value) {
			return false
		}
	}
	return true
}
func policyIDs(values []string, maximum int) bool {
	if len(values) > maximum {
		return false
	}
	for _, value := range values {
		if !frameworkID.MatchString(value) {
			return false
		}
	}
	return true
}
func policyText(value string, minimum, maximum int) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(value))
	return n >= minimum && n <= maximum
}
func policyValidContent(content policyContent) bool {
	if len(content.Sections) < 1 || len(content.Sections) > 100 {
		return false
	}
	orders := map[int]bool{}
	for _, section := range content.Sections {
		if section.Order < 1 || orders[section.Order] || !policyText(section.Title, 1, 180) || !policyText(section.BodyMarkdown, 1, 50000) {
			return false
		}
		orders[section.Order] = true
	}
	return true
}
func policyValidAudience(audience policyAudience) bool {
	return audience.DepartmentIDs != nil && audience.TeamIDs != nil && policyIDs(audience.DepartmentIDs, 100) && policyIDs(audience.TeamIDs, 100)
}
func policyQuery(cursor string, limit int) (url.Values, error) {
	if len(cursor) > 2000 || limit < 0 || limit > 50 {
		return nil, errors.New("Use the returned bounded cursor and page size")
	}
	q := url.Values{}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	return q, nil
}
func addPolicyReadTools(server *mcp.Server, backend *Backend) {
	addWorkspaceTool(server, backend, delegatedToolInput[policiesListInput]{name: "authrail_list_policies", description: "Find visible organisation Policies, owners and exact draft/published references. No Framework is required. Follow the returned cursor with unchanged filters.", scope: PoliciesReadScope, method: http.MethodGet, readonly: true, prepare: func(input policiesListInput) (string, any, error) {
		q, err := policyQuery(input.Cursor, input.Limit)
		if err != nil {
			return "", nil, err
		}
		if input.Limit > 30 || input.Query != "" && !policyText(input.Query, 2, 120) || input.Status != "" && !slices.Contains([]string{"draft", "published", "archived"}, input.Status) || !policyIDs(input.CategoryIDs, 50) || !policyIDs(input.DepartmentIDs, 50) || !policyIDs(input.TeamIDs, 50) {
			return "", nil, errors.New("Use bounded Policy filters and a page size up to30")
		}
		if input.Query != "" {
			q.Set("query", input.Query)
		}
		if input.Status != "" {
			q.Set("status", input.Status)
		}
		for key, values := range map[string][]string{"categoryIds": input.CategoryIDs, "departmentIds": input.DepartmentIDs, "teamIds": input.TeamIDs} {
			if len(values) > 0 {
				q.Set(key, strings.Join(values, ","))
			}
		}
		return "/organisation/mcp/policy-workspace?" + q.Encode(), nil, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[policyIDInput]{name: "authrail_get_policy", description: "Read one visible Policy’s lifecycle, owner, audience and exact current version references. Content is available separately through bounded exact-version reads.", scope: PoliciesReadScope, method: http.MethodGet, readonly: true, prepare: func(input policyIDInput) (string, any, error) {
		if !policyUUIDs(input.PolicyID) {
			return "", nil, errors.New("Select the Policy reference")
		}
		return "/organisation/mcp/policy-workspace/" + input.PolicyID, nil, nil
	}})
	for _, spec := range []struct{ name, path, description string }{{"authrail_list_policy_versions", "versions", "List exact retained visible Policy versions, ten per page. Published versions remain immutable; no latest-version substitution."}, {"authrail_get_policy_history", "history", "Read retained Policy lifecycle events, ten per page. Events remain distinct from publication, independent review and external effects."}} {
		addWorkspaceTool(server, backend, delegatedToolInput[policyPageInput]{name: spec.name, description: spec.description, scope: PoliciesReadScope, method: http.MethodGet, readonly: true, prepare: func(input policyPageInput) (string, any, error) {
			if !policyUUIDs(input.PolicyID) {
				return "", nil, errors.New("Select the Policy reference")
			}
			q, err := policyQuery(input.Cursor, 0)
			if err != nil {
				return "", nil, err
			}
			return "/organisation/mcp/policy-workspace/" + input.PolicyID + "/" + spec.path + "?" + q.Encode(), nil, nil
		}})
	}
	addWorkspaceTool(server, backend, delegatedToolInput[policyContentInput]{name: "authrail_get_policy_content", description: "Read selected exact Policy content as lossless JSON text chunks, at most24000 UTF8 bytes each. Concatenate text by offset until hasNextPage=false, then parse JSON. Current audience and explicit policies.read consent apply to every chunk; changed content requires a fresh read.", scope: PoliciesReadScope, method: http.MethodGet, readonly: true, prepare: func(input policyContentInput) (string, any, error) {
		if !policyUUIDs(input.PolicyID, input.VersionID) {
			return "", nil, errors.New("Select exact Policy/version references")
		}
		q, err := policyQuery(input.Cursor, 0)
		if err != nil {
			return "", nil, err
		}
		return "/organisation/mcp/policy-workspace/" + input.PolicyID + "/versions/" + input.VersionID + "/content?" + q.Encode(), nil, nil
	}})
	for _, spec := range []struct{ name, path, description string }{{"authrail_list_policy_categories", "categories", "Find named active/archived Policy categories with bounded continuation."}, {"authrail_list_policy_owner_candidates", "owners", "Find active same-workspace Policy owners. Current Policy management permission is required."}} {
		addWorkspaceTool(server, backend, delegatedToolInput[policyOptionsInput]{name: spec.name, description: spec.description, scope: PoliciesReadScope, method: http.MethodGet, readonly: true, prepare: func(input policyOptionsInput) (string, any, error) {
			q, err := policyQuery(input.Cursor, input.Limit)
			if err != nil {
				return "", nil, err
			}
			if input.Query != "" && !policyText(input.Query, 1, 120) {
				return "", nil, errors.New("Use a bounded name search")
			}
			if input.Query != "" {
				q.Set("query", input.Query)
			}
			return "/organisation/mcp/policy-workspace/" + spec.path + "?" + q.Encode(), nil, nil
		}})
	}
	addWorkspaceTool(server, backend, delegatedToolInput[policyAuthoringListInput]{name: "authrail_list_policy_authoring_sessions", description: "Find saved Policy authoring sessions within your current audience. Preparation and retained AI turns do not publish a Policy.", scope: PoliciesReadScope, method: http.MethodGet, readonly: true, prepare: func(input policyAuthoringListInput) (string, any, error) {
		q, err := policyQuery(input.Cursor, input.Limit)
		if err != nil {
			return "", nil, err
		}
		if input.Query != "" && !policyText(input.Query, 2, 120) || input.Status != "" && !slices.Contains([]string{"open", "active", "ready", "completed", "cancelled"}, input.Status) {
			return "", nil, errors.New("Use a supported session filter")
		}
		if input.Query != "" {
			q.Set("query", input.Query)
		}
		if input.Status != "" {
			q.Set("status", input.Status)
		}
		return "/organisation/mcp/policy-authoring?" + q.Encode(), nil, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[policyAuthoringReadInput]{name: "authrail_get_policy_authoring_session", description: "Read a saved session summary and bounded turn references. Select part=proposal or part=turn with a turnId for exact lossless JSON chunks. Assemble text in offset order; a changed snapshot refuses old continuation. No model keys are required.", scope: PoliciesReadScope, method: http.MethodGet, readonly: true, prepare: func(input policyAuthoringReadInput) (string, any, error) {
		if !policyUUIDs(input.SessionID) || input.Part != "" && !slices.Contains([]string{"summary", "proposal", "turn"}, input.Part) || input.Part == "turn" && !policyUUIDs(input.TurnID) || input.Part != "turn" && input.TurnID != "" {
			return "", nil, errors.New("Select one saved session and its summary, proposal or exact turn")
		}
		q, err := policyQuery(input.Cursor, 0)
		if err != nil {
			return "", nil, err
		}
		if input.Part != "" {
			q.Set("part", input.Part)
		}
		if input.TurnID != "" {
			q.Set("turnId", input.TurnID)
		}
		return "/organisation/mcp/policy-authoring/" + input.SessionID + "?" + q.Encode(), nil, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[policyLibraryListInput]{name: "authrail_list_policy_library_entries", description: "Search published Policy Library starting material. Read its exact content and contentHash before creating an organisation draft; no Framework copies are created.", scope: PoliciesReadScope, method: http.MethodGet, readonly: true, prepare: func(input policyLibraryListInput) (string, any, error) {
		q, err := policyQuery(input.Cursor, input.Limit)
		if err != nil {
			return "", nil, err
		}
		if input.Query != "" && !policyText(input.Query, 2, 120) || input.Category != "" && !policyText(input.Category, 2, 80) {
			return "", nil, errors.New("Use bounded Library search/category text")
		}
		if input.Query != "" {
			q.Set("query", input.Query)
		}
		if input.Category != "" {
			q.Set("category", input.Category)
		}
		return "/organisation/mcp/policy-library/entries?" + q.Encode(), nil, nil
	}})
	addWorkspaceRead(server, backend, "authrail_list_policy_library_categories", "List published Library categories for template search.", PoliciesReadScope, "/organisation/mcp/policy-library/categories")
	addWorkspaceTool(server, backend, delegatedToolInput[policyLibraryEntryInput]{name: "authrail_get_policy_library_entry", description: "Read one exact published Library entry and its contentHash. Content arrives as bounded lossless JSON text chunks; concatenate by offset and parse only when complete.", scope: PoliciesReadScope, method: http.MethodGet, readonly: true, prepare: func(input policyLibraryEntryInput) (string, any, error) {
		if !policyUUIDs(input.EntryID) {
			return "", nil, errors.New("Select the Library entry reference")
		}
		q, err := policyQuery(input.Cursor, 0)
		if err != nil {
			return "", nil, err
		}
		return "/organisation/mcp/policy-library/entries/" + input.EntryID + "?" + q.Encode(), nil, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[policyLibrarySubmissionsInput]{name: "authrail_list_policy_library_submissions", description: "Follow your organisation’s Library submissions only where their exact Policy audience remains visible. Public review status is distinct from internal staff analysis.", scope: PoliciesReadScope, method: http.MethodGet, readonly: true, prepare: func(input policyLibrarySubmissionsInput) (string, any, error) {
		q, err := policyQuery(input.Cursor, input.Limit)
		if err != nil {
			return "", nil, err
		}
		if input.PolicyID != "" && !policyUUIDs(input.PolicyID) {
			return "", nil, errors.New("Use an exact Policy filter")
		}
		if input.PolicyID != "" {
			q.Set("policyId", input.PolicyID)
		}
		return "/organisation/mcp/policy-library/submissions?" + q.Encode(), nil, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[policyIDInput]{name: "authrail_get_policy_library_origin", description: "Read a visible Policy’s exact Library origin and original content hash. Current Library content never silently replaces the adopted draft.", scope: PoliciesReadScope, method: http.MethodGet, readonly: true, prepare: func(input policyIDInput) (string, any, error) {
		if !policyUUIDs(input.PolicyID) {
			return "", nil, errors.New("Select a Policy reference")
		}
		return "/organisation/mcp/policy-library/policies/" + input.PolicyID + "/origin", nil, nil
	}})
}

func addPolicyManageTools(server *mcp.Server, backend *Backend) {
	addWorkspaceTool(server, backend, delegatedToolInput[policyCreateInput]{name: "authrail_create_policy_draft", description: "Create one unpublished Policy with reviewed sections and audience. Both policies.read/manage and current domain authority apply. Retry unchanged with the same UUID; humans publish.", scope: PoliciesManageScope, method: http.MethodPost, prepare: func(input policyCreateInput) (string, any, error) {
		if !policyUUIDs(input.IdempotencyKey, input.CategoryID) || !policyText(input.Title, 1, 180) || !policyValidContent(input.Content) || !policyValidAudience(input.Applicability) {
			return "", nil, errors.New("Supply reviewed content/audience and stable command UUID")
		}
		return "/organisation/mcp/policy-workspace", input, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[policyContinueInput]{name: "authrail_continue_policy_draft", description: "Create empty or exact-version draft against the current Policy aggregate revision; stable retries retain one draft.", scope: PoliciesManageScope, method: http.MethodPost, prepare: func(input policyContinueInput) (string, any, error) {
		if !policyUUIDs(input.PolicyID, input.IdempotencyKey) || input.PolicyRevision < 1 || !slices.Contains([]string{"empty", "copy_version"}, input.Preparation) || input.Preparation == "copy_version" && !policyUUIDs(input.BasedOnVersionID) || input.Preparation == "empty" && input.BasedOnVersionID != "" {
			return "", nil, errors.New("Select Policy revision and empty or retained-version preparation")
		}
		body := map[string]any{"idempotencyKey": input.IdempotencyKey, "policyRevision": input.PolicyRevision, "preparation": input.Preparation}
		if input.BasedOnVersionID != "" {
			body["basedOnVersionId"] = input.BasedOnVersionID
		}
		return "/organisation/mcp/policy-workspace/" + input.PolicyID + "/drafts", body, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[policySaveInput]{name: "authrail_save_policy_draft", description: "Save exact draft revision without deciding review/publication. Stable UUID retries retain the original edit outcome.", scope: PoliciesManageScope, method: http.MethodPost, prepare: func(input policySaveInput) (string, any, error) {
		if !policyUUIDs(input.PolicyID, input.VersionID, input.IdempotencyKey) || input.Revision < 1 || !policyValidContent(input.Content) || input.Applicability != nil && !policyValidAudience(*input.Applicability) || input.Title != "" && !policyText(input.Title, 1, 180) || input.CategoryID != "" && !policyUUIDs(input.CategoryID) || input.OwnerStaffID != "" && !policyUUIDs(input.OwnerStaffID) || input.ReviewCadence != "" && !slices.Contains([]string{"quarterly", "semi_annual", "annual"}, input.ReviewCadence) {
			return "", nil, errors.New("Supply exact draft revision and reviewed fields")
		}
		body := map[string]any{"idempotencyKey": input.IdempotencyKey, "revision": input.Revision, "content": input.Content}
		if input.Applicability != nil {
			body["applicability"] = input.Applicability
		}
		for key, value := range map[string]string{"title": input.Title, "categoryId": input.CategoryID, "ownerStaffId": input.OwnerStaffID, "reviewCadence": input.ReviewCadence} {
			if value != "" {
				body[key] = value
			}
		}
		return "/organisation/mcp/policy-workspace/" + input.PolicyID + "/drafts/" + input.VersionID, body, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[policyAuthoringInput]{name: "authrail_start_policy_authoring", description: "Start AuthRail-operated Policy drafting with selected business material disclosed to AuthRail and its model recipient. No customer keys. Follow returned asynchronous session/turn references.", scope: PoliciesManageScope, method: http.MethodPost, prepare: func(input policyAuthoringInput) (string, any, error) {
		if !validPolicyAuthoring(input.IdempotencyKey, input.PolicyID, input.BasedOnVersionID, input.Title, input.CategoryID, input.Goal, input.FrameworkVersionIDs, input.Applicability) {
			return "", nil, errors.New("Supply reviewed authoring context/audience and stable UUID")
		}
		return "/organisation/mcp/policy-authoring", input, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[policyAuthoringUpdateInput]{name: "authrail_update_policy_authoring", description: "Update exact saved authoring context; active AI work and stale revisions refuse replacement.", scope: PoliciesManageScope, method: http.MethodPost, prepare: func(input policyAuthoringUpdateInput) (string, any, error) {
		if !policyUUIDs(input.SessionID) || input.Revision < 1 || !validPolicyAuthoring(input.IdempotencyKey, input.PolicyID, input.BasedOnVersionID, input.Title, input.CategoryID, input.Goal, input.FrameworkVersionIDs, input.Applicability) {
			return "", nil, errors.New("Read the session revision and supply reviewed context")
		}
		body := map[string]any{"idempotencyKey": input.IdempotencyKey, "revision": input.Revision, "title": input.Title, "categoryId": input.CategoryID, "goal": input.Goal, "frameworkVersionIds": input.FrameworkVersionIDs, "applicability": input.Applicability}
		if input.PolicyID != "" {
			body["policyId"] = input.PolicyID
		}
		if input.BasedOnVersionID != "" {
			body["basedOnVersionId"] = input.BasedOnVersionID
		}
		return "/organisation/mcp/policy-authoring/" + input.SessionID + "/context", body, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[policyAuthoringTurnInput]{name: "authrail_answer_policy_authoring", description: "Answer the current exact session questions. Each answer supplies exactly one text,boolean or choices. Stable retries retain one original turn; follow status before another turn.", scope: PoliciesManageScope, method: http.MethodPost, prepare: preparePolicyAnswer})
	addWorkspaceTool(server, backend, delegatedToolInput[policyAuthoringRetryInput]{name: "authrail_retry_policy_authoring", description: "Retry one failed original authoring turn against the current session revision; active work is not duplicated.", scope: PoliciesManageScope, method: http.MethodPost, prepare: func(input policyAuthoringRetryInput) (string, any, error) {
		if !policyUUIDs(input.SessionID, input.TurnID, input.IdempotencyKey) || input.Revision < 1 {
			return "", nil, errors.New("Select failed turn/session revision and stable UUID")
		}
		return "/organisation/mcp/policy-authoring/" + input.SessionID + "/turns/" + input.TurnID + "/retry", map[string]any{"idempotencyKey": input.IdempotencyKey, "revision": input.Revision}, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[policyAuthoringProposalInput]{name: "authrail_save_policy_authoring_proposal", description: "Save reviewed proposal sections against the exact session revision; the proposal remains unpublished.", scope: PoliciesManageScope, method: http.MethodPost, prepare: func(input policyAuthoringProposalInput) (string, any, error) {
		if !policyUUIDs(input.SessionID, input.IdempotencyKey) || input.Revision < 1 || !policyValidContent(input.Content) {
			return "", nil, errors.New("Use current session revision and reviewed sections")
		}
		return "/organisation/mcp/policy-authoring/" + input.SessionID + "/proposal", map[string]any{"idempotencyKey": input.IdempotencyKey, "revision": input.Revision, "content": input.Content}, nil
	}})
	for _, spec := range []struct{ name, path, description string }{{"authrail_cancel_policy_authoring", "cancel", "Cancel an exact saved authoring session; completed drafts and published Policies retain their own lifecycle."}, {"authrail_use_policy_authoring_draft", "use-as-draft", "Use the exact ready proposal as one unpublished Policy draft. Context and active work are rechecked; humans retain independent review/publication."}} {
		addWorkspaceTool(server, backend, delegatedToolInput[policyAuthoringRevisionInput]{name: spec.name, description: spec.description, scope: PoliciesManageScope, method: http.MethodPost, prepare: func(input policyAuthoringRevisionInput) (string, any, error) {
			if !policyUUIDs(input.SessionID, input.IdempotencyKey) || input.Revision < 1 {
				return "", nil, errors.New("Select exact session revision and stable UUID")
			}
			return "/organisation/mcp/policy-authoring/" + input.SessionID + "/" + spec.path, map[string]any{"idempotencyKey": input.IdempotencyKey, "revision": input.Revision}, nil
		}})
	}
	addWorkspaceTool(server, backend, delegatedToolInput[policyLibraryAdoptInput]{name: "authrail_adopt_policy_library_entry", description: "Adopt exact reviewed Library contentHash into one unpublished Policy. This is starting material, not publication or automatic Framework copies; same-key retries retain its original draft.", scope: PoliciesManageScope, method: http.MethodPost, prepare: func(input policyLibraryAdoptInput) (string, any, error) {
		if !policyUUIDs(input.EntryID, input.IdempotencyKey, input.CategoryID) || !policyText(input.Title, 1, 180) || !policyValidAudience(input.Applicability) || len(input.ContentHash) != 64 {
			return "", nil, errors.New("Select reviewed contentHash, category/audience and stable UUID")
		}
		return "/organisation/mcp/policy-library/entries/" + input.EntryID + "/adopt", map[string]any{"idempotencyKey": input.IdempotencyKey, "contentHash": input.ContentHash, "title": input.Title, "categoryId": input.CategoryID, "applicability": input.Applicability}, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[policyHandoffInput]{name: "authrail_prepare_policy_handoff", description: "Continue exact human publication/review, discard, archive/restore, Library sharing/withdrawal, categories or private sources in AuthRail. Link applies nothing. Actor/workspace/revision acknowledgement precedes final confirmation; files stay private.", scope: PoliciesManageScope, method: http.MethodPost, readonly: true, prepare: func(input policyHandoffInput) (string, any, error) {
		if !slices.Contains([]string{"publish", "review", "discard", "archive", "restore", "categories", "share_library", "unpublish_library", "sources"}, input.Action) || input.Action != "categories" && !policyUUIDs(input.PolicyID) || input.VersionID != "" && !policyUUIDs(input.VersionID) || input.EntryID != "" && !policyUUIDs(input.EntryID) {
			return "", nil, errors.New("Select a named workflow and exact Policy references")
		}
		body := map[string]any{"action": input.Action}
		if input.PolicyID != "" {
			body["policyId"] = input.PolicyID
		}
		if input.VersionID != "" {
			body["versionId"] = input.VersionID
		}
		if input.EntryID != "" {
			body["entryId"] = input.EntryID
		}
		if input.Revision > 0 {
			body["revision"] = input.Revision
		}
		return "/organisation/mcp/policy-workspace/human-handoff", body, nil
	}})
}
func validPolicyAuthoring(key, policy, baseline, title, category, goal string, frameworks []string, audience policyAudience) bool {
	return policyUUIDs(key, category) && policyText(title, 1, 180) && policyText(goal, 3, 5000) && frameworks != nil && policyIDs(frameworks, 50) && policyValidAudience(audience) && (policy == "" || policyUUIDs(policy)) && (baseline == "" || policy != "" && policyUUIDs(baseline))
}
func preparePolicyAnswer(input policyAuthoringTurnInput) (string, any, error) {
	if !policyUUIDs(input.SessionID, input.IdempotencyKey) || input.Revision < 1 || len(input.Answers) > 20 || !policyText(input.Message, 0, 5000) || strings.TrimSpace(input.Message) == "" && len(input.Answers) == 0 {
		return "", nil, errors.New("Select current session revision and bounded answers/message")
	}
	answers := make([]map[string]any, 0, len(input.Answers))
	seen := map[string]bool{}
	for _, answer := range input.Answers {
		if !policyText(answer.QuestionID, 1, 120) || seen[answer.QuestionID] {
			return "", nil, errors.New("Use distinct guided question references")
		}
		seen[answer.QuestionID] = true
		count := 0
		var value any
		if answer.Text != nil {
			count++
			if !policyText(*answer.Text, 1, 2000) {
				return "", nil, errors.New("Use bounded answer text")
			}
			value = *answer.Text
		}
		if answer.Boolean != nil {
			count++
			value = *answer.Boolean
		}
		if answer.Choices != nil {
			count++
			if len(answer.Choices) < 1 || len(answer.Choices) > 20 {
				return "", nil, errors.New("Use bounded selected answers")
			}
			for _, choice := range answer.Choices {
				if !policyText(choice, 1, 500) {
					return "", nil, errors.New("Use bounded selected answers")
				}
			}
			value = answer.Choices
		}
		if count != 1 {
			return "", nil, errors.New("Each answer requires exactly one value format")
		}
		answers = append(answers, map[string]any{"questionId": answer.QuestionID, "value": value})
	}
	return "/organisation/mcp/policy-authoring/" + input.SessionID + "/turns", map[string]any{"revision": input.Revision, "idempotencyKey": input.IdempotencyKey, "message": input.Message, "answers": answers}, nil
}

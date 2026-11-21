package mcpserver

import (
	"encoding/json"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/url"
	"slices"
)

const ComplianceConfigurationReadScope = "compliance.configuration.read"
const ComplianceConfigurationManageScope = "compliance.configuration.manage"

type complianceConfigReadInput struct {
	ContentCursor           string `json:"contentCursor,omitempty" jsonschema:"Lossless JSON continuation within the exact Framework scope page; keep owner cursor and limit unchanged"`
	FrameworkID             string `json:"frameworkId,omitempty" jsonschema:"Exact custom Framework UUID where required by this tool"`
	OrganisationFrameworkID string `json:"organisationFrameworkId,omitempty" jsonschema:"Exact selected organisation Framework UUID where required"`
	ControlID               string `json:"controlId,omitempty"`
	VersionID               string `json:"versionId,omitempty"`
	DecisionID              string `json:"decisionId,omitempty"`
	ImportID                string `json:"importId,omitempty"`
	ProposalID              string `json:"proposalId,omitempty"`
	Query                   string `json:"query,omitempty"`
	Cursor                  string `json:"cursor,omitempty"`
	Limit                   int    `json:"limit,omitempty" jsonschema:"Page size1 to20"`
}
type compliancePolicyBasis struct {
	PolicyVersionID                string   `json:"policyVersionId"`
	SectionOrder                   int      `json:"sectionOrder"`
	ObligationAssessmentMappingIDs []string `json:"obligationAssessmentMappingIds,omitempty"`
}
type complianceControlDraft struct {
	Title          string                  `json:"title"`
	Statement      string                  `json:"statement"`
	Objective      string                  `json:"objective"`
	Implementation string                  `json:"implementation"`
	Purpose        string                  `json:"purpose" jsonschema:"preventive,detective,corrective"`
	TestPlan       string                  `json:"testPlan"`
	ReviewCadence  string                  `json:"reviewCadence" jsonschema:"event_driven,monthly,quarterly,semi_annual,annual"`
	EffectiveFrom  string                  `json:"effectiveFrom,omitempty"`
	EffectiveUntil string                  `json:"effectiveUntil,omitempty"`
	DepartmentIDs  []string                `json:"departmentIds"`
	TeamIDs        []string                `json:"teamIds"`
	PolicyBases    []compliancePolicyBasis `json:"policyBases"`
}
type complianceControlCreate struct {
	complianceControlDraft
	IdempotencyKey     string   `json:"idempotencyKey"`
	MaintainerStaffIDs []string `json:"maintainerStaffIds"`
}
type complianceControlSave struct {
	complianceControlDraft
	IdempotencyKey   string `json:"idempotencyKey"`
	ControlID        string `json:"controlId"`
	ControlVersionID string `json:"controlVersionId"`
	Revision         int    `json:"revision"`
}
type complianceApplicability struct {
	Global     bool     `json:"global"`
	Countries  []string `json:"countries"`
	Regions    []string `json:"regions"`
	Industries []string `json:"industries"`
}
type complianceCustomFrameworkCreate struct {
	IdempotencyKey string                  `json:"idempotencyKey"`
	Name           string                  `json:"name"`
	ShortName      string                  `json:"shortName,omitempty"`
	Slug           string                  `json:"slug"`
	Summary        string                  `json:"summary"`
	SourceURL      string                  `json:"sourceUrl,omitempty"`
	Applicability  complianceApplicability `json:"applicability"`
}
type complianceFrameworkSection struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Order int    `json:"order"`
}
type complianceFrameworkContent struct {
	Sections []complianceFrameworkSection `json:"sections"`
}
type complianceCustomVersionCreate struct {
	IdempotencyKey string                     `json:"idempotencyKey"`
	FrameworkID    string                     `json:"frameworkId"`
	Version        string                     `json:"version"`
	EffectiveDate  string                     `json:"effectiveDate,omitempty"`
	Content        complianceFrameworkContent `json:"content"`
}
type complianceCustomVersionSave struct {
	IdempotencyKey     string                      `json:"idempotencyKey"`
	FrameworkID        string                      `json:"frameworkId"`
	VersionID          string                      `json:"versionId"`
	ClearEffectiveDate bool                        `json:"clearEffectiveDate,omitempty" jsonschema:"Explicitly clear the nullable effective date; omit effectiveDate when true"`
	Revision           int                         `json:"revision"`
	Version            string                      `json:"version,omitempty"`
	EffectiveDate      string                      `json:"effectiveDate,omitempty"`
	Content            *complianceFrameworkContent `json:"content,omitempty"`
}
type complianceControlAuthoringRetry struct {
	ProposalID        string `json:"proposalId"`
	IdempotencyKey    string `json:"idempotencyKey"`
	ExpectedUpdatedAt string `json:"expectedUpdatedAt" jsonschema:"Exact updatedAt of the failed authoring job returned by its current read"`
}
type complianceControlAuthoring struct {
	IdempotencyKey string                  `json:"idempotencyKey"`
	Instructions   string                  `json:"instructions,omitempty"`
	DepartmentIDs  []string                `json:"departmentIds"`
	TeamIDs        []string                `json:"teamIds"`
	PolicyBases    []compliancePolicyBasis `json:"policyBases"`
}
type complianceImplementationCreate struct {
	IdempotencyKey      string `json:"idempotencyKey"`
	ControlID           string `json:"controlId"`
	ControlVersionID    string `json:"controlVersionId"`
	BoundaryType        string `json:"boundaryType" jsonschema:"department,team,ai_system"`
	BoundaryID          string `json:"boundaryId"`
	Implementation      string `json:"implementation"`
	DifferenceRationale string `json:"differenceRationale"`
	OperatorStaffID     string `json:"operatorStaffId"`
	ReviewCadence       string `json:"reviewCadence"`
}
type complianceImplementationSave struct {
	IdempotencyKey      string `json:"idempotencyKey"`
	ControlID           string `json:"controlId"`
	VersionID           string `json:"versionId"`
	Revision            int    `json:"revision"`
	Implementation      string `json:"implementation"`
	DifferenceRationale string `json:"differenceRationale"`
	OperatorStaffID     string `json:"operatorStaffId"`
	ReviewCadence       string `json:"reviewCadence"`
}
type complianceScopeAnswer struct {
	QuestionID  string  `json:"questionId"`
	SubjectType string  `json:"subjectType"`
	SubjectID   *string `json:"subjectId"`
	Value       any     `json:"value" jsonschema:"Question's exact boolean,string,string array,or number answer; server validates against published question"`
}
type complianceScopeAnswers struct {
	IdempotencyKey                        string                  `json:"idempotencyKey"`
	OrganisationFrameworkID               string                  `json:"organisationFrameworkId"`
	ExpectedOrganisationFrameworkRevision int                     `json:"expectedOrganisationFrameworkRevision"`
	ExpectedDraftRevision                 *int                    `json:"expectedDraftRevision"`
	BaseScopeSnapshotID                   string                  `json:"baseScopeSnapshotId"`
	Answers                               []complianceScopeAnswer `json:"answers"`
}
type complianceFrameworkHandoff struct {
	Action                  string `json:"action" jsonschema:"activate,suspend,scope,custom_create,custom_edit,custom_version,custom_publish,custom_import,custom_scoping_review,custom_obligations"`
	OrganisationFrameworkID string `json:"organisationFrameworkId,omitempty"`
	FrameworkID             string `json:"frameworkId,omitempty"`
	VersionID               string `json:"versionId,omitempty"`
}
type complianceControlHandoff struct {
	Action                  string `json:"action" jsonschema:"create,edit,review,retire,maintainers,implementation,mapping,coverage_review"`
	ControlID               string `json:"controlId,omitempty"`
	ControlVersionID        string `json:"controlVersionId,omitempty"`
	OrganisationFrameworkID string `json:"organisationFrameworkId,omitempty"`
	DecisionID              string `json:"decisionId,omitempty"`
}

const complianceConfigurationPath = "/organisation/mcp/compliance-configuration"

func configUUID(value string) (string, error) {
	if !policyUUIDs(value) {
		return "", errors.New("Select the exact UUID returned by AuthRail")
	}
	return value, nil
}
func configReadPath(kind string, input complianceConfigReadInput) (string, error) {
	if input.Limit < 0 || input.Limit > 20 || len(input.Query) > 120 {
		return "", errors.New("Use at most20 records and120 search characters")
	}
	path := complianceConfigurationPath
	parts := []string{}
	switch kind {
	case "frameworks", "custom-frameworks", "controls":
		parts = []string{kind}
	case "versions":
		parts = []string{"custom-frameworks", input.FrameworkID, "versions"}
	case "version":
		parts = []string{"custom-frameworks", input.FrameworkID, "versions", input.VersionID}
	case "scope":
		parts = []string{"frameworks", input.OrganisationFrameworkID, "scope"}
	case "draft":
		parts = []string{"controls", input.ControlID, "draft"}
	case "implementations":
		parts = []string{"controls", input.ControlID, "implementations"}
	case "decisions":
		parts = []string{"frameworks", input.OrganisationFrameworkID, "coverage-decisions"}
	case "reviews":
		parts = []string{"decisions", input.DecisionID, "reviews"}
	case "imports":
		parts = []string{"custom-frameworks", input.FrameworkID, "imports"}
	case "import":
		parts = []string{"custom-frameworks", input.FrameworkID, "imports", input.ImportID}
	case "authoring":
		parts = []string{"control-authoring", input.ProposalID}
	}
	for i, part := range parts {
		if i%2 == 1 {
			if _, err := configUUID(part); err != nil {
				return "", err
			}
		}
		path += "/" + part
	}
	q := url.Values{}
	if input.ContentCursor != "" {
		if kind != "scope" || len(input.ContentCursor) > 2000 {
			return "", errors.New("Use a content continuation only for the exact scope page")
		}
		q.Set("contentCursor", input.ContentCursor)
	}
	if input.Query != "" {
		q.Set("query", input.Query)
	}
	page, err := workspacePage(q, workspacePageInput{Limit: input.Limit, Cursor: input.Cursor})
	return path + "?" + page, err
}
func addComplianceConfigurationTools(server *mcp.Server, backend *Backend, scopes []string) {
	if slices.Contains(scopes, ComplianceConfigurationReadScope) {
		for _, spec := range []struct{ name, kind string }{
			{"authrail_list_compliance_frameworks", "frameworks"}, {"authrail_list_custom_frameworks", "custom-frameworks"}, {"authrail_list_custom_framework_versions", "versions"}, {"authrail_get_custom_framework_version", "version"}, {"authrail_get_framework_scope", "scope"}, {"authrail_list_controls", "controls"}, {"authrail_get_control_draft", "draft"}, {"authrail_list_control_implementations", "implementations"}, {"authrail_list_coverage_decisions", "decisions"}, {"authrail_list_coverage_reviews", "reviews"}, {"authrail_list_custom_framework_imports", "imports"}, {"authrail_get_custom_framework_import", "import"}, {"authrail_get_control_authoring", "authoring"},
		} {
			if spec.kind == "authoring" && !slices.Contains(scopes, PoliciesReadScope) {
				continue
			}
			addWorkspaceTool(server, backend, delegatedToolInput[complianceConfigReadInput]{name: spec.name, description: "Read bounded existing compliance configuration under current permissions and exact owner references. Continuations bind workspace,caller,tool,filters and content revision. Stored status is not proof of independent review.", scope: ComplianceConfigurationReadScope, method: http.MethodGet, readonly: true, prepare: func(input complianceConfigReadInput) (string, any, error) {
				path, err := configReadPath(spec.kind, input)
				return path, nil, err
			}})
		}
	}
	if !slices.Contains(scopes, ComplianceConfigurationManageScope) {
		return
	}
	addWorkspaceTool(server, backend, delegatedToolInput[complianceControlCreate]{name: "authrail_create_control_draft", description: "Prepare an unapproved organisation Control draft using exact published Policy section references and active maintainers. Stable command key is required; no review is approved.", scope: ComplianceConfigurationManageScope, method: http.MethodPost, prepare: func(input complianceControlCreate) (string, any, error) {
		return complianceConfigurationPath + "/controls", input, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[complianceControlSave]{name: "authrail_save_control_draft", description: "Save one exact Control draft revision; unchanged retries retain the original saved reference and changed-input keys conflict.", scope: ComplianceConfigurationManageScope, method: http.MethodPost, prepare: func(input complianceControlSave) (string, any, error) {
		id, err := configUUID(input.ControlID)
		return complianceConfigurationPath + "/controls/" + id + "/draft", withoutConfigRouting(input), err
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[complianceCustomFrameworkCreate]{name: "authrail_create_custom_framework_draft", description: "Create an unpublished custom Framework starting point; applicability and publication require human review.", scope: ComplianceConfigurationManageScope, method: http.MethodPost, prepare: func(input complianceCustomFrameworkCreate) (string, any, error) {
		return complianceConfigurationPath + "/custom-frameworks", input, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[complianceCustomVersionCreate]{name: "authrail_create_custom_framework_version", description: "Prepare a new unpublished custom Framework version from typed ordered sections. This does not approve Obligations,scope or publication.", scope: ComplianceConfigurationManageScope, method: http.MethodPost, prepare: func(input complianceCustomVersionCreate) (string, any, error) {
		id, err := configUUID(input.FrameworkID)
		return complianceConfigurationPath + "/custom-frameworks/" + id + "/versions", withoutConfigRouting(input), err
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[complianceCustomVersionSave]{name: "authrail_save_custom_framework_version", description: "Save exact unpublished custom Framework version content at its expected revision. Human review and publication stay separate.", scope: ComplianceConfigurationManageScope, method: http.MethodPost, prepare: func(input complianceCustomVersionSave) (string, any, error) {
		if input.ClearEffectiveDate && input.EffectiveDate != "" {
			return "", nil, errors.New("Use effectiveDate or clearEffectiveDate, never both")
		}
		id, err := configUUID(input.FrameworkID)
		if err != nil {
			return "", nil, err
		}
		version, err := configUUID(input.VersionID)
		body := withoutConfigRouting(input)
		if input.ClearEffectiveDate {
			body["effectiveDate"] = nil
		}
		return complianceConfigurationPath + "/custom-frameworks/" + id + "/versions/" + version, body, err
	}})
	if slices.Contains(scopes, PoliciesReadScope) {
		addWorkspaceTool(server, backend, delegatedToolInput[complianceControlAuthoring]{name: "authrail_start_control_authoring", description: "Request a retained AI Control proposal from selected permitted Policy bases; policies.read consent is also required. Read the original proposal reference to follow its job; a proposal is not an approved Control.", scope: ComplianceConfigurationManageScope, method: http.MethodPost, prepare: func(input complianceControlAuthoring) (string, any, error) {
			return complianceConfigurationPath + "/control-authoring", input, nil
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceControlAuthoringRetry]{name: "authrail_retry_control_authoring", description: "Retry an exact failed Control authoring job after reading its current status. Configuration manage plus policies.read consent and current original Policy source access apply. Retain the command key unchanged after uncertainty.", scope: ComplianceConfigurationManageScope, method: http.MethodPost, prepare: func(input complianceControlAuthoringRetry) (string, any, error) {
			id, err := configUUID(input.ProposalID)
			return complianceConfigurationPath + "/control-authoring/" + id + "/retry", struct {
				IdempotencyKey    string `json:"idempotencyKey"`
				ExpectedUpdatedAt string `json:"expectedUpdatedAt"`
			}{input.IdempotencyKey, input.ExpectedUpdatedAt}, err
		}})

	}
	addWorkspaceTool(server, backend, delegatedToolInput[complianceImplementationCreate]{name: "authrail_create_control_implementation_draft", description: "Prepare a scoped Implementation draft for a genuine operational difference under an exact approved Control. Existing boundary permissions apply.", scope: ComplianceConfigurationManageScope, method: http.MethodPost, prepare: func(input complianceImplementationCreate) (string, any, error) {
		id, err := configUUID(input.ControlID)
		return complianceConfigurationPath + "/controls/" + id + "/implementations", withoutConfigRouting(input), err
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[complianceImplementationSave]{name: "authrail_save_control_implementation_draft", description: "Save an exact scoped Implementation draft revision without approving its independent review.", scope: ComplianceConfigurationManageScope, method: http.MethodPost, prepare: func(input complianceImplementationSave) (string, any, error) {
		id, err := configUUID(input.ControlID)
		if err != nil {
			return "", nil, err
		}
		version, err := configUUID(input.VersionID)
		return complianceConfigurationPath + "/controls/" + id + "/implementations/" + version, withoutConfigRouting(input), err
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[complianceScopeAnswers]{name: "authrail_save_framework_scope_answers", description: "Prepare manual answers for exact published Framework scope questions. Saves only draft answers; never accepts applicability decisions,review or scope activation.", scope: ComplianceConfigurationManageScope, method: http.MethodPost, prepare: func(input complianceScopeAnswers) (string, any, error) {
		id, err := configUUID(input.OrganisationFrameworkID)
		return complianceConfigurationPath + "/frameworks/" + id + "/scope-answers", withoutConfigRouting(input), err
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[complianceFrameworkHandoff]{name: "authrail_prepare_framework_handoff", description: "Open a private signed same-site handoff for Framework activation/suspension,scope acceptance,custom publication,Obligation/scoping review or private file import. Navigation applies no values or decisions; review current details yourself.", scope: ComplianceConfigurationManageScope, method: http.MethodPost, prepare: func(input complianceFrameworkHandoff) (string, any, error) {
		return complianceConfigurationPath + "/framework-handoff", input, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[complianceControlHandoff]{name: "authrail_prepare_control_handoff", description: "Open a private signed same-site handoff for exact Control/Implementation review,maintainers,lifecycle and Mapping/coverage decisions. Navigation grants no approval and changes no domain record.", scope: ComplianceConfigurationManageScope, method: http.MethodPost, prepare: func(input complianceControlHandoff) (string, any, error) {
		return complianceConfigurationPath + "/control-handoff", input, nil
	}})
}

// Inputs are typed JSON decoded by the SDK. Routing identities belong only in the fixed URL.
func withoutConfigRouting(input any) map[string]any {
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil
	}
	var body map[string]any
	if json.Unmarshal(encoded, &body) != nil {
		return nil
	}
	for _, key := range []string{"frameworkId", "controlId", "versionId", "organisationFrameworkId", "clearEffectiveDate"} {
		delete(body, key)
	}
	return body
}

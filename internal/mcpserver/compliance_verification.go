package mcpserver

import (
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/url"
	"slices"
	"strconv"
)

const ComplianceVerificationReadScope = "compliance.verification.read"
const ComplianceVerificationManageScope = "compliance.verification.manage"

type complianceVerificationPageInput struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Page size 1 to 20; defaults to 20"`
}
type complianceVerificationRecordInput struct {
	RecordID string `json:"recordId" jsonschema:"Exact visible record UUID"`
}
type complianceVerificationOwnerInput struct {
	ControlID               string `json:"controlId"`
	ControlVersionID        string `json:"controlVersionId"`
	ImplementationVersionID string `json:"implementationVersionId,omitempty"`
}
type complianceVerificationHandoffInput struct {
	complianceVerificationOwnerInput
	DefinitionID string `json:"definitionId"`
	RecordID     string `json:"recordId,omitempty"`
}
type complianceVerificationRemoveInput struct {
	complianceVerificationOwnerInput
	DefinitionID   string `json:"definitionId"`
	Revision       int    `json:"revision"`
	IdempotencyKey string `json:"idempotencyKey" jsonschema:"Stable command UUID; reuse unchanged for retries"`
}
type complianceRuntimeQualification struct {
	DeploymentID       string `json:"deploymentId"`
	OperationVersionID string `json:"operationVersionId"`
	Mode               string `json:"mode"`
	RequiredOutcome    string `json:"requiredOutcome"`
	MinimumActions     int    `json:"minimumActions"`
}
type complianceVerificationSaveInput struct {
	complianceVerificationOwnerInput
	ID                   string                          `json:"id" jsonschema:"Exact draft definition UUID or new UUID"`
	Revision             int                             `json:"revision"`
	IdempotencyKey       string                          `json:"idempotencyKey"`
	Title                string                          `json:"title"`
	Procedure            string                          `json:"procedure"`
	CadenceDays          int                             `json:"cadenceDays"`
	FreshnessDays        int                             `json:"freshnessDays"`
	RuntimeQualification *complianceRuntimeQualification `json:"runtimeQualification,omitempty"`
}
type complianceEvidenceSaveInput struct {
	complianceVerificationOwnerInput
	ID             string `json:"id"`
	Revision       int    `json:"revision"`
	IdempotencyKey string `json:"idempotencyKey"`
	Title          string `json:"title"`
	ExpectedProof  string `json:"expectedProof"`
	SourceType     string `json:"sourceType" jsonschema:"attachment test_run attestation or observation"`
	CadenceDays    int    `json:"cadenceDays"`
	FreshnessDays  int    `json:"freshnessDays"`
	PeriodDays     int    `json:"periodDays"`
	OwnerStaffID   string `json:"ownerStaffId"`
}
type complianceTaskCreateInput struct {
	FindingID       string `json:"findingId"`
	IdempotencyKey  string `json:"idempotencyKey"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	OwnerStaffID    string `json:"ownerStaffId"`
	AssigneeStaffID string `json:"assigneeStaffId,omitempty"`
	Priority        string `json:"priority"`
	DueAt           string `json:"dueAt,omitempty"`
}
type complianceTaskUpdateInput struct {
	TaskID          string `json:"taskId"`
	IdempotencyKey  string `json:"idempotencyKey"`
	Revision        int    `json:"revision"`
	State           string `json:"state" jsonschema:"open in_progress completed or cancelled"`
	OwnerStaffID    string `json:"ownerStaffId,omitempty"`
	AssigneeStaffID string `json:"assigneeStaffId,omitempty"`
	ClearAssignee   bool   `json:"clearAssignee,omitempty" jsonschema:"Clear the current assignee; cannot combine with assigneeStaffId"`
	DueAt           string `json:"dueAt,omitempty"`
	ClearDueDate    bool   `json:"clearDueDate,omitempty" jsonschema:"Clear the current due date; cannot combine with dueAt"`
	CompletionProof string `json:"completionProof,omitempty"`
	Reason          string `json:"reason"`
}
type compliancePackageHandoffInput struct {
	Action     string `json:"action" jsonschema:"create edit upload download review archive or restore"`
	EvidenceID string `json:"evidenceId,omitempty"`
}
type complianceExceptionHandoffInput struct {
	Action                  string `json:"action" jsonschema:"request decide or withdraw"`
	OrganisationFrameworkID string `json:"organisationFrameworkId"`
	ExceptionID             string `json:"exceptionId,omitempty"`
}
type complianceReportListInput struct {
	OrganisationFrameworkID string `json:"organisationFrameworkId"`
	Cursor                  string `json:"cursor,omitempty"`
	Limit                   int    `json:"limit,omitempty"`
}
type complianceReportInput struct {
	OrganisationFrameworkID string `json:"organisationFrameworkId"`
	ReportID                string `json:"reportId"`
	Cursor                  string `json:"cursor,omitempty"`
	Limit                   int    `json:"limit,omitempty"`
}
type complianceReportHandoffInput struct {
	OrganisationFrameworkID string `json:"organisationFrameworkId"`
	Action                  string `json:"action" jsonschema:"request export or download"`
	ReportID                string `json:"reportId,omitempty"`
}

func complianceVerificationPage(cursor string, limit int) (string, error) {
	if limit < 0 || limit > 20 || len(cursor) > 2000 {
		return "", errors.New("Use page size 1 to 20 and the exact previous cursor")
	}
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	return q.Encode(), nil
}
func validComplianceOwner(input complianceVerificationOwnerInput) bool {
	return frameworkID.MatchString(input.ControlID) && frameworkID.MatchString(input.ControlVersionID) && (input.ImplementationVersionID == "" || frameworkID.MatchString(input.ImplementationVersionID))
}
func addComplianceVerificationTools(server *mcp.Server, backend *Backend, scopes []string) {
	if slices.Contains(scopes, ComplianceVerificationReadScope) {
		addResourceHistoryTool(server, backend)
		addWorkspaceTool(server, backend, delegatedToolInput[complianceVerificationHandoffInput]{name: "authrail_get_test_definition", description: "Read one exact visible definition with bounded procedure or expected-proof content and the current owner revision. This neither records proof nor selects another version.", scope: ComplianceVerificationReadScope, method: http.MethodGet, readonly: true, prepare: func(input complianceVerificationHandoffInput) (string, any, error) {
			if !validComplianceOwner(input.complianceVerificationOwnerInput) || !frameworkID.MatchString(input.DefinitionID) || input.RecordID != "" {
				return "", nil, errors.New("Use exact owner and definition UUIDs")
			}
			q := url.Values{"controlId": {input.ControlID}, "controlVersionId": {input.ControlVersionID}, "definitionId": {input.DefinitionID}}
			if input.ImplementationVersionID != "" {
				q.Set("implementationVersionId", input.ImplementationVersionID)
			}
			return "/organisation/mcp/verification/test-definitions/detail?" + q.Encode(), nil, nil
		}})

		addWorkspaceTool(server, backend, delegatedToolInput[complianceVerificationHandoffInput]{name: "authrail_get_evidence_requirement", description: "Read one exact visible definition with bounded procedure or expected-proof content and the current owner revision. This neither records proof nor selects another version.", scope: ComplianceVerificationReadScope, method: http.MethodGet, readonly: true, prepare: func(input complianceVerificationHandoffInput) (string, any, error) {
			if !validComplianceOwner(input.complianceVerificationOwnerInput) || !frameworkID.MatchString(input.DefinitionID) || input.RecordID != "" {
				return "", nil, errors.New("Use exact owner and definition UUIDs")
			}
			q := url.Values{"controlId": {input.ControlID}, "controlVersionId": {input.ControlVersionID}, "definitionId": {input.DefinitionID}}
			if input.ImplementationVersionID != "" {
				q.Set("implementationVersionId", input.ImplementationVersionID)
			}
			return "/organisation/mcp/verification/evidence-requirements/detail?" + q.Encode(), nil, nil
		}})

		addWorkspaceTool(server, backend, delegatedToolInput[complianceVerificationPageInput]{name: "authrail_list_operational_findings", description: "Find visible source-owned compliance gaps; remediation completion does not resolve the source.", scope: ComplianceVerificationReadScope, method: http.MethodGet, readonly: true, prepare: func(input complianceVerificationPageInput) (string, any, error) {
			q, err := complianceVerificationPage(input.Cursor, input.Limit)
			return "/organisation/mcp/verification/findings?" + q, nil, err
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceVerificationPageInput]{name: "authrail_list_remediation_tasks", description: "Read visible remediation Tasks and lifecycle; completing work does not close its Finding.", scope: ComplianceVerificationReadScope, method: http.MethodGet, readonly: true, prepare: func(input complianceVerificationPageInput) (string, any, error) {
			q, err := complianceVerificationPage(input.Cursor, input.Limit)
			return "/organisation/mcp/verification/tasks?" + q, nil, err
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceVerificationPageInput]{name: "authrail_list_readiness_exceptions", description: "Read current exact Exceptions without treating the source as healthy.", scope: ComplianceVerificationReadScope, method: http.MethodGet, readonly: true, prepare: func(input complianceVerificationPageInput) (string, any, error) {
			q, err := complianceVerificationPage(input.Cursor, input.Limit)
			return "/organisation/mcp/verification/exceptions?" + q, nil, err
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceVerificationPageInput]{name: "authrail_list_evidence_packages", description: "Read bounded Evidence package status; file bytes and download capabilities remain private.", scope: ComplianceVerificationReadScope, method: http.MethodGet, readonly: true, prepare: func(input complianceVerificationPageInput) (string, any, error) {
			q, err := complianceVerificationPage(input.Cursor, input.Limit)
			return "/organisation/mcp/verification/evidence-packages?" + q, nil, err
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceVerificationRecordInput]{name: "authrail_get_operational_finding", description: "Read an exact visible Finding and retained status; source facts retain their owning authority.", scope: ComplianceVerificationReadScope, method: http.MethodGet, readonly: true, prepare: func(input complianceVerificationRecordInput) (string, any, error) {
			if !frameworkID.MatchString(input.RecordID) {
				return "", nil, errors.New("Use an exact visible record UUID")
			}
			return "/organisation/mcp/verification/findings/" + input.RecordID, nil, nil
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceVerificationRecordInput]{name: "authrail_get_remediation_task", description: "Read one exact visible remediation Task.", scope: ComplianceVerificationReadScope, method: http.MethodGet, readonly: true, prepare: func(input complianceVerificationRecordInput) (string, any, error) {
			if !frameworkID.MatchString(input.RecordID) {
				return "", nil, errors.New("Use an exact visible record UUID")
			}
			return "/organisation/mcp/verification/tasks/" + input.RecordID, nil, nil
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceVerificationRecordInput]{name: "authrail_get_readiness_exception", description: "Read one exact visible Exception and effective state.", scope: ComplianceVerificationReadScope, method: http.MethodGet, readonly: true, prepare: func(input complianceVerificationRecordInput) (string, any, error) {
			if !frameworkID.MatchString(input.RecordID) {
				return "", nil, errors.New("Use an exact visible record UUID")
			}
			return "/organisation/mcp/verification/exceptions/" + input.RecordID, nil, nil
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceVerificationRecordInput]{name: "authrail_get_evidence_package", description: "Read exact Evidence package metadata without file contents or signed storage URLs.", scope: ComplianceVerificationReadScope, method: http.MethodGet, readonly: true, prepare: func(input complianceVerificationRecordInput) (string, any, error) {
			if !frameworkID.MatchString(input.RecordID) {
				return "", nil, errors.New("Use an exact visible record UUID")
			}
			return "/organisation/mcp/verification/evidence-packages/" + input.RecordID, nil, nil
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceReportListInput]{name: "authrail_list_framework_reports", description: "Read bounded visible immutable report captures; point-in-time evidence is not legal certification.", scope: ComplianceVerificationReadScope, method: http.MethodGet, readonly: true, prepare: func(input complianceReportListInput) (string, any, error) {
			if !frameworkID.MatchString(input.OrganisationFrameworkID) {
				return "", nil, errors.New("Use the exact Framework workspace UUID")
			}
			q, err := complianceVerificationPage(input.Cursor, input.Limit)
			return "/organisation/mcp/verification/frameworks/" + input.OrganisationFrameworkID + "/reports?" + q, nil, err
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceReportInput]{name: "authrail_get_framework_report", description: "Read bounded retained Obligation readiness summaries and safe own export status. Private proof manifests and archives remain in AuthRail; exact stored source audiences are rechecked.", scope: ComplianceVerificationReadScope, method: http.MethodGet, readonly: true, prepare: func(input complianceReportInput) (string, any, error) {
			if !frameworkID.MatchString(input.OrganisationFrameworkID) || !frameworkID.MatchString(input.ReportID) {
				return "", nil, errors.New("Use exact Framework and report UUIDs")
			}
			q, err := complianceVerificationPage(input.Cursor, input.Limit)
			return "/organisation/mcp/verification/frameworks/" + input.OrganisationFrameworkID + "/reports/" + input.ReportID + "?" + q, nil, err
		}})
	}
	if slices.Contains(scopes, ComplianceVerificationManageScope) {
		addWorkspaceTool(server, backend, delegatedToolInput[complianceVerificationSaveInput]{name: "authrail_save_test_definition", description: "Prepare an exact revision-bound verification definition in the existing Control or scoped Implementation draft. Current permissions and audience apply; no publication or proof is recorded. Reuse the stable command UUID only with unchanged input.", scope: ComplianceVerificationManageScope, method: http.MethodPost, prepare: func(input complianceVerificationSaveInput) (string, any, error) {
			if !validComplianceOwner(input.complianceVerificationOwnerInput) || !frameworkID.MatchString(input.IdempotencyKey) || input.Revision < 1 {
				return "", nil, errors.New("Use exact owner IDs, current draft revision and stable command UUID")
			}
			return "/organisation/mcp/verification/test-definitions", input, nil
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceEvidenceSaveInput]{name: "authrail_save_evidence_requirement", description: "Prepare an exact revision-bound verification definition in the existing Control or scoped Implementation draft. Current permissions and audience apply; no publication or proof is recorded. Reuse the stable command UUID only with unchanged input.", scope: ComplianceVerificationManageScope, method: http.MethodPost, prepare: func(input complianceEvidenceSaveInput) (string, any, error) {
			if !validComplianceOwner(input.complianceVerificationOwnerInput) || !frameworkID.MatchString(input.IdempotencyKey) || input.Revision < 1 {
				return "", nil, errors.New("Use exact owner IDs, current draft revision and stable command UUID")
			}
			return "/organisation/mcp/verification/evidence-requirements", input, nil
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceVerificationRemoveInput]{name: "authrail_remove_test_definition", description: "Prepare an exact revision-bound verification definition in the existing Control or scoped Implementation draft. Current permissions and audience apply; no publication or proof is recorded. Reuse the stable command UUID only with unchanged input.", scope: ComplianceVerificationManageScope, method: http.MethodPost, prepare: func(input complianceVerificationRemoveInput) (string, any, error) {
			if !validComplianceOwner(input.complianceVerificationOwnerInput) || !frameworkID.MatchString(input.IdempotencyKey) || input.Revision < 1 {
				return "", nil, errors.New("Use exact owner IDs, current draft revision and stable command UUID")
			}
			return "/organisation/mcp/verification/test-definitions/remove", input, nil
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceVerificationRemoveInput]{name: "authrail_remove_evidence_requirement", description: "Prepare an exact revision-bound verification definition in the existing Control or scoped Implementation draft. Current permissions and audience apply; no publication or proof is recorded. Reuse the stable command UUID only with unchanged input.", scope: ComplianceVerificationManageScope, method: http.MethodPost, prepare: func(input complianceVerificationRemoveInput) (string, any, error) {
			if !validComplianceOwner(input.complianceVerificationOwnerInput) || !frameworkID.MatchString(input.IdempotencyKey) || input.Revision < 1 {
				return "", nil, errors.New("Use exact owner IDs, current draft revision and stable command UUID")
			}
			return "/organisation/mcp/verification/evidence-requirements/remove", input, nil
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceTaskCreateInput]{name: "authrail_create_remediation_task", description: "Create a Task for an exact visible Finding. This records work, not a passing Test, accepted Evidence or resolved source. Use a stable command UUID.", scope: ComplianceVerificationManageScope, method: http.MethodPost, prepare: func(input complianceTaskCreateInput) (string, any, error) {
			if !frameworkID.MatchString(input.FindingID) || !frameworkID.MatchString(input.IdempotencyKey) {
				return "", nil, errors.New("Use exact Finding and command UUIDs")
			}
			return "/organisation/mcp/verification/tasks", input, nil
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceTaskUpdateInput]{name: "authrail_update_remediation_task", description: "Assign or progress one exact Task revision with a reason and completion proof when completing. Task completion never resolves its Finding. Retry unchanged with the same command UUID.", scope: ComplianceVerificationManageScope, method: http.MethodPost, prepare: func(input complianceTaskUpdateInput) (string, any, error) {
			if !frameworkID.MatchString(input.TaskID) || !frameworkID.MatchString(input.IdempotencyKey) || input.Revision < 1 {
				return "", nil, errors.New("Use exact Task, current revision and stable command UUID")
			}
			if input.ClearAssignee && input.AssigneeStaffID != "" || input.ClearDueDate && input.DueAt != "" {
				return "", nil, errors.New("Choose a value or clear it, not both")
			}
			body := map[string]any{"idempotencyKey": input.IdempotencyKey, "revision": input.Revision, "state": input.State, "reason": input.Reason}
			if input.OwnerStaffID != "" {
				body["ownerStaffId"] = input.OwnerStaffID
			}
			if input.ClearAssignee {
				body["assigneeStaffId"] = nil
			} else if input.AssigneeStaffID != "" {
				body["assigneeStaffId"] = input.AssigneeStaffID
			}
			if input.ClearDueDate {
				body["dueAt"] = nil
			} else if input.DueAt != "" {
				body["dueAt"] = input.DueAt
			}
			if input.CompletionProof != "" {
				body["completionProof"] = input.CompletionProof
			}
			return "/organisation/mcp/verification/tasks/" + input.TaskID, body, nil
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceVerificationHandoffInput]{name: "authrail_record_test_run", description: "Continue exact proof recording or independent review in authenticated AuthRail. This navigation records no run or Evidence decision and returns no private source or storage capability. Current owner, source audience and permissions are checked again.", scope: ComplianceVerificationManageScope, method: http.MethodPost, readonly: true, prepare: func(input complianceVerificationHandoffInput) (string, any, error) {
			if !validComplianceOwner(input.complianceVerificationOwnerInput) || !frameworkID.MatchString(input.DefinitionID) || input.RecordID != "" {
				return "", nil, errors.New("Use the exact owner and definition; an exact record is required only for review")
			}
			return "/organisation/mcp/verification/test-runs/handoff", input, nil
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceVerificationHandoffInput]{name: "authrail_submit_evidence_record", description: "Continue exact proof recording or independent review in authenticated AuthRail. This navigation records no run or Evidence decision and returns no private source or storage capability. Current owner, source audience and permissions are checked again.", scope: ComplianceVerificationManageScope, method: http.MethodPost, readonly: true, prepare: func(input complianceVerificationHandoffInput) (string, any, error) {
			if !validComplianceOwner(input.complianceVerificationOwnerInput) || !frameworkID.MatchString(input.DefinitionID) || input.RecordID != "" {
				return "", nil, errors.New("Use the exact owner and definition; an exact record is required only for review")
			}
			return "/organisation/mcp/verification/evidence-records/handoff", input, nil
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceVerificationHandoffInput]{name: "authrail_review_evidence_record", description: "Continue exact proof recording or independent review in authenticated AuthRail. This navigation records no run or Evidence decision and returns no private source or storage capability. Current owner, source audience and permissions are checked again.", scope: ComplianceVerificationManageScope, method: http.MethodPost, readonly: true, prepare: func(input complianceVerificationHandoffInput) (string, any, error) {
			if !validComplianceOwner(input.complianceVerificationOwnerInput) || !frameworkID.MatchString(input.DefinitionID) || !frameworkID.MatchString(input.RecordID) {
				return "", nil, errors.New("Use the exact owner and definition; an exact record is required only for review")
			}
			return "/organisation/mcp/verification/evidence-records/review-handoff", input, nil
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[compliancePackageHandoffInput]{name: "authrail_manage_evidence_package", description: "Continue private Evidence creation, file upload/download or human review. No file or signed storage URL is exposed and navigation changes nothing.", scope: ComplianceVerificationManageScope, method: http.MethodPost, readonly: true, prepare: func(input compliancePackageHandoffInput) (string, any, error) {
			if input.EvidenceID != "" && !frameworkID.MatchString(input.EvidenceID) {
				return "", nil, errors.New("Use an exact Evidence package UUID")
			}
			return "/organisation/mcp/verification/evidence-packages/handoff", input, nil
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceVerificationRecordInput]{name: "authrail_manage_operational_finding", description: "Continue source-owned Finding remediation or a reserved manual source decision in the authenticated workspace. This link applies no changes.", scope: ComplianceVerificationManageScope, method: http.MethodPost, readonly: true, prepare: func(input complianceVerificationRecordInput) (string, any, error) {
			if !frameworkID.MatchString(input.RecordID) {
				return "", nil, errors.New("Use the exact visible Finding UUID")
			}
			return "/organisation/mcp/verification/findings/" + input.RecordID + "/handoff", struct{}{}, nil
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceExceptionHandoffInput]{name: "authrail_manage_readiness_exception", description: "Continue exact Exception request, independent decision or withdrawal privately. This does not approve an Exception or make a source healthy.", scope: ComplianceVerificationManageScope, method: http.MethodPost, readonly: true, prepare: func(input complianceExceptionHandoffInput) (string, any, error) {
			if !frameworkID.MatchString(input.OrganisationFrameworkID) || (input.ExceptionID != "" && !frameworkID.MatchString(input.ExceptionID)) {
				return "", nil, errors.New("Use exact Framework and optional Exception UUIDs")
			}
			return "/organisation/mcp/verification/exceptions/handoff", input, nil
		}})
		addWorkspaceTool(server, backend, delegatedToolInput[complianceReportHandoffInput]{name: "authrail_manage_framework_report", description: "Continue exact report capture, archive export or sensitive download in AuthRail. Current source audiences, data-export permission and download recent authentication remain required. Navigation changes nothing.", scope: ComplianceVerificationManageScope, method: http.MethodPost, readonly: true, prepare: func(input complianceReportHandoffInput) (string, any, error) {
			if !frameworkID.MatchString(input.OrganisationFrameworkID) || (input.ReportID != "" && !frameworkID.MatchString(input.ReportID)) {
				return "", nil, errors.New("Use exact Framework and optional report UUIDs")
			}
			body := struct {
				Action   string `json:"action"`
				ReportID string `json:"reportId,omitempty"`
			}{input.Action, input.ReportID}
			return "/organisation/mcp/verification/frameworks/" + input.OrganisationFrameworkID + "/reports/handoff", body, nil
		}})
	}
}

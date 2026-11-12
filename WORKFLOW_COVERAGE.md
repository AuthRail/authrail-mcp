# AuthRail management MCP workflow reference

This retained mapping describes proposed management tools and reviewed server
routes; it is not a list of released tools or a delivery sequence. Current code,
API.md and ARCHITECTURE.md own executable contracts. Governance v1 sequencing is
in ../plan.md; compliance-specific rows are retained for later work. Some phase
and proposal wording below predates the current source: inspect actual handlers
before relying on a tool's availability. The planned unified public MCP entry,
A2A path and root URL require coordinated implementation, not textual renaming.

## Retained server-route and tool mapping

The tables are an on-demand reference for management workflow coverage. They do
not schedule old slices or require deferred compliance work for governance v1.
Inspect actual registered tools and server handlers for availability and exact
human confirmation. Authentication and domain commands remain server owned.

## Connection and authority

The current implemented MCP resource is `https://mcp.authrail.io/mcp`; the planned
root entry `https://mcp.authrail.io` requires a coordinated route/OAuth migration.
Use Streamable HTTP
with authenticated, per-request access. Tool discovery may show only authorised
tools, but hidden discovery entries are not an authorization boundary: every
call still checks the server grant, live membership, capability and object.

Organisation is taken from the authenticated grant. The adapter supplies that
exact `X-Organisation-Id` to the server. Do not accept model-supplied user/staff
identity, arbitrary tenant headers, API origins, paths or methods. There is no
existing `/api` or `/v1` prefix on the server routes listed here.

All tool schemas are closed typed objects. Individual domain content uses its
reviewed DTO/schema; there is no `call_api`, arbitrary JSON patch or generic
SQL/HTTP escape hatch. IDs, exact versions and useful paging tokens appear in
structured results. Unknown fields, unsupported enum values, oversize inputs
and mismatched tenant context are rejected before dispatch.

## Proposed consent capabilities

These retained proposed capability names are not an inventory of currently
registered OAuth scopes or existing organisation permission names.
The consent page translates them into ordinary language. The effective access
is always their intersection with current server authority and target access.

| Capability | Permitted purpose |
| --- | --- |
| `workspace.read` | Read this connection and its selected workspace summary. |
| `governance.read` | Inspect authorised Framework, Policy, Control, verification, readiness and remediation records. |
| `governance.prepare` | Save supported drafts/work items; never accept applicability, proof or publication implicitly. |
| `governance.request_review` | Request an exact human governance decision; not authority for the agent to decide. |
| `runtime.read` | Inspect authorised Inventory, Rails, deployment and Action metadata. |
| `runtime.prepare` | Prepare Inventory, action, source and Rail drafts or an Observe deployment. |
| `runtime.request_review` | Request exact review/publication, activation, trust or lifecycle changes. |
| `approvals.read` | Read only the actual user's assigned Runtime requests and permitted review context. |
| `approvals.request_review` | Prepare a decision for that eligible human to confirm. |
| `connections.read`, `connections.manage` | Read authorised collection metadata or request an allowed collection change/handoff. |
| `organisation.read`, `organisation.manage` | Inspect permitted organisational context or request exact access/settings changes. |
| `activity.read`, `activity.record`, `activity.request_review` | Inspect activity, prepare a manual reported record or request human review. |
| `exports.request` | Request a permitted export and inspect status; not disclose its signed download URL to the model. |
| `sensitive.read` | Explicitly requested bounded content in addition to the owning domain capability and object access. |

Default consent is `workspace.read`. No wildcard or automatic grant of every
owner capability. A scope does not bypass reviewer assignment, resource audience,
recent authentication or a required independent reviewer. No account-wide scope
is included by an organisation grant. More scope requires fresh consent.

## Tool classes and common results

- **R — Read:** bounded lookup; no business mutation. Preserve unavailable and
  partial states rather than reporting zero, pass or complete coverage.
- **P — Prepare:** existing draft/work-item change, collection job or explicitly
  consented model-assisted suggestion. A POST test/analysis may consume resources
  and persist results; label it accordingly rather than calling it read-only.
- **H — Human:** the named tool prepares a server-owned exact confirmation intent.
  The route shown is the eventual domain effect, not a direct model-authorised
  call. Execute only through the authenticated confirmation flow.
- **S — Secure handoff:** authenticated human/operator/client interaction for
  credentials, files, provider consent or sensitive downloads. Return safe status
  and a non-authorizing link, not the secret or signed storage capability.

List inputs expose only named filters and their actual domain paging fields.
A proposed MCP default is 25 items, bounded by the endpoint's lower limit.
Detailed content is opt-in; report projection/continuation explicitly. Initial
response budget is proposed at 64 KiB of serialized structured content. A large
single document needs a bounded server content/chunk contract, not silent loss
of text/citations. Never reuse a cursor across tenants, tools or filters.

Write results distinguish `saved`, `awaiting_confirmation`, `queued`, `completed`,
`conflict` and `outcome_unknown`. These are MCP command outcomes, not invented
Runtime Action states. Stable command identity is separate from the JSON-RPC
request ID. Cancellation or timeout does not establish that a mutation failed.
Existing HTTP 401/403/404/409/422/429 and dependency failures retain safe meaning;
errors never include raw provider responses, SQL, credentials or other tenants.

## Workspace and first working slice

| Proposed tool | Class | Existing API | Authority and input/result contract |
| --- | --- | --- | --- |
| `authrail_get_connection` | R | Proposed MCP connection read, composed from server grant introspection and `GET /me/membership` | Return selected organisation, display identity, grant expiry and allowed capabilities. No other memberships, cookies or bearer values. |
| `authrail_get_workspace_overview` | R | `GET /organisation/overview` | Active member; no single overview permission. Server must additionally filter its domain-aware projection by MCP consent, not just the user's full role. Preserve independently unavailable sections. |
| `authrail_get_sdk_integrations` | R | `GET /organisation/mcp/integrations/sdks` | Baseline workspace.read consent and exact named delegation with live user/workspace checks and attributed audit. Returns the server-owned Node/Go/Python released packages, versions, install argv and integration responsibilities; no secrets or installation authority. |

`GET /me/memberships` supplies the authenticated consent page's organisation
choices. It is not a tool that silently expands a single-organisation grant.
The connection baseline includes these three reads, plus sign-in/consent and
revocation. SDK discovery supports onboarding; it does not install an SDK or
protect another MCP tool automatically.

## Frameworks, scope and obligation coverage

Existing authority is active organisation membership unless a capability is
listed. Scope/coverage reads require `framework.select` even though they read.

| Proposed tools | Class | Existing API binding | Required authority / version and effect |
| --- | --- | --- | --- |
| `authrail_list_frameworks`, `authrail_get_framework` | R | `GET /frameworks`; `GET /frameworks/:frameworkId` | Published catalog; typed catalog filters and cursor/limit. |
| `authrail_list_framework_sections`, `authrail_list_framework_obligations` | R | `GET /frameworks/:frameworkId/sections`; `GET /frameworks/:frameworkId/obligations` | Exact catalog Framework; do not infer accepted applicability. |
| `authrail_list_organisation_frameworks`, `authrail_get_organisation_framework` | R | `GET /organisation-frameworks`; `GET /organisation-frameworks/:organisationFrameworkId` | Tenant-owned activation with its exact Framework Version. |
| `authrail_get_framework_scope` | R | `GET /organisation-frameworks/:organisationFrameworkId/scope` | `framework.select`; paged visible subjects and exact snapshot/revisions. |
| `authrail_save_scope_proposal` | P, restricted | `PUT /organisation-frameworks/:organisationFrameworkId/scope-draft` | `framework.select`; expectedOrganisationFrameworkRevision, expectedDraftRevision, baseScopeSnapshotId. Preparation must not set reviewed decisions or acceptResolvedProposals. Those effects use H; missing prepare-only enforcement must be closed first. |
| `authrail_request_scope_decisions` | H | `PUT /organisation-frameworks/:organisationFrameworkId/scope-draft` | Same authority/revisions; exact answers/decisions, reviewed flags and acceptance consequences shown to the human. |
| `authrail_request_framework_activation`, `authrail_request_scope_activation` | H | `POST /organisation-frameworks/activate`; `POST /organisation-frameworks/:organisationFrameworkId/scope/activate` | `framework.select`; activation source/id/exact version and UUID idempotencyKey; scope activation exact owner/draft revisions plus key. |
| `authrail_request_framework_suspension` | H | `POST /organisation-frameworks/suspend` | `framework.select`; source/frameworkId/expectedRevision, no existing idempotency key. |
| `authrail_list_obligation_coverage`, `authrail_list_control_mappings` | R | `GET /organisation/controls/coverage-decisions`; `GET /organisation/controls/mappings` | `framework.select`; exact Framework/applicability filters, visible Controls, cursor/limit<=100. |
| `authrail_get_mapping_replacement_target`, `authrail_list_coverage_reviews` | R | `GET /organisation/controls/:controlId/mappings/:mappingId/replacement-target`; `GET /organisation/controls/coverage-decisions/:decisionId/reviews` | `framework.select` plus mapped Control visibility. |
| `authrail_request_mapping_acceptance`, `authrail_request_mapping_withdrawal` | H | `POST /organisation/controls/:controlId/mappings`; `POST /organisation/controls/:controlId/mappings/:mappingId/withdraw` | `framework.select` + `control.review`; UUID key; acceptance exact applicability decision and Control/optional Implementation Version, contribution/rationale; withdrawal reason. POST creates an accepted Mapping, not a draft. |
| `authrail_request_coverage_review` | H | `POST /organisation/controls/coverage-decisions/:decisionId/review` | Same capabilities; UUID key, exact mappingIds, complete/incomplete and rationale. |

There is no standalone generic Framework-applicability CRUD endpoint. Obligation
applicability is owned by scope drafts and activated Scope Snapshots. Policy
applicability below is a separate exact-version workflow.

## Policies, Controls, Tests, Evidence and remediation

| Proposed tools | Class | Existing API binding | Required authority / version and effect |
| --- | --- | --- | --- |
| `authrail_list_control_policy_bases` | R | `GET /organisation/mcp/controls/:controlId/versions/:controlVersionId/policy-bases` | governance.read plus policy.read and exact Control/Policy audiences. Distinct referenced versions, bounded cursor pages and restricted-reference flag; section bodies excluded. |
| `authrail_list_policies`, `authrail_get_policy`, `authrail_list_policy_versions`, `authrail_get_policy_version` | R | `GET /organisation/policies`; `GET /organisation/policies/:policyId`; `GET /organisation/policies/:policyId/versions`; `GET /organisation/policies/:policyId/versions/:versionId` | `policy.read` + object visibility; bounded content and domain paging. |
| `authrail_create_policy_draft`, `authrail_start_policy_draft`, `authrail_save_policy_draft` | P | `POST /organisation/policies`; `POST /organisation/policies/:policyId/drafts`; `PATCH /organisation/policies/:policyId/drafts/:versionId` | `policy.manage`; typed catalog/content/applicability fields; save revision. No general create/start/save idempotency key exists: ambiguous writes need server replay/result support. |
| `authrail_get_policy_applicability` | R | `GET /organisation/policies/:policyId/versions/:versionId/applicability-statements` | `policy.read`; exact versionRevision and statement context. |
| `authrail_request_policy_applicability_review`, `authrail_request_policy_applicability_confirmation` | H | `PATCH /organisation/policies/:policyId/versions/:versionId/applicability-statements/:statementId/review`; `PUT /organisation/policies/:policyId/versions/:versionId/applicability-review` | `policy.review`; versionRevision; exact statement decision or full reviewed applicability factset. |
| `authrail_request_policy_publication`, `authrail_request_policy_discard` | H | `POST /organisation/policies/:policyId/versions/:versionId/publish`; `POST /organisation/policies/:policyId/drafts/:versionId/discard` | `policy.publish` / `policy.manage`; revision, exact review prerequisites; not generic retry-safe. |
| `authrail_request_policy_archive`, `authrail_request_policy_restore` | H | `POST /organisation/policies/:policyId/archive`; `POST /organisation/policies/:policyId/restore` | `policy.lifecycle.manage`; revision and lifecycle context. |
| `authrail_get_control` | R | `GET /organisation/mcp/controls/:controlId/versions/:controlVersionId` | Exact version metadata under the existing membership/version-audience policy. Private bodies require a separately reviewed content contract. |
| `authrail_list_controls`, `authrail_list_approved_control_versions` | R, contract gap | `GET /organisation/controls`; `GET /organisation/controls/approved-versions` | Preserve membership/version-audience visibility and bounded projections. These general indexes are separate from exact mapped-version metadata. |
| `authrail_create_control_draft`, `authrail_start_control_draft`, `authrail_save_control_draft` | P | `POST /organisation/controls`; `POST /organisation/controls/:controlId/drafts`; `PATCH /organisation/controls/:controlId/draft` | `control.manage`; create UUID key and typed draft/policy bases/maintainers; saves owner revision. Start-next-draft has no key. |
| `authrail_request_control_maintainers`, `authrail_request_control_retirement` | H | `PATCH /organisation/controls/:controlId/maintainers`; `POST /organisation/controls/:controlId/retire` | `control.manage` / `control.lifecycle.manage`; exact maintainers + revision, or versionId/revision/reason. |
| `authrail_list_control_implementations`, `authrail_create_implementation_draft`, `authrail_save_implementation_draft` | R/P | `GET/POST /organisation/controls/:controlId/implementations`; `PATCH /organisation/controls/:controlId/implementations/:versionId` | Read Control/boundary visibility; write control.manage. Create UUID key, exact controlVersionId and genuine difference/boundary; save revision. |
| `authrail_request_implementation_submission`, `authrail_request_implementation_decision` | H | `POST /organisation/controls/:controlId/implementations/:versionId/submit`; `POST /organisation/controls/:controlId/implementations/:versionId/review` | `control.manage` / `control.review`; submit revision+assigned reviewer; decision reviewId+decision/note and exact independent eligibility. No decision revision/key field. |
| `authrail_get_version_review`, `authrail_list_eligible_reviewers` | R, contract gap | `GET /organisation/reviews/:subject/:versionId`; `GET /organisation/reviews/eligible-reviewers` | Path subjects are policy-versions/control-versions/custom-framework-versions; eligible-reviewer query uses subject=policy_version|control_version|custom_framework_version plus versionId. Existing latest-review read needs subject permission/audience checks before MCP use. Reviewer candidates retain subject manage/visibility rules. |
| `authrail_request_version_review`, `authrail_request_version_review_decision` | H | `POST /organisation/reviews/:subject/:versionId/submit`; `PATCH /organisation/reviews/:requestId/decision` | Subject manage/review capabilities and assignment; submit revision/reviewerStaffIds; decide decision/note. Empty reviewer submission can immediately sole-operator approve, so is never an ordinary P tool. |
| `authrail_list_tests`, `authrail_save_test_definition`, `authrail_request_test_definition_removal` | R/P/H | `GET/POST /organisation/controls/:controlId/versions/:controlVersionId/tests`; `POST /organisation/controls/:controlId/versions/:controlVersionId/tests/:definitionId/remove` | Read exact owner membership/audience; draft changes control.manage. The MCP Test summary preserves that owner policy and Runtime visibility. Definition writes have UUID key, owner revision and typed procedure/cadence/freshness. |
| `authrail_list_test_runs`, `authrail_list_runtime_test_candidates`, `authrail_request_test_run_recording` | R/R/H | `GET/POST /organisation/controls/:controlId/versions/:controlVersionId/tests/:definitionId/runs`; `GET /organisation/controls/:controlId/versions/:controlVersionId/tests/:definitionId/runtime-candidates` | Read owner + Runtime visibility; record control.manage with approved owner, UUID key, completed period/time, outcome and exact source references. Recording can affect Health; no agent-manufactured pass. |
| `authrail_list_evidence_requirements`, `authrail_save_evidence_requirement`, `authrail_request_evidence_requirement_removal` | R/P/H | `GET /organisation/evidence/requirements`; `GET/POST /organisation/controls/:controlId/versions/:controlVersionId/evidence-requirements`; `POST /organisation/controls/:controlId/versions/:controlVersionId/evidence-requirements/:requirementId/remove` | evidence.read / control.manage; exact owner revision + UUID key and typed expectation/freshness. |
| `authrail_list_evidence_record_decisions` | R | `GET /organisation/mcp/controls/:controlId/versions/:controlVersionId/evidence-requirements/:requirementId/records/:recordId/decisions` | governance.read plus evidence.read/exact verification owner. Retained decision/reviewer/time metadata with Record revision/status; private review reasons excluded. |
| `authrail_list_evidence_records`, `authrail_request_evidence_record_submission`, `authrail_request_evidence_record_review` | R/H/H | `GET/POST /organisation/controls/:controlId/versions/:controlVersionId/evidence-requirements/:requirementId/records`; `POST /organisation/controls/:controlId/versions/:controlVersionId/evidence-requirements/:requirementId/records/:recordId/review` | evidence.read; submission evidence.manage+read; review evidence.review+read. UUID key, exact source/period/replacement; review record revision. Submission may autoaccept nonattachment proof, so requires H. |
| `authrail_list_evidence_packages`, `authrail_get_evidence_package`, `authrail_create_evidence_package`, `authrail_save_evidence_package` | R/R/P/P | `GET/POST /organisation/evidence`; `GET /organisation/evidence/:evidenceId`; `PATCH /organisation/evidence/:evidenceId/draft` | evidence.read/manage + linked Control visibility; create UUID key/controlVersionIds; save lockVersion, not package revision. Package alone is not accepted Control-owned Evidence. |
| `authrail_request_evidence_analysis`, `authrail_get_evidence_analysis`, `authrail_request_evidence_claim_review` | P/R/H | `POST/GET /organisation/evidence/:evidenceId/analysis`; `POST /organisation/evidence/:evidenceId/claim-decisions` | evidence.manage/read/review; analysis UUID key+revision; claim decisionId, exact analysis/revision/proposal or cited claim. Model disclosure must be consented; AI claim is not accepted proof. |
| `authrail_get_framework_readiness`, `authrail_get_framework_activity` | R | `GET /organisation/readiness/frameworks/:organisationFrameworkId`; `GET /organisation/readiness/frameworks/:organisationFrameworkId/activity` | readiness.read + underlying visibility; bounded snapshot/activity. There is no edit-readiness tool. |
| `authrail_list_audit_reports`, `authrail_get_audit_report`, `authrail_create_audit_report`, `authrail_request_audit_export` | R/R/P/H | `GET/POST /organisation/readiness/frameworks/:organisationFrameworkId/reports`; `GET /organisation/readiness/frameworks/:organisationFrameworkId/reports/:reportId`; `POST /organisation/readiness/frameworks/:organisationFrameworkId/reports/:reportId/exports` | readiness.read, plus organisation.data.export for creation/export; UUID keys. Snapshot/export generation is a mutation; download is S. |
| `authrail_list_policy_findings`, `authrail_get_policy_finding` | R | `GET /organisation/findings/obligations`; `GET /organisation/findings/obligations/:findingId` | policy.read; conflict variants bind /organisation/findings/conflicts and /:findingId separately. Typed status/stale/cursor/limit filters. |
| `authrail_request_policy_finding_review` | H | `PATCH /organisation/findings/obligations/:findingId/review` | finding.review; revision, confirmed/dismissed and reason; conflict review uses its separately bound conflict route. |
| `authrail_list_operational_findings`, `authrail_get_operational_finding`, `authrail_prepare_manual_finding` | R/R/P | `GET /organisation/operational-findings`; `GET /organisation/operational-findings/:findingId`; `POST /organisation/operational-findings/manual` | work.read / finding.remediate; create UUID key, exact mapping/owner and typed finding fields. |
| `authrail_update_finding_remediation`, `authrail_request_manual_finding_resolution` | P/H | `PATCH /organisation/operational-findings/:findingId/remediation`; `POST /organisation/operational-findings/:findingId/resolve` | finding.remediate + Control access; revision/reason; only manual source may be manually resolved. |
| `authrail_list_tasks`, `authrail_get_task`, `authrail_create_remediation_task`, `authrail_update_task` | R/R/P/P | `GET /organisation/work/tasks`; `GET/PATCH /organisation/work/tasks/:taskId`; `POST /organisation/operational-findings/:findingId/tasks` | work.read / task.manage; create UUID key; update revision. Narrow P update excludes terminal states; completed/cancelled decisions use H with exact completion proof where required. |
| `authrail_request_task_completion` | H | `PATCH /organisation/work/tasks/:taskId` | task.manage; exact revision/state/reason/completionProof. Does not resolve the source Finding. |
| `authrail_list_readiness_exceptions`, `authrail_get_readiness_exception`, `authrail_prepare_exception_request` | R/R/P | `GET /organisation/readiness/exceptions`; `GET /organisation/readiness/exceptions/:exceptionId`; `POST /organisation/readiness/mappings/:mappingId/exceptions` | readiness.read / exception.request; create UUID key, exact blocker/time window/compensating Controls. |
| `authrail_request_exception_decision`, `authrail_request_exception_withdrawal` | H | `POST /organisation/readiness/exceptions/:exceptionId/decision`; `POST /organisation/readiness/exceptions/:exceptionId/withdraw` | exception.approve / exception.request; revision/decision/reason; requester cannot selfapprove an Exception. Does not rewrite proof or source Finding. |

Definitions and exact payloads come from the owning DTOs, not agent-generated
method/path JSON. Bodies with no existing request key must not be advertised as
safe automatic retries. New confirmation metadata belongs to the proposed
server coordination contract, not unknown fields added to existing DTOs.

## Tenant authoring, imports and library content

| Proposed tools | Class | Existing API binding | Required authority / version and effect |
| --- | --- | --- | --- |
| `authrail_list_custom_frameworks`, `authrail_get_custom_framework`, `authrail_get_custom_framework_version` | R | `GET /organisation/custom-frameworks`; `GET /organisation/custom-frameworks/:frameworkId`; `GET /organisation/custom-frameworks/:frameworkId/versions/:versionId` | Tenant-owned catalog/version projection; bounded content. |
| `authrail_create_custom_framework`, `authrail_save_custom_framework_version` | P | `POST /organisation/custom-frameworks`; `PATCH /organisation/custom-frameworks/:frameworkId/versions/:versionId` | framework.custom.manage; typed metadata/content/scopingDefinition; version save revision, creation has no retry key. |
| `authrail_create_custom_framework_version` | P | `POST /organisation/custom-frameworks/:frameworkId/versions` | framework.custom.manage; version/content and typed optional source/scoping metadata; no key. |
| `authrail_request_custom_framework_publication`, `authrail_request_custom_framework_archive`, `authrail_request_custom_framework_restore` | H | `POST /organisation/custom-frameworks/:frameworkId/versions/:versionId/publish`; `POST /organisation/custom-frameworks/:frameworkId/archive`; `POST /organisation/custom-frameworks/:frameworkId/restore` | framework.custom.manage, not a fictional framework.publish capability. Existing routes have no body/revision/key; exact confirmation must be checked within their server transaction. |
| `authrail_get_custom_obligations`, `authrail_start_custom_obligation_draft`, `authrail_save_custom_obligation_draft` | R/P/P | `GET /organisation/custom-frameworks/:frameworkId/versions/:versionId/obligations`; `POST/PUT /organisation/custom-frameworks/:frameworkId/versions/:versionId/obligations/draft` | Owned version; writes framework.custom.manage; save revision + bounded typed Obligations/citations. |
| `authrail_request_custom_obligation_approval`, `authrail_request_custom_obligation_discard` | H | `POST /organisation/custom-frameworks/:frameworkId/versions/:versionId/obligations/draft/approve`; `DELETE /organisation/custom-frameworks/:frameworkId/versions/:versionId/obligations/draft` | framework.custom.manage; revision. This is distinct from version review and Framework publication. |
| `authrail_request_custom_scoping_analysis`, `authrail_get_custom_scoping_analysis`, `authrail_request_custom_scoping_decision` | P/R/H | `POST /organisation/custom-frameworks/:frameworkId/versions/:versionId/scoping-reviews`; `GET /organisation/custom-frameworks/:frameworkId/versions/:versionId/scoping-reviews/latest`; `POST /organisation/custom-frameworks/:frameworkId/versions/:versionId/scoping-reviews/:reviewId/proposals/:proposalId/decision` | Writes framework.custom.manage; analysis revision+requestKey; decision revision/applied-edited-rejected and exact questions. Model processing is disclosed and separate from acceptance. |
| `authrail_create_policy_authoring_session`, `authrail_get_policy_authoring_session`, `authrail_continue_policy_authoring`, `authrail_save_policy_authoring_draft` | P/R/P/P | `POST /organisation/policy-authoring-sessions`; `GET /organisation/policy-authoring-sessions/:sessionId`; `POST /organisation/policy-authoring-sessions/:sessionId/turns`; `POST /organisation/policy-authoring-sessions/:sessionId/use-as-draft` | policy.manage for writes, policy.read for reads. Creation queues a model turn; explicit disclosure consent required. Turn has string idempotencyKey; promotion revision, creates only a draft. |
| `authrail_list_policy_library_entries`, `authrail_get_policy_library_entry`, `authrail_add_policy_library_draft` | R/R/P | `GET /organisation/policy-library/entries`; `GET /organisation/policy-library/entries/:entryId`; `POST /organisation/policy-library/entries/:entryId/add-to-policies` | Catalog active membership; add policy.manage with typed title/category/applicability; no key. Creates a private draft, not publication. |
| `authrail_request_policy_library_submission`, `authrail_request_policy_library_unpublish` | H | `POST /organisation/policy-library/policies/:policyId/versions/:versionId/submissions`; `POST /organisation/policy-library/entries/:entryId/unpublish` | policy.publish; submission revision and exact public-sharing fields. Make external disclosure explicit; internal library moderation is excluded. |
| `authrail_list_policy_categories`, `authrail_create_policy_category` | R/P | `GET/POST /organisation/policy-categories` | Category reads require active tenant membership; create requires policy.manage with a typed name. Full list is unpaged; use bounded options or a new paged contract. No create replay key; resolve retry/rename semantics before shipping. |

Supporting list/history/edit/retry variants must each have a separately reviewed
binding and typed schema before exposure. Their existence does not license a
wildcard tool. Slice 1 reviewed the corresponding controller families; missing
bounded reads, owner candidates and retries are included in the gap register.
Managed Framework authoring and public library moderation remain internal Admin
work, not delegated tenant tools.

## Runtime management and assigned approvals

| Proposed tools | Class | Existing API binding | Required authority / version and effect |
| --- | --- | --- | --- |
| `authrail_list_ai_systems`, `authrail_get_ai_system`, `authrail_list_ai_system_revisions` | R | `GET /organisation/ai-inventory`; `GET /organisation/ai-inventory/:itemId`; `GET /organisation/ai-inventory/:itemId/revisions` | ai_inventory.read plus exact revision/audience visibility; limit<=100, opaque cursor. Configured provider/model is not execution identity. |
| `authrail_request_ai_system_creation`, `authrail_request_ai_system_update` | H | `POST /organisation/ai-inventory`; `PATCH /organisation/ai-inventory/:itemId` | ai_inventory.manage; create UUID idempotencyKey, update revision with complete fields. Manual create already defaults to confirmed identity; update immediately persists a new revision and retains confirmation status. Neither is a detached candidate draft. Creating an unconfirmed P candidate needs a separate server contract. |
| `authrail_request_ai_system_confirmation`, `authrail_request_ai_system_revision_adoption`, `authrail_request_ai_system_lifecycle_change` | H | `POST /organisation/ai-inventory/:itemId/confirm`; `POST /organisation/ai-inventory/:itemId/adopt-revision`; `POST /organisation/ai-inventory/:itemId/lifecycle` | ai_inventory.manage; exact revision, fields/lifecycle and affected deployments. |
| `authrail_list_operations`, `authrail_get_operation` | R | `GET /organisation/rail-operations`; `GET /organisation/rail-operations/:operationId` | rail.read; afterId UUID/limit<=50, not the Runtime cursor format. |
| `authrail_create_operation_draft`, `authrail_save_operation_draft`, `authrail_request_operation_review` | P/P/H | `POST /organisation/rail-operations`; `PATCH /organisation/rail-operations/:operationId/drafts/:versionId`; `POST /organisation/rail-operations/:operationId/drafts/:versionId/review` | rail.manage / rail.publish; create Idempotency-Key header UUID; update typed contract+revision; review revision. Imported schemas are untrusted drafts, not execution authority. |
| `authrail_list_context_sources`, `authrail_get_context_source`, `authrail_get_context_source_version` | R | `GET /organisation/rail-context-sources`; `GET /organisation/rail-context-sources/:sourceId`; `GET /organisation/rail-context-sources/:sourceId/versions/:versionId` | rail.read; safe source/config metadata by default. |
| `authrail_create_context_source_draft`, `authrail_save_context_source_draft`, `authrail_request_context_source_review` | P/P/H | `POST /organisation/rail-context-sources`; `PATCH /organisation/rail-context-sources/:sourceId/drafts/:versionId`; `POST /organisation/rail-context-sources/:sourceId/drafts/:versionId/review` | rail.manage / rail.publish; create header UUID; update/review revision. Source credentials never enter content. |
| `authrail_list_rails`, `authrail_get_rail`, `authrail_get_rail_version`, `authrail_list_rail_versions` | R | `GET /organisation/rails`; `GET /organisation/rails/:railId`; `GET /organisation/rails/:railId/versions/:versionId`; `GET /organisation/rails/:railId/versions` | rail.read; Policy Rails also policy.read. Version history beforeVersionNumber/limit<=50. |
| `authrail_list_available_facts` | R | `GET /organisation/rails/available-data-facts` | rail.read + policy.read; exact operationVersionId, afterId and limit. Definitions, not customer fact values. |
| `authrail_create_guard_rail_draft`, `authrail_save_guard_rail_draft`, `authrail_test_guard_rail_draft`, `authrail_request_guard_rail_publication` | P/P/P/H | `POST /organisation/rails/guard-drafts`; `PATCH /organisation/rails/:railId/guard-drafts/:versionId`; `POST /organisation/rails/:railId/guard-drafts/:versionId/test`; `POST /organisation/rails/:railId/guard-drafts/:versionId/publish` | rail.manage / rail.publish; create header UUID+exact Operation; save/test/publish revision. Test is persisted validation, not a real business operation. |
| `authrail_create_data_rail_draft`, `authrail_save_data_rail_draft`, `authrail_test_data_rail_draft`, `authrail_request_data_rail_publication` | P/P/P/H | `POST /organisation/rails/data-drafts`; `PATCH /organisation/rails/:railId/data-drafts/:versionId`; `POST /organisation/rails/:railId/data-drafts/:versionId/test`; `POST /organisation/rails/:railId/data-drafts/:versionId/publish` | rail.manage / rail.publish; exact operationVersionId/sourceContextVersionId; same header/revision pattern. |
| `authrail_create_policy_rail_draft`, `authrail_save_policy_rail_draft`, `authrail_test_policy_rail_draft`, `authrail_request_policy_rail_publication` | P/P/P/H | `POST /organisation/rails/policy-drafts`; `PATCH /organisation/rails/:railId/drafts/:versionId`; `POST /organisation/rails/:railId/drafts/:versionId/test`; `POST /organisation/rails/:railId/drafts/:versionId/publish` | rail.manage / rail.publish plus policy.read; exact Operation/Policy versions; same header/revision pattern. |
| `authrail_list_deployments`, `authrail_create_deployment` | R/P | `GET/POST /organisation/ai-inventory/:itemId/deployments` | ai_inventory.read/manage and revision visibility; create name+UUID key. Creates Observe installation without a plaintext deployment credential. |
| `authrail_get_deployment`, `authrail_get_deployment_setup` | R | `GET /organisation/ai-inventory/deployments/:deploymentId`; `GET /organisation/ai-inventory/deployments/:deploymentId/setup` | ai_inventory.read + visibility. Public key/fingerprints and credential metadata only; setup does not return an API-origin field or plaintext token. |
| `authrail_get_deployment_rail_selection` | R | `GET /organisation/ai-inventory/deployments/:deploymentId/activation` | ai_inventory.manage + rail.publish; selected Policy Rails also policy.read. Read does not imply relaxed permission. |
| `authrail_request_deployment_activation`, `authrail_request_deployment_renewal`, `authrail_request_deployment_rollback`, `authrail_request_deployment_revocation` | H | `POST /organisation/ai-inventory/deployments/:deploymentId/activate`; `POST /organisation/ai-inventory/deployments/:deploymentId/renew`; `POST /organisation/ai-inventory/deployments/:deploymentId/rollback`; `POST /organisation/ai-inventory/deployments/:deploymentId/revoke` | Same authority; requestKey+expectedSequence and exact Rail versions or retained source bundle/reason. Show mode and impact. Model cannot assert confirmedProtectedBoundary on a person's behalf. Revocation endpoint is Observe-only. |
| `authrail_list_runtime_actions`, `authrail_list_deployment_actions`, `authrail_get_runtime_action` | R | `GET /organisation/runtime/actions`; `GET /organisation/ai-inventory/deployments/:deploymentId/actions`; `GET /organisation/ai-inventory/deployments/:deploymentId/actions/:actionId` | ai_inventory.read + current/pinned/historical revision visibility; limit<=100/cursor UUID. Default bounded certificates/receipts metadata, not raw signed assertions. |
| `authrail_list_runtime_logs` | R | `GET /organisation/ai-inventory/deployments/:deploymentId/logs` | ai_inventory.read; safe diagnostic fields, typed cursor/actionKey/phase. Logs are not durable effect proof. |
| `authrail_get_approval_policy`, `authrail_request_approval_policy_change` | R/H | `GET/PUT /organisation/rails/operation-versions/:operationVersionId/approval-policy` | BOTH reads/writes require rail.publish + ai_inventory.manage. PUT requestKey, expectedRevision, exact staff IDs/quorum; empty list+zero disables review. |
| `authrail_list_my_runtime_approvals`, `authrail_request_runtime_approval_decision` | R/H | `GET /organisation/runtime/approval-requests`; `POST /organisation/runtime/approval-requests/:requestId/decisions` | Active named reviewer, deliberately no ai_inventory.read or rail.publish prerequisite. List bounded30/cursor; exact review context disclosure requires consent. Vote approve/reject/cancel has no revision/key DTO; add exact server confirmation. A named requester-reviewer may vote; no independent-review claim. |
| `authrail_request_executor_key_change` | H/S | `POST /organisation/ai-inventory/deployments/:deploymentId/executor-key`; `POST /organisation/ai-inventory/deployments/:deploymentId/executor-key/rotate`; `POST /organisation/ai-inventory/deployments/:deploymentId/executor-key/close` | ai_inventory.read/manage; bind exact PUBLIC PEM/fingerprint, prior key and retirement context. Private key stays outside MCP; queuesChecked is an actual operator assertion, not an agent default. |

No hosted tool evaluates a customer call, fetches its private signed bundle,
claims/recovers an execution permit, submits sidecar signals/observations or
creates execution receipts. The customer SDK/sidecar owns `/runtime/deployment/*`
protocol calls. Approving a request does not resume the customer's callback.
Credential/discovery issuance and Runtime-log export/deletion require separately
reviewed secure/human tools; do not smuggle them through the reads above.

## Connections, people, activity and exports

| Proposed tools | Class | Existing API binding | Required authority / version and effect |
| --- | --- | --- | --- |
| `authrail_list_connections`, `authrail_get_connection_status`, `authrail_list_connection_resources`, `authrail_get_resource_review` | R | `GET /organisation/connections`; `GET /organisation/connections/:connectionId`; `GET /organisation/connections/:connectionId/resources`; `GET /organisation/connections/:connectionId/resources/:resourceId/review` | connection.read / resource.read and actual owning-domain visibility. Bounded provider/account/status metadata; no credentials or raw provider bodies. |
| `authrail_request_connection_sync`, `authrail_request_connection_sync_cancel` | P/H | `POST /organisation/connections/:connectionId/sync`; `POST /organisation/connections/:connectionId/sync-runs/:syncRunId/cancel` | connection.manage; UUID key and cancel reason. Preserve queued/running/failed/retry semantics. |
| `authrail_request_resource_monitoring_decision`, `authrail_request_connection_disconnect` | H | `POST /organisation/connections/:connectionId/resources/:resourceId/monitoring-decisions`; `POST /organisation/connections/:connectionId/disconnect` | resource.review / connection.manage; expectedRevision+UUID key and exact decision/reason. Disconnect is not proof remote consent was revoked. |
| `authrail_list_departments`, `authrail_list_teams`, `authrail_list_roles`, `authrail_list_staff`, `authrail_list_invitations` | R | `GET /organisation/departments`; `GET /organisation/teams`; `GET /organisation/roles`; `GET /organisation/staff`; `GET /organisation/invitations/list` | Department/team reads require active membership; roles organisation.role.manage or organisation.staff.invite; staff organisation.staff.manage; invitations organisation.staff.invite. Minimal private-person metadata. |
| `authrail_request_workspace_profile_change`, `authrail_request_staff_invitation`, `authrail_request_staff_change`, `authrail_request_role_change` | H | `PATCH /organisation/profile`; `POST /organisation/invitations`; `PATCH /organisation/staff/:staffId`; `PATCH /organisation/roles/:roleId` | Exact organisation.settings.manage / organisation.staff.invite / organisation.staff.manage / organisation.role.manage capability and target constraints. No general revision/idempotency fields: new server CAS/replay contracts precede executable tools. Invitations send real email. |
| `authrail_get_notification_preferences`, `authrail_list_notifications`, `authrail_mark_notification_read` | R/R/P | `GET /organisation/me/event-preferences`; `GET /organisation/me/notifications`; `PATCH /organisation/me/notifications/:notificationId/read` | Own member only; bounded inbox, no arbitrary URL fetching. Mutating preferences/read-all need separately reviewed field restrictions/retry semantics. |
| `authrail_list_activity_records`, `authrail_get_activity_record`, `authrail_get_activity_revision` | R | `GET /organisation/activities`; `GET /organisation/activities/:activityId`; `GET /organisation/activities/:activityId/revisions/:revisionId` | activity.read, stored audience and linked Runtime identity visibility. Reauthorise linked Policy/Evidence metadata and content; tenant existence alone is insufficient. |
| `authrail_prepare_activity_record`, `authrail_prepare_activity_correction` | P | `POST /organisation/activities` | activity.record; UUID key, typed reported action/outcome/source references; correction exact supersedesActivityId/supersedesRevisionId. Cannot correct Runtime-source records. Existing replay does not compare payload hash: close that gap before promising safe changed-payload rejection. |
| `authrail_request_activity_review` | H | `POST /organisation/activities/:activityId/reviews` | activity.review; reviewed/needs_follow_up+note. Existing route chooses current revision without expected ID/key, so add exact-revision/replay binding before execution. |
| `authrail_get_activity_attestation`, `authrail_verify_activity_attestation` | R | `GET /organisation/activities/:activityId/attestation`; `GET /organisation/activities/:activityId/attestation/verify` | activity.read + parent visibility. Pending/failed are not valid proofs. Signature verification establishes record integrity, not effect truth or compliance. |
| `authrail_request_organisation_export`, `authrail_list_organisation_exports` | H/R | `POST/GET /organisation/exports` | organisation.data.export; request idempotencyKey16..160; results bound to requesting user/org. Download additionally needs recent actual authentication and a secure handoff. |

Account passwords, MFA/backup codes, recovery, invitation acceptance, SSO secrets,
provider OAuth completion, signed downloads, credential rotation and webhook
secrets are S interactions. No model-visible tool accepts or returns those
values. Proposed handoff/status tools must be named by task and use the same
domain permissions, target binding and confirmation where applicable; they are
not permission to perform arbitrary browser actions. Scope-affecting structure,
role, membership or identity changes remain H and need complete server contracts.


## Blocking contract gaps and the slice that owns them

| Gap | Required resolution before the corresponding tool ships |
| --- | --- |
| Extend delegated authority to the remaining surface | Preserve the implemented OAuth/PKCE, exact resource, scope, delegation, live revocation and attributed read foundation. Extend its domain/subject checks to each remaining tool; a generic owner/admin credential cannot replace them. |
| Hosted MCP read fairness | Before hosted load acceptance, partition verified service/delegated read budgets fairly across users/tenants while retaining ingress abuse protection. The shared source-IP bucket was observed during repeated real integration runs; isolate verification namespaces rather than weakening production limits. Discuss the authenticated rate-limit contract before changing it. |
| Incomplete subject read rules | Control/Test MCP summaries preserve existing membership/exact-owner audiences. Complete generic review subject audience checks, scoped mapping reference visibility and linked Activity Policy/Evidence access. Do not compensate with an owner token or drop object checks in Go. |
| Unbounded detail/candidate/history APIs | Slices 3–6: add bounded content/history reads where required; specific owner/reviewer candidate reads without granting broad staff administration; saved version-review history is not available through the latest-only route. |
| Not every write is retry-safe | Slices 4–6: inspect per-tool repository replay and add exact CAS/key/result binding where missing. Activity and manual Inventory UUID replay must reject changed payload; no automatic policy/custom-Framework recreation after timeout. |
| No exact MCP human-confirmation contract | Slice 5: agree technical intent persistence, canonical digest, nonce/expiry, eligible person and atomic consumption with domain effect. Cover sole-operator review, scope decisions, accepted Mappings, recorded Tests and autoaccepted Evidence. |
| Decision/read contracts omit required context | Slices 5–6: exact Activity review revision; generic/implementation decision revision; reviewer-specific Runtime request detail and summary digest; guarded custom publication/key changes. Do not invent extra fields the existing DTO rejects. |
| Credentials/files are embedded in normal API responses | Slices 4–6: secure task handoff and status, direct signed storage POST, live attachment readiness and recent-auth export delivery; no secret JSON proxying. |
| Complete write/confirmation attribution | Preserve implemented user/client/grant/tool read attribution and bind future prepared commands and exact human confirmations to their real actors. Keep tokens/private payloads out of audit/log fields. |

This map covers implemented read families and remaining proposed work; the
continuation and owning API/code identify the verified subset. New scope,
command/confirmation and persistence contracts still require review before their
implementation. A reviewed map is not production acceptance or authority to
mutate AWS, DNS, secrets or customer data.


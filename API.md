# AuthRail MCP API

## HTTP endpoints

Send authenticated MCP JSON-RPC requests over Streamable HTTP.
`POST /`

Receive authenticated Streamable HTTP requests using the same exact endpoint.
`GET /`

Read OAuth protected-resource metadata.
`GET /.well-known/oauth-protected-resource`

Check process liveness.
`GET /health/live`

Check API availability from the MCP service.
`GET /health/ready`

## Private gateway adapter

Read a public HTTPS MCP server's tools using separately supplied account access.
This private operation performs discovery only and grants no tool authority.
`POST /discover`

Send one API-issued ticket through its reviewed MCP, A2A or native provider HTTP route. This private
endpoint returns `result_pending_check`; it does not release a result to an
assistant or establish that the business action succeeded.
`POST /dispatch`

Check adapter liveness.
`GET /health/live`

Check that the adapter has recently synchronized reviewed routing configuration.
`GET /health/ready`

The separate adapter listener requires the internal service credential for
dispatch. Its private gRPC listener implements the pinned gateway's
`agentgateway.dev.ext_mcp.ExtMcp/CheckRequest` and
`agentgateway.dev.ext_mcp.ExtMcp/CheckResponse` methods. These are protocol
callbacks, not public HTTP routes or MCP tools.

## MCP tools

All tools require `workspace.read`. The headings identify additional consent.
The API also checks current organisation permissions and access to each record.
Tool input and result schemas are supplied by MCP discovery and the typed
registrations in [internal/mcpserver](internal/mcpserver).

### Report a problem — `support.reports`

Submit deliberately reviewed text and an optional selected Action reference.
A stable submission UUID deduplicates unchanged retries after uncertain responses.
The authenticated reporter and assistant client are recorded by AuthRail.
`authrail_submit_problem_report`

List only that reporter's saved reports in the connected workspace.
`authrail_list_my_problem_reports`

Read your report status and public support reply. Internal triage notes and the
Admin queue are unavailable to these tools. No diagnostics/files are attached
automatically; selected Action references require current Runtime read permission.
`authrail_get_my_problem_report`

### Workspace — `workspace.read`

Inspect the assistant connection and its consented access.
`authrail_get_connection`

Read the connected organisation and account.
`authrail_get_workspace_summary`

Read released SDK installation guidance.
`authrail_get_sdk_integrations`

### Workspace Guide — `workspace.read`

Browse published workspace guidance, search it, and read section chunks bound to the current publication. Internal Admin guidance is excluded.
`authrail_get_guide_navigation`
`authrail_search_guide`
`authrail_get_guide_article`

### Compliance configuration — `compliance.configuration.read` / `compliance.configuration.manage`

Read the published Framework catalogue, custom Framework versions, exact scope,
permitted Control drafts and scoped Implementations, coverage decisions/reviews
and custom import status. Content continuation retains the selected version. Scope pagination uses `cursor`; lossless content chunks use the separate `contentCursor`. Continue all content chunks for one scope page before requesting its next page.
`authrail_list_compliance_frameworks`
`authrail_list_custom_frameworks`
`authrail_list_custom_framework_versions`
`authrail_get_custom_framework_version`
`authrail_get_framework_scope`
`authrail_list_controls`
`authrail_get_control_draft`
`authrail_list_control_implementations`
`authrail_list_coverage_decisions`
`authrail_list_coverage_reviews`
`authrail_list_custom_framework_imports`
`authrail_get_custom_framework_import`
`authrail_get_control_authoring`

With preparation consent, create or save permitted drafts and request Control
authoring or retry a failed job against its exact saved update time. Reuse the command UUID for unchanged retries; saving requires the exact
current draft revision. Control authoring and its generated text also require `policies.read` consent and current source Policy access. Scope answer preparation preserves the separate human acceptance decision. Preparation never approves a Control or publishes a Framework.
`authrail_create_control_draft`
`authrail_create_control_implementation_draft`
`authrail_save_control_implementation_draft`
`authrail_save_framework_scope_answers`
`authrail_save_control_draft`
`authrail_create_custom_framework_draft`
`authrail_create_custom_framework_version`
`authrail_save_custom_framework_version`
`authrail_start_control_authoring`
`authrail_retry_control_authoring`

Continue exact Framework scope, applicability, publication, import, Control and
mapping review workflows in authenticated AuthRail screens. Signed navigation
links apply no change. Current permission, audience and decision checks still run
in the destination screen. Private upload capabilities stay there.
`authrail_prepare_framework_handoff`
`authrail_prepare_control_handoff`

### Compliance verification — `compliance.verification.read` / `compliance.verification.manage`

Read permitted operational findings, remediation tasks, exact blocker Exceptions,
Framework reports and Evidence package status. Existing exact Control/Test/Evidence
history and Framework readiness tools retain their separate `governance.read` consent.
Private file contents and download capabilities are excluded from these tools.
`authrail_get_test_definition`
`authrail_get_evidence_requirement`
`authrail_list_operational_findings`
`authrail_get_operational_finding`
`authrail_list_remediation_tasks`
`authrail_get_remediation_task`
`authrail_list_readiness_exceptions`
`authrail_get_readiness_exception`
`authrail_list_framework_reports`
`authrail_get_framework_report`
`authrail_list_evidence_packages`
`authrail_get_evidence_package`

With preparation consent, save or remove Test definitions and Evidence Requirements
against the exact owner draft revision, or create/update remediation tasks under
the existing Finding audience. Commands retain their original outcome; a Task
never closes its Finding or changes proof health.
`authrail_create_remediation_task`
`authrail_update_remediation_task`
`authrail_save_test_definition`
`authrail_remove_test_definition`
`authrail_save_evidence_requirement`
`authrail_remove_evidence_requirement`

Continue proof recording/submission, independent review, Finding handling,
Exception decisions, reports and secure upload/download/export in the authenticated
app. These tools return navigation only: no passing Test, accepted Evidence,
approved Exception or delivered file is implied.
`authrail_record_test_run`
`authrail_submit_evidence_record`
`authrail_review_evidence_record`
`authrail_manage_evidence_package`
`authrail_manage_operational_finding`
`authrail_manage_readiness_exception`
`authrail_manage_framework_report`

### Your account — `account.read` / `account.manage`

Read your current account/preferences, security-method metadata, own token-free session pages and own export statuses with read consent.
`authrail_get_my_account`
`authrail_get_my_account_security`
`authrail_list_my_sessions`
`authrail_list_my_exports`

With manage consent, change your display name against its current value or explicit preferences against their current revision. Keep the command UUID unchanged when retrying. Account security, files, session revocation and lifecycle continue privately in AuthRail; handoffs apply no change and reveal no passwords, recovery codes or storage capabilities.
`authrail_update_my_account_profile`
`authrail_update_my_account_preferences`
`authrail_manage_my_account`
`authrail_manage_export`

### Organisation — `organisation.read` / `organisation.manage`

Read the selected workspace and your effective permissions. List/read roles, people and organisational structure under the same owner permissions as the application. Identity metadata requires identity-management permission. Organisation lists return at most20 summaries, and their cursors bind the actor, workspace, tool and filters. Exact role detail supplies grants; exact person detail pages active team memberships instead of returning all memberships in one response.
`authrail_get_organisation`
`authrail_list_organisation_permissions`
`authrail_get_organisation_notification_preferences`
`authrail_list_organisation_roles`
`authrail_get_organisation_role`
`authrail_list_organisation_people`
`authrail_get_organisation_person`
`authrail_list_organisation_invitations`
`authrail_list_organisation_departments`
`authrail_get_organisation_department`
`authrail_list_organisation_teams`
`authrail_get_organisation_team`
`authrail_list_organisation_domains`
`authrail_list_organisation_sso_connections`
`authrail_list_organisation_exports`

With manage consent, continue named role/people/structure/settings/lifecycle and export workflows in authenticated human screens. Selected references must belong to the connected workspace. Invitation acceptance uses the recipient's private email link. These navigation tools apply no values or grants; the browser rechecks current authority and confirmation.
`authrail_manage_organisation`
`authrail_manage_organisation_export`

### Your notifications — `notifications.read` / `notifications.manage`

Read only your account or connected-workspace inbox, separate unread counts and optional export-notification preferences. Internal Admin notifications are excluded.
`authrail_list_my_notifications`
`authrail_get_my_notification_counts`
`authrail_get_my_export_notification_preferences`

With manage consent, mark one addressed notification read or at most500 through an explicit creation-time cutoff. A stable command UUID retains the original changed-count; later arrivals remain unread. When reachedBatchLimit is true, read current counts before another batch with a new key. Preference changes require their exact current booleans; mandatory notification channels retain their rules.
`authrail_mark_notification_read`
`authrail_mark_notifications_read`
`authrail_update_my_export_notification_preferences`

### Reviewed business tools — `business.tools.read`

List currently reviewed tools available under the caller's account and application access.
`authrail_list_available_tools`

Read one available tool's exact reviewed input/result contracts.
`authrail_get_available_tool`

These reads do not execute a provider operation or grant account access. A page
can be empty with a continuation cursor when stale contracts are filtered.

### Human file selection — `business.execute`

`authrail_prepare_business_file` prepares the exact reviewed route/binding,
non-file arguments, caller audience and UUID request key. It returns an authenticated
human handoff; the assistant cannot upload bytes or receive the presigned capability.
`authrail_get_business_file` reads only the original selection and safe immutable
metadata through its original MCP client/session. Only a ready reference belongs
in `selectedFile` for the same route/arguments. `authrail_revoke_business_file`
withdraws that selection without undoing a dispatched provider effect.

Actor selections are specific to the original signed-in assistant connection;
deployment selections are for the named SDK workload and are refused to hosted
assistants. File selection does not approve or dispatch the business operation.
The ordinary business-request tools below remain its execution/status owners.

### Business requests — `business.execute`

Save one exact reviewed route, binding digest and argument object under a stable UUID request key.
`authrail_request_business_action`

Read the original request status and only its permitted released result; this read does not dispatch work.
`authrail_get_business_request`

Continue the saved request at its current revision without replacement arguments.
Stale unstarted review requires fresh facts and a new exact human approval.
`authrail_resume_business_request`

Request cancellation at the current revision. A dispatched effect cannot be undone by cancellation.
`authrail_cancel_business_request`

### Accounts — `connections.read`

List provider Connections and their saved health.
`authrail_list_connections`

Read one provider Connection’s saved status.
`authrail_get_connection_status`

List saved customer MCP connections without contacting their servers.
`authrail_list_custom_connections`

Read saved tool names and review status for one customer MCP connection.
`authrail_get_custom_connection`

List native business providers, app setup availability, non-secret setup fields and
available permission names without connecting an account.
`authrail_list_business_providers`

List saved native business accounts.
`authrail_list_business_accounts`

Read an account's saved status and bounded tool-access summary.
`authrail_get_business_account`

List saved A2A agent connections and review status.
`authrail_list_agent_connections`

Read one agent connection's saved skill summary without credentials.
`authrail_get_agent_connection`

### Account setup — `connections.manage`

Open secure account setup, tool review, access replacement or disconnection.
Connecting accepts an optional bounded draft of non-secret account details and
requested permissions. Credentials and the final human action stay on the
authenticated AuthRail page; preparation never connects or authorises an account.
`authrail_manage_business_account`


Find available provider setup options and exact definition versions.
`authrail_list_connection_providers`

Open a secure page for a person to connect, reconnect or disconnect an account.
`authrail_manage_connection`

Open secure human setup, tool review, refresh or disconnection for a customer MCP server.
`authrail_manage_custom_connection`

Open secure human setup, skill review, refresh or disconnection for an A2A agent.
`authrail_manage_agent_connection`

The handoff does not change account access. Provider consent, credential entry and
disconnect confirmation happen in the authenticated application.

### Actions and Rails — `runtime.drafts`

Read protected MCP connector setup guidance.
`authrail_get_protected_mcp_setup`

List connected action contracts.
`authrail_list_connected_actions`

Read an action and its exact versions.
`authrail_get_connected_action`

Create an unconfirmed action draft.
`authrail_create_connected_action_draft`

Save an action draft at its expected revision.
`authrail_save_connected_action_draft`

Read one exact retained action contract and its lifecycle.
`authrail_get_connected_action_version`

Page numbered action history newest first using the returned cursor and a limit
up to50. Exact-version reads contain the contract.
`authrail_get_connected_action_history`

Continue a confirmed predecessor as an unconfirmed successor using its exact
revision and a retained UUID retry key. Identical retries return the same successor,
including after review; human confirmation remains separate. No Rail or deployment
binding changes automatically.
`authrail_continue_connected_action_draft`

Read an exact Rail draft.
`authrail_get_rail_draft`

Read built-in Guard protections and their limits.
`authrail_list_guard_protections`

Prepare an unsaved Contextual AI Rail proposal.
`authrail_propose_contextual_rail`

Create a Guard Rail draft.
`authrail_create_guard_rail_draft`

Save a Guard Rail draft at its expected revision.
`authrail_save_guard_rail_draft`

Run saved examples for an exact Guard Rail draft revision.
`authrail_test_guard_rail_draft`

Find current Guard Rails and their draft/published versions.
`authrail_list_guard_rails`

Read one Guard Rail before choosing an exact version.
`authrail_get_guard_rail`

Read retained published and superseded Guard version history.
`authrail_get_guard_rail_history`

Start a Guard draft from a reviewed action; a concurrent existing draft returns a conflict to reconcile.
`authrail_continue_guard_rail_draft`

Open exact human review of a saved Guard revision, including boundaries, required processing, selected contextual material and examples.
`authrail_review_guard_rail`

Create a Policy Rail draft for an exact published Policy version.
`authrail_create_policy_rail_draft`

Save a Policy Rail draft at its expected revision.
`authrail_save_policy_rail_draft`

Run saved examples for an exact Policy Rail draft revision.
`authrail_test_policy_rail_draft`

Find exact published Policy sources without Framework or Control setup.
`authrail_list_rail_policy_sources`

Read one exact published Policy source with its current audience.
`authrail_get_rail_policy_source`

Find current Policy Rails and their draft/published versions.
`authrail_list_policy_rails`

Read one Policy Rail before choosing an exact version.
`authrail_get_policy_rail`

Read retained published/superseded version history.
`authrail_get_policy_rail_history`

Find permitted published Data Rail facts for one exact reviewed action.
`authrail_list_policy_data_facts`

Suggest unsaved rules from exact sources and optional reviewed Data facts.
`authrail_propose_policy_rail`

Continue an unpublished draft; copy a retained version only with unchanged sources.
`authrail_continue_policy_rail_draft`

Request a workspace/revision-bound human review link. This never publishes or activates a Rail.
`authrail_review_policy_rail`

### Information sources and Data Rails — `runtime.drafts`

Read supported information-source options.
`authrail_get_data_source_options`

List information-source definitions.
`authrail_list_data_sources`

Read an information source and its versions.
`authrail_get_data_source`

Read an exact information-source version.
`authrail_get_data_source_version`

Create an information-source draft.
`authrail_create_data_source_draft`

Save an information-source draft at its expected revision.
`authrail_save_data_source_draft`

Create a Data Rail draft for reviewed action and source versions.
`authrail_create_data_rail_draft`

Save a Data Rail draft at its expected revision.
`authrail_save_data_rail_draft`

Run saved examples for an exact Data Rail draft revision.
`authrail_test_data_rail_draft`

Read retained reviewed information-source version history.
`authrail_get_data_source_history`

Start a source draft from an exact reviewed version; read and reconcile an existing draft after a conflict.
`authrail_continue_data_source_draft`

Open human review of an exact source draft revision: account, trusted record, projected value, freshness, timeout and unavailable behavior.
`authrail_review_data_source`

Find current Data Rails and their draft/published versions.
`authrail_list_data_rails`

Read one Data Rail before choosing an exact version.
`authrail_get_data_rail`

Read retained published and superseded Data version history.
`authrail_get_data_rail_history`

Start a Data draft from exact reviewed action and source versions; copy examples only when sources match.
`authrail_continue_data_rail_draft`

Open human review of an exact saved Data draft revision and its source and examples.
`authrail_review_data_rail`

Source review and Rail publication happen in the authenticated workspace with
current human permissions. Handoffs bind the workspace, version and revision;
changed content requires fresh review. These tools do not execute a lookup,
connect credentials or activate a deployment. Contextual assessments disclose
only reviewed material to AuthRail and its configured model recipient after
required local filtering. Missing or uncertain findings cannot clear a check.

### Deployments — `runtime.deployments`

List visible deployments for an AI system.
`authrail_list_deployments`

Create an Observe installation without credentials or active Rails.
`authrail_create_deployment`

Read a deployment and its connection signals.
`authrail_get_deployment`

Read deployment setup metadata and public fingerprints.
`authrail_get_deployment_setup`

Read selected Rails, mode and sequence.
`authrail_get_deployment_rail_selection`

Prepare an exact activation for human confirmation.
`authrail_request_deployment_activation`

Prepare an exact Enforce renewal for human confirmation.
`authrail_request_deployment_renewal`

Prepare Observe Rail removal for human confirmation.
`authrail_request_deployment_revocation`

Prepare restoration of a retained Observe bundle for human confirmation.
`authrail_request_deployment_rollback`

### Approvals — `runtime.approvals`

List pending requests assigned to the signed-in reviewer.
`authrail_list_my_runtime_approvals`

Prepare an approve, reject or cancel decision for human confirmation.
`authrail_request_runtime_approval_decision`

### Action history — `runtime.read`

Read the Action summary or cursor-page safe certificate, receipt or approval metadata using historyKind and optional historyCursor; limit is 1–10.
`authrail_get_runtime_action`

### Existing compliance records — `governance.read`

List selected Framework versions.
`authrail_list_organisation_frameworks`

Read a selected Framework’s exact version and status.
`authrail_get_organisation_framework`

Read saved Framework readiness and applicable Obligations.
`authrail_get_framework_readiness`

List visible Control mappings for an Obligation decision.
`authrail_list_control_mappings`

Read exact Control version metadata.
`authrail_get_control`

List Test definitions and their latest reported assessments.
`authrail_list_tests`

List Evidence Requirements for an exact Control owner.
`authrail_list_evidence_requirements`

List exact Policy versions referenced by a Control version.
`authrail_list_control_policy_bases`

Read exact Policy version metadata.
`authrail_get_policy_version`

List recorded runs for an exact Test version.
`authrail_list_test_runs`

List Evidence Record metadata for an exact requirement.
`authrail_list_evidence_records`

List retained review decisions for an exact Evidence Record.
`authrail_list_evidence_record_decisions`

## Local protected connector tools

The separate `authrail-protected-mcp` command uses stdio. Each configured business
tool accepts its reviewed arguments and a stable `requestKey`. The connector
supplies the downstream idempotency field for writes.

Request a reviewed business operation.
`<configured business tool name>`

Continue an unchanged saved request after human review.
`authrail_continue_request`

Read saved request status without executing or releasing cached output.
`authrail_get_saved_request`

Inputs and results follow the registered schemas in
[protectedmcp/connector.go](protectedmcp/connector.go). The checked business result
is `{text: string, data: object}`. Unreviewed content is withheld.

Embedded applications can use `ConnectManagedDownstream` with a fixed account
witness and a trusted read-only `DownstreamIdentityVerifier`. Explicit `Refresh`
verifies the replacement account, server and reviewed schemas, and returns busy
while an operation leases the old session. It preserves pending work and never
retries a business call. `DownstreamError.Status` distinguishes unavailable files,
HTTP rejection, missing session and uncertain transport; expiry/revocation require
trusted verifier facts. The legacy stdio/fixed-session path has no automatic token
reload. Customer API/resource readers refresh private credentials per read while
retaining their exact account and resource checks. See
[the managed connector API](protectedmcp/managed_downstream.go).

## AuthRail API bindings

Validate a resource token using the private service credential.
`POST /auth/mcp/introspect`

Exchange a resource token for an exact named-tool API delegation.
`POST /auth/mcp/delegations/exchange`

Delegated business endpoints belong to the AuthRail API. Their mounted paths and
contracts are defined in [the MCP controllers](../authrail-server/src/auth/mcp).

## Organisation Policies and Policy Library

These named tools use `policies.read` for selected Policy content and metadata,
plus current Policy read permission and exact saved version audience. Draft and
authoring commands additionally require `policies.manage` and the owning domain
permission. Management consent alone does not permit reading document contents.
No Framework or Control activation is required for an organisation Policy.

| Tools | Workflow |
| --- | --- |
| `authrail_list_policies`, `authrail_get_policy` | Search visible Policies and read lifecycle, owner, audience, freshness and exact draft/published references. |
| `authrail_list_policy_versions`, `authrail_get_policy_history` | Follow bounded exact version and lifecycle history without substituting a current version. |
| `authrail_get_policy_content` | Read one selected Policy/version as lossless JSON text chunks, each at most 24,000 UTF-8 bytes. Concatenate `text` by `offset` until `hasNextPage=false`, then parse JSON. Continuation binds the content digest, version, actor and workspace; changes require a new read. |
| `authrail_list_policy_categories`, `authrail_list_policy_owner_candidates` | Find named category options and active same-workspace owners. Owner selection retains Policy management permission. |
| `authrail_create_policy_draft`, `authrail_continue_policy_draft`, `authrail_save_policy_draft` | Create an unpublished Policy, start an empty/exact-version draft, or save the exact draft revision. Owner, audience and renewal cadence remain domain-validated. |
| `authrail_list_policy_authoring_sessions`, `authrail_get_policy_authoring_session` | Find audience-visible saved sessions; read metadata and ten turn references per page. Select `part=proposal` or an exact `part=turn`/`turnId` for snapshot-bound JSON chunks. |
| `authrail_start_policy_authoring`, `authrail_update_policy_authoring` | Prepare AuthRail-operated drafting using the explicitly selected business context and audience. Selected material reaches AuthRail and its configured model recipient; no customer model keys are requested. |
| `authrail_answer_policy_authoring`, `authrail_retry_policy_authoring` | Answer current guided questions or retry one failed original turn. Both use the exact session revision and retain the resulting asynchronous turn reference. Follow status before another turn. |
| `authrail_save_policy_authoring_proposal`, `authrail_cancel_policy_authoring`, `authrail_use_policy_authoring_draft` | Save a proposal, cancel a session, or use one exact ready proposal as an unpublished Policy draft. Active work and current context are rechecked. |
| `authrail_list_policy_library_entries`, `authrail_get_policy_library_entry`, `authrail_list_policy_library_categories` | Search published reusable starting material and read its exact content hash and lossless content chunks. |
| `authrail_adopt_policy_library_entry` | Adopt the reviewed exact Library content hash into one unpublished organisation Policy. Adoption neither publishes content nor creates a copy per Framework. |
| `authrail_list_policy_library_submissions`, `authrail_get_policy_library_origin` | Follow organisation submission status within the exact current Policy audience and inspect original adoption provenance. |
| `authrail_prepare_policy_handoff` | Open signed actor/workspace-bound human publication, independent review, draft discard, archive/restore, Library sharing/withdrawal, category management or private source handling. The link applies no change. Exact saved revision acknowledgement precedes the existing final human confirmation. |

Commands retain a stable idempotency key, canonical input and outcome in the same
PostgreSQL transaction as their domain mutation. Identical retries return the
original references; changed input conflicts. Current authority is checked before
replay. Pending AI work returns references and remains independently observable.
Private source bytes, previews, presigned storage access and downloads stay in the
authenticated browser flow. No tool accepts an arbitrary endpoint, method or SQL.
Existing `authrail_get_policy_version` remains a separate governance metadata read;
it does not disclose full document content or replace the explicit content consent.

### AI Inventory — `inventory.read` and `inventory.manage`

Read consent exposes visible systems, exact configuration and retained history.
Management consent permits explicit record changes with a stable command UUID.
Use the exact revision/current lifecycle returned by AuthRail. Identical retries
retain the original saved revision; current authorized metadata may have advanced.
MCP-created systems start unconfirmed. A person confirms the reviewed identity
or adopts a revision on deployments in AuthRail; these tools do not activate Rails.

Find visible systems and discovered, unconfirmed identities.
`authrail_list_ai_inventory`

Read current system identity and configuration overview.
`authrail_get_ai_inventory_item`

Read exact configuration by `overview`, `providers` or `audience` section.
`authrail_get_ai_inventory_revision`

Read retained revision summaries with an explicit continuation.
`authrail_get_ai_inventory_history`

Create an unconfirmed system record from reviewed business details.
`authrail_create_ai_inventory_item`

Save a new configuration against its exact expected revision.
`authrail_update_ai_inventory_item`

Change lifecycle against the exact revision and current lifecycle.
`authrail_change_ai_inventory_lifecycle`

Open revision-bound human confirmation, deployment-revision adoption or private integration discovery-key setup/revocation.
`authrail_review_ai_inventory_item`

### Activity Records — `activity.read` and `activity.record`

Activity records describe reported work. Runtime permission, execution start,
reported completion and proven external effects remain separate facts. Recording
or correcting an Activity does not execute its action, approve a review or create
accepted compliance Evidence. Attestations establish saved-record integrity only.

List summaries within current Activity and Inventory audiences; full outcome
text is available through the exact record read.
`authrail_list_activity_records`

Read current record overview and its exact revision reference.
`authrail_get_activity_record`

Read an exact retained record by `overview`, `audience` or `references` section.
Policy/Evidence references keep their source permissions and audiences; inaccessible
reference content is withheld. History and reviews use separate bounded tools.
`authrail_get_activity_revision`

Read retained correction revision references.
`authrail_get_activity_history`

Read human reviews for one exact visible revision.
`authrail_get_activity_reviews`

Find permitted categories.
`authrail_list_activity_categories`

Find permitted data classifications.
`authrail_list_activity_classifications`

Append a reported record against the exact Inventory revision, with a stable UUID.
`authrail_create_activity_record`

Append a correction against the exact current manual/imported revision. Runtime
entries cannot be rewritten. Identical retries retain the original correction.
`authrail_correct_activity_record`

Open exact human review; the assistant cannot supply the human decision.
`authrail_review_activity_record`

Open private CSV/JSON upload in AuthRail. No file or storage capability is returned.
`authrail_import_activity_records`

Follow a retained import's status, counts and timestamps without file contents.
`authrail_get_activity_import`

Read exact revision attestation status and safe proof metadata.
`authrail_get_activity_attestation`

Verify the saved payload hash/signature. Missing or unfinished proof remains unknown.
`authrail_verify_activity_attestation`

Open private attestation download for an exact visible revision in AuthRail.
`authrail_download_activity_attestation`

Pagination cursors bind account, workspace, tool, owner and filters. Inventory
providers/audience, Activity references/audience and long histories have explicit
continuations. Secure handoffs are signed account/workspace navigation and apply
no change. Human review rejects stale revisions; signed storage URLs, credentials,
raw files and internal Admin authority never enter these tools.

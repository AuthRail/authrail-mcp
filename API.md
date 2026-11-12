# AuthRail MCP API

## HTTP endpoints

Send authenticated MCP JSON-RPC requests over Streamable HTTP.
`POST /mcp`

Read OAuth protected-resource metadata.
`GET /.well-known/oauth-protected-resource/mcp`

Check process liveness.
`GET /health/live`

## MCP tools

All tools require `workspace.read`. The headings identify additional consent.
The API also checks current organisation permissions and access to each record.
Tool input and result schemas are supplied by MCP discovery and the typed
registrations in [internal/mcpserver](internal/mcpserver).

### Workspace — `workspace.read`

Inspect the assistant connection and its consented access.
`authrail_get_connection`

Read the connected organisation and account.
`authrail_get_workspace_summary`

Read released SDK installation guidance.
`authrail_get_sdk_integrations`

### Accounts — `connections.read`

List provider Connections and their saved health.
`authrail_list_connections`

Read one provider Connection’s saved status.
`authrail_get_connection_status`

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

Create a Policy Rail draft for an exact published Policy version.
`authrail_create_policy_rail_draft`

Save a Policy Rail draft at its expected revision.
`authrail_save_policy_rail_draft`

Run saved examples for an exact Policy Rail draft revision.
`authrail_test_policy_rail_draft`

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

Read bounded Action, decision and execution-receipt metadata.
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

## AuthRail API bindings

Validate a resource token using the private service credential.
`POST /auth/mcp/introspect`

Exchange a resource token for an exact named-tool API delegation.
`POST /auth/mcp/delegations/exchange`

Delegated business endpoints belong to the AuthRail API. Their mounted paths and
contracts are defined in [the MCP controllers](../authrail-server/src/auth/mcp).

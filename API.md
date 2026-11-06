# AuthRail MCP API

The Go service exposes Streamable HTTP at `/mcp`. The official MCP SDK handles
protocol negotiation, tool discovery and typed calls. Requests require an OAuth
bearer token for this resource with `workspace.read` access.

## Tools

The identity tools accept `{}`. Every tool rejects arguments outside its input schema.

| Tool | Structured result |
| --- | --- |
| `authrail_get_connection` | `clientId`, `organisation: {id, name}`, `user: {id, name}`, `scopes` |
| `authrail_get_workspace_summary` | `organisation: {id, name}`, `user: {id, name}`, `permissions` |
| `authrail_list_organisation_frameworks` | `frameworks`, `pageInfo: {hasNextPage, nextCursor}` |
| `authrail_get_organisation_framework` | `organisationFramework` with exact version, status and revision |
| `authrail_get_framework_readiness` | `organisationFramework`, `scopeSnapshotId`, `assessmentStatus`, `restricted`, `recalculating`, nullable `readiness`, `obligations`, `pageInfo` |
| `authrail_list_control_mappings` | Exact Framework/Obligation references, `restricted`, bounded `records`, `pageInfo` |
| `authrail_get_control` | `control` with the exact requested version’s metadata |
| `authrail_list_tests` | Exact owner, `evaluatedAt`, Test records with nullable latest run and assessment, `pageInfo` |
| `authrail_list_evidence_requirements` | Exact owner, requirement metadata and `pageInfo` |
| `authrail_list_control_policy_bases` | Exact Control owner, distinct Policy version metadata, `restricted`, `pageInfo` |
| `authrail_get_policy_version` | `policyVersion` identity, lifecycle, revision and review timestamps |
| `authrail_list_test_runs` | Exact `definitionVersionId`, recorded run metadata, `pageInfo` |
| `authrail_list_evidence_records` | Exact `requirementId`, Record metadata and source restrictions, `pageInfo` |
| `authrail_list_evidence_record_decisions` | Exact requirement/record revision and state, retained decision metadata, `pageInfo` |
| `authrail_list_connections` | `records`, `pageInfo: {hasNextPage, nextCursor}` |
| `authrail_get_connection_status` | `connection` with provider names, status and saved health |

The identity and governance tools read selected records without changing them.
Rail preparation tools below can edit unpublished drafts; none executes a business operation. Results exclude session IDs, tokens and
provider credentials. Workspace permissions in this projection describe MCP
capabilities; they are not a grant of every organisation role permission.

## Framework arguments and results

The three Framework tools require `governance.read` in addition to `workspace.read`.
This is an optional consent choice, not a replacement for domain permissions.
Tool discovery reflects the connection’s granted scopes; the API checks them
again when a tool runs.

```json
{"source":"custom","limit":10}
```

Use `source: "authrail"` for managed Frameworks or `"custom"` for organisation-
authored Frameworks. List pages allow 1–20 entries. Pass `pageInfo.nextCursor` as
`cursor` to continue. Framework names and version labels are bounded; an entry
reports `projectionTruncated` when text was shortened. Document bodies are not
part of this navigation result.

```json
{"organisationFrameworkId":"f221769a-4df6-45c9-9cd0-2668fbd86e20"}
```

Both detail and readiness use the selected `organisationFrameworkId` returned
by the list, rather than the catalogue’s Framework ID. Readiness also accepts
`limit` from 1–10 and its own `cursor`. The readiness cursor belongs to the exact
workspace, Framework and Scope Snapshot; restart pagination after scope changes.

Readiness is read from saved assessments, without triggering a recalculation.
`assessmentStatus` explains whether it is `available`, `restricted`,
`recalculating`, `stale`, `not_evaluated` or `inactive`. Unavailable assessments
have `readiness: null`. Restricted contributors anywhere in the Framework can
withhold its aggregate assessment, including contributors outside the page.
Obligation entries have their own assessment status. Safe blocker codes omit
record IDs and private explanations; `blockersTruncated` reports omitted reasons.

## Follow an Obligation into verification

These four tools require `governance.read`. Mappings additionally require the
workspace’s `framework.select` permission. Control and Test reads use the
domain’s membership and exact-version audience rules. Evidence Requirements additionally
require `evidence.read`.

```json
{
  "organisationFrameworkId":"f221769a-4df6-45c9-9cd0-2668fbd86e20",
  "obligationApplicabilityDecisionId":"b92e5139-a54f-44c5-a704-9ba08c4fb109",
  "limit":10
}
```

Pass those exact references from readiness to `authrail_list_control_mappings`.
It returns visible mapping IDs, Control/version IDs, bounded Control titles,
contributions, effective dates and lifecycle states. History is included; an
ended, withdrawn, scheduled or review-required mapping is not active coverage.
`restricted: true` means the visible list is incomplete. Hidden Control or scoped
Implementation references are not disclosed by this projection. The list does
not expose mapping rationale, private provenance or raw Health blockers.

```json
{
  "controlId":"e426c7b2-c1d1-4614-b9f9-ae9fd83f066f",
  "controlVersionId":"1633c01f-a607-4bc5-91ae-bcb8056b9559"
}
```

Use these arguments with `authrail_get_control`. The version ID is required;
AuthRail does not substitute the latest draft. The result contains Control and
version lifecycle, version number, revision, title, purpose and effective dates.
Statement, implementation and source bodies are excluded.

Use the same owner IDs with `authrail_list_tests` or
`authrail_list_evidence_requirements`. Both also accept an optional exact
`implementationVersionId`, `limit` from 1–10 and `cursor`. Cursors are bound to
the tenant, exact owner and tool; Test cursors cannot page Evidence Requirements.
The owner result identifies its revision, status and whether it is operational.
A draft’s definitions do not establish approved operational coverage.

Test results contain definition/version IDs, a bounded title, cadence/freshness,
a safe latest-run summary and the existing domain freshness assessment. Missing
runs remain missing; failed and errored outcomes remain failed and errored.
If a Runtime-linked Test is outside the user’s Runtime visibility, its title is
restricted and its run/assessment are withheld. Procedures, run input and
explanation text, Runtime payloads and provider responses are not returned.

Evidence Requirements contain requirement/version IDs, title, source type,
cadence, freshness and period. They describe expected proof, not an accepted
record. Expected-proof text, attestations, retained file keys, content and
signed download URLs are excluded. None of these tools records a result,
accepts proof, changes a mapping or recalculates readiness.

## Policy references and retained history

These tools require `governance.read`. Policy reads additionally require
`policy.read` and the exact Policy version’s audience. Reading a Control’s Policy
references also checks that Control version’s audience. Evidence history retains
`evidence.read`; Test history resolves the exact verification owner and checks
Runtime visibility before disclosing a Runtime-linked Test’s runs.

`authrail_list_control_policy_bases` accepts `controlId`, `controlVersionId`,
optional `limit` from 1–10 and `cursor`. Multiple linked sections of the same
Policy version appear as one version reference. The result excludes section
text and identifies incomplete visibility with `restricted: true`.

```json
{
  "policyId":"5265ecde-8f3d-4b6d-8aa6-3f032bc91b01",
  "policyVersionId":"e8dba20b-4bba-41a7-b0cb-8fa721b45bd3"
}
```

Pass that exact pair from the references to `authrail_get_policy_version`.
It returns catalog title/status and exact version number, status, revision,
publication, review-due and discard timestamps. A newer draft is not substituted.
Policy document bodies and private ownership details are excluded.

```json
{
  "controlId":"e426c7b2-c1d1-4614-b9f9-ae9fd83f066f",
  "controlVersionId":"1633c01f-a607-4bc5-91ae-bcb8056b9559",
  "definitionId":"cc0171e0-356a-4ca4-9230-81b414575a99",
  "limit":10
}
```

Use `authrail_list_test_runs` with the Test list item’s **`id`** as `definitionId`.
The item’s separate `definitionId` is its lineage, not the exact version record.
Results contain recorded outcomes, periods, performance time and bounded
evaluator metadata. They exclude inputs, explanations and provider data. An old
passing run is not necessarily fresh or the latest applicable proof; use the
Test summary and its freshness assessment for that question.

For `authrail_list_evidence_records`, use the Evidence Requirement list item’s
**`id`** as `requirementId`, alongside the same exact Control owner. The list’s
separate `requirementId` is lineage, not the required version record. Record
metadata includes source type, period, collection time, review status, revision
and replacement reference. `sourceRestricted` identifies a linked Runtime source
outside the reader’s visibility. Source IDs, contents, attestations and URLs are
excluded. A review status does not establish source availability or provider effects.

Pass a returned Record’s `id` as `recordId` to
`authrail_list_evidence_record_decisions`, retaining all owner and requirement
IDs. It returns retained decision IDs, decisions, reviewer staff references and
timestamps, plus the Record’s saved status/revision. Private review reasons are
excluded. Reading a retained decision never submits or replays it.

All history lists accept `limit` 1–10 and `cursor`; verification histories also
accept an exact optional `implementationVersionId`. Cursors bind the tenant,
owner, tool and definition/requirement/record as applicable. They cannot be
transferred between histories or used to switch to another version.

## Provider Connection arguments and results

These tools require `connections.read` consent and the user’s `connection.read`
workspace permission. This is separate from assistant OAuth connection identity.

```json
{"limit":10}
```

`authrail_list_connections` accepts an optional `limit` from 1–10 and the previous
page’s `cursor`. Each result includes `id`, `displayName`, provider `code`, `name`
and `capabilityName`, `status`, `health`, and creation/update timestamps. Names
are bounded; `projectionTruncated` identifies shortened text. The cursor is
bound to the selected workspace.

```json
{"connectionId":"f221769a-4df6-45c9-9cd0-2668fbd86e20"}
```

`authrail_get_connection_status` uses an ID returned by that list. Health includes
a state and nullable `lastSuccessfulSyncAt`; it follows the owning Connection
health rules. These are saved collection signals, not a live provider check or
a passing compliance assessment. Account boundaries, credentials, raw provider
responses and failure messages are excluded. Neither read starts a sync.

## Authentication

`GET /.well-known/oauth-protected-resource/mcp` identifies the protected resource,
AuthRail authorization server and supported scopes. Missing or invalid credentials
receive an OAuth challenge with the metadata address. A token without the required
scope cannot discover or call the protected tools.

The service asks AuthRail to validate the token on each request. AuthRail checks
the source sign-in session, account status, client status, selected organisation,
live membership, consent, resource and expiry. The organisation comes from that
grant, never from tool arguments or caller-supplied tenant headers.

## Backend bindings

| Request | Authentication | Purpose |
| --- | --- | --- |
| `POST /auth/mcp/introspect` with `{token}` | Private service credential | Validate the incoming resource token and return its bounded identity. |
| `POST /auth/mcp/delegations/exchange` with `{token, tool}` | Private service credential | Issue a short-lived API delegation for the exact tool and grant. |
| `GET /organisation/mcp/workspace` | Exact tool delegation and organisation header | Read workspace identity. |
| `GET /organisation/mcp/frameworks` | `authrail_list_organisation_frameworks` delegation | Read a bounded selected Framework collection. |
| `GET /organisation/mcp/frameworks/:organisationFrameworkId` | `authrail_get_organisation_framework` delegation | Read the selected exact version. |
| `GET /organisation/mcp/frameworks/:organisationFrameworkId/readiness` | `authrail_get_framework_readiness` delegation | Read a bounded, visibility-checked saved assessment. |
| `GET /organisation/mcp/control-mappings` | `authrail_list_control_mappings` delegation | Follow one exact Obligation’s visible mappings. |
| `GET /organisation/mcp/controls/:controlId/versions/:controlVersionId` | `authrail_get_control` delegation | Inspect exact version metadata. |
| `GET /organisation/mcp/controls/:controlId/versions/:controlVersionId/tests` | `authrail_list_tests` delegation | Read bounded Test summaries. |
| `GET /organisation/mcp/controls/:controlId/versions/:controlVersionId/evidence-requirements` | `authrail_list_evidence_requirements` delegation | Read bounded proof requirements. |
| `GET /organisation/mcp/controls/:controlId/versions/:controlVersionId/policy-bases` | `authrail_list_control_policy_bases` delegation | Read visible exact Policy references. |
| `GET /organisation/mcp/policies/:policyId/versions/:policyVersionId` | `authrail_get_policy_version` delegation | Read exact Policy version metadata. |
| `GET /organisation/mcp/controls/:controlId/versions/:controlVersionId/tests/:definitionId/runs` | `authrail_list_test_runs` delegation | Read bounded recorded run history. |
| `GET /organisation/mcp/controls/:controlId/versions/:controlVersionId/evidence-requirements/:requirementId/records` | `authrail_list_evidence_records` delegation | Read safe Evidence Record history. |
| `GET /organisation/mcp/controls/:controlId/versions/:controlVersionId/evidence-requirements/:requirementId/records/:recordId/decisions` | `authrail_list_evidence_record_decisions` delegation | Read retained review metadata. |
| `GET /organisation/mcp/connections` | `authrail_list_connections` delegation | Read a page of Connection status metadata. |
| `GET /organisation/mcp/connections/:connectionId` | `authrail_get_connection_status` delegation | Read one Connection’s saved health. |

Incoming MCP tokens are not forwarded to tenant APIs. The service credential
alone cannot obtain a workspace projection. AuthRail's backend owns OAuth
issuance, consent, revocation and durable audit records.

## Transport boundaries

The listener validates Host and browser Origin. Native clients can omit Origin.
Backend requests use fixed paths, bounded response sizes, deadlines and no
redirect following. Backend errors are returned without raw response bodies.
A failed or expired authorization must be resolved by reconnecting; changing
arguments cannot expand the connection's access.

`GET /health/live` checks process liveness. It does not establish user authority
or a working connection to AuthRail.

## Rail preparation tools

These tools require explicit `runtime.drafts` consent in addition to
`workspace.read`, plus the user's owning domain permissions.

| Tool | Input and result |
| --- | --- |
| `authrail_list_connected_actions` | Optional `limit` 1–50 and `afterId`; returns `operations`. |
| `authrail_get_connected_action` | `operationId`; returns the action and its exact versions. |
| `authrail_get_rail_draft` | `railId`, `versionId`; returns exact version content, revision and review URL. |
| `authrail_propose_contextual_rail` | `kind`, reviewed `operationVersionId`, business `instructions` and a published `policyVersionId` for Policy; returns unsaved assessment, rules and unresolved questions. |
| `authrail_create_guard_rail_draft` | `name`, reviewed `operationVersionId`, UUID `idempotencyKey`; returns draft identity. |
| `authrail_create_policy_rail_draft` | Same creation fields plus published `policyVersionId`. |
| `authrail_save_guard_rail_draft` / `authrail_save_policy_rail_draft` | `railId`, `versionId`, expected `revision`, `name`, complete `content`; returns the saved draft and review URL. |
| `authrail_test_guard_rail_draft` / `authrail_test_policy_rail_draft` | Exact draft IDs and `revision`; returns example results. |

Creation retries reuse the same UUID only for identical content. After a revision
conflict, read and reconcile the draft before saving again. Contextual assessment
and examples are part of exact version content; saving invalidates an earlier
test result. These tools cannot publish. Testing examples does not establish model
accuracy, tool coverage or a provider effect.

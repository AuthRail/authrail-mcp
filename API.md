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
| `authrail_list_connections` | `records`, `pageInfo: {hasNextPage, nextCursor}` |
| `authrail_get_connection_status` | `connection` with provider names, status and saved health |

These tools read identity, selected Frameworks and stored readiness. They do not change governance
records or execute business operations. Results exclude session IDs, tokens and
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

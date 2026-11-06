# AuthRail MCP architecture

The Go service translates named MCP tools into AuthRail API calls. AuthRail's
server owns sign-in, consent, tenant membership, permissions and audit history.
The tenant client supplies the sign-in and consent screens and lets a person
revoke their assistant connections.

## Request flow

1. The MCP client obtains a resource-bound OAuth token through AuthRail sign-in
   and consent for one organisation.
2. We ask AuthRail to validate the token on every HTTP request. This checks
   whether the session, consent and workspace access still allow the connection.
3. A typed tool exchanges the incoming token for a short-lived API delegation
   bound to that grant and tool.
4. The named API endpoint rechecks the source grant and organisation, retains
   the tool-access audit record and returns a bounded projection.
5. Go checks that the returned user and organisation match the request's identity
   before returning the tool result.

The transport is stateless. It does not retain browser sessions, tokens or
workspace selections between requests. A request to one organisation cannot
change another connection's selection.

## Authentication ownership

The backend uses Better Auth's OAuth provider for S256 PKCE, exact registered
callbacks, authorization codes, opaque access tokens and rotating refresh tokens.
Client registration exposes a public PKCE client with explicit bounded capabilities.
Dynamic registration and arbitrary resource registration are disabled.

The backend stores provider records in PostgreSQL and one-use authentication
state in Redis. Access token storage uses SHA-256 hashes. The source browser
session, consent and membership are checked when a token is used. A delegated
API credential expires within one minute and never outlives its source token
or session. It is not accepted as a browser or Runtime deployment credential.

The private service credential authenticates Go to the introspection/exchange
endpoints. It supplies no user or tenant permission by itself. The API delegation
is a separate, purpose-bound credential; raw resource tokens do not reach tenant
API routes.

## Process and network boundary

The listener runs behind HTTPS ingress. We use koanf to load the API URL, public
MCP URL, OAuth issuer and listener address from the environment. Explicit
command-line flags override those values. Addresses require HTTPS;
HTTP loopback development needs explicit opt-in. A private owner-readable file
or injected deployment secret supplies the service credential. Graceful shutdown
allows in-flight requests to finish within a bounded deadline.

Backend responses are bounded to 64 KiB, redirects are rejected, and returned
identity is checked before disclosure. Tool failures do not expose raw provider
bodies or credentials. Diagnostic liveness is separate from authentication.

AuthRail MCP does not contain a policy engine or execute the customer's protected
business tools. Runtime authorization remains with the customer's SDK, protected
executor and local AuthRail sidecar.

## Governance visibility

A workspace-only grant cannot discover or call governance tools. The person
explicitly selects governance reads during consent. Increasing the resource’s
supported capability list does not change existing tokens or consent records.
The backend merges that code-owned list into the OAuth resource configuration
while preserving its disabled flag and other operator settings.

Framework navigation uses the owning Framework repository, exact selected
versions and bounded cursor pages. Readiness uses a read-only repeatable-read
transaction over saved assessments. The backend checks the person’s readiness,
Control and Policy access, including contributors outside the requested page
and contributors retained in the assessment manifest. It withholds assessments
whose audience cannot be established. Missing, stale or recalculating results
remain unavailable. Governance read tools neither write product state nor manufacture readiness.

Connection reads have their own consent scope, connections.read, and retain the
existing connection.read domain permission. The projection selects status and
health inputs only; credentials, account boundaries and provider bodies are not
loaded for tool output. Pagination is workspace-bound and capped at ten entries.
These reads neither contact a provider nor enqueue collection work.

## Follow the verification owner

Mapping reads reuse the Control mapping domain’s actor and subject visibility
rules. They return bounded navigation metadata, never raw blocker or provenance
payloads. Restricted references are withheld and the result identifies incomplete
coverage. Version reads use the exact requested Control version and its audience;
they do not substitute a draft or introduce a new universal read capability.

Test and Evidence Requirement reads resolve the same verification owner used by
the dashboard, including scoped Implementation boundaries. They query only safe
summary fields. Test freshness uses the domain’s existing assessment function;
Runtime-linked summaries also check Runtime visibility. Evidence Requirements
remain separate from accepted proof and retained files. Tool-specific cursors
bind the tenant and exact owner. These reads do not record proof, mutate workflow
state or invoke external providers.

Policy navigation checks Policy read permission and the exact version audience,
as well as Control visibility when following Policy bases. Distinct version
pages keep repeated sections bounded without loading document JSON. Version
metadata includes discard and review timestamps; it never selects a newer draft.

History readers resolve the exact verification owner before reading the Test
or Evidence Requirement version. Queries select safe metadata rather than raw
inputs, attestations or review reasons. Runtime-linked Test Run history requires
Runtime visibility; Evidence metadata flags restricted Runtime sources without
revealing their IDs. Record-review reads retain the exact Record binding and
revision/status. Cursor bindings include the tool and the definition/requirement/
record, keeping one history’s cursor out of another. No read calls storage,
creates signed URLs, records proof or replays a human decision.

## Rail draft preparation

The optional runtime.drafts scope exposes action discovery, exact draft reads,
contextual proposals and draft create/save/test tools. Consent is an access
ceiling; the backend still checks live Rails and Policy permissions. Writes use
the ordinary owning domain services, revision conflicts, idempotent creation and
audit history. Successful preparations also retain the named-tool access record.
No MCP publication or approval route exists in this group.

A contextual proposal contains the assessment meaning, selected fields, typed
answer and deterministic conditions. The hosted interpretation client and its
operational configuration remain in the private server. Go does not receive a
model credential or run the interpretation itself. Exact publication remains an
authenticated human decision in the tenant workspace.

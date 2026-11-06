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

Deployment management and assigned approvals have separate optional consent
groups (`runtime.deployments` and `runtime.approvals`). The Go service exchanges
the current user's token for each exact named backend tool. Deployment reads and
Observe installation creation reuse Runtime's visibility, permission and
idempotency contracts. Setup exposes only public metadata. No hosted tool issues
executor credentials or evaluates a customer call.

Reserved deployment changes and approval decisions return signed ten-minute
proposals. Their purpose-separated signature binds the exact user, organisation,
target, request fields and expected sequence or immutable review hash. Preparing
a proposal changes no Runtime state. Its browser fragment avoids sending the
proposal through page request URLs. Cookie-only human preview/confirmation rejects
Authorization headers and requires the configured frontend origin. The human
reads the exact proposed Rail content or assigned review context and explicitly
confirms; Enforce also needs the operator's boundary assertion. Confirmation
rechecks live domain access and state, then uses the existing domain transaction.
An approval's expected context hash is checked under its decision lock. MCP
receives only minimal assigned-request metadata, never application review context.
No new customer record type, hosted executor or durable proposal store is created.

A contextual proposal contains the assessment meaning, selected fields, typed
answer and deterministic conditions. The hosted interpretation client and its
operational configuration remain in the private server. Go does not receive a
model credential or run the interpretation itself. Exact publication remains an
authenticated human decision in the tenant workspace.

## Customer-side protected connector

`cmd/authrail-protected-mcp` is a separate local stdio service. The hosted
management process remains a server-API client. The `protectedmcp` package uses
the Go Runtime SDK and the customer's loopback sidecar; it contains no evaluator.
It connects to one fixed authenticated Streamable HTTP business-tool server.
Bearer credentials stay in owner-only files. Redirects and tool-call retries are
disabled, and responses, schemas, discovery pages and request durations are bounded.

The private operator configuration binds one authenticated application principal
and session, reviewed Operation versions, exact downstream names/schema digests,
resource ownership and business idempotency fields. `tools/list` metadata is draft
input for that review, never permission. Discovery exposes only those bound tools.
A write tool must have an application-owned idempotency field, resource binding
and explicit review fields including its resource. The downstream implementation
must honour that idempotency contract; a declaration cannot make an arbitrary
provider safe. Resource ownership is resolved before checking the call and again
before execution. A transformed argument cannot change the checked resource.

The connector calls `Enforce` around the real downstream call. The SDK handles
Guard/Data/contextual requirements, signing, retained start/completion reports,
approval permission and result checks. The Operation's result contract is the
normalised object `{text: string, data: object}`. Non-text content, interactive
input requests and unreviewed output shapes are withheld. This integration is
Enforce-only; it does not silently turn an Observe deployment into enforcement.
AuthRail records permission and what the connector reports; it does not
independently verify the downstream business effect.

Private recovery state uses an AES-256-GCM journal, exclusive process ownership,
atomic file replacement and file/directory synchronisation. A durable marker
precedes every effect. The same request key is bound to the original arguments
and reviewed configuration; replay returns metadata rather than rerunning the
tool or releasing cached output. Keep the journal, its encryption key and the
sidecar's complete state together through upgrades. Never clear them to retry.
A process crash at an uncertain point requires operator investigation.

Waiting approvals retain the SDK's original continuation encrypted on disk.
Continue first reads exact approval state with a separate purpose-bound proof.
Pending requests remain saved without consuming permission. Approved requests
claim once through the SDK. A lost claim response can recover only its existing,
unstarted, unexpired permit. Rejection, expiry, changed configuration or an
unknown effect cannot trigger a fresh business call. No agent tool votes,
extends review or submits reconciliation evidence. Trusted application code can
use `Connector.Reconcile` after checking the provider with the original business
idempotency key; recording that result never reruns the effect or releases output.

The command reuses the keys and IDs created by sidecar setup. It loads supported
Stripe, QuickBooks and fixed customer-API reader bindings from private settings;
other named readers use the package's trusted application callbacks. API resource
lookups bind the fixed principal and account and require an explicit freshness
limit. Provider authentication/refresh remains customer-owned. No arbitrary SQL,
MCP read tool or live provider access is inferred from a source label.

The agent must reach this connector instead of the equivalent direct business
connection. Keep downstream credentials and private files outside the agent's
execution boundary. Filesystem permissions alone do not isolate a same-user
agent with unrestricted host access. Neither this connector nor hosted management
MCP proves that another route, shell or credential has been contained.

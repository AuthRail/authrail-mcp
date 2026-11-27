# AuthRail MCP

AuthRail governs the actions AI agents take in your business systems. You connect
accounts and applications, choose what agents may access, and publish rules for
their requests and results. Sensitive requests can require a person's approval.
The activity record shows what was allowed, what ran and what outcome was confirmed.

These rules are called **Rails**. Policy Rails set business conditions, Data Rails
check trusted values, and Guard Rails inspect or protect the material an agent
sends and receives. Teams manage them in the AuthRail workspace and connect
their governance records to Policies, Controls, Tests and Evidence for compliance.

**AuthRail MCP** lets an AI assistant work with that workspace through the Model
Context Protocol. Instead of navigating each screen yourself, you can ask the
assistant to prepare a Rail, find a connected account, open a request awaiting
your review or explain an action's outcome. It uses your AuthRail identity and
the organisation access you grant it. You confirm reserved human decisions in
AuthRail.

For example, ask: “Prepare a rule requiring my approval before our agent changes
a production system.” The assistant finds the action, prepares the rule and
checks its examples. You review and publish the exact version. You can then ask
which requests were stopped, which were approved and what happened afterward.

[API reference](API.md) · [Architecture](ARCHITECTURE.md) ·
[Connection guide](https://docs.authrail.io/docs/mcp)

## How the pieces work together

AuthRail MCP is the assistant's governed entry into AuthRail and connected business
systems. Its scope covers business actions, account connections, Policy, Data and
Guard Rails, approvals, application and organisation management, activity and
compliance workflows. You use one AuthRail connection, with access determined by
your identity, organisation permissions and the accounts and tools authorised for
the task.

Requests use the same domain rules as the AuthRail workspace. Rails govern the
business request and its result; credentials stay private, and reserved human
decisions require your confirmation. [API.md](API.md) lists the individual tools,
inputs and consent contracts.

The API owns accounts, approvals and durable execution records. The sidecar
evaluates published Rails. The private gateway adapter and agentgateway deliver
already-authorised work to reviewed provider tools or agent endpoints. For native
provider calls, credentials stay in the API. Responses are recorded separately
from permission, and results need their own checks before disclosure.

Developers can also connect their own applications through an SDK and sidecar,
or use the protected MCP connector described below. The
[architecture guide](ARCHITECTURE.md) describes these execution paths and their
implementation boundaries.

## Connect an assistant

Use the MCP address supplied by your AuthRail installation. The standard
configuration uses `https://mcp.authrail.io` as a remote Streamable HTTP
connection in a compatible assistant.

1. Start the assistant's connection flow and sign in to AuthRail.
2. Choose your organisation and review the requested access.
3. Return to the assistant and ask it to show your workspace or prepare a task.

Assistants supporting client identity documents or dynamic registration can
register during connection. If an assistant requires a pre-registered client ID,
open `/mcp/clients` in AuthRail, register its exact callback address and copy the
public client ID. HTTPS and HTTP loopback callbacks are supported without
wildcards. Registration identifies the assistant; sign-in and consent grant access.

Review or revoke assistant access at `/mcp/connections`. Your role,
membership and record visibility apply on every request. The MCP service calls
the AuthRail API on your behalf; it has no direct database access or shared
administrator account.

## Connect an account, MCP server or agent

Ask the assistant to show available business providers or open account setup.
Provider discovery includes app setup availability, safe account field names and
permission names, so the assistant can prepare a draft without guessing.
The assistant can prepare a name, non-secret account/resource details and requested
permissions before opening setup. The link carries this untrusted draft in its URL
fragment; AuthRail removes the fragment after reading it. You review and edit the
details in AuthRail, then supply private access or complete provider sign-in.
The API checks a permitted read before
saving encrypted account access. You then review which apps and business actions
may use each tool.

Custom MCP follows a similar journey: supply your server's public HTTPS address
and bearer access, discover its tools, then review their inputs and results
against your business actions. A customer-owned MCP server is one connection
option; named provider accounts have their own authorisation and setup.

For an A2A agent, supply its HTTPS card address and bearer access in AuthRail,
then review a selected skill against exact business input/result contracts. A card
read is discovery evidence, not proof of the bearer token's execution permissions.

The assistant can read saved account summaries and open setup, tool review,
access replacement or disconnection. Credentials and final human confirmation
stay on the authenticated page. Connecting an account does not grant every
assistant permission to use its tools. Changing access withdraws the old tool
review; disconnecting stops new use and retains history.

The account workflow uses the selected business providers, Custom MCP servers and
A2A agents. Saved compliance evidence and monitoring history remain available
through compliance tools; collecting information never authorises business writes.

## Request a business action

Grant reviewed-tool discovery and business-request access during assistant consent.
Ask the assistant to find an available tool, inspect its exact contract and request
one change using a stable request key. AuthRail saves the request, checks current
access and Rails, and returns its status. A request can wait for human approval;
continuation uses the original saved input and refreshes authority before execution.

The assistant can read status, continue the saved request or request cancellation.
Reading status does not run work. An unknown outcome is checked using its original
reference; a new request key is not a safe retry. Results are released only after
the reviewed result contract and result Rails pass. A withheld result cannot undo
an effect that already happened.

You can also connect a reviewed A2A agent through secure human setup. Current
source supports JSON-input A2A 1.0 JSON-RPC recipients with manual bearer access
and an interface on the same HTTPS origin as the agent card. You choose messaging,
status and cancellation access separately. Files, streaming/push, tenant overrides,
required extensions, text-only skills and OAuth acquisition are unsupported. Agent
skills describe intended work; they do not restrict the recipient's internal tools.
Source/local checks do not prove live recipient permissions or provider effects.

## Prepare and review Rails

Grant **Rail draft preparation** during assistant consent using an account that
can manage Rails. Describe the action, the rule and examples of requests that
should pass or stop. The assistant can read the action's reviewed contract,
create or update a Policy, Data or Guard Rail draft, and run its examples.

For example:

> Prepare a Guard Rail for our code-change action that blocks changes weakening
> authentication. Show me the material used for the assessment before saving it.

For Contextual AI, AuthRail operates the model connection. Review the question,
selected material and expected answer on the Rail. You do not supply a model key.
Only permitted selected material is sent for assessment, and an unavailable or
uncertain assessment cannot count as permission.

For a Data Rail, describe the value needed, its source and how fresh it must be.
A saved source or passing example does not establish that a live reader is
connected. Use the capabilities of the installed SDK and sidecar when connecting
your own application.

Draft preparation and publication are separate actions. A person reviews the
exact version before it takes effect. The same permissions and revision checks
apply whether the draft starts in the workspace or an assistant.

## Review requests and understand results

An assistant can list approval requests assigned to you and prepare an approve,
reject or cancel decision. You confirm it in AuthRail under your own identity.
The assistant cannot cast your vote or directly resume an application's callback.

Action history distinguishes permission, execution reports and evidence of what
actually happened. Allowing a request does not mean the provider completed it.
An accepted background job stays pending until its original status is checked.
If a response is lost, recovery checks the original reference before considering
another write. A timeout does not establish failure or make a duplicate safe.

The private adapter records transport evidence. The API keeps native provider
credentials and encrypted results; result disclosure requires its own checks.
Account-management tools open setup and review; they do not execute provider
business actions.

## Manage applications and compliance

An assistant can inspect registered deployments and prepare an Observe
installation. Activation, renewal, rollback and revocation use an exact review
link for a person to confirm. A later configuration change requires a fresh
review; an older proposal cannot authorise a different version.

Compliance tools let you follow a Framework assessment through its
Controls, Tests and Evidence. Ask, “Which duties need attention, and what supports
this assessment?” Results respect your visibility and explain unavailable or
stale records. Evidence collection alone does not establish a passing Test or a
ready Framework.

Choose only the consent needed for your work:

| Access | What the assistant can do |
| --- | --- |
| `workspace.read` | Read your connected workspace and available capabilities. Required for all tools. |
| `support.reports` | Submit deliberately reviewed problems and follow your own reports and public support replies. |
| `business.tools.read` | Discover currently reviewed business tools and their exact contracts. |
| `business.execute` | Request, inspect and continue original business work, or request cancellation. |
| `connections.read` | Read saved account and connection summaries. |
| `connections.manage` | Open secure human account setup and management. |
| `runtime.drafts` | Prepare and check unpublished Rails and source drafts. |
| `runtime.deployments` | Inspect deployments and prepare reviewed changes. |
| `runtime.approvals` | List assigned requests and prepare a human decision. |
| `runtime.read` | Read permitted Action and execution metadata. |
| `governance.read` | Inspect existing compliance definitions and proof metadata. |

New tools do not expand an existing consent automatically. Domain permissions
still apply after consent.

## Connect your own application

Developers can protect their own business functions with the
[Node](https://github.com/AuthRail/authrail-sdk-node),
[Python](https://github.com/AuthRail/authrail-sdk-python) or
[Go](https://github.com/AuthRail/authrail-sdk-go) SDK and the
[AuthRail sidecar](https://github.com/AuthRail/authrail-sidecar). The sidecar
evaluates published Rails; the application owns its provider calls and credentials.

The separate `authrail-protected-mcp` source command applies the Go SDK and a
customer-hosted sidecar to reviewed downstream MCP tools. It checks calls and
results, saves waiting requests and retains execution recovery. Its default
journal is local; an owning application can opt into the shared API-backed store.
It is a source integration, not a released generic installer. See the
[connector contract](API.md#local-protected-connector-tools) and
[recovery design](ARCHITECTURE.md#request-recovery).

Equivalent direct tools or credentials can bypass this protected route. Remove
or block those paths before relying on enforcement. Recover an uncertain write
using its original identity; a new request key does not resolve an earlier effect.

## Run locally

The existing AuthRail Compose project owns the API, worker, database, MCP service,
hosted sidecar, adapter and pinned agentgateway. Follow the
[server setup](../authrail-server/README.md) first and apply its database migrations.
From `authrail-server`, build and start the MCP components:

```sh
docker compose build mcp gateway-adapter
docker compose up -d --no-deps mcp gateway-adapter agentgateway
```

The API and its dependencies must already be running. Provider access comes from
protected account setup; it is not embedded in gateway configuration. The adapter
and gateway remain on the private network. The MCP listener is available on host
port 3016, with Host and resource checks matching its configured address.

`authrail-mcp health` checks API readiness. The adapter has a readiness check for
recent routing configuration, and agentgateway has its own health check.
OpenTelemetry sends service signals through the established Collector. Credentials
and raw request/result content are excluded from operational signals.

## Configure the MCP service

For a separate installation, build with Go 1.26.5 or newer and provide the API
origin, public MCP address and private service credential:

```sh
go build -o bin/authrail-mcp ./cmd/authrail-mcp
export AUTHRAIL_MCP_API_URL=https://api.example.com
export AUTHRAIL_MCP_AUTHORIZATION_SERVER_URL=https://api.example.com/auth
export AUTHRAIL_MCP_SERVICE_TOKEN_FILE=/run/secrets/authrail-mcp-service-token
./bin/authrail-mcp
```

Replace the example API address with your installation's address and set
`AUTHRAIL_MCP_RESOURCE_URL` to its canonical root OAuth resource identifier (for
example `https://mcp.authrail.io`, without a path or trailing slash) before
starting the process. The API's `MCP_RESOURCE_URL` must match that identifier. Compose also uses
`MCP_AUTHORIZATION_SERVER_URL` for the exact API-advertised issuer, including
its `/auth` path; it must not append another `/auth` to an existing issuer.
The [service configuration reference](ARCHITECTURE.md#listener-and-resource-configuration)
defines the listener and resource settings together. The mounted credential
must match the API's private MCP service credential and be readable only by its
owner. It authenticates the service; users still sign in and grant consent.

| Variable | Setting |
| --- | --- |
| `AUTHRAIL_MCP_API_URL` | Required HTTPS API origin, without a path. |
| `AUTHRAIL_MCP_RESOURCE_URL` | Required HTTPS OAuth resource identifier matching the API and listener. |
| `AUTHRAIL_MCP_AUTHORIZATION_SERVER_URL` | OAuth issuer; defaults to the API origin followed by `/auth`. |
| `AUTHRAIL_MCP_LISTEN` | Defaults to `127.0.0.1:3016`; use `0.0.0.0:3016` in a container. |
| `AUTHRAIL_MCP_SERVICE_TOKEN_FILE` | Private mounted service credential. |
| `AUTHRAIL_MCP_SERVICE_TOKEN` | Alternative injected credential; choose one credential source. |
| `AUTHRAIL_MCP_ALLOW_LOOPBACK` | Allows HTTP loopback development; defaults to `false`. |
| `AUTHRAIL_MCP_TRUSTED_API_ORIGIN` | Exact private HTTP API origin for a configured internal network. |

Settings load from defaults, environment and command-line flags. The process does
not load `.env` files. Flags include `-listen`, `-api`, `-resource`,
`-authorization-server`, `-service-token-file`, `-allow-loopback` and
`-trusted-api-origin`; `-local` also enables HTTP loopback development.
The private HTTP exception must match the configured API origin exactly.

Put a deployed listener behind HTTPS ingress at `https://mcp.authrail.io`.
Configure its listener, OAuth resource, audience and discovery metadata together.
See [API.md](API.md) for health, discovery, gateway and tool contracts, and
[ARCHITECTURE.md](ARCHITECTURE.md) for ownership, transactions and recovery.

Licensed under [Apache 2.0](LICENSE).

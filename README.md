# AuthRail MCP

AuthRail helps organisations govern how they use AI. AuthRail MCP connects an
assistant to an AuthRail workspace through tools authorised by the person who
connects it.

Sign-in and consent belong to AuthRail. Each connection selects one organisation
and grants specific access. The Go service calls AuthRail APIs on that person's
behalf; it has no direct database access or shared administrator account.

## Tools

| Tool | What it returns |
| --- | --- |
| `authrail_get_connection` | The connected account, workspace, client and granted access. |
| `authrail_get_workspace_summary` | The workspace and account names and their effective MCP capabilities. |
| `authrail_get_sdk_integrations` | Released Node.js, Go and Python installation paths and checked integration guidance. |
| `authrail_list_organisation_frameworks` | A page of selected Frameworks and their exact versions. |
| `authrail_get_organisation_framework` | One selected Framework’s version, status and revision. |
| `authrail_get_framework_readiness` | Saved readiness and a page of applicable Obligations, within your content permissions. |
| `authrail_list_control_mappings` | Visible mappings for one exact Obligation decision, including their lifecycle. |
| `authrail_get_control` | Metadata for the exact Control version named by a mapping. |
| `authrail_list_tests` | Test definitions, latest reported outcomes and freshness for an exact owner. |
| `authrail_list_evidence_requirements` | The proof requirements for an exact Control owner. |
| `authrail_list_control_policy_bases` | Distinct exact Policy versions referenced by a visible Control version. |
| `authrail_get_policy_version` | One permitted Policy version’s lifecycle and review metadata. |
| `authrail_list_test_runs` | Recorded run outcomes and periods for an exact Test version. |
| `authrail_list_evidence_records` | Evidence Record periods, revisions and review states. |
| `authrail_list_evidence_record_decisions` | Retained decisions, reviewer references and timestamps for one Record. |
| `authrail_list_connections` | Provider Connection names and saved health, with pagination. |
| `authrail_get_connection_status` | One provider Connection’s saved status and last successful collection time. |

The identity tools require `workspace.read` and accept an empty arguments object.
Framework tools also require `governance.read`, which you select on the consent
screen. Your workspace role and content permissions still apply. Existing
workspace-only connections do not gain governance access automatically.

Provider Connection tools require a separate `connections.read` choice. Ask your
assistant which connections need attention, then inspect one result by ID.
These tools report saved status; they do not start collection, connect to a
provider, reveal its credentials or change its access. Connection health does
not establish that a Test passed or a Framework is ready.

For example, ask your assistant to list your organisation’s custom Frameworks,
then inspect one Framework’s readiness. It follows the selected Framework ID and
pagination cursor returned by AuthRail; you do not supply an organisation ID.
A restricted or unevaluated assessment is shown as unavailable, never as ready.
Access is rechecked for each request. Revoking the connection, signing out or
losing workspace membership stops access.

To investigate a readiness result, ask the assistant to follow an Obligation’s
mappings. A mapping identifies the exact Control version to inspect. Its Tests
show recorded outcomes and freshness; Evidence Requirements describe what proof
the team expects. A requirement is not accepted Evidence, and a Test result does
not independently verify a provider effect. Restricted results are incomplete
and must not be treated as full coverage.

The assistant can follow a Control’s Policy references or inspect its Test and
Evidence history. Use the exact `id` returned by the definition/requirement list
when requesting history. A past passing run can be stale, and an Evidence review
decision does not independently prove a provider effect. History tools return
safe metadata; they do not disclose document text, attestations or file links.

## Prepare a contextual Rail with your assistant

Grant **Rail draft preparation** during connection consent, using an account
that can manage Rails. This separate access does not get added to existing
connections automatically. Then ask, for example:

> Prepare a Guard Rail for our reviewed code-change action. Enable AuthRail
> Contextual AI to block changes that weaken authentication. Show me the question
> and exactly which fields would be sent to AuthRail before saving the draft.

The assistant uses these tools as one preparation flow:

| Tool | Purpose |
| --- | --- |
| `authrail_list_connected_actions` | Find the action to protect. |
| `authrail_get_connected_action` | Read its exact reviewed version and permitted fields. |
| `authrail_propose_contextual_rail` | Prepare an unsaved contextual assessment and rule. |
| `authrail_create_guard_rail_draft` / `authrail_create_policy_rail_draft` | Create the appropriate unpublished draft. |
| `authrail_get_rail_draft` | Read its content and revision before editing. |
| `authrail_save_guard_rail_draft` / `authrail_save_policy_rail_draft` | Save an exact revision with its examples. |
| `authrail_test_guard_rail_draft` / `authrail_test_policy_rail_draft` | Run those examples before human publication review. |

Contextual AI is an AuthRail service enabled on the Rail that needs interpretation.
You describe the business rule; AuthRail prepares its bounded assessment. Review
the question, permitted material and answer that the rule uses. Your connected
application sends that material through the SDK and sidecar to AuthRail. AuthRail
returns a signed finding, and the local sidecar applies the deterministic rule.
An uncertain answer never becomes permission. No model key or extractor setup is
needed in your application or assistant.

These tools use your ordinary workspace permissions. They cannot publish a Rail,
approve a request or execute the business operation. A person reviews the exact
tested version before publication. Connecting management MCP alone also does not
protect another tool: the business function needs its SDK/sidecar boundary.

## Build and run

The service uses Go 1.26.5 or newer:

```sh
go build -o bin/authrail-mcp ./cmd/authrail-mcp
export AUTHRAIL_MCP_API_URL=https://api.example.com
export AUTHRAIL_MCP_RESOURCE_URL=https://mcp.example.com/mcp
export AUTHRAIL_MCP_AUTHORIZATION_SERVER_URL=https://api.example.com/auth
export AUTHRAIL_MCP_SERVICE_TOKEN_FILE=/run/secrets/authrail-mcp-service-token
./bin/authrail-mcp
```

Replace the example URLs with your deployed API, public MCP address and OAuth
issuer. The backend’s `MCP_RESOURCE_URL` must match `AUTHRAIL_MCP_RESOURCE_URL`.

The private file contains the service credential provisioned by the AuthRail API
operator. Its value must match the backend's MCP service credential. Restrict the
file to its owner; keep it out of source control. This credential authenticates
the service and does not grant access to any user's workspace.

Configuration loads through koanf: defaults, environment values, then explicit
command-line flags. The process does not load `.env` files.

| Variable | Purpose |
| --- | --- |
| `AUTHRAIL_MCP_API_URL` | Required HTTPS AuthRail API origin, without a path. |
| `AUTHRAIL_MCP_RESOURCE_URL` | Required public HTTPS MCP URL ending in `/mcp`. |
| `AUTHRAIL_MCP_AUTHORIZATION_SERVER_URL` | OAuth issuer; defaults to the API origin followed by `/auth`. |
| `AUTHRAIL_MCP_LISTEN` | HTTP listener; defaults to `127.0.0.1:3016`. Set the container listener explicitly when needed. |
| `AUTHRAIL_MCP_SERVICE_TOKEN_FILE` | Private mounted service credential file. |
| `AUTHRAIL_MCP_SERVICE_TOKEN` | Alternative secret injected by deployment tooling; never set both credential sources. |
| `AUTHRAIL_MCP_ALLOW_LOOPBACK` | Set `true` only for HTTP loopback development. Defaults to `false`. |

Put the listener behind HTTPS ingress. Streamable HTTP is served at `/mcp`.
`-listen`, `-api`, `-resource`, `-authorization-server` and `-service-token-file`
override their environment settings. `-local` enables HTTP loopback development.
Run `go test -race ./...` for the transport and isolation checks.

See [API contracts](API.md) and [architecture](ARCHITECTURE.md) for the connection
and authority boundaries.

## AuthRail Runtime

MCP reads AuthRail workspace information. Protecting an application's business
tools uses an AuthRail SDK and customer-side sidecar. Provider credentials stay
in that protected application. Connecting an assistant to MCP does not protect
its other tools automatically.

- [AuthRail documentation](https://docs.authrail.io)
- [Sidecar](https://github.com/AuthRail/authrail-sidecar)
- [Node SDK](https://github.com/AuthRail/authrail-sdk-node)
- [Go SDK](https://github.com/AuthRail/authrail-sdk-go)
- [Python SDK](https://github.com/AuthRail/authrail-sdk-python)

Licensed under [Apache 2.0](LICENSE).

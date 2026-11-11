# AuthRail MCP

AuthRail MCP lets you work with your AuthRail organisation through an assistant.
You can ask it to inspect recorded actions, prepare a Rail for a connected
tool or investigate the proof behind a Framework assessment. It signs in with your account and
works within the organisation access you grant it.

For example, you could ask it to prepare a rule that sends sensitive tool
requests for human review, then inspect the resulting Actions once your
application is connected. Preparing the rule and publishing it are separate
decisions: an authorised person still reviews the change before it takes effect.
For governance work, the assistant can also follow a duty through its Controls,
Tests and Evidence to help explain what supports the assessment.

[Connection guide](https://docs.authrail.io/docs/mcp) ·
[First protected function](https://docs.authrail.io/docs/getting-started) ·
[Compatibility](https://docs.authrail.io/docs/compatibility)

## Connect your assistant

Add `https://mcp.authrail.io/mcp` as a remote Streamable HTTP connection in
your assistant. Start its OAuth connection flow, sign in to AuthRail, choose
an organisation and review the access it requests. The backend supports client
identity documents (CIMD) and automatic dynamic registration, with PKCE S256.
Registration identifies the assistant; sign-in and consent grant access.

If your client asks for a pre-registered ID, open `/mcp/clients` in AuthRail,
register its exact callback address and copy the public client ID into the
assistant. HTTPS and HTTP loopback callbacks, including `localhost`, are
supported without wildcards. The ID alone provides no organisation access.

Live availability of the public address has not yet been verified for this
source update. Running this repository locally does not deploy that service.

You can inspect or revoke that access later at `/mcp/connections`. Your role and
content permissions continue to apply on every request. Revoking the connection,
signing out or losing membership stops its access. The Go service calls AuthRail
APIs on your behalf and has neither direct database access nor a shared
administrator account.

## Prepare a Rail together

Grant **Rail draft preparation** during consent using an account that can manage
Rails. You can then ask, for example:

> Prepare a Guard Rail for our code-change action that blocks changes weakening
> authentication. Enable AuthRail Contextual AI and show me the question and the
> fields that would be sent to AuthRail before saving the draft.

The assistant finds the connected action and its permitted fields, proposes the
assessment, then saves and tests a draft. The following tools support that flow:

| Tool | Purpose |
| --- | --- |
| `authrail_list_connected_actions` | Find the action you want to protect. |
| `authrail_get_connected_action` | Read its reviewed version and permitted fields. |
| `authrail_propose_contextual_rail` | Propose the assessment and rule before saving. |
| `authrail_create_guard_rail_draft` / `authrail_create_policy_rail_draft` | Create an unpublished draft. |
| `authrail_get_rail_draft` | Open the draft and its current revision. |
| `authrail_save_guard_rail_draft` / `authrail_save_policy_rail_draft` | Save changes and examples against that revision. |
| `authrail_test_guard_rail_draft` / `authrail_test_policy_rail_draft` | Run the examples before publication review. |

Contextual AI is operated by AuthRail and enabled on the Rail that needs
interpretation. You review the question, permitted material and the answer the
rule will use. At runtime, your application's SDK and sidecar send that material
to AuthRail, which returns a signed finding for the local sidecar to evaluate.
You do not configure a model key or extractor for this feature, and uncertainty
cannot be treated as permission.

Before publication, a person reviews the tested version and its disclosure.
Draft tools cannot publish the Rail, approve a business request or execute it.
The same organisation permissions and version checks apply whether the work
starts in the workspace or through an assistant.

## Prepare data sources, deployments and approval decisions

The `runtime.drafts` permission also provides
`authrail_list_guard_protections`, `authrail_get_data_source_options` and tools
to create, read, save and test source and Data Rail drafts. Start by describing
the business value the rule needs, which account supplies it and how fresh it
must be. Check the installed SDK's capabilities before choosing a reader or
protection; a saved source, imported schema or passing example does not connect
the application to live data. Credentials stay with the protected application.

With `runtime.deployments`, an assistant can inspect deployments and prepare an
Observe installation. For activation, renewal, rollback or revocation, it
prepares a review link for the person to confirm. That proposal expires and
identifies the version and sequence being considered, so a later change needs
another review. Enforce confirmation also asks the person to review the route
to the tool and its credentials.

With `runtime.approvals`, the assistant can list your assigned requests and
prepare an approve, reject or cancel decision for your authenticated review.
It cannot vote or resume the application's callback. The `runtime.read` tools
let it inspect Action, certificate and receipt metadata within your visibility
permissions.

## Protect a business tool as well as managing AuthRail

Connecting management MCP lets an assistant work with AuthRail configuration
and records. The function that reads or changes a business system needs its own
SDK and customer-side sidecar integration. If the assistant can reach the same
provider through another tool or credential, that route can bypass protection.

The `authrail-protected-mcp` source command provides a separate customer-side
path for existing MCP tools. It uses the Go SDK and local sidecar to check the
selected tools, hold work for approval, save continuations and check results
before returning them. It is a source integration rather than a released generic
installer. Follow the [protected-tool contract](API.md#customer-side-protected-mcp-contract)
and review each tool's execution and recovery behaviour before relying on it.

Remove equivalent direct tool registrations and credential routes before
claiming enforcement. If a write's outcome is uncertain, establish what happened
to the original request at the provider; submitting another write with a new key
cannot resolve that uncertainty.

## Investigate a Framework or Control

Try asking, “Which duties in this Framework need attention, and what is blocking
them?” The assistant can list your organisation's Frameworks, open an assessment
and follow a duty's Mappings to the Control versions that address it. It uses
the identifiers and pagination returned by AuthRail; you do not supply an
organisation ID with each request.

The Control's Tests explain what is checked and how long results remain
current. Evidence requirements describe the supporting material, while Evidence
Records and decisions show what was submitted and accepted. A requirement alone
is not proof, and a past passing Test may have become stale. If part of the
assessment is unavailable or restricted, the assistant must explain that limit
rather than treat its results as complete coverage.

| Tool | What you can inspect |
| --- | --- |
| `authrail_get_connection` | Your connected account, workspace, client and granted access. |
| `authrail_get_workspace_summary` | Account and workspace names and available MCP capabilities. |
| `authrail_get_sdk_integrations` | Released SDK installation paths and integration guidance. |
| `authrail_list_organisation_frameworks` | Your selected Frameworks and their versions. |
| `authrail_get_organisation_framework` | A selected Framework's version, status and revision. |
| `authrail_get_framework_readiness` | The saved assessment and applicable Obligations you can access. |
| `authrail_list_control_mappings` | The Control relationships for an Obligation's applicability decision. |
| `authrail_get_control` | Metadata for the Control version named by a Mapping. |
| `authrail_list_tests` | Test definitions, latest reported outcomes and freshness. |
| `authrail_list_evidence_requirements` | The proof a Control or Implementation requires. |
| `authrail_list_control_policy_bases` | Policy versions referenced by a Control. |
| `authrail_get_policy_version` | A permitted Policy version's lifecycle and review metadata. |
| `authrail_list_test_runs` | Recorded outcomes and periods for a Test version. |
| `authrail_list_evidence_records` | Evidence submissions, periods and review states. |
| `authrail_list_evidence_record_decisions` | A submission's decisions, reviewers and timestamps. |
| `authrail_list_connections` | Provider Connection names and saved collection health. |
| `authrail_get_connection_status` | A Connection's saved status and last successful collection time. |

Identity tools need `workspace.read` and accept an empty arguments object.
Choose `governance.read` at consent to use the Framework and proof tools, and
`connections.read` to inspect collection status. An existing connection does
not gain these permissions automatically when new tools become available.

History requests use the `id` returned by the definition or requirement list.
Their results contain metadata rather than document text, attestations or file
links. Connection tools likewise report saved status without collecting data,
connecting a provider or revealing credentials. A healthy Connection means
collection is working; it does not establish a passing Test or ready Framework.

## Run the management service

The following setup is for the installation operator. It requires Go 1.26.5 or
newer and a service credential provisioned by the AuthRail API operator:

```sh
go build -o bin/authrail-mcp ./cmd/authrail-mcp
export AUTHRAIL_MCP_API_URL=https://api.example.com
export AUTHRAIL_MCP_RESOURCE_URL=https://mcp.example.com/mcp
export AUTHRAIL_MCP_AUTHORIZATION_SERVER_URL=https://api.example.com/auth
export AUTHRAIL_MCP_SERVICE_TOKEN_FILE=/run/secrets/authrail-mcp-service-token
./bin/authrail-mcp
```

Replace the example addresses with your deployed API, public MCP endpoint and
OAuth issuer. Set the backend's `MCP_RESOURCE_URL` to the same value as
`AUTHRAIL_MCP_RESOURCE_URL`. The private token file must contain the matching
backend MCP service credential and be readable only by its owner. That token
authenticates this service; users still need their own sign-in and consent for
workspace access.

Configuration loads through koanf in the order defaults, environment, then
command-line flags. The process does not load `.env` files.

| Variable | Purpose |
| --- | --- |
| `AUTHRAIL_MCP_API_URL` | Required HTTPS API origin without a path. |
| `AUTHRAIL_MCP_RESOURCE_URL` | Required public HTTPS MCP URL ending in `/mcp`. |
| `AUTHRAIL_MCP_AUTHORIZATION_SERVER_URL` | OAuth issuer; defaults to the API origin followed by `/auth`. |
| `AUTHRAIL_MCP_LISTEN` | Listener address; defaults to `127.0.0.1:3016`. Set it explicitly for a container. |
| `AUTHRAIL_MCP_SERVICE_TOKEN_FILE` | Private mounted service credential file. |
| `AUTHRAIL_MCP_SERVICE_TOKEN` | Alternative secret injected by deployment tooling; choose one credential source. |
| `AUTHRAIL_MCP_ALLOW_LOOPBACK` | Allows HTTP loopback development when `true`; defaults to `false`. |

Put the listener behind HTTPS ingress; it serves Streamable HTTP at `/mcp`.
The flags `-listen`, `-api`, `-resource`, `-authorization-server` and
`-service-token-file` override their environment values. Use `-local` for HTTP
loopback development and `go test -race ./...` for the transport and isolation
checks when changing the service.

For implementation details, see [API contracts](API.md) and
[architecture](ARCHITECTURE.md). To connect an application, continue with the
[sidecar](https://github.com/AuthRail/authrail-sidecar) and the
[Node](https://github.com/AuthRail/authrail-sdk-node),
[Go](https://github.com/AuthRail/authrail-sdk-go) or
[Python](https://github.com/AuthRail/authrail-sdk-python) SDK.

Licensed under [Apache 2.0](LICENSE).

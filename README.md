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

Both tools require `workspace.read`. They accept an empty arguments object.
Access is rechecked for each request. Revoking the connection, signing out or
losing workspace membership stops access.

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

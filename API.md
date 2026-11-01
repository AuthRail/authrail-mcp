# AuthRail MCP API

The Go service exposes Streamable HTTP at `/mcp`. The official MCP SDK handles
protocol negotiation, tool discovery and typed calls. Requests require an OAuth
bearer token for this resource with `workspace.read` access.

## Tools

Both tools accept `{}`; extra arguments are rejected by their input schema.

| Tool | Structured result |
| --- | --- |
| `authrail_get_connection` | `clientId`, `organisation: {id, name}`, `user: {id, name}`, `scopes` |
| `authrail_get_workspace_summary` | `organisation: {id, name}`, `user: {id, name}`, `permissions` |

These tools read identity and granted access. They do not change governance
records or execute business operations. Results exclude session IDs, tokens and
provider credentials. Workspace permissions in this projection describe MCP
capabilities; they are not a grant of every organisation role permission.

## Authentication

`GET /.well-known/oauth-protected-resource/mcp` identifies the protected resource,
AuthRail authorization server and supported scope. Missing or invalid credentials
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
| `GET /organisation/mcp/workspace` | API delegation and exact `X-Organisation-Id` | Read the delegated workspace projection after checking authority again. |

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

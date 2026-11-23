# Protected connector recovery

The private journal retains original requests, approval continuations and execution
identities. Keep it, its encryption key, the original connector configuration and
sidecar state through restarts. Reusing a request key reads its saved status; it
never repeats an effect or returns a previously withheld result.

Stop the connector before using its operator recovery command. The command takes
the same exclusive journal lock as the serving process and makes no provider or
Runtime requests:

```sh
authrail-protected-mcp --config /private/connector.json \
  --recover-request original-request-key --tool exact-configured-tool
```

Output contains private operator diagnostics, including the original Action and
attempt identity when retained. It excludes arguments, business results, credentials
and approval continuation contents. Keep this output out of ordinary agent inputs.

New local requests save a versioned checkpoint before SDK execution start, advance
it durably before requesting start, and save a dispatch marker before calling the
downstream tool. The connector requires a checkpoint-capable SDK. After interruption,
only a checkpoint proving no SDK start may be closed locally:

```sh
authrail-protected-mcp --config /private/connector.json \
  --recover-request original-request-key --tool exact-configured-tool \
  --close-interrupted
```

This records `interrupted_before_start`. It does not assert a provider outcome,
reconcile an Action, grant new permission or restart the saved request. Any separately
authorised new work remains the application's responsibility.

A legacy record, possible execution start or dispatch cannot be closed this way.
Its outcome remains unknown; preserve the original request key and inspect the
original provider identity. A missing local dispatch marker is not provider proof.
Trusted embedding application code can use `Connector.Reconcile` with a real
provider lookup and the retained recovery identity. Agent arguments and operator
file assertions are not reconciliation evidence. No generic provider lookup or
business retry command is supplied.

The operator command supports the local encrypted journal only. API-backed shared
requests retain their existing leases, fences and application-owned reconciliation;
local checkpoint fields and this offline command do not change that protocol.

# Remote agents

`flame.a2a` is an optional installed connector. It discovers configured Agent
Cards and calls A2A JSON-RPC or HTTP+JSON endpoints. Runtime owns installation,
Tool approval, invocation identity, cancellation and recovery. Internal
`delegate_task` remains Runtime-owned.

Configure the private `agents` installation input with a JSON array:

```json
[
  {
    "name": "research",
    "cardURL": "https://agents.example/research/.well-known/agent-card.json",
    "allowedRPCOrigins": ["https://rpc.example"],
    "description": "Research assistant"
  }
]
```

An Agent Card may redirect only within its configured origin. RPC endpoints must
use that origin or an explicitly configured additional origin; RPC redirects
are refused. URL userinfo is rejected. Query-bearing URLs are private installation
configuration and are never included in connection error messages. The connector
does not negotiate remote authentication; authenticated deployments must supply
an admitted gateway endpoint. No parent process credentials are inherited.

`a2a_agents`, `a2a_send`, `a2a_status` and `a2a_cancel` share the same owners as
the `remote-agents` contribution. Send and cancel use durable command receipts.
The send receipt retains the original remote response, and any task binding is
committed atomically with it. Status and cancel accept the original `requestId`;
they use its saved endpoint, task ID and context ID, even after restart or an
Agent Card change. A changed installation agent authority refuses that binding.
Remote task status remains a remote fact; the connector does not advance a
second local task state machine.

A lost response or local cancellation leaves an unconfirmed command. Reusing
that command identity never sends again. Local cancellation does not assert
remote task cancellation. Only a confirmed remote reply supplies that outcome.

Rebuild all consumers and install this package to replace the removed Runtime
`a2a` configuration and `FLAME_A2A_AGENTS` entrypoint. There is no configuration
adapter or old `a2a` Tool source variant. Approval rules now bind the installed
MCP source. This changes the persisted schema; Runtime refuses incompatible data. Start
with a fresh product data directory and preserve the old directory separately.

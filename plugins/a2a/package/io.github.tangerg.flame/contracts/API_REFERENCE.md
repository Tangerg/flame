# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.a2a/agents.v1` | query | installation | List configured remote agents without contacting them. |
| `flame.a2a/cancel.v1` | command | installation | Request cancellation of the remote task bound to the original requestId. Only the returned remote state confirms cancellation; local interruption or transport failure does not. An unconfirmed cancel must never be resubmitted. |
| `flame.a2a/send.v1` | command | installation | Send one message up to 64 KiB to a configured remote agent. Return the remote task or message and the original requestId. A response is bounded to 2 MiB. Preserve the command identity: an unconfirmed send must never be resubmitted. |
| `flame.a2a/status.v1` | query | installation | Read the remote task bound to the original requestId, using its captured endpoint and task identity. This does not replay a send or advance local task state. |

Capability `flame.a2a.v1` uses `collection` composition in `installation` scope. Required backend protocol: `io.github.tangerg.qimu/portable/1`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.

# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.computer/close.v1` | command | installation | Human-only: revoke the exact control session, cancel pending permissions and execution, and join its original engine. Already dispatched external effects cannot be undone. |
| `flame.computer/context.v1` | query | installation | Read the current worker identity and original runtime instructions and js/js_reset schemas before execution. This never opens a session or grants application access. |
| `flame.computer/execute.v1` | command | installation | Execute js or js_reset on the exact accepted worker and surface. Arguments follow the captured upstream schema. An observed result is durable; transport loss never authorizes replay. Reset discards the original JavaScript heap. |
| `flame.computer/history.v1` | query | installation | Read original operation headers in admission order, newest first. This is a projection of durable receipts, not an execution log with a second owner. |
| `flame.computer/open.v1` | command | installation | Human-only: open one exclusive computer-control session on the Runtime host for the stated lease. Existing sessions are not replaced; expiry cancels and closes the engine. |
| `flame.computer/read.v1` | query | installation | Read the actual Runtime host, current computer-control session and original receipt capacity including reserved outcome bytes. This does not start an engine or authorize computer access. |
| `flame.computer/reply.v1` | command | installation | Human-only: answer the exact live upstream permission request. The original runtime owns allowed scopes; cancelled, settled or replaced requests cannot be answered. |
| `flame.computer/result.v1` | query | installation | Read original input, identity and the one observed outcome. An absent outcome means unconfirmed; this method never executes or infers desktop effects. |
| `flame.computer/stop.v1` | command | installation | Human-only: interrupt the exact current operation and retire its engine before any later operation can run. A missing original result remains unknown. |
| `flame.computer/watch.v1` | query | installation | Observe bounded complete snapshots of the computer-control owner. Disconnecting this observation does not close the session. Stream chunks: schema.json#/$defs/WatchV1Chunk; final result: schema.json#/$defs/WatchV1Output. |

Capability `flame.computer.v1` exposes the declared methods. Required backend protocol: `io.github.tangerg.frame/portable/1`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.

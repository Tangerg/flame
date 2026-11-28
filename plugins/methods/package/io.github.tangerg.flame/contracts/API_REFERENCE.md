# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.methods/approve.v1` | command | installation | Approve the original method report and retain the human decision. |
| `flame.methods/board.v1` | query | installation | Read the current project method and recent executions. |
| `flame.methods/execution.v1` | query | installation | Read the original method execution and captured evidence. |
| `flame.methods/save.v1` | command | installation | Save an immutable project method version. |
| `flame.methods/start.v1` | command | installation | Admit one execution of a captured method on a managed worktree. |

Capability `flame.methods.v1` exposes the declared methods. Required backend protocol: `io.github.tangerg.frame/portable/1`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.

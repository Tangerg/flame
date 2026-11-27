# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.opencode/cancel.v1` | command | installation | Request cancellation of the original prompt and await its stop reason. |
| `flame.opencode/fork.v1` | command | installation | Fork the native session when OpenCode advertises the capability. |
| `flame.opencode/get.v1` | query | installation | Read the captured session, progress, permissions and turn settlement. |
| `flame.opencode/list.v1` | query | installation | List native sessions in the canonical workspace. |
| `flame.opencode/open.v1` | command | installation | Load the original OpenCode session and its history. |
| `flame.opencode/permission.v1` | command | installation | Select an option from the original pending permission. |
| `flame.opencode/prompt.v1` | command | installation | Accept one prompt for the bound native session. |

Capability `flame.opencode.v1` exposes the declared methods. Required backend protocol: `io.github.tangerg.frame/portable/1`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.

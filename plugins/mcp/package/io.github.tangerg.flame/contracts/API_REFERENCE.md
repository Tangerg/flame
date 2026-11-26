# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.external-mcp/attempt.v2` | query | installation | Read a retained external authorization attempt. |
| `flame.external-mcp/authorize.v2` | command | installation | Begin an authorization attempt against the current enabled entry. |
| `flame.external-mcp/change.v2` | command | installation | Compare and change one external directory entry. |
| `flame.external-mcp/list.v2` | query | installation | List external servers and their opaque host launch grants. |
| `flame.external-mcp/recover.v2` | command | installation | Mark interrupted attempts from earlier Runtime connection epochs as unconfirmed. |
| `flame.external-mcp/retire-authorization-start.v2` | command | installation | Return the original authorization start decision or retire the command before it can accept an attempt. |
| `flame.external-mcp/settle.v2` | command | installation | Record the original authorization attempt's terminal result. |

Capability `flame.external-mcp.v2` exposes the declared methods. Required backend protocol: `io.github.tangerg.frame/portable/1`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.

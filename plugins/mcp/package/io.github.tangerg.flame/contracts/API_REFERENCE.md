# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.external-mcp/attempt.v1` | query | installation | Read a retained external authorization attempt. |
| `flame.external-mcp/authorize.v1` | command | installation | Begin an authorization attempt against the current enabled entry. |
| `flame.external-mcp/change.v1` | command | installation | Compare and change one external directory entry. |
| `flame.external-mcp/list.v1` | query | installation | List external servers and their opaque host launch grants. |
| `flame.external-mcp/recover.v1` | command | installation | Cancel uncompleted attempts admitted by a previous Runtime incarnation. |
| `flame.external-mcp/settle.v1` | command | installation | Record the original authorization attempt's terminal result. |

Capability `flame.external-mcp.v1` uses `selection` composition in `installation` scope. Required backend protocol: `io.github.tangerg.qimu/portable/1`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.

# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.code-intelligence/format.v1` | query | installation | Compute a transformation of the supplied exact source version without writing workspace files. Absent content means unsupported formatting. |
| `flame.code-intelligence/inspect.v1` | query | installation | Analyze the supplied source version and render newly introduced problems against the captured baseline. Unavailable or stale diagnostics are explicit failures. |

Capability `flame.code-intelligence.v1` uses `selection` composition in `installation` scope. Required backend protocol: `io.github.tangerg.qimu/portable/1`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.

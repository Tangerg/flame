# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.usage-reports/capture.v1` | command | installation | Capture a daily or weekly core accounting report with its actual observation time and lower bound. Totals remain core-owned; saved reports are immutable. |
| `flame.usage-reports/delete.v1` | command | installation | Remove one immutable saved report without changing core accounting evidence or totals. |
| `flame.usage-reports/get.v1` | query | installation | Read one saved core report verbatim, preserving missing usage, unknown prices and recorded zeros. |
| `flame.usage-reports/list.v1` | query | installation | List saved accounting report headers in capture order, newest first. |

Capability `flame.usage-reports.v1` uses `collection` composition in `installation` scope. Required backend protocol: `io.github.tangerg.qimu/portable/1`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.

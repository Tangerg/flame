# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.filewatch/configure.v1` | command | installation | Replace or rearm the trigger at its current revision; omitting trigger disables observation. Configuration acceptance does not claim a live watch. Captured work remains immutable. |
| `flame.filewatch/event.v1` | query | installation | Read one retained observation and its complete original dispatch capture. |
| `flame.filewatch/events.v1` | query | installation | Read at most 256 retained observation headers, newest first. Accepted means core confirmed the captured Run opening, not that execution completed. Pending intents are never pruned. |
| `flame.filewatch/get.v1` | query | installation | Read this installation's single persistent file trigger and configuration revision. |
| `flame.filewatch/status.v1` | query | installation | Inspect the actual background observation revision and failures. A failed watch requires explicit configuration rearm; accepted dispatch intents continue reconciliation. |

Capability `flame.filewatch.v1` uses `collection` composition in `installation` scope. Required backend protocol: `io.github.tangerg.flame/backend/2026-10-15.3`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.

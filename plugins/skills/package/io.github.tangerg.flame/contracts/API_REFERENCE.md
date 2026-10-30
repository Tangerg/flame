# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.skill-learning/archive.v1` | command | installation | Return an optional inactivity threshold when an archive check is due at the caller's time. The caller alone admits and commits archival. |
| `flame.skill-learning/mine.v1` | command | installation | Apply learning cadence to a completed Run and return optional proposal advice using call-scoped history, current skill and model callbacks. Never publish or store active skills. |

Capability `flame.skill-learning.v1` uses `selection` composition in `installation` scope. Required backend protocol: `io.github.tangerg.qimu/portable/1`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.

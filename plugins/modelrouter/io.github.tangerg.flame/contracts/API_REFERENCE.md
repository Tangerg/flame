# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.model-router/get-policy.v1` | query | installation | Inspect the private routing policy and revision before editing. |
| `flame.model-router/replace-policy.v1` | command | installation | Replace the routing policy at the inspected revision. Each use has at most one exact target and an optional caller-envelope ceiling. |

Capability `flame.model-router.v1` uses `collection` composition in `installation` scope. Required backend protocol: `io.github.tangerg.flame/backend/2026-10-15.3`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.

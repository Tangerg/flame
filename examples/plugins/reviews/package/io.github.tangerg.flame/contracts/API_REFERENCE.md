# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `io.github.tangerg.flame.examples/reviews.analyzers` | query | installation | Analyze the current review through its installed analyzer collection and return admitted findings. |
| `io.github.tangerg.flame.examples/reviews.list` | query | installation | Read the review backend's current review and revision. |
| `io.github.tangerg.flame.examples/reviews.update` | command | installation | Change one review's status using its inspected revision. |

Capability `io.github.tangerg.flame.examples/reviews` uses `collection` composition in `installation` scope. Required backend protocol: `io.github.tangerg.flame/backend/2026-10-15.3`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.

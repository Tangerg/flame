# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.memory/add.v1` | command | installation | Add a human-authored memory item in the requested scope. |
| `flame.memory/delete.v1` | command | installation | Delete one memory item through its private owner. |
| `flame.memory/items.v1` | query | installation | Read active memory items that can inform a Run. |
| `flame.memory/list.v1` | query | installation | List memory items visible in the requested scope. |
| `flame.memory/maintain.v1` | command | installation | Extract and curate captured conversation under the private ledger and watermark. |
| `flame.memory/review.v1` | command | installation | Apply a human review decision to a pending memory item. |
| `flame.memory/search.v1` | query | installation | Rank active memories for the supplied project and query. |
| `flame.memory/update.v1` | command | installation | Edit the current content or pin state of a memory item. |

Capability `memory` uses `selection` composition in `installation` scope. Required backend protocol: `io.github.tangerg.flame/backend/2026-10-15.3`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.

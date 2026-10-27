# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.guardrails/get-policy.v1` | query | installation | Inspect the private policy revision and protected phrase count without returning protected values. |
| `flame.guardrails/replace-policy.v1` | command | installation | Replace exact, case-sensitive protected phrases at the inspected revision. Matching model-visible string fields refuse provider dispatch. |

Capability `flame.guardrails.v1` uses `collection` composition in `installation` scope. Required backend protocol: `io.github.tangerg.qimu/portable/1`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.

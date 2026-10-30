# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.hooks/discover.v1` | query | installation | Read ordered lifecycle rules. Exclude all project configuration when the Runtime has not admitted its trust. |
| `flame.hooks/execute.v1` | command | installation | Execute one trusted lifecycle command under its original receipt, bounded input, output and process lifetime. Return advice; never grant Runtime authorization. |

Capability `flame.hooks.v1` uses `selection` composition in `installation` scope. Required backend protocol: `io.github.tangerg.qimu/portable/1`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.

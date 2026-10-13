# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.schedules/create.v1` | command | installation | Create one schedule definition. |
| `flame.schedules/delete.v1` | command | installation | Delete one schedule definition. |
| `flame.schedules/dispatch.v1` | command | installation | Reconcile captured work through the original Runtime Run acceptance. |
| `flame.schedules/list.v1` | query | installation | List current schedule definitions and revisions. |
| `flame.schedules/prepareManual.v1` | command | installation | Capture manual dispatch input and stable Session and Run identities. |
| `flame.schedules/scan.v1` | command | installation | Capture due occurrences at the supplied time and return pending dispatch. |
| `flame.schedules/update.v1` | command | installation | Change one schedule using its inspected revision. |

Capability `schedules` uses `selection` composition in `installation` scope. Required backend protocol: `io.github.tangerg.flame/backend/2026-10-13.3`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.

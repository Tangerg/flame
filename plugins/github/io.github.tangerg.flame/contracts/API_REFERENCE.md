# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.github/get-issue.v1` | query | installation | Read one GitHub issue or pull request by its repository and issue number. |
| `flame.github/list-issues.v1` | query | installation | Read one explicit page of repository issues and pull requests, exposing the next page number supplied by GitHub. |
| `flame.github/read-file.v1` | query | installation | Read a UTF-8 repository file up to one MiB, optionally at an exact Git ref. Directory and binary content are refused. |

Capability `flame.github.v1` uses `collection` composition in `installation` scope. Required backend protocol: `io.github.tangerg.flame/backend/2026-10-15.3`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.

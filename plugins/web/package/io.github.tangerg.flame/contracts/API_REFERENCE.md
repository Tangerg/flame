# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.web/browse.v1` | query | installation | Read a bounded page of a previously captured snapshot by ID and Unicode codepoint offset. This reads captured material without network I/O; an evicted snapshot is refused. |
| `flame.web/fetch.v1` | query | installation | Fetch a URL through configured Jina Reader credentials and capture a private immutable snapshot. Return its first 8192 Unicode codepoints and exact continuation offset. |
| `flame.web/request.v1` | command | installation | Execute one HTTP request within the installed host allowlist. GET and HEAD are allowed; other methods are refused. Responses are bounded to 256 KiB. Preserve the command identity: an unconfirmed request must never be resubmitted. |
| `flame.web/search.v1` | query | installation | Search through configured Tavily credentials with a query up to 8192 bytes, honoring explicit domain and supported recency filters. Each call queries the provider; results are bounded to 128 KiB. |

Capability `flame.web.v1` uses `collection` composition in `installation` scope. Required backend protocol: `io.github.tangerg.qimu/portable/1`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.

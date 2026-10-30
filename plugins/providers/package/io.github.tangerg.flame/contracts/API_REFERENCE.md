# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.model-provider/call.v1` | command | installation | Execute one original provider exchange using the captured client and preserve its complete outcome. |
| `flame.model-provider/close.v1` | command | installation | Retire the captured client after its accepted calls have joined. |
| `flame.model-provider/count.v1` | command | installation | Count the complete request using the same captured provider instance, only when that capability was advertised. |
| `flame.model-provider/embed.v1` | command | installation | Execute one original embedding exchange and preserve the complete provider response. |
| `flame.model-provider/open-chat.v1` | command | installation | Capture one immutable provider client shared by chat, streaming and optional token counting. |
| `flame.model-provider/open-embedding.v1` | command | installation | Capture an immutable embedding client and its vector-space identity. |
| `flame.model-provider/profiles.v1` | query | installation | Read this installed adapter's provider policies and published model facts. |
| `flame.model-provider/remote-models.v1` | query | installation | Read model identities from the exact configured provider endpoint. |
| `flame.model-provider/reopen-chat.v1` | command | installation | Reconstruct a fresh client from the original confirmed open receipt, without reading current credentials or authorizing generation. |
| `flame.model-provider/stream.v1` | command | installation | Stream the original provider deltas and retain the final outcome without replaying an unconfirmed exchange. Stream chunks: schema.json#/$defs/Method_flame_2emodel_2dprovider_2fstream_2ev1Chunk; final result: schema.json#/$defs/Method_flame_2emodel_2dprovider_2fstream_2ev1Output. |

Capability `flame.model-provider.v1` uses `selection` composition in `installation` scope. Required backend protocol: `io.github.tangerg.qimu/portable/1`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.

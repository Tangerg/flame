# TypeScript Runtime client

`@flame/runtime-client` owns HTTP/SSE transport, typed methods, pagination,
mutation identity, uncertain-response reconciliation and persistent command
journals. Runtime's registry and protocol own operation shapes and semantics;
`make generate` emits the package's `src/generated` projection.

```ts
import { createFlameClient, createHttpTransport } from "@flame/runtime-client";
const client = createFlameClient(createHttpTransport({ baseUrl: "http://127.0.0.1:17171" }));
const discovery = await client.runtime.discover();
```

Consumers use explicit public entries: the main API, `wire`, `methods`,
`validation`, `testing`, and `testing/samples/*`. Internal implementation paths
and unit tests are not exports. `testing` supplies the deliberate memory-transport
fixture and sample index. Its raw sample JSON is an exact generated copy of
`runtime/contract/samples`; edit canonical samples at that Runtime owner.

Mutation handles retain the original key and receipt authority after uncertain
responses. Retrying continues that original command. Presentation components
must not rebuild settlement or invent a replacement identity. Streaming reads
and durable commands retain their registry-classified distinction.

Run `npm run check --workspace @flame/runtime-client` from the root after
`make setup`. Independent author checks consume a real packed package, rather
than workspace-private paths. No former package name or wildcard client export
is provided.

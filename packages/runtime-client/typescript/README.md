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

Transport receives JSON losslessly before method validation. Schema-declared
numeric DTO fields project into JavaScript numbers and reject values they cannot
represent exactly. Opaque JSON, including Tool schemas, retains exact numbers.
For evidence such as accounting archives, `client.queryJSON(method,
params, { signal })` returns canonical JSON with the original numeric lexemes,
wide integers, null and missing members retained. It uses the same request,
generated validation and cancellation path. It accepts only unary queries;
commands and streams keep their existing lifecycle APIs. Use `parseJSON`,
`stringifyJSON`, `bigint` and `ExactNumber` to inspect or return exact data.

Mutation handles retain the original key and receipt authority after uncertain
responses. Retrying continues that original command. Presentation components
must not rebuild settlement or invent a replacement identity. Streaming reads
and durable commands retain their registry-classified distinction.

The client installs a mutation journal by default, using the supplied capabilities
reader for its scope. Every mutation, including one with an explicit key, must
reserve its immutable input and Runtime store namespace before
dispatch. Missing scope or failed persistence refuses the command without opening
transport. That refused handle cannot later capture a different target through
`retry()`; start a new user action after the prerequisite recovers. A successfully
reserved command retains its original identity and receipt lookup even while
discovery is unavailable, and cannot dispatch into another store.

Run `npm run check --workspace @flame/runtime-client` from the root after
`make setup`. Independent author checks consume a real packed package, rather
than workspace-private paths. No former package name or wildcard client export
is provided.

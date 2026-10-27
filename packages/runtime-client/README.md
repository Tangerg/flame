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

Consumers use explicit public entries: the main API, `json`, `wire`, `methods`,
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
The `json` entry owns only that pure data mechanism. Its dependency closure
cannot enter Runtime calls, protocol records or validation; neutral application
input and presentation code may consume it without importing the client API.

Mutation handles retain the original key and receipt authority after uncertain
responses. Retrying continues that original command. Presentation components
must not rebuild settlement or invent a replacement identity. Streaming reads
and durable commands retain their registry-classified distinction.

`createMutationSettler().settle` accepts `onMutation(idempotencyKey)` to project
the identity of the selected original mutation, including retained or shared
recovery attempts. It grants no replacement command or independent retry owner;
the existing mutation handle and journal retain settlement authority.

Each `plugins.pendingCommands()` handle exposes `matchesIntent(intent)`, a
read-only comparison against the original journal's salted intent fingerprint.
Use the same authored intent when associating pending work with a document or
action; equal bindings alone cannot identify that input. Direct calls without a
stable intent do not match. The predicate neither claims nor executes a command,
and it does not validate changed input. Original input, contract and route remain
validated by command capture and receipt recovery. No raw intent or authored
input is added to the persistent journal.

Each client owns one mutation journal and retires it when `close()` begins.
Consumers may supply `mutationStorage` to retain unresolved commands across
client replacement. The `capabilities` reader supplies the sole namespace and
retention authority; there is no separate scope, clock, or journal lifecycle
option. Recovery handles and `createMutationSettler` retain their explicit
operation lifetimes. Every mutation, including one with an explicit key, must
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

# Flame backend capabilities for TypeScript

`@flame/backend-sdk` owns the Flame managed MCP authoring protocol and its
application capabilities. `@flame/backend-sdk/mcp` supplies the backend author,
Flame composition and scope metadata, callbacks, streaming and command receipts.
Portable method admission and schema validation use the released `@frame/sdk/node`
Capability; exact JSON uses `@frame/sdk/json`. The Runtime executes managed
backends through Frame process, Host, runner and lifetime ownership.

```ts
import { Backend, Capability, parseJSON } from "@flame/backend-sdk/mcp";
import { runtime } from "@flame/backend-sdk";

const backend = new Backend({ name: "example.notes", version: "1" });
const contract = new Capability(parseJSON(publishedDescriptor));
backend.implement(
  contract.portable.method("notes.read"),
  decodeInput,
  async (context, input) => {
    const client = await runtime(context);
    return readNotes(client, input);
  },
);
backend.bindCapability("notes", contract);
export default backend;
```

`runtime(context)` creates the public Flame client from the current call's
captured callback and invocation. For setup, call `runtime(context, invocation)`
with an explicit original invocation. The helper keeps the fixed loopback RPC
endpoint, refuses redirects and proxy substitution, retains Unicode invocation
identity, and uses the captured response budget. Cleanup is reserved on the
original call Lifetime before discovery starts. It does not create another
resource owner.

`@flame/backend-sdk/sqlite` supplies `Database` for synchronous private SQLite
transactions. Admit the plugin's schema first, then transfer the original idle
`node:sqlite` connection to this owner and register `close` with the setup Lifetime.
Use its statements immediately; do not retain statements or use the original
connection after transfer. Business rules and receipt formats stay with the plugin.
External I/O must finish outside `transaction`; its callback cannot return a promise.
A failed mutation rolls back before another operation is admitted. If rollback
cannot be confirmed, the owner fences all new statements and transactions before
closing the original connection. Failed close remains retryable; an uncertain
connection never supplies a receipt or a read projection. Reopening and reading
the original durable evidence establishes whether a lost commit succeeded.

Use `toolCommandId(context)` to derive the Flame Tool's private command identity.
`toolReceipts(backend, handler)` exposes the Flame Tool receipt contract through
the same Flame backend. Return `{}` for an absent receipt. A recorded receipt carries
`{result: {output, isError?}}` with the original structured ToolOutput.
The backend validates this result against the published schema before replying. The helper does not cache results or repeat business execution.

Build with the compiler and budgets provided by the Flame authoring project:

```js
import { rolldown } from "rolldown";
import { buildBackend } from "@flame/backend-sdk/build";
import protocol from "@flame/backend-sdk/protocol" with { type: "json" };

await buildBackend({
  input: absoluteSource,
  directory: absoluteOutput,
  compiler: rolldown,
  target: protocol.constants.nodeTarget,
  maximumFrameBytes: protocol.constants.maximumFrameBytes,
});
```

The consumer protocol projection contains Flame declaration admission, Node
target, Tool command prefix, Tool receipt, and the managed MCP frame budget.
The MCP wire constants and cross-language identity fixtures are generated from
`packages/go-sdk/plugin/mcp` into `src/mcp/protocol.generated.json`. The separate
`src/protocol.generated.json` projects Flame runtime callbacks and Tool receipts.
Use `declareMethod(descriptor)` when declaring a standalone method: it returns
the actual Frame method after portable admission. `Capability.portable` holds
the generic Frame contract; `Capability.identity` also binds the Flame
composition and scope. Method descriptions do not change either callable identity.
Frame admission methods return the captured value they validated. Backends pass
that value to decoders, handlers and encoders; reading the caller's source again
can change both the validated input and the recorded outcome. MCP tool results
retain their `_meta` extension payloads and exact numeric evidence after SDK validation;
known tool errors do not have to satisfy a successful result's output schema.

In the Flame declaration, a managed Node backend names the packaged entry:

```json
{
  "managedBackends": [
    { "id": "notes", "runtime": "node", "entry": "backend/start.mjs" }
  ]
}
```

The package builder maps the entry to author source and bundles the complete
module graph. Installation does not run npm or a compiler. The Runtime selects
its managed Node executable and leases the release. Portable packages need no
OS suffix. See the [Frame SDK](https://github.com/Tangerg/frame/tree/v0.2.0/sdk)
for portable admission and lifetime rules and [Usage Reports](../../plugins/usagereports/README.md)
for a Flame consumer with durable business receipts.

The authoring tool generates Flame capability projections with
`flame-sdk feature --feature descriptor.json --out package/contracts --ts component`.
Generated schemas and method types come from Frame portable contract generation;
Flame adds only its composition, scope and capability identity projection.

# Flame backend capabilities for TypeScript

`@flame/backend-sdk` contains Flame application capabilities used by a Qimu Node
plugin. The generic backend, portable contracts, exact JSON, schema validation,
MCP transport, setup, streaming, and build implementation live in `@qimu/node`.
There are no compatibility re-exports or duplicate implementations here.

```ts
import { Backend, Capability, parseJSON } from "@qimu/node";
import { runtime } from "@flame/backend-sdk";

const backend = new Backend({ name: "example.notes", version: "1" });
const contract = new Capability(parseJSON(publishedDescriptor));
backend.implement(
  contract.method("notes.read"),
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
original Qimu Lifetime before discovery starts. It does not create another
resource owner.

Use `toolCommandId(context)` to derive the Flame Tool's private command identity.
`toolReceipts(backend, handler)` exposes the Flame Tool receipt contract through
the original Qimu backend. Its output is the original ToolOutput JSON, which
Flame validates. The helper does not cache results or repeat business execution.

Build with the compiler and budgets provided by the Flame authoring project:

```js
import { rolldown } from "rolldown";
import { buildBackend } from "@qimu/node/build";
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
Generic protocol constants and schemas have one owner in `qimu/mcp`.

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
OS suffix. See [Qimu Node](../../qimu/node/README.md) for generic contract and
lifecycle rules and [Usage Reports](../../plugins/usagereports/README.md) for a
Flame consumer with durable business receipts.

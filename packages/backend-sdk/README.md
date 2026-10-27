# TypeScript backend SDK

`@flame/backend-sdk` binds TypeScript business handlers to the same published
capability descriptors and MCP backend protocol as the Go SDK. Import an existing
descriptor as exact JSON with `parseJSON`; an ordinary JSON module import cannot
preserve wide numeric schema bounds.

`Capability` and `Method` retain the published identity. `parseJSON`,
`stringifyJSON`, `ExactNumber` and `JSONNumber` are the public Runtime client's
codec, shared by both languages' consumers. Local codecs may impose stricter
structural admission, but must remain deterministic and free of I/O.

The schema adapter uses the maintained `jsonschema` engine with instance-local
exact numeric validation. Unsupported schema keywords and external references
are construction errors. It never coerces, strips or defaults received values.
The installed Runtime retains its complete schema validation at admission.

Create one `Backend` and export it as the author module's default. The build
bootstrap redirects ordinary console logging to stderr before importing this
module; stdout belongs to MCP. The maintained MCP SDK owns protocol requests,
cancellation and sessions. Flame's custom transport decodes exact JSON before
that SDK receives feature data and never parses around malformed frames.

```ts
import { Backend, Capability, parseJSON } from "@flame/backend-sdk";
import type { CapabilityDescriptor } from "@flame/backend-sdk";

const contract = new Capability(
  parseJSON(publishedDescriptor) as CapabilityDescriptor,
);
const backend = new Backend({ name: "example.notes", version: "1" });
backend.implement(
  contract.method("notes.read"),
  decodeInput,
  async (context, input) => {
    context.signal.throwIfAborted();
    return notes.read(input.id);
  },
);
backend.bindCapability("notes", contract);
export default backend;
```

The same synchronous `decodeInput(value, canonicalSource)` runs for pure
validation and invocation. It must reject any local decoder constraint the
published schema alone cannot express, without business I/O, state changes or a
clock. Input is frozen. Missing, null and extra members remain distinct. The
handler receives original canonical input and digest, command ID, cancellation,
invocation origin/parent/target and callback reference in its immutable
`CallContext`. A method or export cannot register twice or change after publish.
Every exported capability must have its complete exact implementation.
`serve(transport)` validates the complete binding set, freezes registration and
owns the MCP connection until it ends. `close()` aborts work, closes transport,
joins admitted handlers and releases resources once; concurrent callers share
its settlement. The MCP server and publication step are private to `Backend`.
The Node entry point supplies stdio and signal handling to this lifecycle.
Failures retain their lifecycle phase, error category, conventional system code,
source-map locations and cause/aggregate relationships in bounded stderr
records. Exception messages, thrown payloads and arbitrary error properties are
not logged: they may contain credentials or user data, and generic text
redaction cannot prove their absence. A source location identifies the owning
operation without printing its arguments. Cyclic or oversized diagnostics are
marked, not recursively dumped. This projection never changes the original
failure, command outcome or shutdown owner. Authors remain responsible for their
own console output.

`context.runtime()` creates the public Runtime client from this call's captured
grant. It keeps the original invocation and fixed loopback endpoint, uses no
proxy or redirect, bounds response bytes and closes when the handler returns.
Invocation header JSON uses ASCII escapes for Unicode code units, preserving the
exact decoded identity, parent and target through both the public Runtime client
and typed host calls. Request bodies and canonical receipt input are unchanged.
`client.queryJSON` preserves exact Core evidence; ordinary typed methods refuse
unrepresentable numeric projections. `callHost` supports a captured typed host
method. No call substitutes the activation's setup grant. Ordinary MCP Tools use
`backend.tool(descriptor, handler)` and receive the same explicit context;
`context.toolCommandId()` derives their original private command identity.
`toolReceipts(handler)` projects that private owner's result through the existing
Go `ToolReceipt` contract. Its `output` is the original Scope ToolOutput JSON,
which the Runtime validates; it is not a second result cache or a re-execution.
`implement`/`implementStream` accept a `Method`, so standalone published methods
and capability members use the same registration path.

Register `onSetup` before publication when acquiring resources or consuming
declared dependencies. Setup receives a deeply frozen list of named contributions
and exact selected bindings. Each binding includes installation, release digest,
contribution and contract; absence is an empty list. Use these values for installed
Runtime calls. Selection changes do not mutate this captured set. `defer(cleanup)`
owns partial acquisitions even when setup fails. A returned resource may have
`start()` for background work and must have `close()`. Activation starts it once
after accepted setup. EOF, signals and failed setup cancel and join owned work,
then close resources once. Async work must observe its supplied signal.

`implementStream` takes an async iterable of explicit `{ kind: "chunk", value }`
and one final `{ kind: "result", value }`. Each chunk waits for the original
consumer's acknowledgement. The final remains unpublished until iteration ends;
its value is captured when yielded, so later producer mutation cannot rewrite it.
Missing, invalid, duplicate or late final work is a protocol failure. Cancellation
closes and joins the source. A command stream settles through the same original
receipt as a unary command; recovery does not replay chunks.

Throw `Refusal(code, message)` only for a known business refusal. The published
Go contract requires a nonempty code and valid Unicode strings; its message may
be empty. Construction and received replies preserve that same classification,
without inventing display text or turning an empty message into uncertainty.
Unexpected exceptions, cancellation, malformed data and invalid output are
protocol failures, including output failure after a mutation committed.
`receipts(handler)` exposes
the existing receipt method. Its implementation owns durable business admission
and atomic business/receipt settlement; this SDK has no result cache or database.
A missing receipt never permits another mutation or a new Core observation.
The [Usage Reports implementation](../../plugins/usagereports/README.md) shows
the full SQLite transaction and unknown-outcome behavior.

Schemas support the documented draft-2020-12 object/array/type, local JSON Pointer
reference, composition, string/format, required/property, enum/const and exact
numeric constraints exercised by the published contracts. External resources,
`$id`, anchors, dynamic references, tuple/prefix items, dependent and unevaluated
keywords are rejected. Additional schema features require shared conformance
coverage before they can be advertised. Opaque schema names are never rewritten.

Build with an explicit compiler, without a globally discovered compiler:

```js
import { rolldown } from "rolldown";
import { buildBackend } from "@flame/backend-sdk/build";
await buildBackend({
  input: absoluteSource,
  directory: absoluteOutput,
  compiler: rolldown,
});
```

The output contains `start.mjs`, its complete static/dynamic dependency chunks and
source maps. Relative literal dynamic imports are supported. Missing modules,
unresolved computed imports/requires and native addons refuse portable packaging.
Built-in Node modules are the only externals. Keep declared resource files in the
package graph and address them through `PLUGIN_ROOT` or relative file URLs.

In the package's Flame declaration use:

```json
"managedBackends": [{ "id": "notes", "runtime": "node", "entry": "backend/start.mjs" }]
```

The ordinary package builder maps that entry to author source `backend/start.ts`;
it does not accept interpreter paths, flags or another MCP startup for `notes`.
The Runtime resolves its executable-relative companion and leases the complete
release. No system Node, npm install or build runs at installation. Portable ZIPs
need no OS/architecture suffix. Native backends remain supported by the same
builder. An independently exported TS author project includes packed SDKs, its
own npm lock and prebuilt development tools, with no Go compiler prerequisite.

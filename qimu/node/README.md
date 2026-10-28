# Qimu Node plugin SDK

`@qimu/node` implements the portable contract authoring and MCP process profile
for Node plugins. It depends on the maintained MCP SDK and schema validators.
It has no Flame or browser dependency. The Go-owned protocol constants,
capability descriptors, receipt contracts, stream contracts, and conformance
vectors and portable profile limits are projected into `src/protocol.generated.json` by
`qimu/mcp/cmd/protocolgen`.

## Authoring

```ts
import { Backend, Capability, parseJSON } from "@qimu/node";
import type { CapabilityDescriptor } from "@qimu/node";

const contract = new Capability(
  parseJSON(publishedDescriptor) as CapabilityDescriptor,
);
const backend = new Backend({ name: "documents.parser", version: "1" });
backend.implement(
  contract.method("documents.read"),
  decodeInput,
  async (context, input) => {
    context.signal.throwIfAborted();
    return files.read(input.path);
  },
);
backend.bindCapability("parser", contract);
export default backend;
```

`Capability` and `Method` retain the complete published identity. Construct them
from exact JSON rather than ordinary JSON imports when schemas contain wide
numeric bounds. The shared codec is `@qimu/sdk/json`; this package adds Node-only
cryptographic identity and schema authoring. These implementations are not
included in the browser SDK entry point.

The synchronous decoder runs during both pure validation and invocation. It must
not perform business I/O or mutate state. The schema adapter rejects unsupported
keywords and external references, never coerces, strips, or defaults input, and
retains exact numeric comparisons. The Host performs its own boundary admission.
An export cannot register twice or change after publication. Every capability
must have its complete exact implementation before serving begins.

`PORTABLE_LIMITS` is an immutable projection of Go's
`portable.ProfileLimits()`. Descriptor and schema admission bounds their bytes,
JSON depth, value nodes, method count, local references and reference expansion
before cloning or compiling. Input, output and stream validation apply the same
depth and numeric limits, a separate payload node budget and a conservative
schema traversal budget. Payload byte limits remain with the original transport
or application owner; the descriptor byte limit does not truncate chat bodies or
vector batches. Numeric lexemes remain exact, including wide integers and decimal
spelling, within the published literal length and absolute exponent limits.
Enum admission also bounds pairwise equality work. Schemas that can apply
`uniqueItems` have a conservative array-comparison budget that includes strings,
member names and numeric text. Ordinary arrays and large text bodies do not
inherit that equality limit when their schema has no uniqueness constraint.

References must be local JSON pointers. Recursive properties and items are
supported; a reference or composition cycle that consumes no instance depth is
rejected. The traversal bound considers every possible schema branch rather than
executing predicates to predict a cheaper path. It is an admission limit, not a
wall-clock guarantee. Patterns support fixed-width atoms, character classes and
bounded fixed repetitions; alternatives, groups, variable repetitions and
backreferences are outside this profile. Schema predicates remain implemented by
the existing validators, and no defaults, coercions or fallback engine are added.

`CallContext` carries the captured signal, immutable invocation, original input
and digest, command identity, callback grant, and Lifetime. Origin and target are
consumer-defined data. `callHost` uses an explicit published method and the
captured callback; it never replaces a call grant with an activation setup grant.
`callbackFetch` supplies a fixed, nonredirecting, loopback transport for a
consumer's own application client.
Host callback admission refusal preserves `QimuError.code`: a full request
budget is `capacity`, a retired grant is `closed`, and other non-success HTTP
responses remain `outcome_unknown`. These are transport/admission observations,
not manufactured business refusals or permission to repeat a mutation.

## Setup, work, and cleanup

`onSetup` receives exact selected dependencies from the Host. The SDK preserves
those bindings and does not run another resolver. Register `context.defer`
before starting acquisition. Its confirmation handle can settle an unconfirmed
resource close when the actual adapter obtains late evidence.

A returned setup resource may define `start()` and must define `close()`.
Activation starts it once. EOF, failed setup, or an explicit close cancels and
joins admitted work before releasing resources. Setup, per-call work, and
resources all use `@qimu/sdk` Lifetimes. A thrown cleanup is unconfirmed and does
not get replayed. `Backend.wait()` observes the original eventual exit.

`implementStream` yields explicit chunk frames followed by one final result.
Each chunk waits for the original consumer acknowledgement. The result remains
unpublished until iteration ends. Missing, duplicate, invalid, or late final
work is a protocol failure. Cancellation closes and joins the producer.

Throw `Refusal(code, message)` for a known business refusal. Empty messages are
valid; nonempty codes are required. Unexpected exceptions and invalid output
remain protocol failures, including failure after a mutation may have committed.
`receipts(handler)` delegates durable business admission and result recovery to
the plugin. The SDK does not create a result cache or business database.

## Distribution

```js
import { rolldown } from "rolldown";
import { buildBackend } from "@qimu/node/build";

await buildBackend({
  input: absoluteSource,
  directory: absoluteOutput,
  compiler: rolldown,
  target: "node24",
  maximumFrameBytes: applicationFrameBudget,
});
```

Compiler, Node target, and MCP frame budget are explicit consumer choices. The
Host process driver must use the corresponding frame budget. `startBackend` from
`@qimu/node/bootstrap` also requires `maximumFrameBytes` when used directly.
The custom exact-JSON stdio adapter bounds both incoming and outgoing frames.
It never resynchronizes after malformed or oversized input. Stdout is reserved
for MCP; console diagnostics are redirected to stderr before importing author
code.

The build output contains `start.mjs`, the complete static and literal dynamic
module graph, and source maps. Only Node builtins remain external. Unresolved
computed imports, missing modules, and native addons fail portable packaging.
The consumer owns package format, installation policy, managed executable
selection, and declared resource files.

Diagnostics retain bounded lifecycle phases, categories, source locations, and
cause relationships. They do not log exception messages, arbitrary properties,
or thrown payloads that may contain credentials or user data. This projection
never rewrites the original business result or exit evidence.

Run `npm run check --workspace @qimu/node` from the monorepo root. Callback tests
use real local HTTP servers; subprocess and network permissions must permit
isolated loopback fixtures.

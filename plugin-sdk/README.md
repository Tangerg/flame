# Public component authoring

`@flame/plugin-sdk` is the public TypeScript boundary for ordinary installed React
components. It exposes an explicit Runtime client, frozen installed contribution
binding and owned cancellation signal. The binding is absent for a component
without its own callable contribution; `bindOperations` refuses such a context.
Static content uses ordinary Hooks and shared UI controls without a backend. The
SDK exposes no application alias, private state store or ambient service lookup.

Workspace-scoped components receive `context.workspace`: a frozen public
`WorkspaceRef` plus file-focus path and revision from the Host's existing owners.
They use `context.runtime.workspace(ref)` for canonical workspace operations. The
context has no private store or navigation object. Target/focus replacement retires
the render instance; calls honor its signal and track boundary. A component may
publish an optional generic material key for a navigation consumer; the loader
projects it without a business-specific branch.

Client-scoped components receive optional frozen Session/workspace targets and can
render when either is absent. They do not receive file-focus changes; workspace-scoped
components retain their required focus projection. Memory uses this scope to keep user
memory available without a workspace while capturing project targets explicitly.
`WorkspaceContext.focus` is optional for client scope; focus consumers require it.

File-scoped renderers receive `context.file`, a frozen `CapturedFile` containing
one accepted Runtime read and its workspace, path and bounds. It is recorded data;
navigation does not retarget it. Loading, failure and retirement retain the existing
source inspection. Partial/truncated reads remain in source mode. The Markdown
component is the concrete consumer; its parser is bundled while React/SDK/UI stay shared.

Session-scoped components receive `context.sessionId`, a frozen projection of the
existing navigation owner. They call public Session operations directly. A changed
Session remounts the instance and cancels/joins its old reads; rendering never becomes
an owner of Session or Run transitions. Trajectory is the concrete consumer.

Tool-result components receive `context.toolResult`, a detached frozen
`CapturedToolResult` with the original call/Run IDs, argument/result bytes and recorded
status, bounds and error. It contains data, never historical module or replay authority.
The current installed component may format it with ordinary Hooks and shared controls;
generic inspection remains available when a renderer fails or is absent. Navigation
never retargets the captured record. Tool results is the concrete consumer.

Contextual Session actions receive `context.sessionTarget`, a frozen `CapturedSession`
projection of the accepted row's ID, title and revision at interaction time. The action
placement is local metadata; offering a menu does not activate code or query a backend.
An opened component submits public Core/installed operations with this inspected target
and retains original mutation handles on unknown acknowledgement. The Trajectory
package's naming form calls canonical `sessions.update`; it owns no special Core action
or private Host form. Later navigation never replaces an open action target.

Every component receives `context.telemetry`, the explicit client-local observation
collaborator. Public row types describe traces, metrics and logs; `read()` returns
a detached snapshot of the current buffer and `clear()` delegates to that buffer's
owner. Both refuse after the component signal ends. Runtime accounting and remote
telemetry are separate facts. Diagnostics is the concrete ordinary component consumer.

`bindOperations` consumes the owner's generated `CAPABILITY_JSON` and
`MethodInputs`/`MethodOutputs`. Its query and command functions enter the ordinary
`plugins.call` endpoint. Runtime admits the exact callable contract and validates
input/output at the owner boundary. Contracts and feature input/output/chunks
cross Core as JSON strings, so JavaScript cannot round their numbers before
admission or receipt recovery. The binding uses the shared exact JSON codec;
generated numeric fields use `JSONNumber` (`number | bigint | ExactNumber`).
Safe ordinary numbers stay ordinary; use bigint for wide integers and
`new ExactNumber("1.0")` to retain another original literal. Render numeric values
with `String(value)` and choose any arithmetic conversion explicitly. Copy exact
values through this codec or preserve their JSON source; `structuredClone` does
not retain an `ExactNumber` prototype.
Known refusals become `OperationFailure`;
unconfirmed transport remains the shared Runtime client's uncertainty. The mapped
command preserves its original idempotency key and explicit retry closure.

The component captures its inspected resource/revision before submitting a
command. It must not turn response loss into another command identity or render
an unconfirmed change as saved. Closing a component cancels observation; it does
not cancel an independently accepted Runtime Run or replace a business receipt.
The existing client mutation journal retains original command evidence. For a
logical command that must survive client/provider generation changes, use
context.runtime.plugins.command with a stable intent, generated contract/method/input,
a binding preparation callback and an output decoder. Recovery runs before the
callback and retains the original route/key/retention guard. The caller supplies
the original input meaning and exact numeric lexemes; changed input is refused.
Object order and string escape spelling follow Go's admitted JSON identity.
The decoder must validate
the publisher acknowledgement before settlement. Journal routing/fingerprints are
durable; authored input and business results are not copied into it. Low-level
calls and stable-intent commands share that journal and retain routing metadata.
For a recreated component, `bindOperations(...).pendingCommands(method)` projects
unresolved unary commands for that original installation/contribution/target.
Its recovery handle reads `plugins.getCommand` from Runtime's saved route before
querying the original backend receipt under the same key. It does not reconstruct
input from the current draft, prepare another provider or persist business output.
Unavailable, missing, expired or conflicting evidence stays unresolved. Runtime
refuses route recovery before dispatch admission, so this read cannot initiate work.
`context.runtime.plugins.pendingCommands()` exposes the same unary journal
projection for client management. Streaming commands retain their existing
explicit original-input stream recovery contract.

The client journal now requires method metadata. Former records are retained
and refused unchanged; use their original client to reconcile pending work before
upgrading, and a fresh client profile for this pre-release cut. There is no
automatic conversion, deletion or fallback fingerprint. Neither client journal
nor Runtime routing owns the business outcome. Actual standalone Web recovery
now covers backend-to-Core reply loss across UI/Runtime restart and same-role
provider replacement through generic management. A separate Core-to-UI loss
followed by original uninstall keeps that old binding unknown and leaves the
new provider unchanged. Resource connection/reload proof remains a distinct
qualification; it is not the evidence for command recovery.

Use `buildComponent` from `@flame/plugin-sdk/build` with a source entry, output
directory and `compiler: rolldown` imported from the author toolchain's declared
`rolldown` dependency. The compiler has one owner: the author's locked build graph.
The SDK owns shared-module externalization and the prebuilt ESM/relative-asset
format. The former implicit compiler lookup is removed; all in-repository build
callers supply the compiler explicitly. Private dependencies are bundled. `HOST_MODULES` names shared React,
React DOM, JSX runtimes, SDK and UI modules; UI subpath imports are also external.
The graphical client must map those imports to its actual shared instances before
evaluating an entry. The workbench emits an import map to its actual shared ESM exports. Core resource
subscriptions and exact-release asset queries provide package-relative URLs. The
component signal cancels observation; its explicit track boundary joins SDK calls
through the existing contribution lifetime. Development uses this same format.

The checked Reviews Board is the concrete consumer. It displays current owner
data, submits inspected-revision changes, exposes conflict and uncertainty,
retains the original recovery identity and invokes the installed analyzer query.
The Reviews workspace declaration and actual ESM loader use the same shared
module graph as other components. Source, type, build and actual graphical
qualification retain their own recorded scope; compilation alone does not prove
component or recovery lifetimes.

Core Runtime failures are classified with the shared SDK's `isErrorType` export.
Components must use this host module rather than bundling another RPC error class;
class identity is part of the Runtime client's typed failure contract. Business
operation refusals use the shared Runtime client's OperationFailure and code/message,
which the SDK exports as the same class.


Generated streaming methods publish a distinct chunk schema and final result.
Supply the generated MethodChunks as bindOperations' third type map to use its
stream(intent, method, input, consume, signal) function. The captured target/binding
and original contract are checked before the common client plugins.stream entrance.
Each consume(chunk, signal) may return a Promise; the request waits for it while
nested RPC remains available. The returned Promise resolves only the final output.
Consumers must honor the supplied signal; client closure cancels and joins their
returned Promises alongside the transport reader.
Component tracking and signal bound the same operation; no browser relay is added.
Ordinary query/command bindings still refuse stream metadata before delivery.

The TS stream uses the existing durable intent journal. Missing/invalid terminal,
consumer/decoder failure and client replacement preserve the original identity,
binding and input; restoration reads the original receipt without chunk replay or
current-provider preparation. Raw JSON source and the shared exact codec preserve
numeric evidence; already rounded unsafe JS inputs are refused. Installed
model/dependency lifetimes stay with their captured backend and calling owners.

Streaming query methods use `bound.queryStream(method, input, consume, signal)`.
Their chunks follow the same shared client backpressure and component lifetime as
command streams, but carry no mutation intent or journal reservation. Query failure
is a read failure; it cannot become an unconfirmed command or a private receipt
lookup. Command streams continue through `bound.stream(intent, method, input,
consume, signal)` with their original durable command identity.

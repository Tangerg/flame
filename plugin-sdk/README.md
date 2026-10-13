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

Session-scoped components receive `context.sessionId`, a frozen projection of the
existing navigation owner. They call public Session operations directly. A changed
Session remounts the instance and cancels/joins its old reads; rendering never becomes
an owner of Session or Run transitions. Trajectory is the concrete consumer.

Every component receives `context.telemetry`, the explicit client-local observation
collaborator. Public row types describe traces, metrics and logs; `read()` returns
a detached snapshot of the current buffer and `clear()` delegates to that buffer's
owner. Both refuse after the component signal ends. Runtime accounting and remote
telemetry are separate facts. Diagnostics is the concrete ordinary component consumer.

`bindOperations` consumes the owner's generated `CAPABILITY_JSON` and
`MethodInputs`/`MethodOutputs`. Its query and command functions enter the ordinary
`plugins.call` endpoint. Runtime admits the exact callable contract and validates
input/output at the owner boundary. The full contract remains a JSON string in
the request, so JavaScript cannot round schema integers before admission. Known refusals become `OperationFailure`;
unconfirmed transport remains the shared Runtime client's uncertainty. The mapped
command preserves its original idempotency key and explicit retry closure.

The component captures its inspected resource/revision before submitting a
command. It must not turn response loss into another command identity or render
an unconfirmed change as saved. Closing a component cancels observation; it does
not cancel an independently accepted Runtime Run or replace a business receipt.
The existing client mutation journal retains original command evidence; complete
UI recovery after a new component generation still needs host integration.

Use `buildComponent` from `@flame/plugin-sdk/build` with a source entry and output
directory. It uses the existing Rolldown compiler to emit prebuilt ESM and relative
assets. Private dependencies are bundled. `HOST_MODULES` names shared React,
React DOM, JSX runtimes, SDK and UI modules; UI subpath imports are also external.
The graphical client must map those imports to its actual shared instances before
evaluating an entry. The workbench emits an import map to its actual shared ESM exports. Core resource
subscriptions and exact-release asset queries provide package-relative URLs. The
component signal cancels observation; its explicit track boundary joins SDK calls
through the existing contribution lifetime. Development uses this same format.

The checked Reviews Board is the concrete consumer. It displays current owner
data, submits inspected-revision changes, exposes conflict and uncertainty,
retains the original recovery identity and invokes the installed analyzer query.
The Reviews workspace declaration and actual ESM loader are implemented. Shared
renderer/provider execution proof, remaining placements and complete browser/Wails
lifecycle acceptance remain open. No UI/E2E execution
is part of this development slice.

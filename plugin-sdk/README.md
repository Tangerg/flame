# Public component authoring

`@flame/plugin-sdk` is the public TypeScript boundary for ordinary installed React
components. It exposes an explicit Runtime client, frozen installed contribution
binding and owned cancellation signal. It has no application alias, state store,
ambient service lookup or private source import.

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
evaluating an entry. This package does not itself establish that mapping or module
retirement; those remain required loader work. Development uses this same format.

The checked Reviews Board is the concrete consumer. It displays current owner
data, submits inspected-revision changes, exposes conflict and uncertainty,
retains the original recovery identity and invokes the installed analyzer query.
Component loading, public placement declaration, shared renderer/provider proof
and complete browser/Wails lifecycle integration remain open. No UI/E2E execution
is part of this development slice.

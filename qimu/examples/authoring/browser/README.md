# TypeScript document author task

This is a maintainer-authored public-API exercise. It is not an external-user
study. The author implements new native JavaScript FileReader, Converter and
Validator Units using `@qimu/sdk`, while the application uses `@qimu/browser`
to execute the original Go Host's approved plans. Neither package requires
React, Dougong or Flame's Runtime and SDK modules.

`src/plugins.ts` contains the author code. The Converter imports FileReader
through its declared `files` slot at setup. The FileReader reads the original
HTTP source using the admitted call's AbortSignal, then verifies the document
revision. The Converter escapes source text and emits HTML. The Validator
contributes document-title validation metadata and a callable implementation.
Its contribution becomes public only after successful setup and Host admission.

The document consumer owns the contract identities and value DTOs in
`examples/document/contracts.go`. The generated projection in
`src/contracts.generated.ts` comes from those exact declarations. `contracts.ts`
supplies the native JavaScript function signatures. All functions remain in the
same JS Realm; this is not a claim that Go and JavaScript native function values
are interchangeable across a protocol.

`src/client.ts` is the example application's composition layer. It captures the
Converters and Validators Points in one snapshot, applies named selection rules,
and acquires one original Use. It converts and validates through that Use, then
returns a resource-valued `Acquired<RenderedDocument>` backed by an actual Blob
URL. The SDK reserves the resource cleanup before creation. The convenience
handle's `close` and `wait` target the original approved Use, which also owns the
result resource. It does not mirror the resource lifecycle in application state.
Closing removes the Blob URL, invalidates subsequent access, cancels active work
and joins it before the Use's proof can release protection.

The `partial` Unit demonstrates failed setup after a resource has been reserved
and a contribution staged. It intentionally throws an error containing its Blob
URL so the exercise can verify that the original owner actually revoked it. The
failed Unit never contributes a public Validator. No special framework failure
switch or replacement cleanup owner is used.

## Build and run

The trusted Go fixture reads the already-built module bytes, registers the
original document descriptors, installs the authored Units and exposes the real
authenticated `control/host.Bridge`. The build script projects Units from their
actual `defineUnit` declarations; it does not ask an author to repeat contract
hashes, slots, export tables, LocalPlans or exit proofs. The fixture chooses the
`browser-js` execution profile and the Host's unique-provider policy. The narrow
author declaration deliberately has no `Providers` field: an unresolved import
must not be serialized as an exact empty provider selection.

From the Qimu module, build the application fixture and regenerate public types:

```sh
GOWORK=off go run ./examples/authoring/cmd/authorhost \
  -generate examples/authoring/browser/src/contracts.generated.ts
GOWORK=off go build -o /tmp/qimu-authorhost ./examples/authoring/cmd/authorhost
```

From this directory, using the installed npm dependencies:

```sh
npm run check
QIMU_AUTHOR_HOST_BINARY=/tmp/qimu-authorhost npm run test:workflow
```

The workflow is a mandatory real Go-process, SQLite, loopback HTTP and native
JavaScript test. It has no mock Host or LocalPlan. It verifies successful
conversion, a Validator's nonempty diagnostic, one multi-Point acquisition,
resource access after the initial callback, actual Blob revocation, cancellation
of a pending HTTP body, and failed setup without publication. Native JavaScript
executes in Node in this exercise. Actual Chrome/DOM execution is tested by the
separate document workbench browser workflow; the two environments must not be
reported as equivalent.

For an independent author exercise, copy this directory into a new npm project,
install packed release artifacts for `@qimu/sdk` and `@qimu/browser`, create an
independent lock and build the fixture with `go install
github.com/Tangerg/flame/qimu/examples/authoring/cmd/authorhost@<release>`. Use
`GOWORK=off` and the public module artifact. The committed generated contract
projection is sufficient; this npm author project does not need a Go compiler.
When regenerating that projection, invoke the fixture from the exact same
release. No workspace links, checkout imports, React, Dougong or Flame SDK
dependencies are part of that independent project.

## Author observations

The maintainer consulted Qimu's README, the SDK and Browser README files, the
public `defineUnit`, `Setup`, `Lifetime`, `acquire`, `select`, `Runtime`,
`RealmModules`, `ControlPump` and Bridge APIs, and the document contract source.
Prior experience implementing these packages is part of this exercise.

Public export names occur both in the declaration and the returned setup object
so Runtime can check the actual export set. A Point export name also occurs in
`contribute`, where its selection metadata is staged. The contract identity and
value itself are reused, not re-created. The local function signature is an
intentional native-language projection; the generated reference remains its
single identity source.

The first integration attempt reported `required import files`: the fixture had
incorrectly projected an unresolved slot as an exact empty provider list. The
fix was to remove provider selection from the author DTO and leave it with the
Host's application policy. The subsequent full workflow passed. Command times
and final release hashes belong in the external acceptance record. The complete
learning and implementation interval was not measured, and no independent
third-party author completion time is claimed.

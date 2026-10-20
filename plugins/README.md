# First-party portable packages

Memory, Schedules, Knowledge base, GitHub, Model router, Guardrails, Web research,
Saved usage reports, Native sandbox and File watch are complete optional backends. Memory,
Schedules and Saved usage reports use ordinary public components; the other seven
are backend-only packages.
MCP model Tools and human Runtime commands
enter the same existing backend owners. A component does not create a second store.

The Flame namespace in root plugin.json references one local declaration file.
That strict declaration names each independent contribution ID, its contract file,
server and export. Composition belongs to the consumer contract. Multiple
contributions may share one backend; selection never equates their IDs with the
server name. Installation reads these files without executing package code.

A callable contribution may declare named `requires` entries with `id`, a local
`contract` resource path and `optional`. Resolution uses exact admitted contracts:
selection uses the configured installation/contribution, and collections return
the enabled members. `plugins requirements --request '{"installationId":"…","digest":"…","contribution":"…"}'`
projects these bindings through the same Runtime Go/Protocol/TypeScript query.
An absent optional collaborator has an empty binding list. Required activation
edges are checked across all contributions sharing an executable entry; optional
collection members do not introduce startup edges. Managed admission captures
one immutable dependency set, waits for required providers' already owned connection
attempts, verifies the exact backend exports and methods, and sends the named set
through `operations.setup` before publishing the session or Tools. Fresh cross-plugin
installed calls check this captured set; a later selection changes a future
activation without changing an existing binding. Failed setup retires callbacks,
session and leased executable without changing desired enablement. The inspection
query resolves current metadata and does not activate code.

Backend authors register `Backend.OnSetup` before `Publish`. `Setup.Contributions`
returns detached named bindings; an optional absence has an empty list. Return a
closer for acquired resources and honor the setup lifetime's cancellation. It is
closed once after failure or session retirement; `Backend.Close` cancels and joins
all setup lifetimes. Setup prepares registrations and collaborators; irreversible
business effects belong to explicit operations with their command/receipt contract.
Point values and applicability are validated by the consumer-owned contract.
Requirement inspection and the existing MCP connection status explain selected
bindings and activation failures; transitive failures preserve the consumer path.
No separate readiness ledger advances those facts. CLI `plugins compositions`,
`plugins set-composition` and `plugins resolve-composition` inspect/change these
existing owners without loading a graphical component.


Build target-specific packages from the repository's local `go.work`:

```sh
node plugins/build.mjs --os darwin --arch arm64 --output /absolute/new/packages
```

The output directory must not exist. Supported targets are Darwin, Linux and
Windows on amd64 or arm64. The output contains the complete first-party catalog,
including all workspace pages and renderers and compiled Memory/Schedules backends.
The same build discovers and compiles the Knowledge base backend and copies its
generated operations/context contracts.
GitHub uses the same package path with a declared optional secret token input;
its unset configuration supports public reads without blocking catalog initialization.
Memory/Schedules package namespaces also contain canonical contracts generated
from the typed methods registered by their actual backends. Run
`go generate ./internal/plugins/memory/rpc ./internal/plugins/schedules/rpc`
from `runtime` after changing those methods; the package builder copies the
namespace resources into the output.
Runtime and CLI build targets place it in `plugins` beside their executable.
Runtime installs and enables this catalog only when initializing a fresh
database. Restart preserves user decisions. Package updates still require explicit
release selection.

The source folders contain no compiled backend; installation never builds code or
installs dependencies. Executables are shipped in the admitted package. `--replace`
is for replacing an owned build output after the new distribution has been built.

See [Memory](memory/README.md) and [Schedules](schedules/README.md) for fact owners
and lifecycle contracts, [Knowledge base](knowledgebase/README.md) for document
publication/retrieval, and [GitHub](github/README.md) for the read-only connector and
credential/cache boundaries. [Model router](modelrouter/README.md) owns role and
captured-envelope routing policy while core constructs and prices the model.
[Guardrails](guardrails/README.md) owns protected phrases and refuses matching
model-visible inputs before provider dispatch.
[Web research](web/README.md) owns Jina/Tavily calls and captured-page browsing;
its private archive preserves immutable source versions.
[Saved usage reports](usagereports/README.md) archives daily and weekly responses
from the public core accounting callback without creating another ledger.
[Native sandbox](sandbox/README.md) owns bounded code execution in private scratch
workspaces and original command outcomes through the shared native confinement.
[File watch](filewatch/README.md) owns configured file triggers and durable original
dispatch intents over the public Runtime workspace observation stream.
Other packages contain only their declared content or presentation resources.

Backend source is compiled from `runtime/internal/plugins/memory` and
`runtime/internal/plugins/schedules`, `runtime/internal/plugins/knowledgebase` and
`runtime/internal/plugins/github`, `runtime/internal/plugins/modelrouter` and
`runtime/internal/plugins/guardrails`, `runtime/internal/plugins/web` and
`runtime/internal/plugins/usagereports`, `runtime/internal/plugins/sandbox` and
`runtime/internal/plugins/filewatch`
inside the local Runtime application module.
These are separate executables with private facts and process lifetimes, without
separate published Go modules or first-party version resolution.

## Standalone author artifacts

Build a complete Usage author kit outside the checkout:

```sh
node plugins/build-author-kit.mjs --output /absolute/new/author-kit --offline
```

The command builds the existing icon/UI artifacts, packs the public SDK, UI,
icons and Runtime contract, and copies the complete Usage source/declaration.
It installs those archives in the independent directory, typechecks and builds
using only that directory's declared toolchain, then compares every emitted
resource byte with the bundled Usage package. Dependencies must resolve inside
that directory. It never loads or renders a component. Omit `--offline` only when
npm must fetch declared registry dependencies missing from the local cache.

The result includes its own lockfile and build script. After moving it away from
Flame, `npm ci --ignore-scripts`, `npm run typecheck` and `npm run build` reproduce
the installable `usage` directory. Current-source tarballs are public author
artifacts; this does not assert registry publication or rendered/native behavior.
The SDK build function requires the author's declared `compiler: rolldown`, so
source links and archives use the same explicit compiler owner.

## Shared page resources

First-party components consume the shared `@flame/ui` and public SDK. Each
component owns its read requests, accepted pagination history and local drafts;
failed reads retain the last accepted snapshot and cursor. File and Tool result
renderers use ordinary components over public frozen captured data. Memory, Schedules, Skills, Diagnostics, Trajectory, Diff, Usage and Icon gallery are ordinary
React components built through the public SDK. Usage reads the existing public
Runtime accounting query; Diff reads the canonical workspace query using a captured
public context. Trajectory reads canonical Session history through a frozen public
Session ID, preserving accepted pages/cursors after failed reads. Skills reads canonical catalogs/documents and performs library/proposal commands
with captured revisions and original uncertain-command handles. It consumes the
existing public Runtime change subscription; no private Host forms remain.
Icon gallery bundles the icon owner's canonical exports.
Diagnostics uses the public SDK client-telemetry collaborator for detached local
reads and owner-directed clearing. These packages require no own callable contribution
or backend process.

Build `icons` and `ui` first, then run:

```sh
npm ci --prefix plugins
npm run build --prefix plugins
npm run check --prefix plugins
```

`build-components.mjs` uses the public SDK to build each `src/component.tsx` as
prebuilt ESM into its declared namespace resource. `--check` verifies exact
reproducibility. The graphical loader supplies the actual shared host modules.

The retired HTML view and renderer sources, resources, channels and build paths are removed. Reinstall changed packages and
explicitly select their new digests.

## Contextual Session actions

A public component may declare Session scope and `sessionAction` placement. It is
registered in the existing Session row menu from local admitted metadata; the module
loads when opened. The SDK captures the accepted ID/revision/title. Trajectory owns
its naming form and calls canonical `sessions.update`, preserving unknown-command
reconciliation. No closed contributes.actions or plugins.renameSession wrapper remains.

# First-party portable packages

Memory, Schedules, Knowledge base, GitHub, Model router, Guardrails, Web research,
Saved usage reports, Native sandbox and File watch are complete optional backends. Memory,
Schedules and Saved usage reports use ordinary public components; the other seven
are backend-only packages.
MCP model Tools and human Runtime commands
enter the same existing backend owners. A component does not create a second store.

Each source plugin uses `backend/`, `component/` and `package/` for the capabilities it actually has. The Flame namespace in `package/plugin.json` references one local declaration file.
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
node plugins/scripts/build-packages.mjs --os darwin --arch arm64 --output /absolute/new/packages
```

The output directory must not exist. Supported targets are Darwin, Linux and
Windows on amd64 or arm64. The output contains the complete first-party catalog,
including all workspace pages, renderers and the ten compiled backends.
The same build copies each backend's generated contracts and declared resources.
GitHub uses the same package path with a declared optional secret token input;
its unset configuration supports public reads without blocking catalog initialization.
Memory/Schedules package namespaces also contain canonical contracts generated
from the typed methods registered by their actual backends. Run
`go generate ./memory/backend/rpc ./schedules/backend/rpc`
from `plugins` after changing those methods; the package builder copies the
namespace resources into the output.
Regenerate consumer projections with `go generate ./...` from Runtime and CLI,
then rebuild component resources before packaging.
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

Backend implementations live in `<name>/backend` beside their manifests, contracts
and optional `<name>/src` UI. The `plugins` Go module is separate from Runtime.
Its business code and unit tests import only public Runtime APIs; host-specific
integration tests stay with Runtime's host adapters. Shared invariants and native
mechanisms have one public implementation consumed by both modules.

## Independent author projects and release packages

Export one complete plugin outside the checkout:

```sh
node plugins/scripts/build-author-kit.mjs --plugin guardrails --module-path example.com/author/guardrails --output /absolute/new/author --offline
```

The author owns its explicit module namespace and regenerates its schemas. Go
production and tool dependencies are vendored from immutable standard root/nested
module artifacts; SDK source and manifests are never rewritten or trimmed.
[Consumer distribution](../docs/reference/consumers.md) explains frozen candidate
versions, verified local materials and isolated offline execution. UI-backed
projects consume packed SDK/UI/icons packages and their own npm lock. JS
independent installation is separate from the Go offline guarantee.

In the independent author directory:

```sh
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GONOPROXY=none GOFLAGS=-mod=vendor go test ./...
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GONOPROXY=none GOFLAGS=-mod=vendor go generate ./...
# UI-backed plugins additionally run npm ci --ignore-scripts, npm run typecheck and npm run build.
node build.mjs --source "$PWD" --os darwin --arch arm64 --output /absolute/new/packages --archives /absolute/new/releases
```

The collection coordinator calls the same explicit source/output builder used by an exported author project. Install the resulting ZIP through the ordinary
Runtime `plugins.install` operation or CLI with its absolute Runtime-host path:

```sh
flame --runtime-url http://127.0.0.1:17171 plugins install --request '{"source":"/absolute/releases/plugin-flame.guardrails-1.0.0-darwin-arm64.zip"}'
```

The ZIP contains the platform's executable and declared resources, with executable
permissions preserved. Runtime computes and admits its ordinary immutable release;
installation needs no Go, Node, compiler, source checkout or dependency installation.
Choose the Runtime host's OS/architecture; native sandbox availability follows the
actual confinement mechanism. Package acquisition from a registry/URL and automatic
updates remain separate work.

Module relocation changes the generated schema reference identities. Regenerate
plugin contracts, host/CLI projections and component resources together. Existing
installations must explicitly admit/select rebuilt packages. Original pending work
retains its old release and receipt route; finish it with the matching original
host/binary before upgrading. Private business schemas and core accounting ownership
are unchanged. No compatibility reader or automatic data conversion is supplied.

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

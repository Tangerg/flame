# First-party portable packages

Memory, Schedules, Knowledge base, GitHub, Model router, Guardrails, Web research,
Saved usage reports, Generated interfaces, Computer use, Native sandbox and File watch are complete
optional backends. Memory, Schedules, Saved usage reports, Generated interfaces and Computer use expose
ordinary public components; the remaining packages are backend-only.
MCP model Tools and human Runtime commands
enter the same existing backend owners. A component does not create a second store.

Generated interfaces (`genui`) accepts a bounded interactive document through
`genui_ui_present`. Its immutable presentation owner supplies the component,
state, expression and action contract. Runtime Tool history retains the accepted
document; OpenUI owns local draft values. Queries and confirmed commands use real
captured plugin bindings and the existing Runtime command journal. Followups enter
the original Session through the shared Agent input owner. Isolated HTML previews
have no Runtime bridge. See [`genui/README.md`](genui/README.md).

Computer use (`computer`) directly loads a selected installed CUA engine on
the Runtime host. Its public workspace component owns no permission or execution
state: it observes the backend's human-authorized control lease, exact live
permission requests and original command receipts. The backend serializes desktop
execution, retains original media, ends each upstream control turn, and retires
uncertain processes without replay. Installing the plugin does not launch a
desktop engine. Operator-console isolation and the real native prerequisites are
described in [`computer/README.md`](computer/README.md).

Each source plugin uses `backend/`, `component/` and `package/` for the capabilities it actually has. The Flame namespace in `package/plugin.json` references one local declaration file.
That strict declaration names each independent contribution ID, its contract file,
server and export. Composition belongs to the consumer contract. Multiple
contributions may share one backend; selection never equates their IDs with the
server name. Installation reads these files without executing package code.
The declaration uses the public `plugin.DeclarationProtocol`, currently
`io.github.tangerg.flame/plugin/1`. This Flame metadata version is independent of
the Qimu portable descriptor and transport versions.

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
through `qimumcp.SetUp` and its compiled `qimumcp.SetupMethod` before publishing
the session or Tools. Fresh cross-plugin
installed calls check this captured set; a later selection changes a future
activation without changing an existing binding. Failed setup retires callbacks,
session and leased executable without changing desired enablement. The inspection
query resolves current metadata and does not activate code.

Go backend authors import `github.com/Tangerg/flame/qimu/mcp` as `qimumcp` and
register `qimumcp.Backend.OnSetup` before `Publish`. `qimumcp.Setup.Contributions`
returns detached named bindings; an optional absence has an empty list. Return a
closer for acquired resources and honor the setup lifetime's cancellation. It is
closed once after failure or session retirement; `Backend.Close` cancels and joins
all setup lifetimes. TypeScript authors use the same generic setup contract
through `@qimu/node`. Setup prepares registrations and collaborators; irreversible
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

The output directory must not exist. Native backend compilation accepts Darwin,
Linux and Windows on amd64 or arm64. Saved usage reports is a portable managed
Node backend; the distribution's selected companion targets live in
`packages/local-runtime/node-release.json`. A distribution build refuses targets
without a selected Node artifact. The output contains the first-party catalog,
including all workspace pages, renderers, native executables and the complete
managed JavaScript graph.
The same build copies each backend's generated contracts and declared resources.
It invokes the prebuilt `pluginpack` author command to write the neutral
`manifest.json` and `plugin.lock.json`, using the same Flame-to-Qimu projection as
Runtime. Raw source directories without these files are not installable releases.
GitHub uses the same package path with a declared optional secret token input;
its unset configuration supports public reads without blocking catalog initialization.
Package namespaces contain projections of their owner's canonical contracts.
Memory binds the public SDK contract. Each other native feature keeps its Go
contract definition in its own `contractgen/` publisher and carries the generated
`contracts/contract.generated.go` descriptor beside its implementation. Pure
`internal/wire` DTOs and schema hooks do not import that generated projection.
Runtime bindings use `ImplementMethod` against the published descriptor; moving
an implementation to another Go module cannot advance the contract identity.
Run `make generate` from the repository root after changing definitions. It
publishes consumer contracts before backend, CLI and component projections;
the package builder copies those namespace resources into the output.
Runtime and CLI build targets place it in `plugins` beside their executable.
Runtime installs and enables this catalog only when initializing a fresh
database. Restart preserves user decisions. Package updates still require explicit
release selection.

The source folders contain no compiled backend; installation never builds code or
installs dependencies. Executables are shipped in the admitted package. `--replace`
is for replacing an owned build output after the new distribution has been built.

See [Memory](memory/README.md) and [Schedules](schedules/README.md) for fact owners
and lifecycle contracts, [Knowledge base](knowledgebase/README.md) for document
publication/retrieval, and [GitHub](github/README.md) for connector behavior and
credential/cache boundaries. Its [generated operation reference](github/package/io.github.tangerg.flame/contracts/API_REFERENCE.md)
describes the published methods. [Model router](modelrouter/README.md) owns role and
captured-envelope routing policy while core constructs and prices the model.
[Guardrails](guardrails/README.md) owns protected phrases and refuses matching
model-visible inputs before provider dispatch.
[Web research](web/README.md) owns Jina/Tavily calls and captured-page browsing;
its private archive preserves immutable source versions.
[Saved usage reports](usagereports/README.md) archives daily and weekly responses
from the public core accounting callback without creating another ledger.
[Native sandbox](sandbox/README.md) owns bounded code execution in private scratch
workspaces and original command outcomes through the shared native confinement.
[Professional deliverables](deliverables/README.md) owns immutable native Office/PDF
revisions, local edits, confined rendering and recalculation, source-bound review
evidence and delivery exports. Its component and model tools share that owner;
the host's public binary file-save capability owns only the final transfer.
[File watch](filewatch/README.md) owns configured file triggers and durable original
dispatch intents over the public Runtime workspace observation stream.
Other packages contain only their declared content or presentation resources.

Each native backend has one process entry in `<name>/backend` and private
implementation under `<name>/internal`. Domain types own deterministic policy;
Application owns workflows; private SQLite adapters own persistence and atomic
receipts; contract code publishes typed projections. Delivery, Tool codecs and
callback adapters live with the process that composes them. A plugin only keeps
the boundaries its behavior requires.

The first-party backends belong to the independent `plugins/go.mod` member of
`go.work`. They import the public SDK, never Runtime implementation. The
`architecture_test.go` guard checks these dependency directions and excludes
production imports of test fixtures. Host integration tests exercise published
contracts and installed executables. Author export carries a selected plugin's
backend, required internal sources, component and release resources as one
independent module; it does not grant Memory's canonical publisher authority.
Generic method, setup, callback and receipt transport mechanisms have one Qimu
implementation. Flame's public SDK retains its model roles, Tool adapters and
product-specific contracts.

## Independent author projects and release packages

Export one complete plugin outside the checkout:

```sh
node plugins/scripts/build-author-kit.mjs --plugin guardrails --module-path example.com/author/guardrails --output /absolute/new/author --offline
node plugins/scripts/build-author-kit.mjs --plugin usagereports --output /absolute/new/typescript-author --offline
```

Go authors own their explicit module namespace. TypeScript authors need no Go
module, DTO source or compiler. Their kit carries packed `@qimu/sdk`,
`@qimu/node`, `@flame/backend-sdk`, the public UI/client packages, its own npm
lock and prebuilt development-only
contract generators and `pluginpack` release builder. Backend and component sources use separate TypeScript programs
projected from this repository's respective compiler configurations.
New features publish one owned descriptor; implementations of existing consumer
contracts reuse the published exact schemas. Native feature kits carry their
original `contracts/` projection, while `contractgen/` stays with the publisher.
Author `go generate` refreshes package and UI projections from that descriptor,
not from schemas re-derived after moving the implementation types. Memory uses the public
[`go-sdk/plugin/memory`](../packages/go-sdk/plugin/memory/README.md) contract, so its
author namespace cannot redefine the host's role. Go
production and tool dependencies are vendored from immutable workspace-member
module artifacts; SDK source and manifests are never rewritten or trimmed.
[Consumer distribution](../docs/reference/consumers.md) explains frozen candidate
versions, verified local materials and isolated offline execution. UI-backed
projects consume packed SDK/UI/icons packages and their own npm lock. JS
independent installation is separate from the Go offline guarantee.

Managed feature authors define exactly one `contracts/<name>.json` or
`contracts/<name>.ts` for each declared `<name>.generated.json` resource. A TypeScript
source exports its descriptor as the default value; it may derive schemas from the
same types used by its backend. Repository and exported-author generation share
one implementation. The generator uses the public exact JSON codec, preserves
wide numeric bounds, and publishes staged resources only after the development
tool succeeds. Independent descriptors require separate projection directories;
a component's generated method file has one descriptor owner. Tool-only backends
need no descriptor or contract generator. Native backends retain their Go-owned
generation path.

In the independent author directory:

```sh
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GONOPROXY=none GOFLAGS=-mod=vendor go test ./...
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GONOPROXY=none GOFLAGS=-mod=vendor go generate ./...
# UI-backed plugins additionally run npm ci --ignore-scripts, npm run typecheck and npm run build.
node build.mjs --source "$PWD" --os darwin --arch arm64 --output /absolute/new/packages --archives /absolute/new/releases
```

In an exported TypeScript author project:

```sh
npm ci --ignore-scripts
npm run generate
npm run typecheck
npm test
npm run build
npm run package -- --output /absolute/new/package --archives /absolute/new/releases
```

These commands run outside the Flame checkout without Go. The package builder
compiles a declared `backend/start.mjs` from author `backend/start.ts` with an
explicit compiler and includes its complete chunks, maps and package resources.
It rejects unresolved modules, computed module loading and native addons in
portable output. There is no dependency installation at activation.

The collection coordinator calls the same explicit source/output builder used by an exported author project. Install the resulting ZIP through the ordinary
Runtime `plugins.install` operation or CLI with its absolute Runtime-host path:

```sh
flame --runtime-url http://127.0.0.1:17171 plugins install --request '{"source":"/absolute/releases/plugin-flame.guardrails-1.0.0-darwin-arm64.zip"}'
```

Native ZIPs contain the platform's executable and declared resources, with
executable permissions preserved. Portable managed ZIPs have no platform suffix
or bundled interpreter. Both contain Qimu's neutral manifest, lock and complete
artifact inventory. That manifest supplies the sole immutable release digest;
Runtime verifies it and admits the matching Flame metadata interpretation.
Installation needs no Go, system Node, compiler, source checkout or dependency installation.
Choose the Runtime host's OS/architecture; native sandbox availability follows the
actual confinement mechanism. Package acquisition from a registry/URL and automatic
updates remain separate work.
Installed process entries must be packaged executables or managed Node entries;
ambient commands and installed HTTP backends are refused. Ordinary user-configured
MCP connections retain their existing transport support.

The exporter includes `backend`, `component`, owned `contracts` and `package`,
and generates the project README with its selected author commands.
Contract definition tools stay with their publisher; the author's pinned public
dependencies carry existing consumer contracts. Local generation refreshes package
and UI projections without deriving a replacement role from relocated Go types.
New author-owned features may define their own schemas and consumers explicitly.
Existing installations must explicitly admit/select rebuilt packages. Original pending work
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
npm ci
npm run build --workspace @flame/icons
npm run build --workspace @flame/ui
npm run build --workspace @flame/plugins
npm run check --workspace @flame/plugins
```

`build-components.mjs` uses the public SDK to build each `component/component.tsx` as
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

## Contract changes after private package consolidation

Go publishers derive schemas from their current owned types. Moving those types
changes Scope's qualified schema definition names and therefore the release
contract identity. Regenerate the canonical descriptor, SDK and client bindings,
backend resources and component together, then build and explicitly select the
complete new release. Former Go paths and schema aliases are not retained.
Descriptor-owned contracts, including Saved usage reports, keep their published
opaque definition names. Memory's implementation binds its canonical SDK contract;
only the separate canonical publisher can advance that contract. Native private
feature publishers follow the same ownership rule within their own plugin:
`make generate` runs the original `contractgen/` programs before regenerating
implementation, package and client projections. An exported author module binds
the published descriptor; it cannot redefine the original named contract merely
by deriving equivalent-looking schemas from its relocated local types.

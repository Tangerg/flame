# First-party portable packages

Memory and Schedules are complete optional backends. Their HTML views, MCP model
Tools and human Runtime commands all enter the same backend owner. Installing a
view does not create a second store in the host.

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
Full point value/applicability/order semantics, dependency status projection and
consumer/stream/component migration remain pending.


Build target-specific packages from the repository's local `go.work`:

```sh
node plugins/build.mjs --os darwin --arch arm64 --output /absolute/new/packages
```

The output directory must not exist. Supported targets are Darwin, Linux and
Windows on amd64 or arm64. The output contains the complete first-party catalog,
including all workspace pages and renderers and compiled Memory/Schedules backends.
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
and lifecycle contracts. Other packages contain only their declared content or presentation resources.

Backend source is compiled from `runtime/internal/plugins/memory` and
`runtime/internal/plugins/schedules` inside the local Runtime application module.
These are separate executables with private facts and process lifetimes, without
separate published Go modules or first-party version resolution.

## Shared page resources

All first-party workspace pages consume `@flame/ui`. `src/read-view.ts` owns each
admitted read projection, pending request and pagination history; individual pages
own only filters, disclosures and the rendering of their canonical data. Errors
retain the last accepted snapshot and cursor. Appearance preferences remain with
the Host, and its shared v3 bootstrap updates the existing page without rereading
business data for the remaining HTML pages. Diff, Usage and Icon gallery are ordinary
React components built through the public SDK. Usage reads the existing public
Runtime accounting query; Diff reads the canonical workspace query using a captured
public context. Icon gallery bundles the icon owner's canonical exports.
These packages require no own callable contribution or backend process.

Build `icons` and `ui` first, then run:

```sh
npm ci --prefix plugins
npm run build --prefix plugins
npm run check --prefix plugins
```

`build-components.mjs` uses the public SDK to build each `src/component.tsx` as
prebuilt ESM into its declared namespace resource. `--check` verifies exact
reproducibility. The graphical loader supplies the actual shared host modules.

`build-views.mjs` builds each `src/view.tsx` into its manifest-declared resource and
checks the Runtime page bound. Generated HTML is tracked for installation; source
and shared library styles are authoritative. The Desktop gate checks reproducible
resources and browser/native acceptance checks their real read-only channel. The
package remains private. Reinstall changed packages and explicitly select their new digests.

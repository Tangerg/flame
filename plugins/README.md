# First-party portable packages

Memory and Schedules are complete optional backends. Their HTML views, MCP model
Tools and human Runtime commands all enter the same backend owner. Installing a
view does not create a second store in the host.

A managed MCP server binds exactly one closed service kind. Separate service
kinds require separate declared servers, so invocation authority is unambiguous.

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
business data. Static icon content is rendered at build time from the icon owner.

Build `icons` and `ui` first, then run:

```sh
npm ci --prefix plugins
npm run build --prefix plugins
npm run check --prefix plugins
```

`build-views.mjs` builds each `src/view.tsx` into its manifest-declared resource and
checks the Runtime page bound. Generated HTML is tracked for installation; source
and shared library styles are authoritative. The Desktop gate checks reproducible
resources and browser/native acceptance checks their real read-only channel. The
package remains private. Reinstall changed packages and explicitly select their new digests.

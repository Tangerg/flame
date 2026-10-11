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
Windows on amd64 or arm64. Install `/absolute/new/packages/memory` and
`/absolute/new/packages/schedules`, then approve the exact release and enable it.
The source folders contain no compiled backend; installation never builds code or
installs dependencies. Executables are shipped in the admitted package.

See [Memory](memory/README.md) and [Schedules](schedules/README.md) for fact owners
and lifecycle contracts. Other packages contain only their declared content or
presentation resources and retain their existing build procedures.

Backend source is compiled from `runtime/internal/plugins/memory` and
`runtime/internal/plugins/schedules` inside the local Runtime application module.
These are separate executables with private facts and process lifetimes, without
separate published Go modules or first-party version resolution.

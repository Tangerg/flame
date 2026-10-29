# Native Desktop host

`apps/desktop` owns the Wails v3 host, native window/chrome, platform bridge,
local Runtime credential handoff and macOS packaging. The product UI is the shared
[Web application](../web/README.md), with architecture at
[Web ownership](../web/ARCHITECTURE.md). Browser and Wails do not have separate
product state or a second protocol client.

Use the Go version in `go.work` and the exact Wails v3 version selected in
`go.mod`. macOS is the shipping target. Framework v2 commands and generated
binding conventions do not apply.

```sh
# From the repository root
make setup
make dev-desktop
make check-native
make package-desktop
```

Root staging builds shared icons/UI, installed components and `apps/web/dist`,
then replaces ignored `assets/web`. The host embeds only that package-local
staging directory. Edit Web sources in `apps/web`; do not edit staged assets.
The native Taskfile delegates this shared graph to the root and owns platform
compilation, icon packaging and signing steps. Product name and slug belong to
`apps/web/src/app/product.ts`; native identifier, version, description and copyright
belong to `build/product.mjs`. `tooling/presentation.mjs` projects these owners and
the Web theme canvas into the native Go constants, Task metadata and both plists.
Run `make generate` after changing an owner; generated files are not packaging
configuration. Deliveries are under `out/desktop/darwin-<architecture>`.

Each macOS distribution targets one explicit `amd64` or `arm64` architecture.
`ARCH` selects `TARGET_ARCH` once; binary output, bundle output, Go compilation and
companions consume that same target, including cross-architecture builds.
The bundle's `Contents/MacOS` contains the Desktop executable, a separately
started `flame-runtime`, `qimu-runner`, the verified Node companion and first-party
plugin packages built for that same target. Packaging uses the existing pinned
Node artifact cache prepared by `make setup` and signs the complete bundle after
all companions are present. Universal packaging is rejected: joining only the
Desktop binaries would not produce a valid multi-architecture plugin release.
The Desktop does not silently start its packaged Runtime; deployment starts the
Runtime separately, and the Desktop attaches through its configured local
endpoint and credential handoff.

The actual exported `DesktopHost` receiver methods own the native IPC entrance.
`internal/bridge` owns its pure wire types, typed notification constants and value
refinements. `cmd/bridgegen` reads those declarations without compiling Wails,
reuses Qimu's public Schema/TypeScript renderer and emits the Web method,
type and Zod projection. Unsupported types or schema keywords fail generation.
The notification permission projection is generated in Web foundation so Browser
does not depend on the native platform adapter.

From `apps/desktop`, regenerate the bridge with:

```sh
go run ./cmd/bridgegen --source . --receiver DesktopHost \
  --out ../web/src/platform/desktop.generated.ts \
  --foundation ../web/src/foundation/notificationAuthorization.ts
go test ./cmd/bridgegen ./internal/bridge
```

These pure generator checks are self-contained within the Desktop module and run
on Linux as well as macOS. The repository's `make check-generated` and
`tooling/integration` compare the actual generator output with both checked-in
Web projections byte for byte. Full service binding and native I/O still require
`make check-native` on macOS. The Web adapter owns
in-flight bootstrap sharing, endpoint normalization, cancellation translation and unmeasured
window geometry; the generator does not replace those behaviors. Browser uses its
own host adapter. Selecting a remote Runtime does not grant native filesystem
identity to server paths. Runtime owns provider credentials and execution.

Each connection inspection that uses native credentials reads the current host
observation. The connection lifetime rejects a read that finishes after its
inspection is retired. User-configured credentials stay bound to their selected
endpoint; resetting the connection selects the live native observation again.
A Desktop opened before Runtime can therefore recover when Runtime creates its
token, without restarting the renderer. Failed native reads can also be retried.

Vite owns frontend HMR at the one `VITE_PORT` selected by the Taskfile. Wails
watches native Go sources and ignores generated Web staging. Teardown closes the
window generation and its client resources; the native host attaches to its configured local Runtime and reads its credential through local-runtime.

`SaveFile` accepts a suggested basename and canonical base64 bytes. The native
save panel owns the actual destination and reports completion only after the write
finishes; cancellation remains distinct from an error. This is the single file
save owner for text exports and binary plugin deliverables. It does not own a
plugin's document revisions, validation or delivery decisions. The specialized
inline-image decoder retains its existing image-saving behavior.

`make check-native` stages the same Web product, runs native Go test/vet/build
and the supported Wails build. A browser bundle alone is not native verification.

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
compilation, icon packaging, bundle metadata and signing steps. Packaging inputs
under `build/` are source. Deliveries are under `out/desktop/<platform>`.

The existing hand-written native bridge validates incoming/outgoing values with
Zod and Go contracts. Keep this as one bridge implementation. Browser uses its
own host adapter; selecting a remote Runtime does not grant native filesystem
identity to server paths. Runtime owns provider credentials and execution.

Vite owns frontend HMR at the one `VITE_PORT` selected by the Taskfile. Wails
watches native Go sources and ignores generated Web staging. Teardown closes the
window generation and its client resources; the native host attaches to its configured local Runtime and reads its credential through local-runtime.

`make check-native` stages the same Web product, runs native Go test/vet/build
and the supported Wails build. A browser bundle alone is not native verification.

# Workspace ownership

The repository root coordinates `go.work` and npm workspaces. It is not a Go
module. Each maintained Go module has an independent compilation and dependency
boundary; packages within one module follow cohesive ownership rather than a
module-per-directory rule.

| Module directory                   | Responsibility                                                    | Allowed first-party dependencies                         |
| ---------------------------------- | ----------------------------------------------------------------- | -------------------------------------------------------- |
| `packages/go-sdk`                  | Public values, protocol, client, plugin SDK and author generators | None                                                     |
| `packages/local-runtime`           | Runtime process and companion distribution contracts              | None                                                     |
| `plugins`                          | First-party plugin implementations and publisher contracts        | Go SDK                                                   |
| `runtime`                          | Product domain, application, adapters and delivery                | Go SDK, local-runtime; plugins only in integration tests |
| `apps/cli`                         | Workbench and CLI/terminal delivery                               | Go SDK, local-runtime, Runtime                           |
| `apps/desktop`                     | Native host, pure bridge DTOs and source generator                | Go SDK, local-runtime                                    |
| `tooling`                          | Repository coordination, API inventory and cross-product tests    | Required public modules                                  |
| `examples/plugins/reviews/backend` | Independent author namespace                                      | Go SDK                                                   |

Root npm workspaces own one lock. Exported author projects select their own
dependencies and locks. `module-candidates.json` maps an already selected module
version to immutable source; each dependent `go.mod` alone selects its version.
The coordinator freezes its own source and recursively reads dependencies from
those pinned module commits, then writes standard Go module artifacts. It never
uses a mutable checkout to replace a published candidate.

Runtime owns durable product behavior. `runtime/internal/delivery` registers each
operation's name, Go name, shape, kind, capability and failure policy once.
`runtime/cmd/contractgen` projects that registry into local/remote Go methods,
remote metadata, protocol schemas, TypeScript and references. Local and remote
calls enter the same server delivery semantics. The remote client never imports
the host, Domain, Application, adapters, bootstrap or server transport.

`packages/go-sdk/protocol` owns wire shapes, strict request/response admission, call
options and shared problem vocabulary. `packages/go-sdk/protocol/jsonrpc` owns the
shared envelope codec; server request metadata and handlers remain private.
`packages/go-sdk/identity` owns lexical declaration IDs and `packages/go-sdk/fingerprint` owns
pure digest encoding. Backend SDKs, ordinary SDK tests and author commands consume these public
mechanisms without importing Runtime. SDK enum declarations carry closed-vocabulary
markers; the Host generator resolves their typed constants. Derived runtime-event
sets remain projections of their existing descriptor owners. Generic schema/TS
rendering and feature generation live in `packages/go-sdk/contractgen`; host
registry walking remains in the Host command. Repository API discovery lives in
`tooling/goapi` and consumes generated method names from the manifest. Runtime production and ordinary testsupport never
import first-party implementation packages. Product integration uses public
bindings; private adapter/bootstrapping contract tests remain adjacent to the
boundary they exercise.

Web `app` composes product policy, `platform` supplies Browser/Wails effects,
`features` owns actual user capabilities and `extensions` supplies generic
registration/lifetimes/React hosting. Extension mechanisms never import product
features. Business point declarations, types and selectors live at the feature's
real public source. Connection owns endpoint/client/journal/generation retirement;
app injects product request metadata. Plugins owns installed component leases,
module evaluation and contribution retirement independently of Settings mounts.
Agent adapters retain wire-rich drivers and original opening retry handles;
Application/public views carry neutral input, receipt, key and display facts.

Settings composes public feature controls while features contribute Settings
panes. These are distinct modules and contracts in the same owner directories.
Compiler guards enforce real module cycles, cross-feature private imports and
inner-to-outer dependencies; collapsing whole directories to graph nodes would
incorrectly merge those independent contracts. Resolved Agent public types are
also checked through aliases, mapped types and method return types for SDK leaks.

Each plugin has only its actual `backend/`, `component/` and `package/` sources.
The builder accepts explicit source/output directories. Installation consumes an
assembled package in `out/`, never a source directory. Generated ESM and contracts
have the corresponding component/backend owner. Memory, Schedules and Skills
keep query/draft/command coordination separate from presentation without adding
a second command settlement mechanism.

Web feature construction captures explicit dependencies and creates each feature's mutable state and retirement in one owner instance. Rebinding a global port
slot cannot preserve state from a previous instance because such slots do not exist.

Desktop embeds generated `assets/web`, staged from the exact Web `dist` output.
Wails tasks delegate the shared build graph to the root. Native packaging inputs
remain source under `apps/desktop/build`; icon masters and brand originals remain
source under `packages/icons/source` and `assets/brand`.

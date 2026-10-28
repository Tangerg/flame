# Workspace ownership

The repository root coordinates `go.work` and npm workspaces. It is not a Go
module. Each maintained Go module has an independent compilation and dependency
boundary; packages within one module follow cohesive ownership rather than a
module-per-directory rule.

| Module directory                   | Responsibility                                                        | Allowed first-party dependencies     |
| ---------------------------------- | --------------------------------------------------------------------- | ------------------------------------ |
| `qimu`                             | Generic contracts, Host, lifecycle, drivers, storage and distribution | None                                 |
| `packages/go-sdk`                  | Flame values, protocol, client and product-specific author APIs       | Qimu                                 |
| `packages/local-runtime`           | Runtime process and companion distribution contracts                  | None                                 |
| `plugins`                          | First-party plugin implementations and publisher contracts            | Go SDK, Qimu                         |
| `runtime`                          | Product domain, application, adapters and delivery                    | Go SDK, Qimu, local-runtime          |
| `apps/cli`                         | Workbench and CLI/terminal delivery                                   | Go SDK, Qimu, local-runtime, Runtime |
| `apps/desktop`                     | Native host, pure bridge DTOs and source generator                    | Go SDK, Qimu, local-runtime          |
| `tooling`                          | Repository coordination, API inventory and cross-product tests        | Required public modules              |
| `examples/plugins/reviews/backend` | Independent author namespace                                          | Go SDK, Qimu                         |

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
options, shared problem vocabulary and strict RPC envelope admission. Native
JSON-RPC message types retain their released upstream owner; server request
metadata and handlers remain private.
`packages/go-sdk/identity` owns lexical declaration IDs and pure digest encoding. Backend SDKs, ordinary SDK tests and author commands consume these public
mechanisms without importing Runtime. SDK enum declarations carry closed-vocabulary
markers; the Host generator resolves their typed constants. Derived runtime-event
sets remain projections of their existing descriptor owners. Generic schema/TypeScript rendering and feature projection live in
`qimu/contractgen`, exposed by `qimu/cmd/qimu`. `qimu/portable` and
`qimu/jsonvalue` are pure value dependencies; the Qimu root owns Host and
Lifecycle mechanisms. MCP, SQLite, runner and distribution packages remain
infrastructure dependencies. Qimu source, tests and author commands cannot
import a consumer module. Flame backend projection retains only Flame policy;
Qimu owns its shared Go/TypeScript wire constants and descriptors.
Host registry walking remains in the Host command. Repository API discovery lives in
`tooling/goapi` and consumes generated method names from the manifest. Runtime production and ordinary testsupport never
import first-party implementation packages. Product integration uses public
bindings; private adapter/bootstrapping contract tests remain adjacent to the
boundary they exercise.

Web `app` composes product policy, `platform` supplies Browser/Wails effects,
`features` owns actual user capabilities and Qimu supplies generic registration, lifetimes and React hosting.
The consumer adapters never import product features to select providers. Business point declarations, types and selectors live at the feature's
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

Each plugin keeps executable composition in `backend/`, private Go behavior in
its feature-local `internal/`, optional browser UI in `component/`, and distributable
resources in `package/`. Cross-product fixtures consume published contracts and
real executables; they do not import private application, persistence or transport
packages. Memory retains its canonical publisher, while independent Reviews
retains the contract its analyzer consumes.
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

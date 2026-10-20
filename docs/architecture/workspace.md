# Workspace ownership

The root module is `github.com/Tangerg/flame`. Runtime, CLI, Go SDKs and first-party
backends share one coordinated source graph. Desktop and local-runtime retain
independent modules because they have distinct native/process contracts. The
Reviews backend has its own author module identity. Root npm workspaces own one
lock; exported author projects select their own dependencies and locks.

Runtime owns durable product behavior. `runtime/internal/delivery` registers each
operation's name, Go name, shape, kind, capability and failure policy once.
`runtime/cmd/contractgen` projects that registry into local/remote Go methods,
remote metadata, protocol schemas, TypeScript and references. Local and remote
calls enter the same server delivery semantics. The remote client never imports
the host, Domain, Application, adapters, bootstrap or server transport.

`runtime/protocol` owns wire shapes, strict request/response admission, call
options and shared problem vocabulary. `runtime/protocol/jsonrpc` owns the
shared envelope codec; server request metadata and handlers remain private.
`runtime/identity` owns lexical declaration IDs and `runtime/fingerprint` owns
pure digest encoding. Backend SDKs consume these public mechanisms without
importing Runtime Domain. Runtime production and ordinary testsupport never
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

Desktop embeds generated `assets/web`, staged from the exact Web `dist` output.
Wails tasks delegate the shared build graph to the root. Native packaging inputs
remain source under `apps/desktop/build`; icon masters and brand originals remain
source under `packages/icons/source` and `assets/brand`.

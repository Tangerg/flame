# Flame graphical client architecture

The same React application runs in a browser and a Wails WebView. Runtime owns
execution and durable product facts; the graphical client owns authoring,
navigation, projections, and presentation. The IDE is a separate consumer of the
same Runtime-owned TypeScript client.

Repository rules live in [`../../AGENTS.md`](../../AGENTS.md). The Runtime
contract lives in [`../../runtime/contract/`](../../runtime/contract/), including
its generated API reference. Visual rules remain in [`DESIGN.md`](DESIGN.md) and
[`DESKTOP_UI_POLISH.md`](DESKTOP_UI_POLISH.md). This document describes the
current ownership and dependency structure.

## Directory map

| Location | Responsibility |
| --- | --- |
| `src/main.tsx`, `src/App.tsx` | Construct the client environment, mount React, and supply the selected plugins |
| `src/main/` | Concrete connection ownership, product plugin selection, renderer startup and shutdown, protocol client identity |
| `src/platform/` | Browser/Wails bootstrap and actual native capabilities |
| `src/plugins/host/` | React integration with the plugin Host, installation readiness, disposal, slots, shortcuts, error boundaries |
| `src/plugins/sdk/` | Typed extension points, contribution policy, plugin contracts, selectors, installation state |
| `src/plugins/builtin/` | Product feature owners and their plugin contributions |
| `src/ui/primitives/` | Headless interaction contracts backed by Base UI |
| `src/ui/atoms/` | Token-dressed controls and shared visual vocabulary |
| `src/ui/agent/` | Presentation composites for the workbench and conversation |
| `src/lib/` | Shared technical mechanisms, including query caching, navigation ports, publication identity, task lifetimes, highlighting, and telemetry |
| `src/foundation/` | Consumer-neutral scalar contracts, including exact sequence values |
| `src/styles/` | StyleX tokens and the existing global stylesheets |
| `src/test/`, `visual/` | Test setup and explicit visual-fixture composition |
| `../../runtime/contract/typescript/` | Generated wire contract and reusable HTTP/SSE client |
| `../../icons/` | Original icon geometry, optical masters, SVG exports, and React components |

There is no graphical-client copy of the protocol implementation and no global
client locator. `main/` constructs dependencies and passes them outward through
plugin factories; consumers never import the composition root.

`src/ui/icons` owns the application's contributed-name mapping. Its names are
derived from that mapping, with every glyph supplied by `@flame/icons/react`.
`src/lib/fileKind` owns file classification. `src/ui/icons/fileIcon` selects the
file glyph used by the explorer, preview, and composer references.
The theme preference owner
publishes normalized UI font size through `lib/appearance`; `lib/iconScale`
derives both SVG pixel sizes and CSS layout tokens. The library chooses its
optical master from those pixels. Composer glyphs use an explicit fixed size,
without overriding the meaning of a global size token.

## Product owners

| Feature | Owns |
| --- | --- |
| `agent/` | Session and Run projections, streaming fold, input submission, interruption responses, recovery, and consumer-facing command/read ports |
| `chat/composer/` | Drafts, local attachments, authoring history, explicit Run overrides, and submission intent |
| Other `chat/` features | Conversation rendering, tool previews, message actions, Goal/Plan presentation, search, and related contributions |
| `providers/` | Provider/model catalog, exact model identity, configuration drafts, role selection, and configuration mutations |
| `runtime/` | Endpoint configuration, capability negotiation, connection generation, server scope, and mutation-journal storage installation |
| `navigation/` | Work Index projections and working-directory selection |
| `workspace/` | Workspace materials, view navigation, file and review reads, skill/memory curation, conversation export, and subscription invalidation |
| `settings/` features | Their own settings policies and presentation contributions |
| `shell/workbench/` | Product composition of chat, sidebar, settings, and dock surfaces |
| Other `shell/` features | Native chrome, notifications, status, route, and overlay contributions |
| `sidebar/` | Work Index rendering and actions |
| `theme/` | Appearance preferences, theme publication, and document appearance |
| `defaults/` | Default commands, titles, roles, and accents |

Providers are consumed by Composer, schedules, and setup prompts as well as
Settings. They therefore own a top-level feature directory and contribute their
configuration pane through the Settings extension point. Settings placement does
not own the provider catalog.

The workbench is product UI composition. The plugin platform remains in
`plugins/host` and `plugins/sdk`; product layout does not become a second kernel.

Workspace views are organized by the material they present:

```text
workspace/
  application/        navigation, queries, curation, export, subscription policy
  adapters/           Runtime translations, local actions, and stores
  domain/             deterministic workspace rules and values
  public/             cross-feature commands, queries, and navigation vocabulary
  ui/
    files/            file workspace, tree, and content
    review/           diff workspace and review presentation
    skills/           available skills, library, and proposals
    memory/           Agent memory presentation
    ViewHeader.tsx
    WorkspaceViewLayout.tsx
    viewStyles.ts
  views.ts            view contributions
  events.ts           workspace subscription installation
```

Trajectory analysis is a first-party portable package in `plugins/trajectory`, admitted by
Runtime and rendered through `settings/plugins-pane`'s scoped isolated carrier. Workspace
owns the contribution catalogue and full evidence export commands; it has no compiled
Timeline or dedicated trajectory data provider.

These UI groups share the existing Workspace owner. They do not each introduce a
new application, domain, adapter, or navigation stack.

## Dependencies and public surfaces

A feature uses only the rings it needs. Pure values and transitions belong in
`domain`; use-case ordering and consumer ports belong in `application`; external
translation belongs in `adapters`; view-model construction belongs in
`presentation`; rendering belongs in `ui`. A small contribution can remain one
file. Directory names do not create an architectural obligation to add layers.

Within an owner, dependencies point inward. Domain does not import application,
adapters, rendering, stores, or host effects. Application consumes its own ports,
not concrete adapters. An adapter may depend on a Domain or Application
contract; it never reaches back into `main` to locate a client.

Cross-feature consumers use the owner's `public/` modules. Public modules expose
that owner's language and supported behavior; they do not rename another
owner's types or re-export adapter implementations. Shared SDK vocabulary is
imported directly from the SDK. The small Settings presentation kit has existing
explicit entrypoints (`index.ts`, `panes.ts`, `settingStyles.ts`); its other
implementation files remain private without another forwarding directory.

`chat`, `command`, `settings`, and `shell` group independent feature owners.
Architecture checks classify those owners even when their directories are flat.
Shared conversation styling lives in `ui/agent/chatStyles.ts`, so the `chat`
namespace does not also act as an enclosing feature.

Static registration belongs beside the feature's plugin entry. A function that
only wraps an extension specification does not need an Application package.
Interfaces are shaped around real consumer needs. Neither a broad dependency
bag nor a parallel service locator is a substitute for explicit construction.

## Composition and connection lifetime

`main.tsx` chooses a `ClientHost`, constructs `createRuntimeConnection(host)`, and
passes that connection and host to `createBuiltinPlugins`. `App` supplies the
resulting list to `PluginProvider`. The Host knows how to install the supplied
plugins; it does not choose Flame's built-ins.

`ClientRenderer` owns host initialization, initial window chrome, the window
watcher, the React root, and final connection disposal. Closing the window or
retiring the module during HMR uses that same teardown path. A renderer retired
while bootstrap is pending cannot later mount or publish its bootstrap result.

The concrete Runtime connection owns a cached client for the exact normalized
endpoint, token, and mutation-journal storage identity. Replacing any of these
retires the previous client before successor work is issued. Final disposal joins
all outstanding retirements and reports their failures. A disposed connection
cannot create new clients.

Plugins receive a required provider such as `runtimeClient: () => FlameClient`.
The provider resolves the current connection. A query captures that client once
for the whole operation, including pagination or a provider-to-model lookup, so
one logical read cannot cross endpoints halfway through.

A long-lived mutation owner captures a gateway for its own generation. On a
Runtime replacement, only the current installed owner may construct the
successor gateway. The old generation is retired with its original gateway;
late replies cannot publish into the successor's projection. Merely replacing a
task queue while retaining its old gateway is insufficient.

Application ports use `lib/ports/singletonPort.ts` where a React or command
consumer requires a published binding. Each installation returns its disposer.
The owning plugin registers that disposer with `ctx.cleanup`. Publication and
withdrawal compare exact owner identity, so a predecessor's late cleanup cannot
remove a successor. This publication mechanism carries the installed value;
concrete owners retain task, error, cache, and mutation state.

`PluginProvider` tracks each installation generation independently of the plugin
array identity. Removing and later reinstalling the same list must wait for the
new Host to become ready. It stops only the Host it started, including when
startup finishes after the installation has been retired.

The installed-plugin list is a read-only projection of the current Host's
diagnostics. Installing, removing, or rolling back a plugin changes the list
through Dougong alone; the client has no separate registration path for names.
Dougong's `SnapshotPublisher` owns notifications to React and query consumers,
including observer failure isolation and reentrant publication ordering.

Plugins transfer each acquired resource to `ctx.cleanup` immediately. Dougong
owns rollback, reverse-order release, and failure aggregation; a plugin does not
collect a second disposer stack. Plugin subscriptions follow the Host during
renderer retirement and HMR. Only subscriptions created at module scope need a
separate module HMR hook.

Each package realization attempt owns its contributed views, themes, and event iterator
under one Dougong child lifetime. A failed attempt joins that lifetime before publishing
its failure, so a rejected page cleanup cannot strand sibling contributions or hide the
original read or event failure. The Runtime generation owns the observer task, allowing
it to join the contribution lifetime without waiting for itself. Retrying consumes only
the current failure; a consumed or retired failure cannot start another observer.

Extension contribution handles preserve Dougong's `update` operation. The SDK
translates the item into its existing envelope without withdrawing the
contribution or changing its domain key, owner, or precedence. Dynamic appearance
preferences update the same contribution, so readers never observe a temporary
missing theme. Extension point identity and key policy are captured and immutable.
Contribution options are captured when registered; `update` never rereads the caller's
options. Ordering belongs only to the contribution value's `order`; the registration
options and envelope carry no separate sorting hint.

Dougong's `SerialQueue` owns ordered execution. Client task cohorts partition
queues by product identity and fence retired generations, releasing pending
callers even when a remote operation ignores cancellation. Retirement cannot
start queued dependencies or publish a late result into a successor generation.

## Query ownership

React Query caching and `DATA_PROVIDER` lookup are shared mechanisms. Query keys,
read models, fetchers, Runtime translation, and registration belong to the
feature that understands the data:

- Agent registers Session and model-invocation reads.
- Workspace registers projects, files, diffs, skills, and memory reads.
- Providers registers provider/model catalogs, configuration, and role reads.
- Hooks registers inspection and trust reads.
- MCP registers its server and tool data through its own plugin.

Each feature registers once and removes its contribution with its own plugin
installation. Defaults has no business-data registry or cross-feature mapper.
Components consume query hooks and read models rather than RPC envelopes.

A complete list uses the SDK's paging surface. A deliberately bounded preview
names its limit. Awaiting one page and discarding its continuation is not a
complete catalog.

## State and command ownership

Runtime alone advances durable Session, Run, Segment, Item, Goal, Plan,
Interrupt, and execution state. The Agent context folds Runtime events into a
view, and recovery replaces that view from a coherent durable snapshot. Those
are projections of one authority, not independent product transitions.

The client owns authoring facts: drafts, attachment preparation, queued intent,
and unresolved command identity. Changing the endpoint scopes this local state
to the selected Runtime. A remote path describes the Runtime machine; a browser
file selection does not turn it into a server workspace.

The URL owns current navigation: active Session, main view, dock target, and
Settings pane. Stores retain tab sets, drafts, per-view state, and continuity.
Restored continuity can seed navigation; effects do not keep a second writable
copy of the same current-location value.

Provider identity is the exact provider/model pair. An existing Session supplies
its own durable selection. An explicit user choice in Composer becomes a Run
override; incomplete catalog reads do not overwrite the Session's selection.

Composer owns submission intent and accepts an injected send function. Agent
owns the send use case. Workbench consumers call Agent's public input surface
directly; Composer does not republish that same hook under another name.

A mutation's prepared parameters, idempotency key, and Runtime namespace survive
an uncertain acknowledgement. A transport failure does not authorize creating a
new command identity. The reusable client owns strict journal encoding and
replay rules; the graphical application supplies its storage lifetime and renders
the outcome. A delayed result may settle its original command, but cannot
advance a retired view generation.

## Runtime events and interruption

The shared client validates wire values and owns HTTP/SSE transport. Agent fold
routes the resulting events directly to its own projection handlers. Plugins
cannot register event reducers or replace the core Agent view. View models
preserve exact sequence and identity values; timestamps come from the event
that established a fact rather than the client clock.

An accepted start establishes the Runtime's Run, Segment, and opening user Item.
The returned Item identity reconciles optimistic input. Steer is reconciled by
its reserved Item identity; identical text is not command identity.

A waiting Run exposes durable interrupts. The client submits responses against
those exact identities; Runtime owns admission and continuation. Closing a
client releases its transport and projection resources. It does not stop the
Runtime or cancel accepted execution. Cancellation is an explicit command.

An accepted resume settles local submission identity, then replaces its original
transport with the existing atomic Session snapshot and Run tail. Answers,
approval decisions, and pending interrupts come from that material and subsequent
Runtime events. Snapshot failure after acceptance remains a synchronization
failure; it cannot turn an accepted command into a new submission. A connection
failure in recovery or reattachment is reported to the Runtime connection owner,
which controls generation replacement and reconnection.

Workspace subscriptions publish invalidation, not a second filesystem database.
The Workspace owner manages subscription replacement and reconnect, then
revalidates the affected reads. A failed or retired stream cannot claim current
workspace state from a stale callback.

## Native and browser boundaries

`platform/clientHost.ts` selects the environment before bootstrap. The desktop
host validates native IPC replies. The browser host provides same-origin
bootstrap and browser downloads without importing Wails into the protocol SDK.

Only a connection matching the Runtime bootstrapped by Wails may use native
workspace Open, Reveal, or folder selection. These actions recheck current
locality when invoked. A connection replacement fences late picker results.
The relevant features consume narrow local-action, notification, window, and
image-save ports rather than a host bag.

The Go package under `desktop/` remains a small Wails host. Its files name actual
host responsibilities. Runtime execution, domain packages, and persistence are
not copied into it. Native method names are a cross-language contract guarded by
`binding_names_test.go` and the validated TypeScript host bridge.

## Design system and observability

The presentation dependency direction is primitives, atoms, then Agent
composites. Business UI consumes dressed controls, not headless library internals.
Shared UI does not fetch Runtime data or read plugin-owned state. StyleX and the
existing global stylesheets remain the styling mechanism; this ownership
refactor does not introduce another visual system.

Observability installs through an explicit product plugin. It owns telemetry
setup and cleanup, while shared telemetry mechanisms stay in `lib/observability`.
Transport and execution instrumentation remain at their respective boundaries;
no per-token UI instrumentation is needed to express these lifetimes.

## Verification

Run the complete frontend gate from this directory:

```sh
npm run check
```

The gate includes typechecking, lint, formatting, tests, dead-code checks,
architecture, content and visual rules, and a production bundle. The HTTP e2e
suite builds an isolated Runtime and uses local fake providers; it requires the
repository's Go toolchain and no live provider credentials.
It runs in a separate Vitest project after the frontend checks so native process
and local server deadlines do not compete with the DOM worker pool.

Layer, context, and cycle checks share one compiler-resolved dependency graph.
Every TypeScript source must be loaded, every local import must resolve, and
runtime cycles use the frontend's TypeScript erasure rather than matching
filenames or guessing from import text. Flat features receive the same boundary
as layered ones. Port usage is attributed through TypeScript symbols, so an
unrelated method with the same name cannot satisfy an unused interface clause.
Source coverage is checked against actual files, not minimum file or edge counts.

Guard regression tests cover unresolved imports, omitted source files, module
extensions, type-only erasure, real cycles, private feature dependencies, and
port consumers. Product tests cover failure ordering, installation replacement,
connection retirement, command identity, and surviving feature behavior.

Visual fixtures compose their own plugins and memory-transport clients. Their
resources belong to fixture cleanup; they do not discover a default production
connection. Native package verification follows the supported Wails target in
[`../Taskfile.yml`](../Taskfile.yml); a successful Web bundle does not establish a
macOS packaging result.

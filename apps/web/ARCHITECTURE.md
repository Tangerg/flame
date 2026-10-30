# Flame graphical client architecture

The same React application runs in a browser and a Wails WebView. Runtime owns
execution and durable product facts; the graphical client owns authoring,
navigation, projections, and presentation. Browser and Wails environments use the
same Runtime-owned TypeScript client.

Repository rules live in [`../../AGENTS.md`](../../AGENTS.md). The Runtime
contract lives in [`../../runtime/contract/`](../../runtime/contract/), including
its generated API reference. Visual rules remain in [`DESIGN.md`](DESIGN.md) and
[`DESKTOP_UI_POLISH.md`](DESKTOP_UI_POLISH.md). This document describes the
current ownership and dependency structure.

## Directory map

| Location                         | Responsibility                                                                                                                                |
| -------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `src/main.tsx`, `src/App.tsx`    | Construct the client environment, mount React, and supply the selected plugins                                                                |
| `src/router.tsx`, `src/pages/`   | TanStack route wiring and the page shells it mounts                                                                                           |
| `src/app/`                       | Composition, product plugin selection, renderer startup and shutdown, protocol client identity                                                |
| `src/platform/`                  | Browser/Wails bootstrap and actual native capabilities                                                                                        |
| `src/extensions/host/`           | React integration with the plugin Host, installation readiness, disposal, slots, shortcuts, error boundaries                                  |
| `src/extensions/`                | Generic point registration, contribution lifetimes, host contracts and selectors                                                              |
| `src/features/`                  | Product feature owners and their plugin contributions                                                                                         |
| `../../packages/ui/`             | Shared UI package: first-batch controls, token declarations, document rules and appearance projection                                         |
| `src/ui/atoms/`                  | Product presentation composites consuming `@flame/ui`                                                                                         |
| `src/ui/agent/`                  | Presentation composites for the workbench and conversation                                                                                    |
| `src/lib/`                       | Shared technical mechanisms, including query caching, navigation contracts, publication identity, task lifetimes, highlighting, and telemetry |
| `src/foundation/`                | Consumer-neutral scalar contracts, including exact sequence values                                                                            |
| `src/styles/`                    | Product stylesheets and application palette defaults                                                                                          |
| `src/test/`, `visual/`           | Test setup and explicit visual-fixture composition                                                                                            |
| `../../packages/runtime-client/` | Generated wire contract and reusable HTTP/SSE client                                                                                          |
| `../../packages/icons/`          | Original icon geometry, optical masters, SVG exports, and React components                                                                    |

There is no graphical-client copy of the protocol implementation and no global
client locator. `app/` constructs dependencies and passes them outward through
plugin factories; consumers never import the composition root.

`foundation/protocolValues.generated.ts` publishes the closed scalar vocabularies
shared by client read models. Runtime contract generation derives this limited
projection from the same Go enum and schema owners as the wire binding, without
bringing Runtime records or operation handles into the client models.

The pure `@qimu/sdk/json` entry supplies exact JSON data handling.
Its checked dependency closure excludes Runtime calls and wire records; those
remain translated at adapters rather than entering context application models.

`src/ui/icons` owns the application's contributed-name mapping. Its names are
derived from that mapping, with every glyph supplied by `@flame/icons/react`.
`src/lib/fileKind` owns file classification. `src/ui/icons/fileIcon` selects the
file glyph used by the explorer, preview, and composer references.
The theme preference owner
publishes normalized UI font size through `lib/appearance`. `@flame/ui/font-scale`
owns shared icon sizing; `lib/productIconScale` projects that scale into product
CSS and adds the fixed composer size. `@flame/icons` chooses its optical master
from those pixels. Composer glyphs use an explicit fixed size,
without overriding the meaning of a global size token.

## Product owners

| Feature                     | Owns                                                                                                                                    |
| --------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| `agent/`                    | Session and Run projections, streaming fold, input submission, interruption responses, recovery, and consumer-facing command/read ports |
| `composer/`                 | Drafts, local attachments, authoring history, explicit Run overrides, and submission intent                                             |
| Other `chat/` features      | Conversation rendering, tool previews, message actions, Goal/Plan presentation, search, and related contributions                       |
| `providers/`                | Provider/model catalog, exact model identity, configuration drafts, role selection, and configuration mutations                         |
| `connection/`               | Endpoint configuration, capability negotiation, connection generation, server scope, and mutation-journal storage                       |
| `navigation/`               | Work Index projections and working-directory selection                                                                                  |
| `workspace/`                | Workspace materials, view navigation, file and review reads, conversation export, and subscription invalidation                         |
| `settings/` features        | Their own settings policies and presentation contributions                                                                              |
| `shell/workbench/`          | Product composition of chat, sidebar, settings, and dock surfaces                                                                       |
| Other `shell/` features     | Native chrome, notifications, native window/document presentation and overlays                                                          |
| `sidebar/`                  | Work Index rendering and actions                                                                                                        |
| `theme/`                    | Appearance preferences, theme publication, and document appearance                                                                      |
| `app/` and feature defaults | Product composition and owner-specific defaults                                                                                         |

Providers are consumed by Composer, schedules, and setup prompts as well as
Settings. They therefore own a direct feature directory and contribute their
configuration pane through the Settings extension point. Settings placement does
not own the provider catalog.

The workbench is product UI composition. The plugin platform remains in
`extensions/host` and `extensions`; product layout does not become a second kernel.

Installed components may consume their own Runtime point contracts through the
public SDK's captured `compose` operation. Independently installed packages export
components without host placements and contribute same-release references/data.
Runtime owns membership, applicability and order; the existing graphical package
adapter realizes exact bindings through the common module loader and render
lifetime. Composed descendants inherit captured resources, receive their own
identity/callable binding, and cancel/join on replacement. Window-owned ESM requires
reload after evaluated resource retirement or version/generation replacement;
retired exports cannot be reused. This introduces no parallel contribution store
or client business authority.
Composition observation follows captured context and child lifetimes, including
effect teardown and replay. Module acquisition is joined before a connection
generation retires; only ESM evaluation escapes that join. Evaluation admission
atomically captures its generation/release, and acquisition failures do not enter
the window's permanent evaluation cache.

Workspace views are organized by the material they present:

```text
workspace/
  application/        navigation, queries, curation, export, subscription policy
  adapters/           Runtime translations, local actions, and stores
  domain/             deterministic workspace rules and values
  public/             cross-feature commands, queries, and navigation vocabulary
  ui/
    files/            file workspace, tree, and content
    skills/           trusted document selection and Skill curation forms
    ViewHeader.tsx
    WorkspaceViewLayout.tsx
    viewStyles.ts
  views.ts            view contributions
  events.ts           workspace subscription installation
```

Trajectory analysis is an ordinary portable React component admitted from an exact
Runtime release. It consumes public Session/evidence operations and captured target
context. Workspace owns navigation and export commands; the generic loader owns
module/resources and lifetime rather than trajectory business data.

Small features keep cohesive files directly under their feature directory:
`chat/context-usage`, `chat/plan-progress`, `chat/narrative-rails`,
`navigation/session-search`, `composer/quote-selection`, and `composer/slash-hints`.
`workspace/toolViewOpener.ts` is a single contribution; it needs no wrapper folder.
Layer checks apply to these flat features as well as larger layered ones.

These UI groups share the existing Workspace owner. They do not each introduce a
new application, domain, adapter, or navigation stack. Memory's published client consumes its generated feature contract and public
Runtime client. Workspace adapters translate that client into their application
ports; Workspace's public surface exports application commands and queries.

Scheduling presentation and templates live in `plugins/schedules`. Its ordinary
component owns authoring forms, inspected revisions and original command handles,
and enters the backend's public operations. Templates only initialize new drafts.
The declared backend background contribution owns wake-ups, fair pending scans and
dispatch; Runtime owns Session/Run acceptance. Withdrawal stops new background
work and retires component observation; accepted Runs continue under Runtime.

Usage presentation lives in `plugins/usage`. The ordinary component reads public
Runtime accounting summaries for its chosen period and owns only local filters and
accepted snapshots. Runtime remains the sole accounting owner. Contribution
retirement cancels/joins reads; there is no read-only page bridge or second cache.

Diff presentation lives in `plugins/diff`. Runtime owns workspace/Git reads and
resource bounds. The captured component target determines the workspace and file;
local React state owns layout, filtering and folding. Navigation resolves admitted
Workspace view contributions with `material: "diff"` and owns no separate renderer
registration. The former compiled Diff view and guest carrier are removed.

## Dependencies and public surfaces

A feature uses only the rings it needs. Pure values and transitions belong in
`domain`; use-case ordering and consumer ports belong in `application`; external
translation belongs in `adapters`; view-model construction belongs in
`presentation`; rendering belongs in `ui`. A small contribution can remain one
file. Directory names do not create an architectural obligation to add layers.

Within an owner, dependencies point inward. Domain does not import application,
adapters, rendering, stores, or host effects. Application consumes its own ports,
not concrete adapters. An adapter may depend on a Domain or Application
contract; it never reaches back into `app` to locate a client.

Cross-feature consumers use the owner's `public/` modules. Public modules expose
that owner's language and supported behavior; they do not rename another
owner's types or re-export adapter implementations. Shared SDK vocabulary is
imported directly from the SDK. Settings owns its public pane point and registration. Shared settings row/group presentation lives in `ui/settings`; each feature owns its preference controls and values.

Each direct `features` directory is an actual owner. Chat groups its conversation rendering submodules; Composer, Goal, notifications and command/navigation owners remain independent.
Shared conversation styling lives in `ui/agent/chatStyles.ts`, so the `chat`
namespace does not also act as an enclosing feature.

Static registration belongs beside the feature's plugin entry. A function that
only wraps an extension specification does not need an Application package.
Interfaces are shaped around real consumer needs. Neither a broad dependency
bag nor a parallel service locator is a substitute for explicit construction.

## Composition and connection lifetime

`main.tsx` chooses a `ClientHost` and constructs the mutation-journal storage before
`createRuntimeConnection(host, runtimeRequestMeta, storage)`. It creates the navigation,
Agent, Composer, Workspace, working-directory, Notifications, Theme, and InstalledPlugins
instances with their explicit collaborators. `createBuiltinPlugins` binds those same
instances into plugin commands; `App` supplies the resulting list to `PluginProvider`.
The Host installs the supplied contributions and starts their owners; it does not
choose Flame's built-ins or construct a second copy of their state.

`ClientRenderer` owns host initialization, initial window chrome, the window
watcher, the React root, and final connection disposal. Closing the window or
retiring the module during HMR uses that same teardown path. A renderer retired
while bootstrap is pending cannot later mount or publish its bootstrap result.
The startup-failure root follows this same lifetime: a late rejection cannot
replace a successor renderer, and the failure page is unmounted on teardown.

The concrete Runtime connection owns a cached client for the exact normalized
endpoint and token. Its mutation-journal storage is required at construction and
remains owned by that connection. Changing the target retires the previous client
before successor work is issued. Final disposal joins
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

React consumes the corresponding feature Provider; ordinary commands capture the
required access object at construction or receive it as an argument. There is no
module-global port installation, default feature instance, or application service
locator. Each owner creates its stores, persistence scope, task cohorts, subscriptions,
and generation publication together. Providers expose that instance rather than
creating another store. Settings panes only consume these dependencies.

Provider settings, MCP servers, Goal commands and composer mode, project hook
trust, and message feedback follow this lifetime explicitly. Renderer composition
constructs their owners, plugins bind and retire Runtime generations, and renderer
shutdown disposes the owners. UI commands capture the admitted generation rather
than resolving a mutable current service when a deferred callback runs.

The Agent owns session/view state, command and usage generations, shared material,
opening retries, and interruption coordination. Composer owns draft persistence,
image staging, and focus/presentation leases. Workspace owns Dock persistence,
layout state, local effects, and conversation archive generations. Connection owns
negotiated capability/status projections, bootstrap leases, and retiring clients.
Notifications owns authorization requests, completion subscriptions, and notification
preferences. InstalledPlugins owns package operations, realization failure, component
modules, and its joined retirement queue.

`createTheme(document, window)` owns preferences, system appearance, font detection,
and a read-only appearance projection. Its plugin starts the document painter and
registers release; renderer shutdown disposes the actual Theme instance. Motion,
icons, and highlighting consume the same projection through Theme's Provider.

Image saving uses one narrow, stateless callback Context because attachment,
Markdown-image, and SVG renderers share a deep gallery consumer. The callback is
constructed at the application boundary; the Context only passes it through and
has no installation protocol, mutable service slot, or independent state. Error
classification and Task readout remain pure functions.

Installed components receive an explicit `saveFile` collaborator captured at the
composition root. The existing Browser/Native host owns file transfer and the
user-selected destination; Workspace text exports encode UTF-8 through that same
owner. Plugins supply exact bytes and keep their document, review and delivery
facts private. Browser download dispatch is `requested`; only a completed native
write is `saved`, and native cancellation is `cancelled`. Component and Qimu Use
lifetimes join accepted saves and refuse new saves after retirement. No file
ledger, artifact state or download implementation is added to the client.

Publication identity remains private to owners that need generation retirement.
A predecessor's late reply or cleanup cannot publish into, or withdraw, its successor.
Plugin cleanup and renderer teardown converge on the same idempotent disposers.

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
The generation reads Runtime admission directly before reconciling its resources and
publishes that snapshot into the Settings query cache. Cancelling or refreshing that
cache cannot cancel realization or create a contribution transition.

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
- Workspace registers projects and files. Installed Memory and Skill components read their own generated publisher contracts through release-bound operations; drafts, pending command observation and accepted pages belong to those components. Workspace has no parallel business cache, mutation gateway or catalog page.
- Providers registers provider/model catalogs, configuration, and role reads.
- Hooks registers inspection and trust reads.
- MCP registers its server and tool data through its own plugin.

Each feature registers once and removes its contribution with its own plugin
installation. Defaults has no business-data registry or cross-feature mapper.
Components consume query hooks and read models rather than RPC envelopes.

Agent usage query keys include the owning instance. Replacing a gateway or
retiring its Workspace observation only invalidates that instance's usage
queries, even when another renderer reads the same Session ID.

Each data provider owns its read generation. Adding another provider to the same Host
preserves unchanged providers and their pending reads. Replacement or removal retires
only the affected provider; Host replacement retires every predecessor provider. Cache
handoff checks that exact provider identity, so unrelated contributions cannot strand it.

A complete list uses the SDK's paging surface. A deliberately bounded preview
names its limit. Awaiting one page and discarding its continuation is not a
complete catalog.

## State and command ownership

Runtime alone advances durable Session, Run, Segment, Item, Goal, Plan,
Interrupt, and execution state. The Agent context folds Runtime events into a
view, and recovery replaces that view from a coherent durable snapshot. Those
are projections of one authority, not independent product transitions.

Goal presentation preserves the Runtime's stopping code and safe detail from the
same Session snapshot. Status wording and stopping explanations are derived from
that material; renderer state and connection failures cannot author a new reason.

Terminal Run presentation distinguishes the published local outcome from unresolved
external effects. Root and delegated narratives render the same read-only evidence;
observed output stays an exact document and never becomes a final ToolResult or an
instruction to resume the finished Run.

The client owns authoring facts: drafts, attachment preparation, queued intent,
and unresolved command identity. Changing the endpoint scopes this local state
to the selected Runtime. A remote path describes the Runtime machine; a browser
file selection does not turn it into a server workspace.

The URL owns current navigation: active Session, main view, dock target, and
Settings pane. Stores retain tab sets, drafts, per-view state, and continuity.
Restored continuity can seed navigation; effects do not keep a second writable
copy of the same current-location value.

The Workbench owns the persistent window header above each full-width workspace
view. It derives identity and navigation from the admitted view specification and
keeps recovery actions outside the content's scrollport, loading state, and error
boundary. Pages own their content headers, filters, and actions; file identity
and metadata stay in the file content bar. AppShell derives whether a sidebar is
expanded, collapsed, or absent and reserves only the corresponding window controls.

Provider identity is the exact provider/model pair. An existing Session supplies
its own durable selection. An explicit user choice in Composer becomes a Run
override; incomplete catalog reads do not overwrite the Session's selection.
Composer sends its explicit provider/model pair and authored reasoning effort
directly from its preference owner. Catalog reads only supply labels and capability
availability. A missing model or retired effort remains the authored override;
only a new user selection initializes defaults from the chosen catalog entry.

Composer owns submission intent and accepts an injected send function. Agent
owns the send use case. Workbench consumers call Agent's public input surface
directly; Composer does not republish that same hook under another name.

A mutation's prepared parameters, idempotency key, and Runtime namespace survive
an uncertain acknowledgement. A transport failure does not authorize creating a
new command identity. The reusable client owns strict journal encoding and
replay rules; the graphical application supplies its storage lifetime and renders
the outcome. A delayed result may settle its original command, but cannot
advance a retired view generation.

A null connection generation withdraws callable Provider, MCP, and Hook mutation
owners. Discovery must establish a successor before installing writable owners
again. MCP configuration drafts capture one concrete generation's command handle
when opened and retire with it. Sequential JSON import belongs to that same
Application handle, which reports its confirmed completed prefix if retirement
interrupts the batch. A successor never receives the remainder of an old intent.

Cancellation projects the Runtime's committed canceled Run. A `run_finished`
refusal followed by a successful terminal read preserves the committed outcome;
other command failures remain visible even when a terminal Run is readable.
Terminal-query failures belong to synchronization and are reported separately.
Neither a Stop click nor a command failure authors a Run outcome. Full Session
reads restore the same terminal evidence when the interface is reopened.

Steering retains the SDK's original mutation and retry authority. The SDK caches
Runtime's opaque Item nomination before dispatch and exposes it before ACK; the
Agent binds the authored message to that identity without inferring acceptance.
Equal payloads remain distinct commands. Retry never changes the original body,
key, namespace or target Segment.

Runtime receipts and local delivery observations are separate facts. A late
inconclusive request cannot erase acceptance read from Runtime. Only the exact
committed User Item proves model-context application, even if ACK has not arrived.
Only a coherent terminal Session read proves non-application for an accepted
input with no such Item; a separate terminal Run observation cannot prove Item
absence. The snapshot projector owns that read-only proof, and command callbacks
cannot change it. Retired material remains unavailable. Session snapshots retain all current
Steer origins, including applied inputs, so live observation and reopening render
the same conclusion. Root and delegated messages share the same derived feedback.
Connection replacement retires command observers and retry actions while the SDK
preserves unresolved journal evidence; committed Items survive that retirement.
Desktop stores no independent input-application transition.

Start and Resume query `runs.getAcceptance` with their original namespace, key,
and operation before delivery. The journal reserves identity independently of
dispatch admission: replay expiry and a replaced Runtime store forbid a write,
but do not prevent a receipt read or renew the original deadline. A failed read
retains uncertainty; `notRecorded` never grants a fresh key. Journal records use
the current strict shape. Start, Resume and Steer persist their complete frozen
requests, including input images, model options, reviewed answers, edited arguments,
reasons and standing-policy choices. Other mutations retain identity without their
request body. These client records contain authored content and must use the existing
application storage lifetime. A definite receipt or refusal settles the record;
inconclusive reads, expiry and store replacement preserve it.

`runs.recoverCommands()` enumerates those original Run commands and reads their
receipts without dispatch. Its explicit retry uses the captured original request,
key, namespace and deadline. Session mounting renders unresolved opening submissions
with those original choices and blocks a fresh Start or Resume until the original
outcome is resolved. After a committed opening, Agent refreshes coherent Session
material and uses the existing subscription owner for its current Segment. Acceptance
never depends on observing that stream. The opening list and retry actions are
ephemeral SDK projections; Desktop adds no durable command ledger.

The former Steer-only journal shape and recovery API are removed. Incompatible
records fail closed and remain intact; no reader reconstructs omitted request bodies
or migrates unresolved work to a new identity. Retain prior journal evidence when
upgrading the SDK and Desktop together.

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

The SDK returns an opening receipt separately from its observation resource:
`stream` owns the original response stream, `snapshot` requires recovery through
Session material, and `retired` means the original material is unavailable.
Snapshot recovery follows the current Run and Segment from coherent material,
without rewriting the original opening references. A retired receipt settles
the local submission and refreshes Session material without attaching a Run
that reused its ID. Submitted response batches settle from opening results;
waiting-set withdrawal and projection generations cannot acknowledge them.

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
image-save dependencies rather than a host bag.

The Go module under `apps/desktop` remains a small Wails host. Its actual
`DesktopHost` receiver methods own the method list; `internal/bridge` owns the pure
wire DTOs and their existing refinements. `cmd/bridgegen` reads the actual method AST,
uses Qimu’s public schema/TypeScript renderer, and generates
`platform/desktop.generated.ts` plus `foundation/notificationAuthorization.ts`.
The host adapter validates those generated wire contracts. Native
`bridge_contract_test.go` compares the actual binding surface; pure generator drift
checks run on Linux without compiling Wails. Runtime execution and persistence
remain with Runtime.

## Design system and observability

The presentation dependency direction is atoms, then Agent composites. The
control owner imports Base UI directly; business UI consumes dressed controls.
Shared UI does not fetch Runtime data or read plugin-owned state. StyleX and the
existing global stylesheets remain the styling mechanism; this ownership
refactor does not introduce another visual system.

Observability installs through an explicit product plugin. It owns telemetry
setup and cleanup, while shared telemetry mechanisms stay in `lib/observability`.
A telemetry session owns its SDK providers, global registrations and instruments.
Replacement waits for the previous session to close; an older teardown cannot
unbind a later provider. Metric identity includes instrumentation scope and
structured attributes, preserving distinct values without delimiter collisions.
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

Layer, context, and cycle checks share the compiler-resolved dependency graph in
`tooling/source-graph.mjs`. Generic graph tests belong to tooling; Web policy tests
belong to `scripts/check-layers.test.mjs`.
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

`test/renderClient.tsx` constructs independent feature instances for each test.
Visual fixtures use `visual/client.tsx` and pass the fixture explicitly to their
setup functions; browser checks read that fixture rather than private production
singletons. Visual fixtures compose their own plugins and memory-transport clients. Their
resources belong to fixture cleanup; they do not discover a default production
connection. Native package verification follows the supported Wails target in
[`../Taskfile.yml`](../desktop/Taskfile.yml); a successful Web bundle does not establish a
macOS packaging result.

## Shared UI and portable appearance

`@flame/ui` owns shared control behavior and presentation. Desktop and ordinary
portable components consume its compiled components, stylesheet and actual shared
React/provider instances. Icons retain their independent authoring owner.
Appearance selection and preference persistence stay in `theme/`; components inherit
the workbench's current theme, typography and controls. No guest token channel,
read-only bootstrap or private appearance projection remains.

## Neutral Agent types

`features/agent/adapters/runtimeDriver.ts` owns the wire-rich RuntimeAgentDriver.
Runtime sources and selection remain inside Agent adapters. Application and public
views expose neutral input, interruption and session/run facts. Pending opening
views contain the original key, receipt and renderable request. Only the adapter
of that generation retains the SDK retry handle; `retryOpening(key)` resolves that
original handle without creating a second settlement path. Generated closed string
enums such as `ChangeStatus` are shared vocabulary and may appear in public views;
Runtime-client records, command handles and anything mixing them stay inside adapters.
Compiler checks trace exported types through aliases, containers and inferred
method results.

Settings pages compose theme, i18n, notification and chat controls through their
real public modules. Features independently contribute panes to Settings. Guards
check resolved module cycles and private imports, rather than treating an entire
feature directory as one coarse initialization node. Installed plugin component
lifetimes belong to `features/plugins` and survive Settings page unmounts.

## Owned consumer capabilities

The Agent adapter retains its runtime gateway, commands, session projection,
shared-material writers and interrupt coordinator. It publishes bound business
operations and read subscriptions through `AgentAccess`. Its instance-owned
React provider supplies the implementation context to Agent hooks; `useAgent`
returns only the consumer capabilities. Composition owns `start` and `dispose`.
Tests compose their mutable graphs in `src/test/agentFixture.tsx`; production has
no testing backdoor to its dependency aggregate.

Each extension host creates its own `Configuration` and `WindowTitle` service.
Configuration retains values by copy and publishes copied notifications; callers
cannot mutate the retained map or subscribers. Window title behavior owns its
private title, badge and activity state.

UI atoms own both control semantics and presentation. They import Base UI at
the actual control owner, or implement native semantics when needed. Tree's
keyboard, ARIA and styling live together in `ui/atoms/tree.tsx`. The design-system
gate keeps native interaction and Base UI inside the design system.

## Client persistence encoding

Plugin storage keys encode the plugin namespace as a JSON tuple before appending
its opaque key. Hierarchical plugin names cannot alias another plugin's keys or
expand its `clear` authority. The former dotted namespace encoding is not read,
migrated or deleted. Current mutation records also require original request
metadata captured at admission; retry and recovery keep that original value.
Upgrading this development client is a breaking storage change.

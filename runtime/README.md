# Flame Runtime

Flame Runtime is the local product backend for Flame. It owns durable agent semantics and exposes the same behavior through an in-process Go binding and the Runtime Protocol.

Atomic Session/Run openings admit an omitted workspace and use the same default
workspace projection as ordinary Session creation. An explicitly supplied empty
workspace remains invalid. Acceptance lookup retains the original omission and
request identity; retries do not resolve a different default into that request.

Installed model Tool dispatch commits its immutable caller/source alongside the
original pre-call observation. `installed_tool_callers` retains only logical
caller, installed Tool, release and source authority, independently of transient
checkpoints and deleted Session/Run material. Background restoration checks that
exact evidence against the authenticated current Actor; it cannot invent another
caller, borrow a replacement release or resurrect a completed execution member.
Published commands retain their existing routing owner. Neither record contains
a plugin business result. The additional private routing schema requires a fresh
Runtime data directory; incompatible directories are refused without conversion
and must be retained separately.

Post-segment maintenance captures its finishing Session and workspace explicitly.
A new root opened by another Run's plugin call cannot inherit that caller's
Session, Goal or physical-model invocation for detached title generation.

Runtime is not another agent framework. Scope owns process execution, strategies, tools, and provider libraries; Runtime adapts those capabilities to Flame's Session, Run, Goal, Plan, persistence, recovery, and protocol model.

Scope v0.58.0 owns complete Tool failure classification. Runtime uses
`tool.ClassifyFailure` for dispatch and original-result recovery: independent
joined errors cannot establish a definite result. A plugin's classified Failure
retains its diagnostic cause inside that outcome instead of joining it as a
second outcome. Accepted human answers use the Engine's public Signal constructor
for exact receipt reconciliation and the current wait's `ResponseSignal` for
fresh admission.

Unknown catalog pricing preserves unknown cost, including an empty rate schedule
with reported zero usage. Model listings expose effective base-band cache rates:
an omitted catalog cache rate uses the input rate, while an explicit zero is free.
Scope owns cost calculation and threshold selection; historical settled costs
are not repriced. Pricing validation failures return through the endpoint boundary
while retaining the original generation, usage and unknown cost in the journal.
Core wire shapes and storage formats are unchanged by this
dependency upgrade. Retain waiting work with its captured executable; the existing
BuildID boundary continues to reject incompatible checkpoints.

Scope's public contracts own framework behavior. Tool discovery implements
`interaction.AdvertisingTool`, Tool child dispatch uses
`ToolSetConfig.DecorateDispatcher`, and restoration reads
`interaction.DelegateAttributions` alongside the captured tree's statuses.
Definite model failures settle through Scope without synthetic cancellation;
an unacknowledged initial commit retains Scope's `StartError` evidence.
Model failures use `chat.CallFailure` and `chat.ClassifyFailure` for the complete
invocation. Nested failures cannot replace that classification, and known
generation remains available when later publication fails. Custom JSON codecs
describe their wire model with `JSONSchemaModel`.
Endpoint model discovery calls the same Scope binding's `ListModels` API;
Scope owns authentication, HTTP admission and pagination, while Flame supplies
its resource budgets and product identity rules. Connection configuration contains
no model identity; chat and embedding construction receive the selected model,
while discovery needs only the connection. Perplexity uses its Responses
binding and endpoint-owned model catalog without a retired Sonar default.
Scope owns external wait acceptance and its checkpoint encoding. Finish earlier
waiting work with its matching executable and retain that data directory;
the existing exact BuildID admission boundary rejects incompatible checkpoints.

Managed Node entries use the same installed MCP activation as native backends.
The package owns only `managedBackends: [{id, runtime: "node", entry}]`; the
distribution owns the pinned companion, launch flags and execution environment
identity. Runtime resolves it beside the actual hosting executable and verifies
its selected bytes before evaluation. It never searches PATH, the workspace or
FLAME_HOME for Node. Missing or damaged companions fail the affected activation;
management, native and UI-only packages remain independently usable. Each entry
gets its own existing process, immutable release lease, frozen requirements and
call-scoped callbacks. There is no shared JavaScript supervisor or result store.

Build Runtime/CLI distributions with `make build-runtime` / `make build-cli` after
`make setup`. Ship the hosting executable, sibling `qimu-runner`, and companion
`runtimes` and `plugins` directories together.
Replace the complete distribution only while quiescent. Desktop attaches to a
Runtime and does not host backends; it does not ship an unused interpreter.
Core `2026-10-24.1` projects managed server declarations explicitly. Rebuild its
Go/TypeScript/CLI/Desktop consumers together. Backend protocol identity remains
owned by the compiled `portable.Protocol` in `qimu/portable`. Usage Reports has a new private SQLite owner and
requires fresh installation data; retain the former installation/data separately.

Run accounting facts report every physically dispatched model call, including
unknown outcomes, original reported usage and captured per-call price. `Steps`
counts these calls; preparation failures without dispatch evidence do not count.
The Run reducer owns accumulation across calls and resumed
segments; terminal segment facts contain outcome, unresolved effects and duration.
`SegmentUsage` and cumulative completion fields are removed. Scope process snapshots
retain their checkpoint/calibration purpose. Unknown consumption cannot become an
exact priced total because a later response is known. Captured selected and
auxiliary calls now publish through the original execution member, with a persisted conversation/independent purpose. Independent outcomes
cannot write primary output or context. Both purposes retain unknown consumption
through live settlement and recovery, so known token counts survive while exact
total cost remains absent. Goal consumption derives from the same committed Run
metrics. Live settlement waits for nested streams to exit before reading physical
evidence, including observer failures, invalid chunks and consumer stops.
Existing settled historical totals are not recalculated on read or restart.
Each calling operation supplies its actual resource
limits; no universal Run/Goal cost ceiling is inferred from accounting totals. The journal now requires logical_purpose for logical
rows; old database shapes are refused unchanged by strict schema admission, without
a migration or an old-row reader. Public wire/export epochs are unchanged.

## Installed Tool proposals

Ordinary installed authors can contribute the published `ToolProposalContract`
through the existing ordered composition owner. Execution captures their exact
bindings and transitive dependencies before starting a Run. The original Tool
retains schema, approval and mutation authority: transformed arguments are
admitted before approval, executed unchanged and retained as canonical history.
Approval recovery consumes the saved final proposal without rerunning contributors.
See [`plugin/README.md`](../packages/go-sdk/plugin/README.md#tool-proposal-transformation) for authoring.

The private checkpoint now requires `tool_proposals`. Preserve earlier directories
and finish old waiting work with its original binary; no compatibility reader or
automatic conversion exists. Existing Core/backend wire versions and business
receipt formats are unchanged. Startup defers an original waiting probe while a
captured contributor's existing connection is still being admitted, without
waiting for its handshake or replaying work.
Later recovery reads that connection attempt's conclusion for the exact retained
binding. A proved original restoration failure loses the waiting tree; catalog
absence or another graph's failure supplies no such conclusion. Interrupted
activation waits, storage reads and unconfirmed process recovery retain their
causes without turning uncertainty into source loss.

Waiting recovery also preserves calls whose provider omitted usage. Run metrics
retain reported usage and unknown pricing; a later known report does not erase
its model breakdown or turn aggregate uncertainty into an exact price. Execution
restores its captured call counts without manufacturing reported-zero usage.

## Evidence retention

Physical model evidence has one journal owner. Run-linked rows follow the Run's
existing deletion lifetime; Session-only rows are deleted atomically with that
Session. Unattached rows are retained without an automatic expiry. Failed Session
deletion rolls evidence removal back. Unknown consumption remains unknown after
restart and does not authorize another provider request. Pending plugin commands
retain their original activation/release until the existing command owner settles
them; there is no age-based permission to redispatch.

## Installed query streams

Core `2026-10-15.2` admits query and command streams through `plugins.stream`.
The contribution's exact method contract owns that distinction. A query has no
command idempotency key, route or private receipt; a command retains its original
route and receipt-only reconciliation. Go `BindPluginStreamingMethod` supports
both with `PluginCallOptions`, as does unary `CallPlugin`. Former Go stream
`CommandOptions` signatures are removed; typed/callback consumers migrate together.
The shared TS client and component SDK expose `queryStream` for reads and
retain `stream` for durable commands. Early consumer withdrawal cancels and joins
the original stream; missing final evidence never synthesizes completion.

## Never-dispatched command retirement

Core `2026-10-16.1` exposes `plugins.retireCommand` and Go `RetirePluginCommand`
using the original `PluginCommandRequest` namespace and key. Commands owns the
`prepared -> retired` transition under the same Admission boundary as dispatch.
Only a never-dispatched route can retire. Retirement retains the original identity
and routing evidence permanently while releasing its executable dependencies;
it is not a backend business result. Repeated retirement is idempotent.
`plugins.getCommand`, unary calls and streams report `plugin_command_retired`
without contacting the backend. Dispatched work remains receipt-only and cannot
retire even when no receipt is available.

The public TS pending-command handle exposes `retire()`, including after client
replacement. Plugin management offers the same operation without requiring the
original component or draft. Failed retirement preserves the client journal;
only a confirmed retirement or original result settles it. Retirement does not
authorize execution and does not renew an expired delivery deadline.

The host SQLite phase constraints changed. Retain previous data directories and
use a fresh directory; Runtime refuses the former schema without conversion.
Rebuild Go, TypeScript, CLI and Desktop consumers together. Backend business
receipts and the backend protocol are unchanged.

## Feature JSON source

Core carries installed call input/output, stream chunk/final values, point values
and applicability attributes as JSON strings. Callable contracts in release,
requirement and resolved-binding projections are canonical JSON strings too.
These fields preserve the feature's numeric ranges and source lexemes; Core does
not turn them into JavaScript numbers. Former inline-object/number frames are
refused. Rebuild Go/CLI/TS consumers and ordinary component artifacts together.
Backend protocol and private command/receipt formats are unchanged.

The shared TS client exports `parseJSON`, `stringifyJSON`, `ExactNumber` and the
generated feature `JSONNumber` projection. Safe canonical numbers stay ordinary
numbers, wide integer literals become `bigint`, and other exact literals retain
their source in `ExactNumber`. This includes decimal precision, exponent spelling
and `1.0`. No huge exponent is expanded. Encoding refuses unsafe JS integer values,
non-finite values, unsupported objects, duplicates and malformed Unicode instead
of silently changing them. Use `bigint` or `new ExactNumber("...")` when authoring
such values. `bindOperations` owns this encoding/decoding for typed components;
direct wire clients send JSON strings and decode their returned source explicitly.

The mutation journal uses the same codec for canonical request identity and binds
the original feature input source separately. Wire string encoding cannot change
the underlying intent identity. It persists neither authored input nor private
business results. Refusal, acknowledgement loss and original-route recovery keep
their existing owners; no former wire reader or replay path is retained.

Model selections publish the same closed `provider`, `model`, `reasoningEffort`
field set that their Go decoder admits. All three strings are required, including
empty strings for the default selection. Schedules contracts and typed CLI/TS
consumers are regenerated from that owner; arbitrary string maps are not accepted.

[`model`](../packages/go-sdk/model/README.md) owns that immutable public choice for Runtime and
independent authors. [`model/catalog`](../packages/go-sdk/model/catalog/README.md) applies the shared
released Scope catalog's effort and modality facts without provider I/O or model
inference. Former private selection/catalog implementations are removed. Moving
the schema owner changes generated reference identities; rebuild Schedules
contracts, typed consumers and component bytes together.

## Accepted interrupt answers

Question and Approval executor answers use disjoint, strict payloads. A Question
contains only its ordered answers; an Approval contains its Domain verdict,
edited arguments, reason and remember scope. Both first delivery and restart
project the settled transcript Item through the same rule. Accepted continuation
storage retains only routing. The former mixed `approved`/`answers` payload is
refused. Finish old waiting work with its original executable and retain that
original directory; the existing exact BuildID boundary governs checkpoint reuse.

## Data directory ownership

One Runtime exclusively owns a physical data directory from before initialization
through execution drain, receipt flush and store close. A second Open, including
through a directory alias, fails immediately. Close may time out while shutdown
continues; the directory remains leased until all owned resources have closed.
Clients share this Runtime through its protocol. Embedded hosts must explicitly
select their directory. Moving or replacing a live data root is unsupported and
fails directory validation. Startup removes abandoned unpublished release staging;
ordinary reclamation preserves live candidates.

Inside that lifetime, the admission Gate owns Session writers and physical
working-tree reservations. The shared Session mutation coordinator owns Goal
drive lifetimes; recovery reads that registry under the same command lock.
Ordered recovery has one process-local sweep slot. Internal Session, working-tree,
Goal and recovery lock files are no longer created or consulted; old empty lock
files carry no product state and may remain on disk. No stored product data changes.
The unused `advisorylock.TryFile` and `TrySharedFile` APIs are removed. Plugin
backend processes continue to own their directories through `AcquireDirectory`.

Stop the original binary before this breaking cut. Retain its directory separately
and complete unresolved work using that binary; an older process does not honor
the new lifetime lease. Opening does not migrate or discard unknown commands.

## First-party initialization

Root `make build-runtime` produces `out/runtime/<platform>/flame-runtime`, its
sibling `qimu-runner`, the `plugins/` distribution and the pinned
`runtimes/node/<os>-<arch>/` companion. Run that executable in the foreground.
The embedded Go binding requires the same companions beside its actual hosting
executable. `localruntime` owns this layout, with no PATH, working-directory or
checkout fallback.

On a fresh `flame-qimu.db`, Runtime admits and enables the distributed catalog
through Qimu Host installation admission, prepares the private backend directories
and records the configured default providers before marking initialization
complete. Delivery is exposed only after initialization succeeds. Each original
installation is durable; interrupted initialization resumes those same identities
and rejects a changed distribution instead of creating replacement installations.

Initialization runs once per database. Restart never reinstalls an uninstalled
package, reenables a disabled one or selects updated bytes.
Users manage subsequent releases through the existing installation commands.
Clearing the database starts a new initialization. The initialization record is an
intentional pre-release schema break; incompatible databases are refused without
migration. Use a fresh data directory when upgrading.

## Public surfaces

- `runtime.Runtime` is the concrete in-process binding; `runtimeclient.Client` from `packages/go-sdk/client` attaches
  over HTTP/SSE. Both enter one Delivery Endpoint for validation, capabilities,
  command identity, invocation lifetime, Application calls and error/event projection.
- `protocol` owns binding-neutral requests, responses, events and validation.
  [`contract/API_REFERENCE.md`](contract/API_REFERENCE.md) and the generated
  manifest, OpenRPC and schema are projections of the operation catalog.
- [`Qimu MCP`](../qimu/README.md#processes-browser-execution-and-distribution) owns generic typed backend methods, streams,
  setup, callbacks and receipt transport. `qimu/portable` owns exact method,
  capability and portable value-point contracts.
- [`plugin`](../packages/go-sdk/plugin/README.md) owns Flame model roles, Tool
  helpers, captured product collaborators and the Tool receipt contract.
  `packages/go-sdk/extension` owns only Flame invocation origin and scope policy.
- `localruntime` owns the executable/companion-package layout and credential handoff.
  The Runtime process and clients consume that layout without checkout discovery.

Installed operations enter `CallPlugin`/`StreamPlugin`, Protocol
`plugins.call`/`plugins.stream`, or CLI `plugins call --request ...`. Typed Go
bindings validate the complete consumer contract. The shared TypeScript client
uses the same query and mutation owners. Input/output and stream values retain
feature JSON source as described above. A query has no command identity; a
mutation retains its exact installation, release, contribution, contract, method,
input, target, origin and parent. Changed meaning conflicts with the original key.

The backend commits its business transition and private receipt atomically.
Runtime retains original dispatch/activation evidence, never a competing business
result. After dispatch admission, original-command recovery queries that receipt;
missing evidence remains unknown and cannot authorize another mutation. Partial
stream output is observation, not a final receipt. Core Run acceptance remains
with Runtime independently of a plugin's acknowledgement or availability.

Memory and Schedules are ordinary selected contributions. CLI, workspace clients,
Tools and components consume their published operations. Private Memory items,
ledger, watermark and curation belong to Memory; definitions, cursors and captured
pending work belong to Schedules. Core has no feature business facade or private
feature store. Memory publishes its read capacity and owns its maintenance budget.
Generic `plugins.dataChanged` invalidates reads without carrying business results
or settling a command. `plugins.changed` describes installation lifecycle.

Knowledge base is another ordinary backend with its own document revisions,
chunk/vector index and private import receipts. Typed operations and MCP search
share retrieval; context v2 supplies original user input separately from host
trailers. Embedding v2 returns the actual vector space with its vectors. See
[`Knowledge base`](../plugins/knowledgebase/README.md) and the public
[`SDK contracts`](../packages/go-sdk/plugin/README.md) for use and coordinated rebuild requirements.

## Selection, composition and host callbacks

Enablement and selection are independent. `plugins.listProviders` and
`plugins.selectProvider` inspect/change the explicit role owner; an unavailable
selected provider never chooses another candidate. Composition operations retain
an explicit outer-to-inner order. Frozen execution and setup dependencies keep
exact contribution/export/contract, source authority and transitive release facts.
Changing a choice affects subsequent captures. A missing selected member refuses
its whole composition, including a previously bound optional dependency.

The published selection, context, input, observation and Tool proposal contracts
use the existing backend binding. Selection fixes the route before provider
construction. Context and input policies prepare the actual model-visible request;
media, byte/output and token admission see that final request. Authored conversation
and retry identity remain original. Transparent observers receive one unchanged
call-bound `next` and preserve canonical response/chunk evidence. Tool proposals
may transform arguments while execution retains Tool identity, schema and approval.
See the [public authoring contracts](../packages/go-sdk/plugin/README.md).

`qimumcp.Callbacks` from `github.com/Tangerg/flame/qimu/mcp` owns one lifecycle-bound loopback link. Bootstrap publishes
canonical RPC once before backends can receive its reference. Nested operations
inherit captured caller/actor authority and cancellation; each HTTP request owns
its own transport metadata. Closing or replacing a session revokes its references,
cancels admitted calls, joins them and then retires the transport. Runtime's
selected-model collaborators share this link; Memory supplies its own private
budget rather than changing the common callback contract.

`plugins.resolvePoint` resolves consumer-owned schemas and applicability without
activating code. Consumers choose explicit value order and interpret their own
point semantics. Installation/release/value bindings are immutable projections;
point resolution does not invent another composition or business-state owner.

## Components and resources

Packages publish ordinary prebuilt ESM exports using the public
[`@flame/component-sdk`](../packages/component-sdk/README.md) and shared UI modules. Components
may be content-only or bind a same-release callable contribution. Workspace,
Session, action, file and Tool-result placements use one component/resource path.
The first-party catalog and independent Reviews author use this same entrance.
The Go Qimu Host approves exact Browser plans and Uses; the Browser executor owns
the actual JavaScript objects and cleanup within that approved realm. Components
borrow their original Use, including exact parent/child bindings. Release evidence
returns to the Go Host before execution protection is removed. Private workbench
registrations do not select another provider or create another public authority.

`plugins.openResources` creates a live exact-release resource subscription. Each
asset read rechecks the enabled release, component and package integrity through
canonical Delivery. Closing the subscription or Runtime withdraws its ephemeral
access. Asset URLs carry no persistent Runtime bearer token. The workbench's
shared module graph supplies React, the SDK and UI; replacement of evaluated code
requires a fresh window generation. Source/native compilation alone does not
verify rendered interaction.

File renderers receive a frozen accepted file read. Tool-result renderers receive
settled historical projections and no invocation authority. Current renderers may
inspect historical results without evaluating historical producer code. Loading,
withdrawal and ambiguity retain generic inspection and existing source access.
See [Web component ownership](../apps/web/ARCHITECTURE.md).

## Run commands and strict request identity

Public Start, Resume and Steer require caller-retained idempotency keys. Preserve
the exact request, capability metadata and discovered store namespace before
sending it. Runtime owns one atomic acceptance with the Run/Segment opening or
Steer input; redelivery reads it before current execution preparation.
`runs.getAcceptance` is a read, not permission to repeat an unknown operation.
Retired material retains its original references and cannot be recreated by import.

A Steer acceptance nominates its exact User Item identity before application.
`steerInputs` keeps accepted original inputs while their material remains; only
that committed Item proves application at the next model boundary. Shared-client
journals preserve request/metadata/item identity across restart. Incompatible
journal and private checkpoint shapes are refused and preserved, without conversion.

RPC parameters and metadata use exact schema names. Duplicate or unknown members,
invalid Unicode, trailing values and explicit null in typed fields are refused.
Go authored JSON uses `protocol.DecodeRequest`; TypeScript uses generated request
validation before reserving command identity. Opaque feature JSON keeps its own
numeric source and null semantics. Rebuild affected clients, contracts and component
artifacts together when public shapes change.

## Portable plugins

`plugins.getCommand` reads an original dispatched command's admitted request by
its retained key and Runtime store namespace. It performs no backend I/O and
stores no business outcome. The existing command owner retains contract, input,
installation/release, target and dispatch evidence even after observation.
Recreated clients can verify that projection against their unresolved journal
identity before querying the original receipt. A missing or merely prepared route
does not authorize execution, and a changed namespace cannot expose another store.
Managed callers retain their actor scope and captured dependency authority.
The public Go binding and generated wire artifacts expose this same query.

The TS client journal includes immutable method/routing metadata without copying
authored plugin input or private results. Its former format is refused and kept
unchanged; finish pending work with its original client before using a fresh
profile for this pre-release journal cut. No automatic data conversion or cleanup
interprets uncertainty as permission to replay.

Current selection rows store contribution_id independently of server identity.
Original command routes retain that ID, named export and complete capability
contract alongside immutable provider/server/input evidence. Former host tables
without these bindings are refused by exact-schema admission without conversion.

Runtime composes Qimu Host for installation admission, exact bindings, execution
protection and lifetime management. Flame owns the interpretation of its plugin
metadata, user configuration, provider choices and product contribution policy.
The complete operation schemas and error categories are generated in
[the API reference](contract/API_REFERENCE.md); `plugins` is an advertised
Runtime capability.

Install from an absolute directory or ZIP path on the Runtime machine. Each
release must contain Qimu's `manifest.json`, `plugin.lock.json` and exactly its
listed artifacts. The neutral manifest supplies the sole immutable release
identity. `plugin.json` remains Flame product metadata using Agent Plugins 1.0.
Its `io.github.tangerg.flame` namespace names one local declaration document with
`plugin.DeclarationProtocol`, independently of `portable.Protocol` used by
callable contracts. Raw legacy source trees without the neutral release files
are refused.

`qimu/distribution` owns deterministic archive creation, path and entry safety,
byte quotas, artifact digests, immutable content and read leases. The composition
root creates one content Store with the Host's shared Admission. Its held physical
directory identity and exclusive ownership protect all operations; closing the
Store seals new work and joins its operations and leases. Slow file verification
and physical cleanup occur outside the admission gate. The Flame adapter neither
computes a second release digest nor maintains an integrity cache or execution
copy of that content.

The author command `runtime/cmd/pluginpack` calls the same
`plugins.ProjectFrameworkDeclaration` used during admission. It interprets Flame
metadata, adds the actual compiled module graph or selected executable artifact,
and builds the neutral manifest and lock. Admission verifies the complete
projection against that manifest. Hidden Units, changed imports, substituted
runtime identities or a different contract cannot be introduced through a
second metadata document. Inert content packages need no executable Unit.

The immutable Flame release catalog retains the accepted product interpretation.
An installation selects or stages the original neutral release digest; catalog
presence is not proof that its bytes remain available. Current resource reads
and activation acquire the content through the original Store. A candidate keeps
its content lease from materialization through installation admission or refusal,
so concurrent reclamation cannot remove a release awaiting that decision.

Flame declarations own inputs, contributions, presentation themes, points,
point values and components. Product diagnostics retain their typed code and
component reference without granting availability. Invalid paths, mandatory
release files, archives or manifest projections are refused before installation;
filesystem and storage failures keep their original causes. Package bytes are
public content. Credentials reach a backend through declared secret inputs,
not static package environment values.

Installed process entries are packaged native executables or declared managed
Node entries. Native variants identify their OS, architecture and original
executable digest; managed entries retain the distribution's exact pinned Node
version and digest. Ambient commands and installed HTTP backends are refused.
Independently configured ordinary MCP connections retain their own transports.
The package adapter reserves `PLUGIN_ROOT` and `PLUGIN_DATA`, validates declared
inputs and projects launch paths onto the original leased content.

`qimu/process` persists the original start attempt and binding before asking
`qimu-runner` to create a process. The original runner owns process creation,
fencing and wait evidence. The content lease survives handshake failure,
retirement and lost communication until that original execution is proved to
have ended. A new runner's empty ledger is not exit evidence. Installed native
code is trusted local code; this process profile does not establish an OS sandbox.

A new installation starts disabled. Configure its required inputs, then enable
it. Disablement withdraws new use without discarding the selected release or
standing Tool decisions. Configuration changes, release selection and uninstall
respect the original business and execution protection under the shared
Admission. Exact old bindings remain the authority for retained work; a current
provider selection cannot replace them.

Advisory leases own their release attempt and settled cleanup result. Non-blocking
contention returns no lease and no error; descriptor or handle cleanup failures remain
errors and cannot be mistaken for contention. Owned Unix directory descriptors are closed
once after attempting unlock, with both failures retained. Borrowed-file unlocks and
Windows mutex releases remain retryable while their resource permits it. Repeated
release calls return the settled result without more OS calls. Data-directory ownership
delegates release to its lease rather than maintaining a second release state.

The registry resolves each server by its origin, realizing an installation
server only as far as the read requires (desired definition, dispatch authority, or launch)
and never its siblings. Ordinary tool dispatch reads admitted declarations and
current installation authority for the called server alone. Resource and Skill reads use the exact release
lease and verify the selected artifact through Qimu's held directory authority.
`${PLUGIN_DATA}` is a private per-installation directory retained through updates
and uninstall; it is never collected automatically. Content remains retained
while an installation selects or stages it, a content lease is live, or business
or execution evidence requires it. Reclamation reads those current owners under
the shared Admission before retiring unretained bytes. The immutable Flame
metadata catalog remains available for history after content reclamation; its
presence never proves executable availability.

Execution dependency projection follows the actual frozen MCP setup bindings,
including transitively bound optional collaborators. It does not resolve current
provider choices to reconstruct a previous activation. Manifest Tools, model
captures and unresolved installed commands retain the resulting canonical release
set; a declared bound provider that is unavailable is an error, not optional absence.
Activation identity includes the actual ordered contribution/export/contract edges
and source authority of every bound entry. Same-release contribution or order
changes refuse fresh dispatch too; reconnecting the exact same graph preserves its
identity. Tool and model deployment identities include this proof for restoration.
Known command receipt lookup remains with its original business owner and does not require replaying collaborators.
Command observation releases its dependencies without erasing the original route.

Fresh command input is validated against the concrete export's contract before
persisting a binding or dispatching. Schema rejection projects as `invalid_params`
and leaves that identity available. Original-route lookup stays first on
reconciliation; it never revalidates an accepted command against a replacement
provider. Storage, reply validation and other post-dispatch failures still retain
execution uncertainty rather than becoming caller-input refusals.

`plugin_commands.activation_json` is required immutable routing evidence containing
that identity and its release projection. The former release-only column is removed.
Former Runtime databases are refused before writes; retain them separately and use
a fresh Runtime data directory. Private plugin databases are unchanged. Existing
waiting executions must finish under their original binary: model dependencies also
participate in deployment identity, and no continuation migration is provided.

Restoration resolves the selected and staged definitions from the admitted catalog
before constructing an installation. The aggregate enforces the same input, header,
component, required-value and staged-package rules used by its transitions. Unknown
bindings, invalid header or authentication values, enabled backends without required
inputs and a staged release from another package fail installation reads. Valid records
keep their format; invalid records fail reads without automatic conversion or repair.

Staging never switches running code. An already selected digest cannot be staged
again. Selecting a distinct staged release requires execution to be quiescent. It is one
explicit trust decision that preserves desired enablement. A candidate whose retained
configuration lacks inputs required by an enabled installation is refused before any
selection or configuration changes. Disable first when new credentials are needed,
select the candidate, configure it and enable it explicitly.
It retains what the candidate declares unchanged: input values whose input keeps its
server, target, key and secrecy, and disabled servers and Skills that are still declared.
An installed process secret follows its executable recipient: the original
release digest and server name. A release change drops those secret inputs and
requires explicit configuration again. Ordinary user-configured HTTP sources
retain their separate endpoint-bound OAuth authority; they are not installed
package process variants. Configuration commands require
the exact selected `digest` alongside `installationId`; a release switch rejects an
older configuration as `plugin_stale` before changing inputs or component enablement.
An omitted digest is invalid. Rebuild clients together; old unbound commands are not
upgraded or replayed against the latest release. Runtime derives quiescence
from the canonical union of owner-provided dependency projections: the installation ID and
release digest set of every owned executable manifest, covering package Skills and
installation MCP tools, and the same set stored relationally with each pending
execution checkpoint in the checkpoint's own transaction, plus unresolved commands'
durable original installation/release bindings. Installation admission
queries only that projection by installation and never decodes a continuation, so
an unrelated or unreadable checkpoint cannot block another installation's change.
Installation changes, including the capacity check of a new install, and the
publication of an assembled execution and command binding/dispatch share one short serialization point. Run
assembly, model resolution and package reads happen outside it, so a pending change
never waits for, or holds back, an assembling Run. The MCP catalog is a source observation, not a
Use grant. Qimu seals superseded admission when the installation change commits;
every subsequent execution must borrow an exact approved Use, so a stale catalog
observation cannot authorize superseded code. Original admitted Uses retain
their own dependency protection. Process retirement, launch and handshake happen
outside the shared gate. The installation owner reports
whether its durable change affected selected execution capabilities; staging and
unchanged configuration do not. Quiescence controls existing executions,
while every committed execution change, including disable, invalidates
unpublished assemblies of that installation. A withdrawal failure after the durable
write preserves this invalidation; a rejected write cannot originate one.
Plugin and Skill read invalidations likewise follow the durable write, outside admission
and before realization. Withdrawal failure cannot suppress a committed change's notices;
a refused write publishes none.

Installed commands durably bind identity, method, admitted raw input and
original installation/release/server before network dispatch. The host records
prepared, dispatch-admitted and observed evidence, never a business result. A
dispatch admission is not proof that the backend received or committed the call.
After admission, retries query the original backend's private receipt without
resending the mutation. Missing, malformed or unavailable evidence remains unknown
and holds the release. A valid original success or business refusal releases the
hold; immutable routing remains retained so the same identity cannot adopt a new
provider after update or uninstall. History is not evicted or capped by an arbitrary
lifetime command count. The public `jsonvalue.Value` normalizes object order
without rounding numbers or replacing original request evidence with decoded DTOs.
The host and private stores persist this input; fingerprints are derived projections.
The public `qimu/portable` package admits consumer-defined value schema, scope,
applicability, optional absence and composition ordering. Namespace declarations
publish `points` and `pointValues` using local contract files. Installation admits
these without starting code. `ResolvePluginPoint` and `plugins point` return
immutable installation/release/value bindings through the ordinary Delivery
Endpoint; `ResolvePointValues[T]` checks the consumer's typed schema before
decoding them. Explicit order belongs to the consumer's composition owner.
This value projection does not execute decorators or persist scoped composition.
The public component SDK can realize these exact value bindings as independent
component exports (`placement: "export"`). Its consumer-owned point schema names
a same-release component and props; Runtime continues to own applicability,
membership and order. The graphical host owns rendering and captured lifetimes,
including explicit collection, selection and decoration. See the
[composition example](../examples/plugins/composition/README.md). Export placement
adds no backend or persisted business owner and requires the current host.
Core `2026-10-16.2` requires the current component composition host and SDK.
Rebuild Go/CLI/TypeScript/Desktop consumers together; older protocol metadata is
refused. This change does not convert storage or pending command identities.
The Reviews analyzer example interprets a value as a same-release callable
contribution and invokes it through its declared optional capability dependency.

The same `qimu/portable` owner admits callable metadata, schemas and canonical
identity without importing MCP or Runtime internals. `qimu/mcp`, Flame's product
contracts and `qimu/contractgen` consume those admitted values. Installation domain
code can validate a contract without activating a backend. Flame's `extension`
package retains only its product origin and scope vocabulary.
The Qimu integration requires rebuilt contracts and a fresh host database; it
provides no compatibility wire reader or state conversion. Private backend owners
continue to admit their own exact schema and receipt contract. Preserve earlier
deployments and their data together to finish unresolved work.

An assembly overtaken by such a change to one of its dependencies is refused as `plugin_changed` on
`runs.start` and `runs.resume`: nothing was started and no interrupt was consumed, so the
person may send the same request again. Clients present it as retryable and never retry
it on their own. Disablement withdraws dispatch authority immediately; an already
started effect can still finish or remain uncertain. Tool wrappers recheck current source
authority, their exact realized connection configuration and the currency of the session
that admitted them before dispatch. Rotating credentials rejects old
executables even before asynchronous reconciliation starts, and a frozen executable whose
session was refused or replaced by a reconnect is rejected as "source connection is no
longer current" instead of failing on the closed transport.
Permission fingerprints bind the admitted code of one server (`release digest + server
name`), so every release change makes standing rules for its tools stale; credential
rotation, disablement and sibling enablement do not erase them.
The rule-list projection observes each referenced MCP source once per request; tool
decisions and dispatch still read the current owner independently.
Permission fingerprints read desired source definitions independently of executable
availability. Missing or changed package bytes cannot hide standing rules; dispatch
still refuses those bytes through its separate admission guarantee.
Rule inspection returns display fields and computed staleness; durable authority
fingerprints and storage scope keys remain with the policy owner.
OAuth credentials are bound to the fingerprint of the OAuth target that requested them:
its MCP identity and credential recipient (for a user server its endpoint and headers; for
an installation server the recipient above). A recipient change makes them stop matching
without any invalidation step; they are removed with their MCP source.
Cold restart preserves exact dependency
checks and command replay identities. Installation commands commit desired state before connection realization.
Every installation read and command result carries `realization`, observed from its live
owners on each read and never stored: `{type: "releaseUnavailable"}` when the selected
release bytes fail verification, otherwise `{type: "available", unavailableBackends?}`
naming declared servers whose backend directory cannot be realized now. A backend whose
preparation failed therefore stays visible in every later `plugins.list` until repaired,
not only in the command that attempted it. Each connection's own outcome is the MCP
supervisor's status (below). Every installation read and command result also carries
`presentation`, the Runtime's closed decision whether the selected release's declarative
presentation contributions (themes) are shown now: `admitted` exactly when the
installation is active and its release is available, otherwise `withheld`. Clients
present admitted themes only and never rebuild that decision from `state` or
`realization`. Uninstall requires
the same quiescence as a release switch and returns `plugin_in_use` while an active or
waiting execution or unresolved command depends on the installation; disablement remains available to withdraw its
authority while in use. Uninstall is an acknowledgement without a result: it durably removes
admission before asynchronous retirement. Qimu retains the installed execution
resources and their original proofs until retirement completes; the MCP Pool
withdraws its session observation without claiming process exit.
Post-commit package preparation, reconciliation, and projection, including a new
installation's first result, survive request cancellation and follow the Runtime's
cancellation root. A reconciliation that cannot
start settles each still-enabled source as `mcp_configuration_failed` at the connection
status owner; a removed source stays absent. The delivery endpoint
joins these calls before closing their dependencies; shutdown does not undo the
committed installation state. When shutdown preempts the post-commit realization read,
the command reports that cause even though its durable change stands.
Connection attempts own credential restoration as well as dialing. Reconfiguration
withdraws the old session and tool snapshot before restoration; restoration failure
settles the source as failed. A failed MCP server status names its closed failure
category as an inline problem type without prose: `mcp_configuration_failed` (the source refused the connection
or stored credentials could not be restored), `mcp_dial_failed` (transport, process or
handshake), `mcp_tool_discovery_failed` and `mcp_authorization_failed`. The connection
owner records the category at the failed transition, including startup admission; the
cause itself stays in traces and logs because it can carry paths, endpoints or
credentials. A connection to a disabled server, or to any server of a disabled
installation, is refused with the MCP `ErrServerDisabled` category. A refusal is a live
transition, not only a command error: when configuration, reconnect or authorization
cannot obtain an admitted configuration, a removed or disabled source is detached and any
other refusal settles as `mcp_configuration_failed`, and either way the previous session
stops serving tools. A connection dispatch or an installation reconciliation that cannot read its source owner
settles the same way, so the failure reaches status readers instead of only the log.
The connection owner admits configuration, authorization and refusal under the same lock
that advances live state. A command canceled before admission changes no session, tools,
credentials or status; a superseded launch cannot withdraw its replacement connection.
Supersession cancels obsolete work; shutdown cancels and joins every admitted attempt.
For an ordinary user-configured stdio server, the original process wait determines
exit even when the status is nonzero. An installed backend instead requires the
original Qimu runner's execution proof; closing its MCP session alone proves no
process exit. Unconfirmed retirement retains its resources and remains visible
to the shutdown owner.
Canceling a wait for session retirement leaves unreported close failures with Connections
until shutdown captures them. MCP shutdown owns one terminal generation: all callers
that observe completion receive its retained result, including later calls after a
failed retirement. Canceling a waiter preserves its cancellation category and cause
without changing the cleanup generation. An already observable completion retains
precedence over that waiter's cancellation.
OAuth target fingerprints encode the owner's exact fields and header bytes through the
shared framed hash, independently of JSON serialization. Credentials recorded with the
former JSON hash require a new OAuth authorization; no dual fingerprint reader is retained.

Installed `mcp.json` declares packaged stdio executables; managed Node entries
use the Flame declaration. The current Qimu process profile refuses installed
HTTP entries. Ordinary user-configured MCP sources continue to support stdio and
Streamable HTTP through their existing connection, exposure, approval and OAuth
owners.
An MCP server's identity is its origin plus the name that origin chose:
`mcpserver.ID{Origin: User | Installation(id), Name}`. Names are unique only within an
origin, so a user server and an installation server may share one. The same structured
identity is the storage key (an `mcp_sources` row with `origin`, `installation_id` and
`name`), the source half of every MCP `tool.Ref`, and the single wire shape
`MCPServerID{origin: {type, installationId?}, name}` used by `MCPServer.id`, every
server-addressed request, `ToolRef.server`, tool listings, exposure, OAuth attempts and
`mcp.changed` events. No component joins the parts into one string or parses one back.
An installation record's `Source` also binds the admitted release digest, the tool
authority of that code and the recipient its credentials follow, so none of them can be
claimed by a user record or omitted from an installation record. Model-visible tool names still project from the local name
alone; equal projections from different origins are excluded symmetrically.
Their connection configuration is changed through installation operations. User MCP CRUD
refuses these sources. Invalid server declarations are diagnosed independently.

User MCP server membership and configuration belong exclusively to the durable resource
commands: `mcp.servers.create`, `mcp.servers.update` and `mcp.servers.delete` (or the
corresponding Go binding methods). Startup only restores these resources and realizes
enabled connections. `FLAME_MCP_SERVERS` and `FLAME_MCP_<NAME>_TOKEN` no longer configure
or seed servers. Configure new servers and credentials through the existing CLI or
Desktop MCP controls, or the public Runtime binding. Previously persisted servers remain
ordinary user resources; deleting one stays effective across restart.

Typed declarations reject explicit `null`, including nested arguments, environment values
and theme colors, instead of converting it to an implicit default. Invalid contribution
lists receive their own diagnostic and do not withdraw independent contributions.
MCP definition reads project durable source membership and configuration without inspecting
release bytes or backend directories. Plugin inspection observes release and backend
availability; the connection owner alone reports live connection outcomes and tool counts.
Unavailable resources therefore leave desired definitions intact, and independent healthy
sources remain usable. Desired descriptor projections never admit connections or dispatch.
Temporary execution unavailability does not remove standing
tool exposure choices. Exposure inspection and edits read durable source definitions;
reconnect and authorization reach the connection owner's launch integrity check, so a
repaired release can recover without replacing its installation.
Stdio is an explicitly trusted executable with the Runtime user's OS access, not an OS sandbox.
Only PATH, the reserved package paths, declared environment values and configured inputs
are inherited for installed executables. Ordinary user-configured HTTP sources
use their explicit credentials and the existing OAuth owner. Credentials must not
be embedded in package files or UI assets.
Package entries, relative commands, working directories and resource reads share one
portable path contract; Windows reserved names and ambiguous path segments are rejected
before filesystem preparation on every host.
Reserved device stems include superscript digit aliases from the
[Win32 naming rules](https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file).

Package Skills join the existing resolver. Explicit project Skills override user Skills,
which override package Skills. Conflicting package names are excluded rather than chosen
by installation order. Discovery, loads and frozen Run dependencies use the same source
resolution. Discovery diagnoses explicit project/user overrides and counts capacity per
selected source. Revoked or disabled package Skills become unavailable for document
and resource reads; existing execution metadata retains its admitted release identity.
Discovery distinguishes package name conflicts from unavailable release bytes or
withdrawn authority. An override diagnostic can accompany its available project
or user Skill; independent installations do not share one Skill capacity budget.
Admission, discovery, detail and model loading share Scope's format and directory-name
binding. Parsing consumes exactly the verified bytes. Names must already be in Unicode
NFKC form, and the directory and frontmatter must have exactly the same name. Rename
noncanonical directories and update their frontmatter before rebuilding a package;
Runtime neither normalizes names nor keeps aliases for their former spelling.

Configuration changes use `valueChanges: { "input-id": { "type": "set", "value": "..." } }`
or an explicit `{ "type": "clear" }`. An optional empty value remains a configured
environment variable or header; clearing removes its binding. Inspection reports
`inputStates` for every declared input of the selected release: `{type: "unset"}`,
`{type: "value", value}` for a configured non-secret input, or `{type: "configured"}` for a
configured secret, including an empty one. Secret text is never copied into any
projection. Configuration is a delta: `serverChanges` and `skillChanges` map a declared
component name to `enable` or `disable`, and inputs and components a request does not
name keep their value or enablement. Required inputs cannot be cleared while their
backend is enabled. Each environment or header binding belongs either to static package
configuration or to one declared host input. Inputs cannot replace static values; HTTP
header binding identity is case-insensitive. Portable environment bindings also reject
case variants that would compete on Windows. Invalid inputs are diagnosed independently.

Themes must declare `scheme: "light"` or `"dark"` and use only the bounded color vocabulary.
The generated Go, JSON Schema, and TypeScript response checks project the domain's
color names and six-digit hexadecimal grammar before colors reach client rendering.
The appearance owner retains a rendered projection for first paint. Runtime scheme
selection reads the registered theme or system appearance, never the paint cache.
Removing, disabling or losing the release of an installation withdraws its selected theme
preference and the first-paint projection with it. Themes use the existing
Dougong Host and child lifetimes; client connection replacement retires the predecessor
before publishing its successor.

Ordinary components declare namespace-local prebuilt ESM and consume public operations
through their captured SDK context. The former contributes.views vocabulary is
unsupported; portable Skills/MCP and unrelated admitted contributions keep their own
failure boundaries. Package integrity and confined resource admission remain in force.

Session-scoped components may declare `sessionAction` placement. It admits ordinary
ESM and an optional callable contribution without a business operation enum. Clients
offer the local menu metadata and capture the accepted Session ID/revision/title when
opened. Trajectory owns its naming form and calls canonical `sessions.update`, using
existing Session revisions, durable command identity and recovery. The Core operation
continues independently of optional component availability after acceptance.

The [first-party trajectory package](../plugins/trajectory/package/plugin.json) exercises a theme,
a Skill, an ordinary Session trajectory component and the current Session rename action. The public page API requires no code
execution in Runtime. Desktop's [component lifecycle](../apps/desktop/README.md#installed-component-rendering)
owns rendering qualification; an unavailable renderer does not change installation state.
Package limits are 128 MiB total copied bytes, 4096 entries, 256 Skills and
128 installations. Skill documents and resources obey the existing 1 MiB Skill limits.
Component ESM resources obey the existing package bounds and immutable resource admission.

This is a breaking protocol/storage change. Rebuild all clients and generated contracts
together. Installation records reference immutable declarations by digest. Installation
IDs and release digests are typed values (`resourceid.InstallationID`,
`fingerprint.Digest`) parsed once at the delivery and storage boundaries; the 64-digit
lowercase SHA-256 spelling has one owner, shared by release digests and source authority
fingerprints. `mcp_sources` holds one row per server identity; the user descriptor
(`mcp_servers`), exposure, OAuth sessions and approval rules reference it by a cascading
foreign key, so deleting a user server or uninstalling a package removes every relation
about its servers. Approval rules store the tool kind, the source-local tool name and the
MCP source reference as columns rather than an encoded reference string; installation
records do not store server identities or declarations. Installation state is relational:
the installation row holds its source, selected and staged digests and its desired
`enabled` integer constrained to zero or one (a staged digest differs from the
selected one), and child tables hold input values and disabled
components, each removed with its installation by cascade. Open admits a data directory
that is empty or holds exactly the current tables, indexes and triggers; any other
directory, such as one with former MCP, installation, OAuth or checkpoint shapes, is
refused without modification, and no former shape is repaired in place. Pre-release
storage compatibility follows
[the repository data policy](../DEVELOPMENT.md#pre-release-data-policy). Completed historical Tool content remains generic and readable.

Stored current and historical Plans require a steps array; clearing a Plan records `[]`.
Empty text and JSON `null` are invalid Plans. Run usage and frozen capabilities use
empty text for absence; usage objects preserve reported zero, and capability objects
must declare at least one capability. Invalid stored values fail reads rather than
inventing empty state or reported usage. Valid records require no migration.

Publishing the new Runtime module and advancing CLI's released dependency is required
before an independent CLI release; workspace checks alone do not prove that release.
Installation Skill declarations expose names and descriptions. The package adapter
derives their fixed portable paths; release records and protocol declarations no
longer store another editable path beside the Skill identity. Rebuild generated
contract consumers and use a fresh pre-release data directory for this storage change.

## Open an in-process Runtime

```go
rt, err := runtime.Open(ctx, runtime.Config{
	DataDirectory:        dataDirectory,
	DefaultWorkspacePath: workspace,
})
if err != nil {
	return err
}
defer rt.Close()

session, err := rt.CreateSession(ctx, protocol.CreateSessionRequest{
	Workspace: &protocol.WorkspaceRef{Path: workspace},
}, protocol.CommandOptions{IdempotencyKey: requestID + ":session"})
if err != nil {
	return err
}
```

Hosts must close each Runtime they open. Protocol errors support `errors.Is` against public sentinel errors and `errors.As` to `protocol.ProblemError` for structured recovery information.

Repeated `Close` calls join or resume an incomplete shutdown. Once terminal
resource teardown finishes, every call returns its retained diagnostic without
repeating cleanup; a later call cannot turn a failed close into apparent success.
Shutdown cancels accepted calls and component-owned work before joining delivery.
Dependencies close only after their calls and components have returned.
Waiting cancellation retains its standard category and caller-supplied cause.
An already observable completion still owns its result; canceling a waiter
cannot change that result or cancel an admitted teardown generation. Keyed
admission checks the waiting caller before granting a reservation and releases
every earlier key if a multi-key admission cannot finish.
Foreground Session and Run admission checks the caller's lifetime under the
Gate lock before reserving resources. Startability waits reject an already
canceled caller even when a reservation just released. Durable Run reads
retain both their own failure and concurrent cancellation before permitting
a continuation. Observation cleanup retires only that waiter's subscription;
the next Start still acquires its own admission.
Startup and periodic recovery share one coordinator that serializes each sweep,
then reconciles Runs before Goals. It checks its lifetime before taking
ownership and before starting each phase, releases acquired ownership on every
exit, and retains a failed phase's cause alongside concurrent cancellation.
Cancellation stops unstarted phases; an already completed recovery retains its
committed result.
Before abandoned Running Runs are classified, startup acquires the ordinary
Run admission for each accepted Resume handoff and attempts its bounded revival.
An inconclusive storage or dependency failure stops startup with the binding
retained; an incompatible head follows the existing Run-lost policy.
Unix directory leases consume their opened descriptor. Acquisition failure or
contention closes it immediately; an acquired lease owns both unlock and close,
retains their combined diagnostic, and never retries a retired descriptor.

## Attach without owning the Runtime

```go
client, err := runtimeclient.Connect(ctx, runtimeclient.RemoteConfig{
	Endpoint: "http://127.0.0.1:17171",
	Token:    token,
})
if err != nil {
	return err
}
defer client.Close()
```

`Client` uses the same typed methods and call options as `Runtime`. Its HTTP requests enter the serving Runtime's existing delivery endpoint. Closing it cancels its requests and subscriptions without shutting down the server or canceling accepted Runs. A connection failure never constructs an embedded Runtime. A caller that intends to cancel execution sends `CancelRun` explicitly.

Operation methods require a non-nil binding returned by `Open` or `Connect`. Their shared implementation no longer supports invoking an operation through a nil `*Runtime`; callers must handle constructor errors before use. `Close` remains nil-safe, and operations on a closed binding return `ErrClosed`.

`Endpoint` names a base URL, including any reverse-proxy prefix, without credentials, a query, or a fragment. Authorization travels in the bearer header. Workspace paths belong to the server's filesystem and operating system; the client does not clean them with its own platform's path rules. Client-local attachments must be read into command content before dispatch. Retries retain the original content, idempotency key, and durable namespace.

## Serve the browser application

The standalone Runtime can publish the built `apps/web/dist` directory on its own HTTP origin. Set `server.webDirectory` or `FLAME_SERVER_WEBDIRECTORY` to that directory's absolute path. Startup rejects a missing distribution instead of presenting a partially configured browser endpoint.

Static assets and application navigation are public. All Runtime calls still use the generated `/v2/rpc` endpoint and its existing bearer gate; static routing never replaces protocol or health routes. The asset server confines reads to the distribution, rejects hidden files and directory listings, and keeps application HTML revalidated on reload. No token is embedded in HTML or placed in a URL.

The browser starts at its own origin and accepts the Runtime token in the connection dialog. Desktop uses its native bootstrap. Both consume the same TypeScript client from `contract/typescript/client`; Wails capabilities belong only to the desktop platform adapter.

The shared TypeScript client owns strict response decoding for JSON-RPC, event streams, and HTTP sidecars. HTTP bytes must be valid UTF-8. Microsoft `jsonc-parser` visits decoded members and strings to reject duplicate members and unpaired Unicode surrogates before native `JSON.parse` constructs values; JavaScript number and object-member semantics remain unchanged. Malformed responses are `RpcProtocolError`, which preserves unknown mutation settlement without treating invalid wire data as a recoverable connection failure. Event-stream observations cancel unread bodies before releasing their readers, including when their sole consumer returns early.

## Errors and acknowledgement certainty

Operation failures separate the stable machine `type` from their human-readable `detail`; a bare category omits redundant detail. Clients render the type once and use structured field errors, capability requirements, and active-Run references when present. Go callers retain underlying causes without parsing the display text. Explicit and inferred failures pass the same wire validation before leaving the endpoint.

Remote Go calls report malformed protocol replies through `ErrInvalidResponse`, transient connection loss through `ErrDisconnected`, and uncertain command acceptance independently through `ErrAcknowledgementUnknown`. A malformed reply can leave acceptance unknown without being safe to retry automatically. Recovery must retain the exact original command parameters, idempotency key, and namespace; the client never silently resends a command or follows an HTTP redirect.

Cancellation failures returned by the Go binding also preserve `context.Canceled` or `context.DeadlineExceeded` for `errors.Is`. Request cancellation causes are local to that invocation; wire problems and persisted replay outcomes retain their protocol category and safe details.

The standalone Runtime, CLI and Desktop resolve `FLAME_HOME` through `localruntime` as
Flame's local product root. Only an empty value selects `~/.flame`; an explicit value
must be absolute and keeps significant whitespace. Runtime-owned state lives under
`$FLAME_HOME/runtime`. Desktop reads the local token only from that selected deployment;
a missing or invalid configured token never redirects it to the default directory.
Go consumers use `ResolveProductRoot` followed by `DataDirectoryUnder`; the former
`DefaultDataDirectory` API has been removed. Existing data directories need no migration.

## Develop

```sh
go test ./...
go vet ./...
go build ./...
go generate ./...
```

Application checks use the root `go.work`; Runtime and its clients resolve local
sources together. The [development workflow](../DEVELOPMENT.md#verification) owns
verification requirements. The default suite is offline. Module rules live in
[Module instructions](#module-instructions) below; current boundaries live in
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).

The source map follows responsibility within each ring: Run execution and input
translation live in `internal/adapter/run`, model translation in
`internal/adapter/integration/model`, and Hook management in
`internal/application/integration/hooks`. Feedback has its own Domain value and
Application recorder because its lifetime is independent of Session cleanup.
The architecture gate rejects unclassified production packages and imports.

Lifecycle Hook configuration requires an explicit `hooks` array in every non-empty
JSON document. Use `{"hooks":[]}` for an empty set. Hook configuration and command
decisions reject explicit `null` at every depth; omit optional members instead.
Empty command stdout or `{}` still means allow; a present `decision` must name
`allow`, `deny` or `ask`. Unknown members, duplicate members,
invalid Unicode and trailing documents are refused by the JSON decoder.
Configuration admission retains simultaneous read, capacity, source-version,
cancellation and close failures. A failed inspection publishes no Hook set.
Command failure diagnostics retain process, cancellation, timeout and cleanup causes
alongside bounded stderr. Non-Unicode stderr becomes bounded escaped-byte text.
Broken commands remain observable non-blocking failures; exit 2 is an explicit deny.
Existing Hook producers must replace null fields with omission or valid typed values.

Waiting executor checkpoints require the exact executable BuildID. A rebuilt
binary can reject them after source-only changes, including directory moves.
Finish or cancel waiting executions before replacing their owning build. This
does not require a migration of completed history or alter the public protocol.

Run the built `flame-runtime` server in the foreground. Use Ctrl-C and wait for
shutdown before replacing it. For isolated data, supply a new absolute
`FLAME_HOME` to that process. Building does not remove or convert existing data.
Data cleanup remains an explicit offline operation.

## Module instructions

Flame Runtime is the product backend and sole owner of durable agent semantics. It exposes one in-process Go binding and one Runtime Protocol for CLI, Desktop, and other hosts.

Read [`../AGENTS.md`](../AGENTS.md), [`../DEVELOPMENT.md`](../DEVELOPMENT.md), and the ordered Runtime baseline in [`doc/README.md`](docs/README.md) before changing this module.

- Runtime is a product backend, not another Agent Framework. Scope owns Process, strategy, tree, provider, and effect execution contracts; Runtime adapts them to Flame product semantics.
- The module root owns the public in-process `Runtime` lifecycle and typed operation methods. `protocol` owns public request, response, event, error, version, and validation values. Do not define synonymous public models or a forwarding binding package.
- [`Qimu MCP`](../qimu/README.md#processes-browser-execution-and-distribution) owns typed backend transport and setup; `qimu/portable` owns immutable callable contracts. [`plugin`](../packages/go-sdk/plugin/README.md) supplies Flame model roles, logical Tool metadata and product receipt contracts. Memory, Schedules and independent Reviews consume those same public owners. Each backend retains its private business mutations and durable receipts; transport never turns an unrecognized failure into a definite business refusal.
- The Go binding and HTTP/JSON-RPC binding must enter the same delivery endpoint before capability admission, idempotency, invocation lifetime, Application use cases, and event or error projection.
- Domain packages own behavior-rich aggregates, value objects, invariants, and pure transitions. Aggregates validate construction, hide mutation, and expose legal domain operations; protocol, storage, configuration, provider, and projection values remain strict data. Application packages own cohesive use-case families and cross-aggregate ordering. Neither depends on protocol, storage, provider SDKs, or delivery types.
- Keep a Domain or Application package only when it protects a real aggregate vocabulary, invariant, or workflow lifecycle. Merge helper, identity, reference, and one-concept packages into their proven owner; do not replace them with one god package.
- Organize each large ring as `ring/context/package`. Context directories are non-package namespaces backed by multiple related packages; direct ring packages are limited to ring-wide mechanisms or aggregate packages that also name their context. Do not create facade parents, single-child namespaces, or deeper decorative nesting.
- Group external code by the system it translates: provider, MCP, SQLite, filesystem, Git, execution, LSP, or transport. Delete adapter-to-infra forwarding layers that own no policy or lifecycle.
- Bootstrap is the only concrete composition and shutdown owner. It constructs one delivery endpoint and one resource graph; do not maintain parallel Assembly, Host, Instance, Server, or optional-service lifecycles.
  Application and execution construction completes before startup reclaims retained content or reconciles abandoned ownership. Failed construction rolls back acquisitions and preserves retained product state.
- Interfaces belong to consuming Application or delivery code and contain only invoked operations. Keep a cohesive single implementation concrete.
- Run is the product execution center. Conversation, Transcript, WorkingContext, checkpoint, stream observation, durable Item, and recovery remain distinct facts with explicit owners.
- Provider/model selection is exact and durable. Provider SDK types, credentials, endpoints, request lowering, and provider-specific errors stay behind provider adapters. Optional provider capabilities use separate narrow consumer contracts; never inflate the ordinary chat client or approximate an unavailable capability in Runtime.
- Protocol operations express product behavior, not internal functions. Catalog metadata is machine truth; generated contracts and typed Go methods are checked projections of it.
- Public API, wire, storage, and generated-contract changes are replaced completely. Migrate all in-scope consumers and delete the former shape without aliases, fallback decoding, dual persistence, or compatibility packages.
- Prefer real single-Runtime end-to-end scenarios for Goal, Plan, steer, HITL, interruption, compaction, long context, long execution, provider failure, restart, and recovery. Test concurrency only where Runtime owns concurrent lifecycle; do not invent multi-client or multi-server scenarios without a product obligation.
- Decide compaction only at an imminent model call from that complete request's token footprint. Do not trigger it from protocol message counts or Run completion. Pre-release SQLite installs the current schema directly; do not add hand-maintained epochs or a migration graph without an explicit migration requirement.

## Model invocation history

`modelInvocations.list` reads recorded provider attempts for one exact Run, newest first, with an opaque cursor and a maximum page size of 100. Child Run reads require the subagent capability, just like `runs.get`. The Go binding exposes the same operation as `ListModelInvocations`.

The invocation journal owns call identity, Segment identity, observed state, and timestamps. These records survive Run completion and restart; deletion of their Run cascades to the records.

`started` has no settlement timestamp. `completed` means Runtime accepted the complete model response. `failed` records an observed failure at the model boundary, including an invalid response or interrupted stream; it does not imply that the provider performed no work. `unknown` means execution or recovery could not establish the outcome: its `settledAt` is when that uncertainty was recorded, not a provider completion time. Consumers must not infer a measured model duration or throughput from an unknown outcome.

The timestamps come from Runtime's call lifecycle. `startedAt` is captured while reducing the start fact, before its durable commit and provider dispatch. Completion is captured after response validation and the stream projection barrier, before the completion commit. For completed and failed calls, their difference measures this Runtime lifecycle interval, including local work and waiting; it is not isolated provider latency. Neither lifecycle timestamp records the first output. Decode throughput cannot be reconstructed from this interval, token totals, or transcript timestamps.

The optional `firstOutputLatencyMillis` is measured with the monotonic clock at the streaming model boundary: from entering the provider stream to arrival of the first valid text, visible reasoning, refusal, media, or tool-call delta. It includes provider-client and network work, but excludes Run admission and the start commit. Metadata, usage, finish markers, citation attachments, and opaque reasoning state do not count. The measurement is committed with a completed or failed attempt; a stream that fails after producing output retains it. Nonstreaming calls, failures before output, unknown recovery outcomes, and historical attempts leave it absent. A measured zero is valid. It is not a token-generation rate or isolated model compute time.

The optional `usage` records provider-reported tokens for that call before Run aggregation. Missing usage means it was not reported or the attempt predates usage recording; an explicit zero remains zero. Prompt inspection is not present in this read. Aggregate Run accounting remains separate.

## Session trajectory

`sessions.trajectory` / `ListSessionTrajectory` reads one bounded page of durable
Run, logical model-invocation, physical model-endpoint, and transcript Item
observations for a Session. The default
and maximum page size is 100. `includeDescendants` defaults to false; requesting
true includes the complete Run tree and requires the subagent capability.

Entries reuse `RunRef`, `ModelInvocation`, `ModelEndpointInvocation`, and `Item`.
Session-associated endpoints without a Run remain included with either descendant
filter. Endpoint request/response JSON strings retain original numeric evidence;
logical calls and physical endpoints can describe the same consumption and must
not be added together. Session deletion removes its endpoint records atomically;
new calls cannot attach to a missing Session. Unattached calls are not included. Their discriminated source
and original identity identify the row; there is no additional trajectory journal
or execution state machine. Run records retain outcomes, unresolved effects,
accounting, and parent/root edges. Model records retain the exact call and Segment
identities and reported measurements. Items retain their content, tool arguments,
results, approval decision, and original timing. An Item without a recorded model
association must not be assigned to a model call by timestamp proximity. Fetch an
entry's owning Run through `runs.get` when that Run is outside the loaded page.

The order is newest occurrence first, followed by source kind and source identity
as deterministic tie breakers. Run admission, model start, and Item occurrence
supply the respective timestamps. This is an observation ordering, not a global
event sequence or proof of causality. Each entry contains its source record's
current state, so completion updates that entry without moving its page position.
Unknown model settlement remains an observation of uncertainty and supplies no
measured execution duration or inferred throughput.

A page's existence check, source selection, and record hydration share one SQLite
transaction. Its cursor is opaque and bound to the Session and descendant filter.
Pages remain bounded as history grows. Continuations do not freeze an entire
Session across subsequent writes; refresh the read after execution changes, and
use the idle Session evidence export when a coherent complete document is needed.
Completed observations survive restart for as long as their source records remain.

## Agent Memory authoring

Agent Memory content edits transfer the item to active user authorship while preserving
its identity and creation time. Automatic curation cannot prune that correction. Pin-only
updates leave authorship and review state unchanged. Persistence stores the Item's complete
transition, including provenance and derived embedding invalidation.
Older automatic items contain no durable evidence of previous user edits, so they are
not reclassified by inference. Explicit review can retain an existing pending correction.

## Auxiliary model observation

Compaction, memory extraction and curation, skill mining, and title generation emit an `auxiliary model` OpenTelemetry span through the existing Runtime exporter. `auxiliary.operation` identifies the caller's purpose. Auxiliary calls follow their caller's cancellation and deadline; the adapter adds no wall-clock timeout. Input and output envelopes remain bounded. The span covers selection resolution, the model request, and response acceptance; its duration is not isolated provider latency. The live resolver records the exact provider/model selection, and a valid response contributes its finish reason and reported token usage. Optional cache and reasoning counts remain absent when unreported, including the distinction between absence and an explicit zero. A valid but incomplete response retains its reported usage even though its text is rejected.

Failed attempts record the `resolve`, `call`, or `response` stage and distinguish cancellation from deadline expiry. These spans do not record prompt text, output text, or raw provider errors. They are diagnostic telemetry, not durable `modelInvocations.list` records or additional Run accounting. Telemetry retention depends on the configured exporter. Physical requests and known outcomes are separately recorded by the same endpoint observer used for Interaction, in the existing invocation journal. Session-associated records participate in trajectory/export and Session deletion; calls without that association are accessible by their original caller ancestry. Physical evidence includes auxiliary consumption in public usage summaries; Run-linked calls accrue through the existing Run accounting owner, while unattached evidence is retained without automatic expiry.

Auxiliary text generation uses Scope's `Client.Output` with its `Text` contract. Scope admits only naturally completed text and reasoning; refusals, media, and Tool calls cannot become summaries, memory, skill proposals, or titles by dropping their non-text parts. Runtime additionally rejects blank text and retains its resource limits and observation policy.

Title generation is nested under `run segment maintenance`, with `run.id`, `gen_ai.conversation.id`, `maintenance.operation`, and `run.parked` identifying its boundary. A parked Run can generate its initial Session title while waiting for user input; this span does not mean the Run has completed. Workspace checkpoints use the same span name and run only at a terminal boundary.

## Trajectory evidence export

`sessions.exportTrajectory` / `Runtime.ExportTrajectory` exports one JSON evaluation
document with `schemaVersion` (`protocol.SessionTrajectoryVersion`) and a collection timestamp. It includes the Session,
all root and child Runs, Items, retained conversation messages, offloaded Tool bodies,
current Plan, logical model attempts, Session-associated physical model endpoints,
Tool attempts, and matching user feedback. Child evidence
requires negotiated `subagents`; the export never silently omits it. The existing
`sessions.export` / `sessions.import` portable artifact keeps its own version and
separate import contract.

Export holds the idle Session admission and reads every source in one SQLite
transaction. An active or waiting Run returns `session_busy`; finish or cancel it
before exporting. The complete stored input is limited to 64 MiB and 100,000 source
records before bodies are loaded, and the final JSON response is independently
limited to 64 MiB. Exceeding either bound returns `export_too_large`, with no partial
document. These bounds apply to evidence export, not ordinary trajectory pagination.

The export contains all currently retained evidence, not every historical state
transition or every provider request. Compaction can replace old conversation context;
retained Session-associated Scope requests and known responses are included in
`modelEndpoints`. Raw provider HTTP bodies, unattached calls, and auxiliary telemetry
spans are absent. Schema version 3 introduces this collection; rebuild Go/TS/Core
consumers together. Original JSON strings must not be decoded and re-encoded through
a lossy numeric representation.
Model and Tool journal records survive terminal Runs and Runtime restart until
their Run is deleted. Tool attempts preserve separate
Segments: `completed` means an observed definite result, `incomplete` includes
suspension for input, and `started` carries no known settlement. Neither these states
nor Run completion evaluates answer quality.

Feedback remains an independent append-only user signal. Matching any supplied
Session, retained Run, or retained Item reference includes the original observation;
references remain unverified and may identify records already removed. General
feedback without a matching reference is excluded. Export never manufactures a score
from execution status, reconstructs missing attempts, or fills absent usage with zero.
Each Run records its own process's usage. Tree totals include the root and every
descendant; combining those totals with the same per-call usage counts consumption
twice. Goal usage derives cost and steps from its terminal trees while counting only
their root Runs as Goal executions. The document carries these limitations
alongside its evidence so downstream evaluation can preserve the distinctions.

## Background shell lifetime

Background commands remain addressable after they exit until `read_shell_output` consumes their final output. That final read reports completion and releases the shell handle and retained buffer; later reads report that the shell is absent. Compaction reminders preserve these retained handles, including commands that have finished with unread output. Reads while a command is running keep its handle available. Stopping a command preserves its unread output for the final read. Session teardown and Runtime shutdown also reclaim owned commands.

The shell owner reads output and retires a completed handle as one operation.
A terminal cleanup failure retains the handle and unread bytes for teardown,
and reads continue to report that failure. Tool errors preserve already observed
output through Scope's `CallError` evidence, including cancellation after process
execution; that evidence does not declare an uncertain command safe to retry.

Reading or stopping a shell requires its owning Session, even when another Session knows the exact shell ID. Host teardown retains its authority to stop a Session, a shared workspace, or the complete Runtime. A call canceled before process admission cannot launch a detached command.

Shell output uses Scope's `tools/content.Content`: `stdout` is an object with `encoding` (`utf8` or `base64`) and `data`. Both foreground completion and `read_shell_output` preserve arbitrary process bytes, including output truncated inside a UTF-8 character. Background launch responses include an explicit `shell_id`; incremental reads include `shell_id`, `status`, `stdout`, and an optional `output_dropped` flag. The command transcript still renders valid text directly and represents binary output with its lossless encoded content. Consumers of raw Tool output must use this current shape; there is no string-output compatibility decoder.

## Execution lifecycle

Replay windows and subscriber queues reuse one publication charge for the full
Item snapshot, including accepted answers, rejected argument source, approval
edits and reasons. The journal adds its assigned cursor before fanout. An event
over the byte budget is evicted and disconnects a slow subscriber explicitly.

Tree-barrier preparation reduces every active member against speculative state.
Only the accepted atomic barrier transaction advances all live reducers, before
publishing any of its events. Refusal or cancellation retains the previously
committed model/tool starts so terminal settlement can close them completely.

A committed waiting Run has no active Segment. Its Run-tree owner retains the
join and admission through parked maintenance; cancellation during that boundary
returns `session_busy` and can be retried after the owner releases it. A live
Running Run must still match its exact active Segment.

Manual and cron firings pass only their captured Run request into the scheduled Run
entry point. That request owns instructions, model selection, title, workspace and stable
Session/Run identities; the launcher cannot author parallel overrides. An omitted workspace
uses the Runtime default, and an omitted model uses the prepared Session's default. Both
scheduled and ordinary starts share the same admission, staging and opening transaction.

Root and delegated Interactions, ordinary Tools, and lifetime child/process counts use Scope's unlimited cumulative quotas. Usage remains metered; completion, cancellation, model-reported blocking, and actual failures control lifecycle. Delegation depth, active-child and Tool concurrency, pending mailbox capacity, and operation-specific waits bound resource use. Execution follows its owner's lifetime.

## Scope execution settlement

Runtime uses the released Scope modules pinned in `go.mod`. Static Tool and Delegate
bindings belong to Scope Deployments through `Definition.ChildDeployments`;
Scope owns their identity, start, and restoration. Provider construction uses
Scope's `Chat`, `Messages`, `ChatCompletions`, or `Responses` entrypoint for the
selected API.

This upgrade changes Scope's Deployment and execution-tree snapshot schemas.
Complete or cancel waiting executions with their owning build before upgrading.
Older checkpoints are rejected under the existing build-identity and strict
snapshot rules. Completed product history requires no migration.

Scope alone propagates root, subtree, and owner cancellation to execution contexts. Runtime submits cancellation intent and projects Scope’s immutable termination; a late cancellation or owner deadline cannot replace an already settled failure. Provider diagnostics enrich only the model-failure stop acknowledged by Scope.

A validated complete model response and its reported usage publish under the release-owned lifetime, including the Delta barrier. Execution cancellation cannot discard that observed response or rewrite Scope's terminal outcome. Failed streams cross the same barrier and commit their validated text and visible reasoning as `incomplete` transcript Items tied to the failed model invocation. Preview loss cannot truncate this retained prefix. Partial Tool calls, opaque reasoning state, and incomplete responses never enter canonical model continuation. Both completion and failure close the preview boundary before terminal publication.

Model-call completion is the sole live source of Run accounting progress. The same
acknowledged fact atomically commits the model invocation, response Items, Run metrics
and conversation; its progress event projects that write-set. There is no independent
usage event that can advance consumption without an identified model call. Final
process accounting retains observed consumption through termination and resume.

Executor interruption uses one validated `TreeInterrupted` control payload. It captures
the checkpoint and external-input bindings for an atomic tree barrier. Individual Run
interruption is only an internal projection of that barrier; executor sources cannot
emit a separate per-Run interrupt event.

Repeated identical Tool results are allowed, including polling. Tool authorization and lifecycle-hook approvals remain enforced.

Canceling a delegated Run still cancels that child. If its in-flight model attempt returns no definite result, Scope retains the unknown Effect and fails the parent instead of delivering a normal delegate result. Runtime projects the actual Scope outcome and preserves unresolved Effect IDs separately; it does not manufacture a settlement or retry the attempt. This intentionally replaces the earlier behavior that allowed the parent to continue despite the child's unresolved Effect. Cancellation before external dispatch and terminal children with definite results remain distinct cases.

Known Tool and Delegate results, including rejected calls, are extracted through Scope's `interaction.SettledResults` at its `TreeCommitter` boundaries. Runtime atomically commits the authoritative execution-tree head, product Items, invocation journal, and content-bound result receipts. Each receipt retains its exact model-visible Tool results, including ordered content, metadata, and structured details. Settled calls retain their original round indexes even when other calls remain unresolved. The model conversation receives one ordered Tool message when the round completes. Live termination, cancellation of a parked Run, and boot recovery share the same conversation closure: already acknowledged results retain their exact output, and only unresolved calls receive an unavailable-result marker. The reducer keeps no separate conversation ledger.

Result receipts bind to the durable source message's unique sequence identity before the complete Tool message is appended. Reused provider call IDs in another round cannot adopt prior results. Compaction, rollback, and history replacement invalidate removed message references while retaining immutable publication identities for deduplication. A Run whose unresolved Tool calls lack exact receipt evidence fails terminal reconstruction explicitly; its acknowledged work is never silently rewritten as failure feedback.

Tree updates compare the previous writer and digest and require the next incarnation-local commit sequence. Checkpoint identities include the writer and sequence; Effect identities include the Effect ID and boundary kind across writers. SQLite commits the head and deduplication facts atomically. Restoration activates a new writer at sequence zero. A historical commit cannot succeed merely because tree content repeats. Repeated known results are checked against durable receipts before retired presentation metadata is accessed. Ambiguous COMMIT responses are reconciled by reading the tree and product receipts, without rerunning tools. Cancellation stops new execution and model continuation while observed settlement writes remain owned by the executor's release lifetime. Storage latency has no separate elapsed-time failure threshold: release cancels pending publication, then joins the execution workers and Scope Engine before revoking Tool resources.

Process-local fact and child-start receipts retain the pump's decision; observing a
receipt never consumes or changes that decision. Withdrawal of a queued child-start
request retains its original cause. After claim, waiting still joins the conclusive
decision even when the producer's context is canceled.

Recovery consumes the storage-owned complete and unique non-terminal Run
catalog. Domain construction and database constraints own individual records and
active-root uniqueness; Recovery owns tree topology, cross-Session consistency
and waiting-barrier coherence.

Recovery probes only validate persisted state; they never activate a writer. Waiting checkpoints retain product bindings, while the committed execution-tree head is authoritative for restoration. A checkpoint holds no copy of the root Run's model selection, capabilities, or Goal incarnation, nor of the Session's workspace or isolation; restoration reads each from its owner. This integration requires the current Scope snapshots and the current execution-tree schema. Older waiting snapshots and databases are not converted or replayed. Schema installation rejects the former execution-tree table without modifying existing data; use a fresh data directory and retain the old one separately.

Plain executor errors, lost responses, cancellation, and host failures do not prove a business outcome. They remain unknown and cannot produce normal model feedback. A concrete executor may return an explicit Scope Tool failure with its known output, including partial success; validation and authorization owners may reject calls they have not executed. Runtime's generic Tool observer never converts arbitrary errors into known failures. Approval-subject validation translates only a definite local argument refusal into a known failure, retaining effective arguments and result metadata through the same durable settlement path. Hook and authorization rewrites are validated before executable entry; arbitrary hook or authorization-store errors remain host failures. Product cancellation closes abandoned Items without inventing Tool results.

A direct Tool completion ends the Run after its ordered results commit. It adds no assistant answer and makes no further model call; a delegated direct completion supplies those same results to its parent.

Tool concurrency declarations describe the effective executable boundary. Wrappers that can change arguments through hooks, authorization, or approval continuation declare exclusive execution, because an inner resource key computed from the original arguments is no longer safe. Immutable invocation paths preserve the inner declaration; Scope remains the scheduler.

Scope owns the model-visible payload. Delegate results use `interaction.Output` directly; the former `{"reply": "..."}` envelope is removed. Runtime adapts the task input, while Scope owns the complete execution and output, including after restoration. Runtime commits assistant content and accounting at the model boundary before returning the response to Scope. Process termination closes the product Segment without another message event, reply cache, or confirmation handshake. Runtime never reconstructs Delegate output from UI text or independently formats Delegate diagnostics. Pending product metadata, including effective arguments, mutation paths, and offload references, accompanies the Scope checkpoint until its complete model round is published. Delegate admission retains its invocation metadata independently of the currently active child batch, so an earlier Delegate result survives when a later batch waits for input. Resuming that tree preserves the completed work.

Rejected calls retain their original input as `argumentsText`; parsed `arguments` remains an empty object. Transcript storage, history artifacts, CLI, and Desktop preserve this distinction, including malformed JSON. Existing records need no schema migration; previously omitted results cannot be reconstructed from the database alone. Run aborts persist the first causal diagnostic instead of replacing it with a generic internal error.

The Interaction snapshot shape changes with this contract. Waiting checkpoints from an older build must be completed before upgrade or discarded under the existing build-identity rule; no dual-schema reader is provided. Runtime uses a Scope engine backed by its SQLite TreeCommitter and restores only committed waiting checkpoints. It has no mid-run crash recovery: a restart with unfinished execution retains the existing `RunLost` policy. A lost Run's failure detail names why it could not continue: a restart outside a waiting boundary, an isolated workspace, an unavailable workspace, waiting state written by another build, waiting state that cannot be restored, or a configuration change while it waited. The probe's finer reason and cause stay in the trace. All Scope commits fence publication by Tree incarnation, commit sequence, and retained commit identity.

Waiting checkpoints declare the offloaded result IDs required by their continuation. SQLite saves these references atomically with the checkpoint, verifies Session ownership, and releases them when the checkpoint is replaced or consumed. Startup and failed-write cleanup delete only bodies held by neither a checkpoint nor an Item. Restoring an executor verifies that the declared references exactly match its pending result metadata; old checkpoints missing this ownership information are rejected under the existing recovery policy, without a compatibility reader.

Unknown-effect observations retain the Effect IDs and the first available local failure diagnostic. The RunLost record preserves these details while keeping the outcome unknown; diagnostic text is never evidence that an external operation succeeded or failed.

Portable Session artifacts preserve unresolved effects on every terminal outcome, including canceled and timed-out Runs. The artifact retains the exact source process and Effect identities, cause, reason, detail, and any observed Scope ToolOutput through export and import. These are read-only historical evidence: imported Runs have no active Segment, open interrupt, or execution checkpoint, and the identities do not authorize resume or retry. Import accepts only the current artifact version; earlier development artifacts are rejected rather than treated as complete evidence.

Unknown-effect termination closes every unfinished member in Run-tree postorder, retaining the same evidence on each lost Run. Completed members keep their outcomes. Each terminal commit settles open model attempts as unknown and abandons unfinished Tool Items without inventing model-visible results; the executor is released only after the terminal publication sequence.

## Workspace observations

Workspace file content has one read operation, `workspace.files.read`. A preview
requests a window starting at line 1 and consumes the same byte budget, source
validation and truncation result as any other read. Core `2026-10-23.1` removes
`workspace.files.head` and its Go request/result types. Rebuild clients and
replace former calls with `ReadWorkspaceFile` and an explicit line window.
This changes the public protocol, not stored content.

`runtime.subscribe` accepts exact workspace-relative `watches[].paths`; `"."` names the workspace directory. Omitted paths request only Git HEAD/index observation. Explicit paths observe ordinary external writes, atomic replacements, removal, and recreation in Git and non-Git workspaces. A directory target observes its immediate entry names and metadata; it does not recursively watch children or follow their symlinks. An opened file needs its own target to observe content changes.

Discovery publishes the subscription's total path budget, per-directory entry budget, and file hashing budget. Content up to that byte budget is hashed; larger files remain observable through size and modification metadata. Exceeding directory or registration bounds fails explicitly. Background content-observation failures, including those of the Hooks and Skills observations behind `hooks.changed` and `skills.changed`, end the subscription, and Git sampling failures retry with bounded backoff before ending it. Clients reopen the stream and revalidate their reads; this is an invalidation channel, not a durable filesystem log or atomic snapshot. Notifications retain workspace, watch, and path scope; overflow can widen them.

Directory registrations follow the filesystem backend's current watch set.
Automatic retirement and explicit removal may overlap; an already absent
registration completes removal. Other registration failures remain explicit
observation failures. Accepted application writes advance only their exact
content identities, so later external writes and recreated paths stay observable.

## Workspace search outcomes

`grep` and `glob` select either one regular file or a directory's file corpus. Selection, physical containment, ignore rules, and resource limits belong to the workspace catalog. Selecting a file never searches its siblings. Glob patterns match paths relative to a selected directory, or the basename of a selected file; returned paths remain workspace-relative. Git selection treats path names literally, including glob metacharacters.
Read and search errors retain their original admission categories and causes,
including the offending line number. Cancellation retains its standard category
and the caller's cause. Search excludes a binary or unpageable text file only
when its actual input did not fail; buffered text cannot conceal a pending read
failure. Read, version verification, capacity and close failures survive together,
and no matches enter the result until the file passes that complete admission.

Filesystem tool manifests require an explicit absolute workspace and share one pinned directory authority with Scope. Tool paths are absolute or workspace-relative; home shorthand (`~` or `~/...`) is rejected rather than consulting ambient process state. Read stamps, mutation protection, edits, patches, and in-process formatting continue to address that directory after a rename or pathname replacement. Path-based search and external formatter configuration require the original named workspace; detached searches fail explicitly, and unavailable formatting or LSP diagnostics are reported with the successful mutation. Multi-file patches are not transactions: their failed results and mutation metadata retain Scope's acknowledged partial effects.

LSP diagnostics succeed only after observing the synchronized document version,
including an explicitly empty analysis. Missing or stale pushes report pending
with their observed version, and older pushes cannot replace newer diagnostics.
Workspace symbol searches retain every failed language alongside successful
symbols; partial results are explicitly marked incomplete.
One connection slot per workspace/server owns startup, the live connection and
retirement before replacement. A later query replaces a disconnected server
after joining its resources, without replaying the failed operation. Concurrent
queries share that replacement; shutdown cancels and joins the entire slot.

Formatting admits complete output up to the existing 8 MiB file bound. An external
formatter owns its process group through cleanup, including after a successful root
exit. Its failure retains process, capacity, cleanup and cancellation evidence;
formatting diagnostics never replace an acknowledged file mutation. An absent
optional formatter is skipped; other executable lookup failures remain explicit.

Only the model's successful stable `read` creates read evidence. An admitted
`edit` or `apply_patch` consumes its targets' evidence before execution, even
when the outcome is unresolved. Read an existing file again before another
mutation; successful writes and formatting no longer grant fresh read evidence.
The guard performs no post-write filesystem observation, so cancellation after
an acknowledged write cannot erase that result. This changes only process-local
Tool policy and requires no protocol or persisted-data migration.

Read and local search share the non-mutating Tool failure boundary. Unsuccessful queries, including invalid patterns and missing paths, become explicit failed Tool outcomes, so Scope commits their feedback before model continuation. Cancellation remains execution control. The generic Tool observer still preserves unknown external outcomes and host or publication failures.

Structured diff code rows always carry `code`, including `""` for a blank line; hunk rows omit it. The Go binding represents this presence with `DiffRow.Code *string`. Go consumers must migrate string construction and access; JSON consumers retain the existing required-string contract. Missing or null code remains invalid for code rows. No persisted-data migration is needed.

Workspace diff responses require `baseline`: `head` and `mergeBase` include the exact resolved commit; `emptyTree` identifies an unborn repository. The comparison uses that resolved object, even if a branch reference moves later. Worktree mode includes untracked files; base mode compares tracked working-tree contents against the merge base. Neither mode attributes all edits to an Agent or provides an atomic snapshot of concurrent filesystem edits. Update consumers together; no stored diff migration is needed.

## Workspace file rollback

File checkpoints require physically separate workspace and checkpoint-storage
trees, including through symlink aliases. Snapshot and restore reject either
tree containing the other with `checkpoint_unavailable`, before creating or
changing checkpoint state. With Runtime's standard `<DataDirectory>/checkpoints`
layout, the data directory itself and its ancestors cannot serve as checkpointed
workspaces: Runtime durability must not be archived as project material.

An existing checkpoint repository is never rebuilt from its current HEAD state:
retained Run boundaries belong to that repository even when HEAD has no commit.
Repository inspection and Git execution failures remain errors, preserving their
causes. Only an absent repository or target ref reports a missing checkpoint;
checkout failure retains the incomplete-restore classification for recovery.
One workspace checkpoint adapter owns snapshots, restoration and Session cleanup.
Restore projects storage refusals into application categories without replacing
their causes; snapshot cancellation retains the inspection error chain.

Session deletion and history replacement commit their resource retirement in the
same SQLite transaction as the history mutation. An uncommitted command never
authorizes checkpoint destruction. After commit, Runtime removes the predecessor's
checkpoint and scratch namespaces, then clears that exact retirement. A cleanup
failure leaves the committed history result intact and retains the durable
responsibility; Session mutations and Run admission remain fenced until cleanup
completes. Startup finishes those responsibilities before workspace and Run
recovery, including when the retired Session row no longer exists.

Scratch copies use a managed per-Session container so recovery can reach copies
left by an abruptly terminated process. They remain ephemeral and are never
written back to the source project. The new committed-retirement table requires
a fresh Runtime data directory. Former database shapes are refused
without conversion; the former scratch layout has no compatibility reader. Retain
earlier data separately.

Git command observation retains simultaneous output-admission, process, cleanup
and cancellation failures. Output above 64 MiB is refused; cancellation does not
erase that capacity failure or its owner-supplied cause. Each invocation owns and
stops its process group, including after a normal root exit. Ordinary nonzero
exits remain observed data for Git's predicate and diff semantics.
Root exit is observed independently of pipe EOF. It starts group retirement and
a bounded output drain; the drain budget never limits a still-executing root.

Workspace file selection converts resource paths into literal Git pathspecs at
one adapter boundary, shared by file listings and raw/structured diffs. The patch
parser consumes each hunk's declared left/right ranges before interpreting file
metadata, so header-like source lines retain their file identity and line counts.

Checkpoint predicates accept exit codes only from admitted Git command results.
A process failure carrying the same status remains a failed observation. Object
alternate admission preserves cancellation and I/O failures; only opening an
absent alternate file permits an empty seed.

The source Git repository selects tracked and non-ignored untracked paths under
the exact workspace, including parent/global ignores and `info/exclude`. Build
backstops exclude only untracked paths. Git interprets its own ordinary, linked
worktree and split indexes; Runtime never copies their bytes or flags. Each
snapshot replaces the shadow index's admitted path set, bounded to 20,000 paths
and 512 MiB of file material, with the per-file limit below. Object borrowing is
storage reuse only. The `flame-source-workspace` metadata binds this ownership
contract; former checkpoint directories are refused without conversion. Retain
old data separately and use a fresh Runtime data directory after upgrading.

Working-tree admission compares physically resolved path ranges. A destructive
mutation excludes pending, live and maintenance Runs in either ancestor or
descendant workspaces, and excludes another overlapping mutation. Disjoint sibling
trees remain independent. Detached shells whose workspace ranges intersect the restore are joined before
checkout. The same path relation fences new and resumed Run admission against
retained mutation intents after restart; recovery refuses an inconsistent set of
overlapping retained intents instead of choosing an order that can erase work.

`sessions.rollback` with `files` or `both` restores a Run's checkpoint for that
Session and workspace. Checkpoints archive admitted regular files and symlinks,
respect ignore rules for untracked paths, and exclude files larger than 2 MiB.
They do not represent every file in the workspace. Before restoration, Runtime
archives the currently admitted state and checks target paths against everything
left unarchived, including ignored files, oversized files, and blocking
directories. A conflict preserves the current files and history and reports
`checkpoint_conflict` after the request's recovery intent has been cleared.
Move conflicting material out of the target paths before issuing a new request.

`both` restores files before committing the history cut. Once checkout begins,
a failure can leave partial filesystem effects, so Runtime retains its durable
intent for recovery. If cleanup of a refusal's intent fails, the response remains
an internal failure with a pending recovery intent; it does not report a
definitive `checkpoint_conflict` or `checkpoint_unavailable`. Re-driving an
existing intent also retains it on refusal, since an earlier attempt may have
already changed files. Consumers must keep
the original command identity while its result is uncertain. Runtime serializes
its own writers and uses Git's protection against overwriting untracked and
ignored blockers. External filesystem writers do not share Runtime's guard;
the operation is not an atomic filesystem or filesystem-plus-history transaction.

## Steer admission

`runs.steer` and `Runtime.SteerRun` return `userItemId`, the identity reserved for that input. Success proves admission to the addressed active Segment, not model consumption. Only the matching committed user Item proves application at a model boundary. Clients reconcile by identity, including when the Item arrives before its receipt; identical text or attachments do not identify a command. A rejected steer must not silently become a new Run.

Protocol `2026-10-14.1` makes keyed Steer acceptance Run-owned. Its original namespace, key, request fingerprint, reserved Item identity and immutable input commit with the Scope Signal admission head and execution context. `runs.getAcceptance` supports `steer` without redelivery or writer recovery. A consumed Signal receipt is not an applied input: only the exact completed User Item proves application. Protocol `2026-10-14.3` projects those receipts through `sessions.snapshot.steerInputs` while their original material remains present, including after the exact Item commits or the Run finishes. The SDK caches the pure-query Item nomination before dispatch so ACK and Item can arrive in either order. Replaying a recorded command returns its original identity even when the original Run finished or its material was retired; importing the same Run ID never refills a retired receipt. Ordinary active execution still follows its existing crash policy.

Upgrade Runtime, Desktop and CLI together. The current SQLite command-record shape refuses older data directories without conversion. Retain the old directory separately and use a fresh directory. The SDK journal requires a concrete frozen Start, Resume or Steer command; other mutations still persist no request bodies. Older journal shapes are refused and preserved, not cleared or decoded through an adapter. Omitted original request bodies cannot be recovered from their hash.

## Command completion and configuration boundaries

Installed Schedule operations return the business owner's refusal through plugins.call; the former Core schedule_not_found/revision/workspace translations no longer exist. The publisher owns missing-item, stale-revision and workspace rules and atomically commits private receipts. Hook trust changes continue to declare workspace_unavailable when their directory cannot be resolved.

A successful start commits the Run, first Segment, and opening user Item. Resume commits the accepted interrupt responses and the new Segment opening. Neither promises a provider call has completed. Cancel is a settlement barrier: success proves a canceled Run; a natural completion that wins the race returns `run_finished`. Failed cancellation settlement remains an internal failure even when the Run is already terminal; native callers retain the cleanup cause, and clients read the actual Run outcome without treating the failed command as successful cancellation. Steer admission is described above. A missing receipt is an unknown command outcome, not permission to repeat it with a new identity.

The TypeScript SDK's Start and Resume now return `RunOpeningResult`: the original
acknowledgement is `result`, and `observation` is a closed `stream`, `snapshot`,
or `retired` resource outcome. Consumers must migrate from the former mandatory
`events` field and supply negotiated idempotency capabilities or a mutation
journal. `createFlameClient` provides an in-memory journal when no durable storage
is supplied; Desktop supplies its persistent journal. Every attempt first reads
`runs.getAcceptance` using the original namespace, key and operation. A known
receipt settles the command without another mutation; failed lookup retains the
original identity, and `notRecorded` only permits dispatch while the original
replay admission still holds. Receipt reads remain possible after expiry or
store replacement. Neither reopening nor cold retry renews that age. Persisted
journal entries use the current shape described under Steer admission. The SDK exposes `runs.recoverCommands()` for reading unresolved original Start, Resume and Steer commands after application restart. It queries their original receipts without dispatch, returns each known or unknown observation, and exposes explicit `retry()` using the frozen original request. Query failure, expiry or store replacement cannot mint a new key, retarget a Segment, or renew the original deadline. Accepted receipts settle the client journal; Runtime snapshots retain any still-unapplied input.

Disabling or deleting a schedule stops future occurrence claims. An already claimed occurrence retains its accepted input and may still start after that change; its Run must be canceled separately when required. Editing a schedule does not rewrite an occurrence already claimed from an earlier revision.

Calendar cron expressions use UTC unless they declare `CRON_TZ`. Creation,
editing, and recurrence use that same rule regardless of the caller's local
timezone. An expression with no reachable occurrence is invalid.

Abrupt process-exit tests exercise claim, business commit, and receipt commit separately against a temporary SQLite database. A committed receipt replays the same identity without executing again. A claim without a receipt remains unresolved, including when the business effect committed: restart and elapsed time do not prove success or failure. Keep the original key and store namespace, inspect authoritative session state, and do not issue a fresh command to bypass the reservation. There is no general automatic reconciliation across command receipts and arbitrary business or external effects.

While the Endpoint retains a known execution result, a retry or graceful shutdown
can persist only that exact result into its original reservation. An acknowledgement
lost after the write is confirmed by the same completion. A missing reservation or
a conflicting stored payload is an internal failure; the Endpoint retains its evidence
and refuses shutdown settlement instead of recreating a claim or adopting another outcome.

For an existing Session, omitting both provider and model uses that Session's stored selection. An explicit pair overrides it for the Run. Global defaults do not silently replace an existing Session's selection.

Process configuration decodes YAML, defaults, and environment overrides into one
typed snapshot. Invalid boolean and integer overrides reject startup, including
settings present only in the environment. List overrides use comma-separated
entries; the resolved configuration consumes the same values that were validated.

| Change                | Effective boundary                                                                                            |
| --------------------- | ------------------------------------------------------------------------------------------------------------- |
| Utility model         | Next utility invocation, including during an existing Run; an in-flight invocation retains its resolved model |
| Embedding model       | Subsequent semantic searches                                                                                  |
| MCP configuration     | Persisted first; enabled servers connect in the background, and resource status reports readiness             |
| Project hook trust    | Next Run opening; waiting resume is not a new Run                                                             |
| Skill archive/restore | Subsequent resolution; existing conversation context is not erased                                            |

An unset utility role uses the Runtime composition's default model selection. It does not follow each Run's explicit main-model override. Changing the utility role does not replace the main-model deployment of a live or waiting Run. Saving configuration is not a universal live-reload guarantee. Restart recovery validates the retained model and tool deployment; it does not silently reinterpret an existing execution using arbitrary current settings.

## Discovered Skill inspection

`skills.discovered.list` returns a complete bounded `skills` catalog and `diagnostics`, replacing the former page shape. `skills.discovered.get` / `GetDiscoveredSkill` reads one named document on demand and returns its selected scope, source path, instructions, and SHA-256 revision of the exact document bytes. Upgrade consumers together; there is no legacy page adapter.

Discovery, inspection, and model loading share one source-selection rule and Scope's document format. Project bundles own a colliding name even when malformed; they never expose the user copy as a fallback. Invalid or oversized selected documents appear as diagnostics alongside usable entries. Filesystem, confinement, capacity, and cancellation failures remain explicit query failures. Instructions retain the existing 1 MiB document bound and source directories retain their entry bounds. Inspection describes current authored content, not proof that it was loaded into an existing model conversation.
Document admission classifies a non-regular `SKILL.md` as an invalid selected bundle,
preserving its file-kind cause; oversized documents retain their size-admission cause.
Cleanup failures remain I/O failures rather than skippable document diagnostics.
Skill sources preserve both the standard cancellation category and any custom cause,
including for an empty catalog. Read failures remain identifiable when cancellation
occurs concurrently. Resource reads enforce their byte limit before returning a count,
so cancellation cannot expose an extra byte or conceal a simultaneous size failure.

Each execution resolves its callable Skill names, selected sources and installation
dependencies together. A later directory change cannot add a name, replace its source
or resolve an admitted package conflict inside that execution. Removing the selected
bundle fails its reads; a fresh execution adopts the current selection. Authored
documents can still be edited within their selected bundle. Package resource authority
retains its exact release dependency even when its document is unavailable or omitted
from discovery, and every resource read still checks current installation authority
and verified bytes. Discovery and inspection resolve a fresh selection for each query.
Package metadata lookup projects the selected release's admitted name and description
without loading instructions. Release admission validates that complete metadata with
Scope; document and resource reads separately enforce current authority and exact bytes.
No storage or protocol format changes are required.

Managed catalogs, proposal review, approval, and idle curation load verified document bytes through the same Scope format and directory-name binding used by discovery and package admission. Idle curation leaves invalid documents in place. Archive and restore serialize lifecycle moves and usage changes under one library lease; a replay does not reset subsequent activity. Malformed usage metadata fails recording and curation without replacing history or moving an active Skill. Repair the retained metadata explicitly before retrying; Runtime does not reset it to a fresh grace floor.

Automatic user-library revision reads use the same bounded document source as
discovery and execution. Invalid or absent documents cannot enter revision
generation; oversized documents retain the Runtime admission category and cause.
Valid revisions remain mined proposals that require explicit approval.

Inspection distinguishes invalid names (`invalid_params`), absent skills (`skill_not_found`), and invalid or oversized selected documents (`skill_unavailable`). Archive and restore also report `skill_not_found` for absent entries. Consumers should refresh a missing resource and offer document correction for an unavailable resource; public errors exclude parser diagnostics and document contents. Proposal revision failures remain `revision_conflict` because the reviewed content must be fetched again before applying it.

## MCP OAuth credentials

MCP OAuth credentials belong to one persisted authorization grant per MCP source, bound to the fingerprint of the OAuth target that requested it. Starting an explicit sign-in replaces that grant; token refresh and credential rejection can update or remove only their own grant. A changed credential recipient (a user server's endpoint or headers; an installation server's endpoint declaration, or for stdio its release) yields a different target, so an earlier credential simply never matches it; there is no trigger or write-time deletion. Descriptive metadata and Tool policy changes preserve the target. Removing the source removes its credentials by cascade. A superseded callback fails explicitly and cannot overwrite or delete replacement credentials. Pre-release databases without the current grant and target bindings require a fresh data directory; saved credentials are not converted during opening.

## Integration probes

`models.list` treats endpoint discovery as authoritative, including an empty catalog. The endpoint adapter validates every advertised identity and collapses repeated identical model IDs; the public page contains each ID once in ascending order. Endpoint failures and malformed identities return a sanitized `provider_error`, rather than `invalid_params` or a successful fallback catalog. A duplicate returned by a custom model-lister implementation violates its unique-identity port contract; tests injecting such a result do not describe raw HTTP discovery. Local registry and catalog defects remain internal failures; cancellation preserves its context identity for Go callers. Bundled metadata may enrich a discovered model but cannot change its provider/model identity.

Provider and MCP probes return a closed `outcome` without server-authored prose. `providers.test` reports `reachable`, `notConfigured`, `invalidCredentials`, `timedOut` or `failed`; `mcp.servers.test` reports `reachable`, `authorizationRequired`, `timedOut` or `failed`. Unknown integration failures are `failed`. Caller cancellation remains a call error. Clients render each outcome locally and reject any other value as a Runtime contract violation; they never branch on raw integration error strings.

## SQLite transaction lifetime

`qimu/sqlite` is the public schema/transaction owner used by Runtime
and plugin authors. `packages/go-sdk/plugin/sqlite` owns Flame private command admission
and atomic receipts. Their former private implementations and entrances are
removed; Core stores and both bundled backends use the same public mechanisms.
Host business tables remain in Runtime, and private business tables stay with their
plugins. Qimu framework records and Runtime's original business references share
one `flame-qimu.db` database, with WAL and FULL synchronous durability. This is a
breaking storage and plugin protocol cutover: use a fresh product root and rebuild
plugin releases. Historical state is retained separately, never converted on open.

Transaction context binds the exact `*sql.DB`, so work for another database cannot
accidentally enter the first owner's write-set. Nested work for one database joins
its transaction; an independent transaction for that same active database is
refused before connection acquisition. Separate owners do not become a distributed
transaction. See [SQLite ownership](../qimu/sqlite/README.md) and
[private receipt ownership](../packages/go-sdk/plugin/sqlite/README.md).

The SQLite transaction boundary owns connection admission, commit and joined
rollback. Connection acquisition and statements retain the caller's context;
transaction lifetime is independent so cancellation cannot start a competing
background rollback or erase its settlement result. Nested write-sets join the
outer owner. Schema admission and workspace intents enter the same boundary with
their required independent transaction. Deferred rollback covers every exit,
including goroutine termination, and consumed transactions require no further
settlement.

Unstarted phases reject cancellation. Failed operations retain the original,
rollback and cancellation causes before returning their connection. Failed begin,
commit or rollback discards the connection because its reusable state is unproved.
An accepted commit remains successful after late cancellation. Successful rollback
rethrows the original panic; failed rollback rethrows an error retaining both the
panic and cleanup failure.

Installation lookup reports record presence separately from read failure, only
after its transaction settles. Missing referenced releases, cancellation and
cleanup failures cannot become an absent installation, withdrawn MCP source or
unavailable Skill. The installation command owner maps admitted absence to its
public not-found error. Storage and protocol formats are unchanged.

## Stored JSON integrity and snapshot cost

Stored JSON uses the standard library's single-pass strict decoder. Columns require exact field names, valid UTF-8, one complete value, and no duplicate or unknown members, including inside nested values. The persisted representation is unchanged; data written by Runtime needs no migration. Manually edited records with mismatched field casing or invalid UTF-8 are rejected as corrupt rather than normalized.

Transcript content decoding also retains conflicting variant fields. A text block
with media or an image block with text fails Domain restoration; readers never
discard those fields to make the record appear valid.

Archive-owned Session, Run, Item and offloaded-result times must fall within
the exact signed 64-bit Unix nanosecond range, inclusive:
`1677-09-21T00:12:43.145224192Z` through
`2262-04-11T23:47:16.854775807Z`. Their Domain owners reject times outside this
range before import can retire existing history resources. The Unix epoch is a
valid instant. Tool-result creation times now preserve nanoseconds on import
and export, using the `tool_result_blobs.created_at_ns` column. This storage
change refuses former data directories without modifying them. Use a fresh
directory, keep the former one separately, and export historical Sessions with
the build that can open it before importing their current artifacts. There is
no seconds-based reader or automatic conversion.

Goal, Plan and agent-memory timestamps enforce the same exact range at their
Domain owners. Pending handoffs, child-start reservations and invocation
journals reject unrepresentable timestamps before writing, so persistence
cannot acknowledge one instant and later reconstruct another.

Model and Tool invocation journals use SQL `NULL` for an attempt that has not
settled. A terminal attempt requires a settlement instant, and numeric `0`
preserves an observed settlement at the Unix epoch. The former sentinel-based
journal schema is also refused without conversion.

Schedules retain their Domain-owned millisecond precision. SQL `NULL` means no
accepted Run or a disabled cron cursor; numeric zero is a real Unix-epoch time.
Due scans and pending-occurrence pagination preserve zero and negative instants.
Schedule list cursors also use milliseconds, matching the Domain and storage
coordinate. Former nanosecond cursors are refused; restart pagination from its
first page after replacing the Runtime.
Occurrence identity binds the Schedule and due cursor; its indexed columns must
encode that same identity. Session favorite and isolation columns accept only
the exact boolean encoding, so decoding and catalog ordering cannot disagree.

Feedback observations preserve their creation nanoseconds in
`feedback_entries.created_at_ns`, within the same exact instant range above.
The former millisecond feedback column, Schedule sentinel columns, and schemas
without the current identity and boolean constraints require a fresh data
directory under the same retention policy; opening them never converts data.

`GOWORK=off go test ./internal/adapter/persistence -run '^$' -bench BenchmarkSessionMaterialSnapshot -benchmem` measures coherent SQLite material reads and application validation at increasing history sizes. It excludes protocol encoding and client rendering; evaluate those separately before changing snapshot completeness or the subscription fence.

## Scope integration

Runtime uses static `ChildDeployments`, the released provider constructors and
transport, and Scope error categories. The Dispatcher owns each Effect's replay
and capability policy; Runtime's Segment gate forwards that complete policy.
An Interaction's final Output carries either a model response or direct Tool
results, without a second completion-source field. Workspace and waiting-tree
validation errors mark a
probe as nonresumable only when their cause is `ErrExecutorStateLost`. Storage,
cancellation, and other unexpected probe errors propagate to recovery instead of
being persisted as irreversible state loss. Malformed or incompatible checkpoints
and missing execution trees still explicitly report that they cannot be resumed.

Resume preflight reads the latest acknowledged paused tree, reconciles exact
wait targets and answer receipts, and validates frozen response and approval input
schemas. The input schemas are projections of the Scope Contracts bound to the
waiting Tool deployment; preflight never resolves models or Tools, assembles
resources, acquires a writer, attaches an observer, or delivers Signals. Remembered
rule subjects and source authority are prepared without writes and commit only in
the acceptance transaction.

After acceptance, the existing Run owner stages the continuation, attaches its
observer, and begins answer delivery under one activation gate. Restore and observe
failures settle the accepted Run through that owner and preserve its command receipt.
A cancellation that wins before activation suppresses the whole activation; steer
and subtree cancellation wait for activation to resolve. Restart uses the same pure
preflight and the same Run lifecycle, retaining inconclusive preflight read failures
for startup retry. No accepted-command queue or second lifecycle owner is introduced.

An accepted Resume can survive a crash while its recipients are still at the
original paused handoff. The acceptance transaction retains the exact Run and
Segment references, answer targets, and optional additional-input Item reference.
Items own the accepted resolutions and input content. The retained execution
context carries frozen instructions/options, accounting, calibration, metadata,
pending input associations, frozen approval input schemas, and immutable Scope
progress anchors; it contains no second recoverable tree. Restoration reads only
the latest execution-tree head.

Recovery requires the original member set and Step/Effect progress, paused
recipients, and unconsumed exact answer receipts. It delivers only missing
answers under their original Signal identities, proves the complete receipt set,
then releases execution through the existing Run owner. Active execution follows
the selected recovery class described below; other active cuts retain the Run loss policy. Terminal Runs
are never revived. The binding survives local Resume intent until the next
committed waiting or terminal boundary retires it atomically. New waiting context
and accepted-continuation storage require a fresh pre-release data directory.

Installed MCP Tools have one captured cold recovery class: a single product
root and its ordinary Tool processes. Before external dispatch, the
effective arguments, exact installation release and source authority, original
Process/Effect attribution, and complete execution context commit with the
canonical Tool Item. Active and waiting executions use the same context payload;
Scope's original JSON remains exclusively in the acknowledged tree head.

Startup acquires the original Session writer lease and reinstalls the original
Run and Segment pump before Scope restoration. It joins only the captured source's
already admitted connection attempt. Recovery reads the optional public `ToolReceiptMethod` query at the original
backend, matching logical identity, remote Tool name and admitted input digest.
The backend alone projects its private receipt through the original Tool codec;
Core imports no business record or private feature method for this read. The
Schedules producer retains creation evidence across edits and deletion. Original
output or a known failed Tool outcome enters Scope through typed settlement and
explicit Unknown resolution. Queries never call the Tool or rerun hooks. An
unsupported query retains Unknown; it never falls back to another provider.

The internal checkpoint field is now `receipt`, replacing the provisional
Schedule-specific `create` field. Strict decoding refuses the former shape; no
legacy reader or converter remains. Private backend receipt formats are unchanged.

The query deadline bounds only evidence collection. Publication uses the existing
Run owner lifetime, atomically committing the Scope cut, product context, result
receipt, and original Item. A known committed Scope settlement is adopted without
another query. The original Segment's committed Items restore its allocation
sequence and Tool attempt times. Absence, conflict, malformed results, and unavailable
evidence retain Unknown and the bounded unresolved-outcome policy. Corrupt local
context or inconclusive local reads stop startup without converting that failure
into Run loss. Unsupported active trees and pending later model calls remain outside
this recovery class. Build and exact Deployment compatibility still fence restoration.

## Scope provider transport

Runtime consumes the released Scope modules pinned in `go.mod`. Provider `Call` and `Stream` use Scope's canonical streaming transport; complete calls aggregate that same validated stream. Runtime does not retain a unary provider fallback. MCP sessions use `github.com/Tangerg/go-sdk`, the same SDK as Scope MCP, so structured results preserve large integers, decimal values, and explicit empty objects through transport and Tool publication.

MCP descriptors retain `mcpserver.Server` from persistence through connection and
dispatch. The domain owns transport, endpoint, process, header and handshake
invariants; the connection configuration adds only live OAuth state. Tool authority
fingerprints and connection comparisons derive from that descriptor rather than
separately stored copies. Environment maps become process entries only at `exec.Cmd`:
an absent environment inherits the parent, and an explicitly empty one stays empty.
Storage preserves that distinction; a previously written absent environment remains
absent, while an explicit empty map no longer silently inherits parent credentials.
Malformed persisted HTTP endpoints fail reads without automatic repair; valid records
and the public protocol keep their format.

For ordinary user-configured sources, the MCP connection supervisor admits every
attempt before configuration, execution resource or credential I/O. Admission withdraws the previous session and tools and
publishes connecting from that same owner. Startup, reconnect and authorization
share this lifecycle; supersession and shutdown cancel and join preparation as well
as dialing. The application coordinates durable configuration, exposure and ordered
identity-only invalidations without storing a second connection phase or tombstone.
Canceled connection and authorization admission, configuration reads and retirement
waits retain both the standard cancellation category and the owner's cause, alongside
concurrent I/O failures. Preparation shares the completion join rule; canceling its
waiter cannot cancel retirement or discard its eventual diagnostic.
Connections owns operation eligibility for both the caller and the pool lifetime.
Startup, reconnect, authorization and refusal read that same rule. Each admitted
attempt links preparation I/O to the pool lifetime and releases its callback when
it settles. A stopped pool cannot start configuration or credential reads, and
concurrent read failures retain both cancellation categories and owner causes.
The supervisor checks the dial lifetime under its publication
lock; an expired authorization cannot publish a verified session. An already published
session retains its successful result if the caller later cancels.
Remote termination of an ordinary MCP session withdraws that exact session and
its tools through the same connection outcome owner. The failure retains the
terminal evidence, and a late predecessor exit cannot withdraw a replacement.
Retirement still runs once; shutdown closes sessions before joining their
termination observers. No failed RPC is replayed.
The handshake owns its temporary callback, timer and untransferred session lifetime.
Admission rejects an expired request or stopped session owner before connection I/O.
Failures retain transport diagnostics, the handshake's timeout or cancellation and
the session owner's cancellation cause before cleanup cancels temporary contexts.
Only the returned release handle transfers the live lifetime to its session owner;
failure and panic keep cleanup with the handshake. Cleanup never reclassifies an
ordinary transport failure as caller cancellation.
Runtime construction validates source declarations and admits their connection attempts
without waiting for configuration I/O, handshakes or tool discovery. Every source
connects independently under the Runtime lifetime. Plugin management stays available
while a source is connecting; disable cancels its attempt and shutdown joins it.
A construction caller's cancellation after admission cannot retire accepted attempts.
Managed MCP catalogs retain only the original instance, session and binding
observation. Discovery holds a short Qimu Use and releases it before publication.
Every Tool, callback and installed command borrows its exact approved Use;
callback preparation and transport work finish before that borrow closes. An idle
catalog cannot prevent release replacement or uninstall. Withdrawal requests
retirement from Qimu, which retains and joins the actual process resource; Pool
shutdown does not claim process exit. Waiting Run restoration obtains original
executable schemas and bindings independently of the current catalog and never
publishes restored providers as a new selection.

Catalog publications include pending source identities atomically with executable tools.
Checkpoints capture MCP Tool references separately for root and delegated groups.
A connection may publish between those two resolutions, so a union cannot reconstruct
either group's original authority. Recovery filters each group to its captured references
before discovery/collision resolution and then validates its exact Deployment. A newly
available backend belongs to future executions; it cannot add Tools to a waiting tree.
The shared current catalog is never changed by this restoration filter.
Recovery defers validation only when a source captured by that group is still connecting,
without granting resumption or blocking startup on unrelated sources. The former flat
`mcp_references` checkpoint field is replaced by required `mcp_manifests`; old shapes
are refused unchanged. Preserve old data and finish waiting work with its original binary.
There is no compatibility reader, migration or provider replay.
Plugin realization describes current release and backend availability separately.
Server listing and startup enumerate desired definitions without realizing plugin resources;
these reads cannot overwrite the state of an admitted connection.
Unmapped connection or authorization states fail their read or projection with
an internal error; they never fabricate a server failure or panic during delivery.

Provider-reported token usage, durable conversation history, and model/Tool/execution telemetry are Scope contracts rather than Runtime restatements: `chat.Usage` survives whole to the protocol, the message store implements `history.Store`, and Scope's OpenTelemetry middleware instruments the provider, Tool, and execution-tree boundaries.

MCP identity follows Scope's capability chain through Tool decorators for discovery, exposure, and approval queries. Invalid MCP identity declarations reject the catalog instead of silently hiding entries or grouping them as built-ins.

`mcp.tools.list` reads the resolver's live MCP snapshot, the one each Run manifest freezes, and derives tool name conflicts from that same snapshot. Connections publish their current catalog when the resolver subscribes, then hand every changed tool set to that same sink inside the critical section that settles the connection. There is no separate startup catalog seed; connected Tool counts and the listing describe one state. Remote tool-change notifications invalidate the admitting catalog revision and schedule one coalescing discovery worker per ordinary session. Discovery and validation run outside the connection lock; only the same session and notification version can publish the complete replacement. A failed refresh withdraws that source and reports tool discovery failure without removing siblings. Listing never queries a separate live catalog. Invalid JSON Schemas, including unresolved references and oversized documents, now fail connection or probe admission through Scope rather than failing the next Run. Diagnostic and MCP schema projections preserve exact numeric literals and retain their existing protocol shape.

Each executable retains the catalog revision that admitted its frozen contract. A notification invalidates that revision before refresh I/O; subsequent dispatch through a captured prior catalog is refused rather than retargeted. Already admitted calls may finish. New Runs discover the newly published catalog. Retirement cancels discovery, and shutdown joins both discovery and termination observers. Description and future handshake-policy updates preserve an equivalent live connection; enablement and the domain connection identity decide when dialing is required.

## Complete AGENTS guidance

Run guidance includes the complete discovered AGENTS.md cascade in source order, with provenance markers. The rendered cascade, including its heading and separators, must fit the 32 KiB guidance budget. An over-budget cascade fails preparation with `prompt_source_too_large`; Runtime does not silently remove ancestor instructions to retain only the most specific files. Shorten the authored documents before retrying. Discovery describes the current documents; it does not prove that a previous Run loaded them.
Discovery and authored-file reads retain the standard cancellation category and
the caller's cause. Complete reads share one document-size rule with retained
cascade values. Input, admission, version and close failures remain identifiable
together; a partial read can establish size but never document text validity.

## Tool authority

Tool identity is a closed built-in, MCP source/tool, or A2A endpoint reference.
Model names remain presentation labels. The closed set of built-in names is owned by the
tool domain (`tool.BuiltInName`); the built-in behavior catalog is keyed by that type and a
test holds it to exactly that set, so startup performs no comparison between them.
Runtime excludes remote name collisions
before model discovery and reserves every built-in name, including unavailable tools.
Cross-server collisions never reject a connection: all competing remote identities
are excluded symmetrically. `mcp.tools.list` reports their model names and conflicting
source references, which CLI and Desktop display alongside the connected catalog.
Scope rejects a whole MCP source when names collide within that source,
before returning any executables. Preserving its unrelated tools requires an
upstream discovery API that can exclude collisions; Flame does not replace
Scope discovery or assign temporary names to work around it.
Standing allow and deny decisions live only in approval rules and retain the
source authority fingerprint. Endpoint changes make existing rules stale;
credential rotation does not. MCP exposure is configured separately through
`mcp.tools.setExposure`; `approval.setRule` owns remembered decisions.
A rule is keyed by scope, scope key, source reference, subject type, and subject value. A
source reference is a closed union: built-in name, A2A endpoint, or MCP server identity
plus remote tool name; each variant's data is reachable only through that variant. Remembering the
same key replaces both its decision and source fingerprint. Equally specific
distinct patterns still resolve to deny when they conflict.

Every pending Tool approval retains its source reference and authority fingerprint,
including one-off decisions that cannot be remembered. Restoration validates this
identity together with the call ID, model-visible name, and effective arguments;
missing identity is rejected rather than reconstructed from a label.

Remembering an approval derives an exact subject from the confirmed command or path,
including literal `*`, `?`, brackets, and backslashes. Tools without a finer subject
use an explicit `all` matcher. `approval.setRule` accepts a required typed subject:
`{type: "all"}`, `{type: "exact", value: "..."}`, or `{type: "glob", value: "..."}`.
Only an explicitly authored `glob` uses Go `path.Match` (`*` does not cross `/`).
Exact subjects outrank globs within the same scope. An
edited command or path cannot grant permission to the original command or path.
The frozen Scope input contract validates approved arguments before a rule is
saved. An invalid edit with a requested remembered rule rejects the complete
Resume; a one-shot invalid edit produces a failed Tool result. Denials
remain bound to the original invocation. Standing rules never replay argument edits.
If the source changes while approval is pending, requesting a remembered rule
rejects the complete Resume. A one-shot choice requires an explicit command.
The standing rule commits with the accepted answers and Segment opening; revival
does not recreate a rule that the user later changed or forgot. Other persistence
failures remain errors.

This contract intentionally breaks the former approval schema and protocol.
Use a fresh Runtime data directory when upgrading from an approval schema
without source identity or explicit subject types. Startup rejects those schemas
without modifying their data. There is no
legacy rule conversion. Update Runtime and every client together.

## Cache prefix and the request tail

A model request is ordered so that everything a provider may have cached stays byte-stable:
tool declarations, then the single System message of stable instructions (base prompt,
pinned memory, agent documents), then the conversation. Nothing that changes during a
Session precedes the conversation. Memory recalled for a prompt is a text part framed
as `<flame-context kind="recalled-memory">` after the user-authored parts of that prompt.
The opening commits that exact annotated User message; a new recall never rewrites an
earlier message or adds an unpersisted conversation entry. It is not inherited by
delegated layers. The current Goal and Plan are not part of the context Scope
adopts: the context reducer reads them from their owners for every call and composes them,
with the deferred catalog below, into that call's tail (`session-goal`, `session-plan`,
`deferred-tools`). The reducer measures that tail in the compaction budget and hands the
same messages to the model boundary for exactly that invocation, which appends them after
the conversation. A Goal or Plan change therefore alters only the tail of the next call.
Waiting checkpoints written by an earlier build carry the former context shape and are
not resumed, under the existing build-identity rule.

## Deferred tool catalog

A Run's tool declarations open the provider prompt-cache prefix, so they do not depend on
which tools are deferred. Built-in, Skill, MCP (user and installation origin) and A2A
tools withheld from the initial manifest form the Run's frozen deferred set, which the
Run's `search_tools` owns. Its declaration is static text; plugin activation, MCP
reconnects and catalog changes reach later Runs only through that set.

Each model request of a layer with deferred tools ends with one Runtime-owned User
message framed as `<flame-context kind="deferred-tools">`, the single framing Runtime
uses for the context it authors (recalled memory and retained shells use the same frame
with their own kind). It lists the deferred model names, the exact
names `select:` accepts, grouped by source in source-then-name order, without
descriptions or schemas. It is a User message because providers hoist System messages
into the instruction block ahead of the conversation. The message is a per-request
projection appended at the model boundary after Scope fixes the context it adopts, so it
never enters the Interaction context, checkpoints, durable conversation history,
compaction input or the transcript. Compaction budgets count it. Restoring a waiting Run
rebuilds the same deferred set, which the deployment configuration digest already binds,
so the catalog is the same projection after restore or compaction. A layer with no
deferred tools has neither `search_tools` nor the catalog message.

## Physical model evidence and accounting

The existing model invocation journal records each physical admission before
provider I/O with original caller, origin, parent, exact provider/model/request,
and captured Session/Run/Segment association where present. Logical conversation
observations and physical endpoint attempts remain distinct. A selected-model
child needs a separate call identity; raw ancestry cannot impersonate a Model or
Tool owner. Reusing an admitted physical identity cannot dispatch again.

Complete endpoint response, reported usage and captured per-call price survive an
outer observer failure. Unknown or missing consumption stays unknown, including
a valid provider that omits usage. Reported zero is distinct from absence. Partial
output is retained as incomplete evidence and never becomes a successful response
or executable Tool proposal. The provider request cannot be replayed from this
journal; it is not a provider receipt.

Run accounting consumes each original model-call fact once. Conversation calls
may update primary context calibration and output; independent calls contribute
consumption without modifying primary continuation or calibration. Live publication,
terminal settlement and boot recovery use this same classification and source
owner. Reconstruction verifies the complete retained source inside its existing
atomic Run/Goal transition. It preserves known usage alongside outer failure and
unknown prices alongside admitted work. Another restart cannot accrue it again.

Usage summaries scan retained physical source records, including auxiliary and
unattached work. Attempt state, token coverage and priced coverage distinguish
empty history, unreported usage and reported zero. Known token/cost subtotals do
not establish an exact total while any selected attempt is unknown. Provider,
served-model, UTC-day, origin and association buckets derive from those same
records; no aggregate cache or Run-based substitute advances them. Corruption,
invalid association or overflow refuses the whole report.

Primary and auxiliary calls capture selected routes, context/input/observer
bindings and their actual resource budgets. Independent installed authors use
`CallSelectedModel` or `StreamSelectedModel` with distinct identities and explicit
`ModelCallBudget`. Unary and lazy streaming share final request admission,
cancellation, join and held-dependency ownership. Scope's exact provider instance
decides whether streaming/counting capabilities exist. No unavailable capability
is fabricated, and an endpoint's accounting total does not invent a universal
Run/Goal budget. Evidence deletion follows the retention rules above.

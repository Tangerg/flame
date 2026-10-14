# Flame Runtime

Flame Runtime is the local product backend for Flame. It owns durable agent semantics and exposes the same behavior through an in-process Go binding and the Runtime Protocol.

Runtime is not another agent framework. Scope owns process execution, strategies, tools, and provider libraries; Runtime adapts those capabilities to Flame's Session, Run, Goal, Plan, persistence, recovery, and protocol model.

## First-party initialization

`make build` produces `build/flame` and its complete companion `build/plugins`
distribution; `make run` starts that executable. The embedded Go binding uses the
same layout: its host executable must ship a sibling `plugins` directory.
`localruntime` owns this layout, with no working-directory or checkout fallback.

On a fresh database, Runtime admits and enables the distributed catalog
through the ordinary installation aggregates, then commits the catalog and its
initialization completion in one transaction before exposing delivery. Managed
backend directories are prepared before completion; connections follow the existing
MCP supervisor lifecycle. A failed admission cannot commit partial defaults.

Initialization runs once per database. Restart never reinstalls an uninstalled
package, reenables a disabled one or selects updated bytes.
Users manage subsequent releases through the existing installation commands.
Clearing the database starts a new initialization. The initialization record is an
intentional pre-release schema break; incompatible databases are refused without
migration. Use a fresh data directory when upgrading.

## Public surfaces

- The module-root `runtime.Runtime` is the concrete in-process binding.
- The module-root `runtime.Client` attaches to an existing Runtime over HTTP/SSE with the same typed operation methods.
- `protocol` contains binding-neutral requests, responses, events, errors, and validation.
- `contract` contains generated machine-readable protocol artifacts and the generated API reference.
- `localruntime` owns the local deployment layout and the strict credential-file handoff.
  It resolves the data directory beneath a product root and names the database and local
  token inside it, so the Runtime process and a trusted desktop client read one layout
  rather than each composing a path.

All Runtime operations enter one delivery endpoint. The Go binding avoids JSON and HTTP encoding but uses the same admission, capability, idempotency, Application, error, and event semantics as the HTTP binding.

Installed unary operations use `CallPlugin` in both Go bindings, `plugins.call`
in Protocol and `plugins call --request ...` in CLI. The request retains the
installation, selected release digest, independent contribution ID, complete
consumer capability, method, concrete target and business input. Runtime requires
exact equality with the declared and live export before a new call can execute. The envelope's
query/command semantics come from that admitted method; queries reject command
identities. Commands use the existing durable original route and private receipt,
without a Runtime result cache. A same-identity change of installation, release,
contribution, contract, method, target, calling origin, parent or input conflicts.

`BindPluginMethod` binds an existing typed `plugin.Method[P, R]` and complete
required capability to a concrete `PluginBinding`. Its `Call` uses the same
public entrance and validates typed input and output. The TypeScript client's
`plugins.call` uses the shared mutation identity/journal path for commands and
the shared query path for reads. A definite business refusal projects the owner's
code/message in `CallPluginResponse.failure`; output and failure are exclusive.
Transport loss remains an unknown command acknowledgement and requires the
original command identity for reconciliation. Delivery and execution adapters
establish human, model and automatic origins; nested callbacks inherit the
captured origin and parent. Command routes persist this evidence separately from
business input and backends receive it as invocation metadata. Declared
cross-plugin dependencies, durable trigger-origin propagation and streaming
decoration remain under development.

All Schedule reads and mutations use the installed
contribution's published operations. The former `ListSchedules`, `CreateSchedule`,
`UpdateSchedule`, `DeleteSchedule`, `RunScheduleNow` Go methods and their Protocol entrances are
removed. Backend business transitions and private receipts retain their owners.
The CLI persists original intent before dispatch and keeps its installation,
release and inputs across selection changes, acknowledgement loss and restart.
The generic installed call supplies Runtime callbacks; the backend owns manual
capture/reconciliation and Runtime owns the accepted Run. Core Schedule DTOs,
error translations and the schedules feature gate are removed. Clients consume
the publisher contract through plugins.call. External consumers must use
`CallPlugin`; no compatibility alias or old-method request translation exists.

Memory catalog readers in CLI and the workspace client now use the public installed
list operation and retain one selected binding across the traversal. ListAgentMemory,
agentMemory.list and their list request DTO are removed without aliases. The public
workspace reader and generated CLI projection consume the same publisher contract;
remaining Memory mutation Core entrances are under migration. Private ledger,
watermark, curation, model maintenance and receipt owners are unchanged.

Core 2026-10-14.13 also removes DeleteAgentMemory/agentMemory.delete and its DTO,
delivery port and selected-provider forwarder. CLI and workspace deletion use the
published installed operation. The TS client's plugins.command captures a logical
intent in the existing mutation journal before dispatch. A successor client restores
the original route before provider preparation and requires the same contract and
serialized input. Changed input, a replaced store, expired retention, active intent
ownership or failed persistence refuse delivery and retain unresolved evidence.
Journal records contain original routing and salted input/intent fingerprints;
they do not store authored input or business results. A typed decoder runs before
settlement, and only witnessed owner refusals can settle as business failures.
Core's removed requests are not converted or resent under a new command identity.
Business storage and backend protocol remain unchanged. Remaining add
and all-consumer/restart/native acceptance are still open.

Core 2026-10-14.14 removes the Memory review method/request/global decision enum,
delivery mapping and selected-role forwarder. CLI and workspace review consume the
generated publisher input and the same private business/receipt owner. Approve and
reject use one logical intent per item; changed decision cannot turn an uncertain
review into another command. CLI recovery precedes workspace/catalog reads, keeps
the original decision and refuses ambiguous pending prefixes. Clients own their
review choices as presentation values; Runtime has no universal plugin-review enum.
The public not_pending owner refusal replaces the former Core invalid_params
translation. Retired requests receive no compatibility alias or automatic conversion.

Core 2026-10-14.15 removes the Memory update API/DTO, delivery mapping and
selected-role forwarder. CLI and workspace editing/pin/unpin use the publisher
operation and its original item intent. Omission and explicit false remain distinct;
an unknown command cannot absorb changed patch values. CLI resumes its stored raw
patch before current target/catalog reads and rejects a changed editor resubmission.
The generated publisher snapshot is decoded/validated before intent retirement.
Workspace acknowledgement rereads the current cache owner rather than projecting
an original provider's item into every current row with the same ID. The remaining
Core add entrance is retired by the following cutover; wider consumer migration
and final acceptance remain open.

Core 2026-10-14.16 removes the last Memory business facade, AddAgentMemory/
agentMemory.add, its request/constraints, delivery handler, selected-role forwarding
and Memory-specific Core feature. Every actual CLI/workspace Memory read and human
mutation uses the publisher's public installed capability. New additions capture
scope/path/content once; the private backend alone resolves project roots, owns
deduplication and reports Changed. Pending authoring resumes its original route,
input and command before current workspace/provider reads. Changed input and
invalid acknowledgement, including a different scope, cannot retire uncertainty.
Workspace projections reread their current owner after acknowledgement.

Rebuild Core/Go/TS clients together; retired requests receive no alias or conversion.
Backend wire remains 2026-10-14.2 and private business/storage formats are unchanged.
Core Memory read models, limits and topics still have real consumers and require
their own ownership migration. Native restart/withdrawal/response-loss acceptance,
wider consumers and final full/vet/race verification remain open.

Core 2026-10-14.17 removes AgentMemoryItem, its scope/origin/status enums, Core
constraints/sample/schema and Go/TS exports. No Core operation consumes those
plugin-owned records. CLI and Desktop own only visible read projections over the
publisher's generated snapshots. Publisher Go models derive closed vocabularies,
identity patterns and Unicode content limits from their private owner; canonical
JSON, generated Go/TS and actual ESM resources are regenerated together. ItemID's
string alias is replaced by its actual patterned schema so identity admission is
enforced at the public operation boundary. Private storage/receipt formats and
backend protocol are unchanged. Native fixtures compare complete owner snapshots,
including vectors, without recreating the retired Core shape. Remaining Core
capacity/topic knowledge and final runtime acceptance remain open.

Core 2026-10-14.18 removes MaximumAgentMemoryItemsPerTarget and its private-domain
import from the public root. The Memory publisher page includes Capacity, with a
schema const derived from MaxVisiblePerTarget. Request page width and response
row limits derive from one private MaxManagementPageItems constant. CLI/Desktop
complete collectors retain one binding and the first admitted capacity; invalid
or changed capacity, cumulative overflow and empty/cyclic continuation fail without
partial results. Workspace and ordinary component readers request the owner's
default page width. Regenerate canonical JSON, Go/TS and ordinary ESM together;
backend protocol and private storage/receipt formats remain unchanged. No Core
capacity alias remains. Business notification migration and final acceptance are open.

Core 2026-10-14.19 replaces agentMemory.changed and schedules.changed with the
generic plugins.dataChanged topic, without business IDs or results. The existing
command route publishes after dispatch/recovery, including unknown outcomes; it
does not match first-party contracts to decide whose reads may be stale. Generic
backend change notices retain authenticated current actor/release/server/contribution
admission and publish this signal instead of forcing a global Core resync. Installed
Tool calls use the same stale-data vocabulary without a Schedule-specific branch.
Private mutation owners publish only after actual change/commit. CLI/workspace and
ordinary Memory/Schedules consumers reread their owners; installation lifecycle
remains plugins.changed. No event can settle a command or advance business state.

Remove the old topics, Schedule event IDs/constructor, closed contract observer and
generated schema/sample/exports together. Rebuild Go/TS/SDK/ordinary ESM packages;
backend wire and private storage/receipt formats are unchanged. External subscribers
must adopt the current topic; no alias or double publication remains. Scoped
subscriptions, model streaming, wider Tool/dependency lifecycle and final acceptance
remain active work.

The public plugin SDK now supplies a captured unary model contract over Scope's
complete request/response and the existing callback endpoint. Its result keeps
validated inner response evidence even when an outer operation fails. A public
external-package subprocess verifies model calls, full usage and causal cancellation
without private imports or a Desktop relay. This is a foundation for the continuing
canonical model execution migration; production Interaction/auxiliary paths,
installed streaming decorators/remote next and durable inner evidence are still open.
No model execution Core method or streaming support is inferred from this unary API.

Production Memory maintenance now uses that public complete Scope model exchange.
The utility model is resolved once before the original maintenance dispatch and
captured by its callback grant. Memory keeps its private two-external-call budget
and original method/input identity fence. Its private prompt policy builds the
canonical request; the backend uses Scope's text output contract only after the
complete response arrives. Usage/reasoning/model evidence is preserved on the link,
including a valid response alongside failure. Embedding has a separate private
text-array/vector method rather than a string-valued model operation. The former
flame.memory/model.v1 text multiplexer is removed. Full durable inner observation,
generic embedding author parity and installed model/decorator composition remain open.

Core 2026-10-14.21 projects optional stream schema on public method descriptors.
The Go plugin SDK implements captured callback streaming on the existing endpoint,
with lazy production, immutable request evidence, synchronous transport backpressure,
cancel/retire joining, independently validated chunks and a final outcome distinct
from them. Scope owns model delta aggregation; completed inner evidence survives
outer failure, while missing terminal transport evidence remains unconfirmed.
Feature generation includes chunk schemas/Go/TS types in the same method identity.

The public subprocess probe consumes a frozen remaining Scope chain without a Desktop
relay. This is real callback streaming, not full installed decorator acceptance.
Installed host-to-backend stream delivery, frozen contribution/dependency retention,
durable inner observation, production model consumers and TS author parity remain
open. Unary installed calls explicitly refuse streaming before storing a route.
Backend remains 2026-10-14.2; no private storage or receipt format changes. Rebuild
current Core/Go/TS consumers together; no new method or descriptor promises streaming
through a transport that has not implemented it.

The public SDK now also registers and consumes real backend streams. One original
MCP request carries the final result; synchronous acknowledged chunk delivery uses
its existing authenticated Runtime callback lifetime. Real stdio executable proof
includes model-next reentrancy, immutable invocation, demand backpressure, cancel/
join and known inner response with outer failure. No backend listener or event queue
is introduced. Runtime's internal installed command stream now shares unary route
admission and the same durable Commands owner. First invocation captures exact
installation/release/contribution/export/contract, raw input, target and caller.
Dispatched recovery reads only the original private receipt; it never reproduces
chunks or invokes a replacement provider. Consumer stop and missing receipt retain
uncertainty. Valid final evidence or explicit owner refusal ends the route; generic
transport failure does not. Callback preparation precedes dispatch and resources
are joined when consumption ends.

The public Go binding and Runtime Protocol now expose this mutation stream through
StreamPlugin/plugins.stream and the same delivery endpoint. Its empty opening
response conveys no business outcome. Ephemeral notifications.plugin.stream frames
carry validated chunks, final results or explicit refusals; no cursor is accepted.
The Go remote client recognizes terminal evidence and reports missing/malformed
terminal delivery as an unknown acknowledgement without reconnecting or resubmitting.
HTTP transport acknowledges each source yield only after its SSE frame is written,
and cancellation joins the source. Typed BindPluginStreamingMethod now uses shared
binding admission and SDK StreamValue, freezing input/options before lazy single
consumption and validating chunks/final output. Final result publication follows
source completion; invalid/missing evidence cannot manufacture success. The caller
keeps transport refusal/uncertainty classification with its existing owner.
PluginCallbackCaller consumes the same stream with its captured original reference,
lazily opening and closing the ordinary Runtime Client. Invoked work missing call
context is refused instead of using activation. TS plugins.stream now consumes
chunks through an awaited callback and resolves only the decoded final result.
It uses the existing intent/binding/input journal; transport loss retains that
original record without automatic replay. Recovery derives stream/unary delivery
from the original contract and restores its exact route before current preparation.
Each HTTP request waits for its own notification consumption while the shared
client continues handling nested/unrelated RPC. Stop/client closure releases the
reader/observers and joins in-flight asynchronous consumers. Unsafe numeric stream input/evidence is refused instead of
silently rounded; full cross-language wide-number parity remains unfinished.
Production model chains, query streams and dependency retention remain open.
Unary entrances continue to refuse stream methods before route creation. Stream
recovery stores no competing business result or chunk history.

The operation catalog owns result nullability for execution, durable replay and
the remote Go client. A required response cannot become a successful null; a
Session without a Goal returns null through `goals.get` in both bindings. Receipt
and remote decoders require the declared fields before accepting typed values.
Diagnostic Tool results remain opaque JSON, where null is a valid value.

Interaction known-usage accounting now consumes its exact persisted physical
endpoint response, not the outer operation's response metadata. Successful output
still supplies the semantic message; usage, model identity and context calibration
come from the captured provider evidence. Failed outer operations retain physical
known response/usage even if the outer response is absent. No complete endpoint
evidence means no guessed usage, and a read/publication failure remains explicit.
The existing single completed/failed fact advances accounting once; there is no
second metering event or aggregate writer. Model-call sequence and prepared context
checks still fence duplicate accrual. Production execution requires the evidence
reader, with no compatibility path that accepts outer-reported usage.
This closes the current transparent single-call Interaction case. Auxiliary/global
usage aggregation, crash reconstruction of aggregate projections and multi-call
composition metering remain explicit follow-up work.

Core 2026-10-14.24 adds modelInvocations.endpoints, with parentCallId identifying
the logical caller or its immediate parent. The same model_invocations journal
now distinguishes logical Interaction observations from actual endpoint admission
through endpoint_json. Every real primary/auxiliary/Memory chat request is
recorded before I/O with its own identity, original caller/origin/parent, exact
provider/model and frozen Scope request. Reusing the same logical model call cannot
start another endpoint. Request model overrides cannot replace the captured route.

Endpoint response, usage and reasoning are committed beneath remote observation;
outer observer failure cannot rewrite a known completed endpoint. The original
request/response JSON remains available through public Go/TS query without another
numeric conversion. A complete source response accompanied by failure remains
failed with known evidence; absence of a complete response remains unknown. Lazy
streams publish nothing before consumption. Early stop retains a complete known
response where available, but never asserts operation success. Startup reconciles
unobserved endpoints as unknown before Run recovery and never replays them.

The exact host schema adds endpoint metadata/ancestry and permits observations
without a Run. Former schemas are refused without conversion/deletion; preserve
them separately and use a fresh directory. Backend/private receipts are unchanged.
Existing Run list/trajectory/export retain their logical observation contract;
endpoint export/lifecycle retention and wider physical-call accounting remain
explicit follow-up work. The operational journal does not authorize Tool
or conversation continuation and is not a recoverable provider receipt.

Auxiliary title, compaction, skill mining and Memory utility work now consume the
same public observer choice. AuxiliaryResolver returns a captured model and a
required release function. Complete releases after response validation; Memory
holds one capture across extraction/curation and releases it only after its callback
grant has canceled and joined. Observer captures register their exact releases at
the existing installation admission point. HeldDependencies projects that live
ownership into installation quiescence/retention; closing first cancels and joins
all active model calls, then removes the held capture. A released model explicitly
refuses further calls, and composition changes affect only subsequent captures.

Public model callbacks now derive each command's call-scoped identity while
preserving the original origin and parent. Distinct extraction/curation requests
share their maintenance parent without sharing a model invocation identity. Unary
and streaming next keep an already matching call identity. Memory's private
two-external-call admission and original receipt recovery remain authoritative;
receipt lookup does not acquire a new utility model or repeat an unknown call.
Durable per-inner-invocation accounting/evidence remains separate unfinished work.

Interaction consumes the public flame.model-observers.v1 decoration contract.
Staging captures its stored outer-to-inner installation/release/contribution/export
bindings and folds them into deployment identity and installation retention.
Checkpoint model_observers records the same order; restoration validates those
original bindings without reading current composition choice. Former checkpoints
without this field are refused without conversion or deletion.

The first production contract is transparent observation. Each layer receives a
call-bound next to the remaining captured chain on the existing callback endpoint.
It permits one unchanged request/continuation, exact response and ordered canonical
chunks; another request or changed outcome is refused. This restriction belongs to
this observer contract, not all decoration. No independent command receipt, Tool
loop or model history is added. Unary inner evidence survives outer failure; stream
chunks retain actual consumer backpressure, usage and cancellation. Early stop
cancels and joins the original producer. Non-streaming providers remain non-streaming.
The physical adapter is checked with real SDK HTTP callbacks and installed release
identity; full native production qualification remains final work. Admitted request
transformation, multiple separate inner invocations and transitive dependency
retention/status remain open.

Core 2026-10-14.23 adds plugins.listCompositions, plugins.setComposition and
plugins.resolveComposition. The consumer supplies the complete decoration contract
and explicit outer-to-inner members; Runtime persists one atomic ordered choice.
Resolution freezes installation/release/contribution/export identities in that
order. Updating the choice affects new bindings only. A missing, disabled or
contract-mismatched selected member refuses the whole composition, including an
optional declared dependency; it never silently removes a member. Empty configured
chains are explicit, and required dependencies refuse absent or empty chains.
Named setup dependencies use the same ordered composition owner and existing
activation-cycle checks. Go and TS management entrances share Delivery. Interaction
now consumes this ordered choice through transparent production model observation.

The exact host schema adds plugin_compositions. Preserve former host data separately
and use a fresh data directory; Runtime refuses former schemas without conversion
or deletion. Plugin private receipt formats and backend protocol remain unchanged.

Core 2026-10-14.22 adds model invocation knownResponse as the original complete
Scope JSON string for a failed outer operation with validated inner completion.
Interaction commits one failed fact containing that response and its accounting;
unary response-plus-error and canonical stream completion followed by outer error
both retain known usage/model/reasoning evidence. Incomplete output remains separate.
The Run stays failed, and Tool calls/messages from the failed outer operation do
not enter executable continuation. The existing model invocation row owns the one
monotonic transition; list/trajectory/export are read projections. Export capacity
counts the retained response bytes. A restarted host reads the same evidence.

The exact host schema now includes known_response and permits failed-call usage
only with validated matching response evidence. Former host schemas are refused
without conversion or deletion; keep them separately and use a fresh data directory.
Rebuild Go/TS consumers together. Plugin backend/private receipt formats are unchanged.
This evidence boundary does not yet complete installed decorator selection/retention,
multiple inner invocation identities or production remote composition.

RPC parameters and `_meta` use exact, case-sensitive schema field names. Unknown members, duplicate members, invalid Unicode, trailing JSON values, and explicit `null` in typed fields are rejected. Omit optional fields; use the declared change variants to clear configuration. Opaque tool arguments may contain `null`. Clients must not rely on case folding or replacement of malformed text.

Go consumers accepting authored JSON use `protocol.DecodeRequest(encoded, &request)`
before invoking either binding. It shares the transport's strict decoding and generated
value validation, preserves opaque numeric evidence, and leaves the target unchanged on
failure. Required JSON fields must be present even when their value is false or an empty
collection. Ordinary JSON decoding can erase omissions and explicit nulls before the
binding observes them.

The shared TypeScript client validates request parameters through the generated method
contract before reserving a mutation identity or sending a request. Authored-value
consumers use `checkRequest` from `@flame/runtime-contract/client/request` for the same
validation and typed result. Typed request objects reject unknown members at every
depth; declared maps and opaque tool arguments keep their own keys. Shared result
shapes continue to accept additional fields. Request metadata is validated before
publishing the request identity or calling the transport. OpenRPC derives closed
request components, including `_meta`, from the same wire graph used by the
generated client checks; its result references retain the reusable schema shapes.

## Provider management clients

The shared TypeScript client exposes `plugins.listProviders` and
`plugins.selectProvider` as the same query/command catalog entries used by Go and
CLI. The built-in graphical package manager displays recorded choices, including
missing/disabled selected candidates, and submits explicit role/installation/
contribution changes. It never infers selection from enablement or picks a default
candidate. The Runtime selection owner continues to validate and persist the choice;
existing execution retains its captured binding set. No protocol/storage change.

The graphical dependency inspector uses the public current requirements query and
labels its accepted read as the last inspected snapshot. Unknown selection outcomes
retain their original command key and retry closure. Actual graphical execution and
cross-reload recovery remain final-verification obligations.

## Protocol 2026-10-13.10

Component declarations admit workspace scope and an optional generic `material`
navigation key. Graphical components receive a captured public workspace reference
and file-focus intent, without private stores or application aliases. The Diff
component calls canonical `workspace.diff.get`. Its HTML/carrier and duplicate
`plugins.readDiff` request/bindings are removed. Rebuild clients and installed
artifacts together. Business storage and workspace authority are unchanged.

## Protocol 2026-10-13.9

Usage is an ordinary installed React component. It reads the public `usage.summary`
operation through the explicit SDK Runtime client, with cancellation and I/O joining
owned by its component lifetime. The former `plugins.readUsage` wrapper/request,
closed usage view kind, HTML resource and graphical carrier adapter are deleted.
Rebuild clients and installed package artifacts together. Accounting and business
storage formats are unchanged.

## Protocol 2026-10-13.8

Client components may omit their own callable contribution. The public SDK admits
that context for component-only content and refuses `bindOperations` without a
binding. An explicitly authored empty, null or unknown contribution remains invalid.
The bundled icon gallery now ships ordinary ESM and uses shared React/UI controls;
its static HTML resource, carrier branch and closed view enum are deleted. Rebuild
all clients and plugin artifacts together. No business storage format changes.

## Protocol 2026-10-13.7

Release projections include public `components`. A workspace component references
one same-release callable contribution and prebuilt namespace-local ESM export.
`plugins.openResources` owns a live Core subscription; its ephemeral capability
permits ordinary module/asset GETs through `pluginResources`. Each asset read
still enters `plugins.readResource` under the captured caller, rechecks the exact
current enabled release/component, and verifies package integrity. Closing the
subscription or Runtime cancels access without persisting a grant or holding an
execution dependency. The asset URL contains no persistent Runtime bearer token.

The browser/Wails workbench builds an import map over actual shared module exports
and loads the declared component in its own React tree. Supported registrations,
observation and owned calls retire through Dougong; ESM evaluation is not unload.
A changed evaluated release or Runtime generation requires a window reload.
The current slice declares the installation-scoped Reviews workspace board;
remaining action placements and final component acceptance still require
migration. Compilation and bundle mapping do not establish rendered acceptance.


`plugins.call.contract` is the owner's complete canonical JSON string. Client
serialization must preserve it as a string; parsing a schema into JavaScript
numbers can round large integer limits and change its exact identity. Go typed
bindings and generated feature `CAPABILITY_JSON` supply that original contract.
Runtime parses and admits it through the same pure capability owner. The former
object request shape is removed; clients and backend packages must be rebuilt.

Plugin release projections include required `points` and `pointValues` arrays,
and `plugins.resolvePoint` resolves the admitted consumer-owned value contract.
Point contracts in both arrays and the query are canonical JSON strings, preserving
numeric schema limits through Go, CLI and TypeScript serialization. The built-in
manager inspects a reviewed target, attributes and explicit order without activation;
results are labeled as the last inspected snapshot. No former object reader remains.
Core clients must rebuild against the generated current shapes; backend callable
contracts use their independent current backend protocol identity.

## Protocol 2026-10-14.28

Public Start, Resume and Steer commands require a caller-retained idempotency key.
The shared Endpoint rejects an absent key before invoking the command. Retain the
exact request and discovered store namespace before dispatch; use them for lookup
or original-command redelivery after an uncertain acknowledgement. Go and HTTP
bindings enforce the same contract. Internal Goal and automation origins retain
their existing acceptance owners. Rebuild Runtime and clients together.

The SDK journal freezes Run command metadata with its original parameters,
including admission capabilities. Changed client preferences cannot change the
original command on redelivery. Former entries without metadata are refused and
preserved without conversion; resolve their pending commands with the owning
client build before replacing it.

## Protocol 2026-10-14.1

Start and Resume use one Run-owned command acceptance. A keyed Resume commits
its request fingerprint and original Run, Segment, and optional input Item
references with the complete answer set, resumed tree, and requested standing
rules. Redelivery reads that immutable acceptance before current prompts or
execution preparation; it never allocates a replacement opening. Delivery no
longer reserves or caches a separate Run-stream response.

Keyed Steer commits its immutable input and reserved Item reference with the
Scope Signal head and pending execution context. Its acceptance query returns
that original input and, when present, the exact applied User Item. The SDK
`runs.recoverCommands()` enumerates frozen original Start, Resume and Steer requests
after application restart, reads their receipts without dispatch, and retains explicit
original-command retry authority for inconclusive reads. The SDK journal persists the
complete prepared request before dispatch. Explicit replay keeps its original key,
namespace and deadline; it never reads changed drafts or reviewed answers. Accepted
openings recover observation through Session snapshots and the existing subscriptions.
The former Steer-only SDK journal shape is rejected and preserved without migration.

`runs.getAcceptance` requires the original `namespace`, `key`, and `operation`
(`start`, `resume`, or `steer`) as query parameters. It only reads. `accepted` includes
the original opening references and a consistent current Run projection;
`retired` retains those references without material. Deleted material cannot
be relinked by importing the same Run ID. `notRecorded` describes this read
and says nothing about an invocation still in flight. Namespace mismatches,
operation conflicts, and inconclusive reads remain errors.
An optional captured start `request` verifies the original admitted meaning,
including its execution capability context. Plugin callbacks retain their
installation-scoped key; lookup cannot read another plugin's command by spelling
the same logical key. A changed captured request conflicts even after retirement.

Only the original material and Segment may supply a redelivery stream. A later
Segment, waiting or terminal Run, retired material, or unavailable replay cursor
returns the original acknowledgement with no replacement stream. A client uses
existing snapshot and subscription reads for the current lifecycle. Observation
defects remain errors and never overwrite acceptance.

Rebuild Runtime and its clients together. The former cached Resume response
policy and acceptance query shapes are removed without compatibility readers.

## Plugin backend 2026-10-13.4

Memory and Schedules are optional, complete managed plugin backends. Their
Local contribution declarations bind independent IDs to exact contracts and
server exports; there is no closed service-kind routing enum. Product composition selects
an exact installation/contribution independently of its enablement. More than
one candidate may be enabled; absence, disablement or removal of the selected
provider never chooses another one. Human
commands and installed components use the same backend as model Tools and maintenance.
The host retains no Memory ledger or Schedule store. With no available selected owner, human
operations return `plugin_unavailable`; optional Memory context is absent.

The generic MCP connection owner validates only captured source, configuration
and session authority. Feature tool preparation belongs to the actual feature
adapter: Memory captures workspace/model collaborators and Schedules observes
dispatched calls to invalidate reads. Preparation does not hold an installation
lock across I/O. The source and cancellation state are checked again before the
prepared call runs; withdrawal releases acquired collaborators without invoking
the backend or publishing a dispatch observation. Read failures and concurrent
cancellation preserve both causes. A failed or lost reply still releases the
Memory callback and invalidates Schedules reads. An unconfigured embedding role
permits keyword retrieval; an unexpected resolver failure remains an error.

The public `plugin.Callbacks` binding owns one lazily opened loopback endpoint for
this Runtime instance. MCP session retirement withdraws its callback references,
cancels all admitted sibling calls before joining them, then closes the backend
transport. Memory search and maintenance use this same unary binding and own
their model budget in Memory's RPC adapter. Public typed operations can bind
other call-scoped collaborators without importing Memory. Bootstrap publishes
the canonical RPC/SSE handler exactly once after the one Delivery Endpoint is
built and before workers start. `OpenRPC` captures that transport's call lifetime;
backend authors use `ConnectCallback` and the ordinary typed Runtime Client.
Closing a client detaches observation; callback retirement cancels and joins its
stream source. Message limits bound replies/frames, not an entire stream.
Memory search/maintenance and Schedules dispatch pass ephemeral callback references
through MCP metadata, outside business input and durable host routing/private
receipts. Receipt lookup and completed Memory maintenance redelivery require no
new callback grant. Current command tables name that input `business_input_json`;
former call-context input schemas are refused without changing retained evidence.
The Schedules executable uses the ordinary Client to query `runs.getAcceptance` with its namespace, actor-scoped key, start operation and original `runs.start` request, then submits
the captured `newSession` only when no acceptance exists. Runtime commits the new
Session, Run and command acceptance atomically. Opened and retired acceptances
settle the same private pending work without recreating deleted material.
The actual managed connection supplies the installed actor; Runtime rechecks its
release and source authority inside the opening transaction. Logical command
keys are scoped by installation independently of activation and release.
Installed dependency rendezvous, canonical model execution and streaming
decorators remain open parts of the refactor.

`plugins.listProviders` and `plugins.selectProvider` expose the same composition
owner through the Go binding and Runtime Protocol. CLI uses `plugins providers`
and `plugins select-provider --request '{"role":"memory","installationId":"…","contribution":"memory-provider"}'`.
Selection changes future resolution, preserves each provider's private data and
never retargets an original command or its receipt. Distribution selects initial
providers by configured package/contribution once in its initial transaction;
restart preserves later choices. The provider-selection table is a strict host
schema cutover, requiring a fresh directory while retaining the old one.

These commands use `ownerResponse` idempotency. The Runtime store namespace and
logical key identify a durable original route, including installation, release,
method, admitted input and exact public method contract. The backend persists its
transition and reply together; after dispatch admission the host reads only the
original receipt and validates it against that captured contract. A missing or
unreachable receipt remains unconfirmed and never permits resending the mutation.
The former route encoding without a method contract is refused without conversion.
Run acceptance remains a separate
Runtime fact, recorded atomically with its opening. Already accepted automated
Runs survive backend disable and can be reconciled after restart.

Protocol `2026-10-13.2` adds mutually exclusive existing-Session and captured
new-Session targets to `runs.start`, and optional original-request verification to
`runs.getAcceptance`. Captured openings require an idempotency key. The former
automation opening API and receipt table are removed. Schedules binds its private
pending work to one Runtime store namespace before capture; attaching that private
directory to another Runtime store is refused. Old host schemas containing the
automation receipt table and old Schedules schemas without the namespace binding
are refused before conversion. Retain those directories and use fresh ones.

Build target-specific backend packages from the repository's `go.work` using
[the first-party package workflow](../plugins/README.md). Their executable sources
share this application module; they require no published first-party module graph.
Rebuild Runtime, CLI and Desktop together. This is an intentional pre-release
schema break: use a fresh
Runtime data directory and retain the old directory separately. No old Memory or
Schedule tables are imported, decoded or maintained in parallel.

## Component-only packages

A namespace declaration may publish ordinary `components` without callable
contributions or a backend process. Each component declares installation, workspace or Session scope,
workspace placement, a namespace-local prebuilt ESM entry and its export name.
An optional `contribution` binds an existing callable contribution from the same
release. An authored unknown or malformed callable ID is refused; absence is valid
for content-only components. Runtime admits resources without executing module code.
The optional icon gallery is the first bundled component-only package. Its former
static HTML view kind is removed. Graphical clients use the same subscription-owned
exact-release module resources and shared React/UI graph as the Reviews Board.
Diagnostics is an ordinary component consuming an explicit public client-telemetry
collaborator; no Runtime diagnostics view kind or HTML resource remains. Its local
buffer and Runtime accounting retain their separate owners.

## File components

The optional [File renderers package](../plugins/file-renderers/plugin.json) supplies
ordinary prebuilt ESM components. Each uses `scope: "file"`, `placement: "fileRenderer"`
and an exact normalized file-extension `selector`. These selectors are not admitted
through the former HTML renderer declaration. Invalid scopes, missing selectors and
callable-resource mismatches are refused during metadata admission without evaluating
module code. Selector projections are detached from admitted immutable metadata.

Runtime still owns file access. The client supplies one frozen accepted read, with its
workspace, path and bounds. Navigation never replaces that captured data. Ambiguity,
partial/truncated reads, loading, failure and withdrawal retain the existing source
presentation; line navigation begins in source mode. Shared React/SDK/UI and exact
release resource leases are the same as for other installed components.

## Tool-result components

The optional [Tool results package](../plugins/tool-results/plugin.json) supplies
ordinary components with `toolResult` scope and `toolResultRenderer` placement.
An exact display-name or known result-shape selector chooses presentation; it grants
no Tool invocation or historical replay authority. Runtime admits ESM resources and
immutable selector metadata without evaluating code. The generic HTML renderer
vocabulary and read operation are removed.

The client captures one frozen settled transcript projection. Exact-name matches
precede shape matches; equally ranked candidates use generic inspection. Running or
absent results stay generic. Inspection remains available during loading, failure,
revocation and connection replacement. Current installed renderers may inspect old
results, but opening history never downloads or evaluates historical producer code.

## Protocol 2026-10-11.1

`usage.session` now returns `SessionUsage`: its `byModel` value is a list of
`UsageBucket` records keyed by the exact `provider/model` pair. Session and summary
reports use the same Runtime-owned aggregation and order: cost descending, input
tokens descending, then identity ascending. CLI and Desktop consume that order.
Run metrics retain model-name maps local to the Run's selected provider. Qualified
aggregate identities therefore do not inherit the length limit of a single model name.

Rebuild Runtime and clients together. The former Session model-name map is removed
without an adapter. Existing Run records require no conversion. Goal cost and steps
also include retained delegated Runs through their root's Goal incarnation; its Run
count continues to describe root executions. Previously omitted spending is reflected
when the Goal is next read, including after restart.

## Protocol 2026-10-10.8

MCP status describes only the connection supervisor's live outcome. The retired
`mcp_release_unavailable` and `mcp_backend_unavailable` status categories no longer
appear in the contract. Plugin realization remains the authority for current release
and backend availability. A refused new launch settles as `mcp_configuration_failed`;
reading package availability cannot mark an existing admitted connection as failed.
Rebuild Runtime and clients together; older protocol versions are refused. No storage
schema changes or data conversion are required.

## Workspace Diff component

The optional [Diff package](../plugins/diff/plugin.json) reads canonical
`workspace.diff.get` through the public SDK's captured workspace context. Runtime
owns workspace confinement, Git baselines and file/row/byte limits. Structured
renames, binary evidence and whole-file truncation remain lossless public data.

Host owns the current workspace reference and file-focus revision; changes retire
that rendering instance and cancel its reads. The component owns comparison mode,
layout, filtering and folding. A file-focus activation starts a fresh working-tree
comparison. The former Host diff-mode preference is removed. Failed baseline reads
retain the last accepted comparison and label its actual mode.

The component declares `material: "diff"` for the existing workspace navigation
consumer. That consumer opens the catalogue if no or several admitted candidates
exist. Generic loading does not interpret the key. Disablement/uninstall withdraws
presentation/resources while canonical workspace files, Git and Tool evidence remain
with their original owners. There is no HTML guest or feature-specific read wrapper.

## Protocol 2026-10-13.15

The optional [Skills component](../plugins/skills/plugin.json) directly consumes
canonical discovery, document, managed-library and proposal operations through the
public SDK. Captured workspace references and proposal revisions remain with their
existing owners. Personal library inspection works without a workspace. Archive and
restore affect only user-managed Skills; installation-provided enablement belongs
to package configuration. Confirmed commands reread the owner, and uncertain replies
retain their original mutation key and retry closure before further commands.

The component observes existing skills/plugins/file notices through the public
Runtime subscription, captures its workspace watch and cancels/joins both notices
and reads on retirement. Read failure retains the accepted same-target snapshot.
Switching catalog or document cancels the old read and clears its presentation.
Proposal confirmation displays its captured instructions and revision; concurrent
replacement remains an explicit revision conflict.

The Skills HTML/view kind, dedicated plugins.readSkills/readSkill/readSkillLibrary/
readSkillProposals operations, private Desktop controls/curation generation and
unused catalog query projections are removed. Rebuild Runtime/CLI/TS/SDK/Host and
installed artifacts together. Disablement/uninstall withdraws component resources;
managed Skills, proposals, published documents, discovery and mining remain with
Runtime. Backend .4 and persisted Skill formats are unchanged. Final installed and
cross-generation recovery execution remains queued.

Native acceptance also repairs recalled-memory composition: recall annotates the opening
User message after its authored parts, and the existing opening write-set persists that
exact message. It no longer introduces a separate unpersisted conversation entry or
replaces the input committed by Run admission. Durable-context equality remains enforced.

## Usage component

The optional [Usage package](../plugins/usage/plugin.json) reads the existing public
`usage.summary` owner. Runtime interprets all-time, seven-day and thirty-day periods
and folds retained physical model attempts once, including main/delegated,
auxiliary, active, Session-only and unattached calls. The component renders known
token totals, a priced subtotal and coverage counts, grouped by exact provider/
served model, admission UTC day, calling origin or Run/Session association.
Unreported usage, unknown outcomes and absent pricing remain explicit; recorded
zero stays zero. Local grouping/filtering cannot change accounting.

An owned request cancels when its selected period changes or the component retires.
Failed reads retain the last accepted report and label its actual period. Refresh
reuses the selected query. Disablement/uninstall withdraws presentation and resources
without changing accounting. There is no view-specific usage query or HTML guest.
Rebuild the prebuilt ESM and explicitly select the changed package release.

## Protocol 2026-10-14.7

Core removes schedules.list and its Go/TS binding, delivery handler and selected-role
forwarding read. CLI catalog readers now capture a public installation/contribution
binding and call the publisher's typed list operation through plugins.call for every
page. Publisher Go contracts generate canonical JSON, TS and the CLI Go projection
from one source, preserving integer bounds and optional values. Metadata/contract
mismatch or withdrawal is an explicit failure; a later selection cannot retarget an
in-progress traversal. No old reader or compatibility alias remains.

Rebuild generated consumers. Backend wire stays 2026-10-14.2; business/storage
formats stay unchanged. Schedule mutation Core facades remain real consumers until
those owning migrations preserve original command/binding recovery. Full native
execution, external author packaging and final full/vet/race checks remain open.

## Protocol 2026-10-14.6

Core removes `plugins.renameSession`, the special Go/TS/CLI wrapper, release action
array and title-only DTO/operation enum. Rebuild generated consumers and reinstall
Trajectory with its `sessionAction` component export. Old closed action declarations
are refused without aliases or a compatibility reader. The ordinary component and
existing CLI Session rename enter canonical `sessions.update` with the inspected
revision and original command identity. Unknown acknowledgement never becomes another
method, target or successful receipt. Former uncertain wrapper requests are retained
as their original evidence; they are not translated into new Core commands.

Backend wire remains 2026-10-14.2. Private business/storage formats are unchanged.
Final canonical receipt/restart execution, supported native component acceptance and
full/vet/race verification remain open. UI/E2E execution is excluded by the task.

## Protocol 2026-10-14.5

Core removes `plugins.readRenderer`, its Go/TS binding, HTML resource DTO and release
renderer array. Renderers now declare ordinary components with file/toolResult scopes,
technical placements and selectors, using the common exact-release resource lease.
Rebuild all generated consumers and reinstall Tool results at the new digest. Former
HTML declarations, resources, readers, builders and browser/native guest machinery
are removed without aliases. The shared UI guest theme/bootstrap exports are removed;
external UI authors must use ordinary shared provider context. Backend wire remains
2026-10-14.2; private business formats and recorded results remain unchanged.

Native component acceptance and final full/vet/race verification remain open. Optional
renderer failure preserves recorded data inspection. Contextual action/Core facade
migration remains separate work with the existing business/receipt owners.

## Protocol 2026-10-14.4

Core adds optional component selector metadata and file scope. Rebuild generated
Go/Protocol/TS consumers and reinstall the File renderers package at its new digest.
The former file HTML declaration, resource, builder and graphical carrier are removed
without an alias or old reader. Markdown is an ordinary ESM component using frozen
public captured-file data. Contextual actions remain separate migration work. Backend wire remains 2026-10-14.2; private business formats do not change.
Native loading/retirement acceptance and final full/vet/race verification remain open.

## Protocol 2026-10-14.3 and plugin backend 2026-10-14.2

Background setup resources implement the public ActivationResource. The connection
supervisor completes setup before calling Activate; an activation attempt starts
at most once and failed/withdrawn lifetimes cancel and join their resources. Setup
itself does not advance business work. Current compiled packages must implement the
new activation frame and exact five-method Schedules contract.

Schedules owns its minute wake-up, bounded private claim/recovery pass and original
Run acceptance reconciliation. Every enabled declared background entry operates its
own installation independently of provider selection and graphical entries. The
host scan driver and Bootstrap worker/join are removed, with the former public
scan/dispatch methods. Manual recovery reads the original private command input and
commits acceptance plus original runNow reply through the existing private owner.
Edits/deletion do not rewrite captures; independent accepted Runs stay in Runtime.

plugins.notifyChange accepts no business value or installation identity. A managed
callback's authenticated Actor must own that enabled release/server/contribution.
It publishes the existing outcome-free resync invalidation, allowing readers to
reread unknown plugin-owned dependencies without a new event bus or result store.
The public Go callback client is the real production consumer; source coverage scans
actual backend/CLI bindings without operation exemptions. Native lifecycle, enabled
non-selected installation and cross-generation acceptance remain final verification.
Rebuild Core/Go/TS/SDK/installed packages together; old frames and original routes are
refused rather than converted. Business data formats remain unchanged.

## Protocol 2026-10-14.2 and plugin backend 2026-10-14.1

Callable contributions may explicitly declare background activation. Setup receives
that entry's immutable installation target and an activation-owned Runtime callback.
Connection publication still waits for declared dependencies and successful setup;
background resources are canceled and joined on session withdrawal. This does not
create another installation-enable or provider-selection owner.

The callback's Actor comes from the connection owner. A backend may supply its
retained invocation through the public callback client context; original human/model
identity must match an already dispatched host command's installation/release/server
and complete invocation. Fresh automatic work is restricted to that installation
and cannot invent a retained parent. The routing owner validates and projects the
caller without copying its business result or advancing command state.

Schedules declares this role and checks the attached Runtime namespace during setup
without binding or modifying private work. Its callback connection is joined before
the private database closes. Its existing host scan driver still owns scheduling;
the subsequent cutover must move the loop/clock and captured recovery into the backend
and delete the driver and retired scan/dispatch exports together. No second loop is
introduced by this activation slice.

Known business refusal is capability.Refusal, independent of MCP. The former
plugin.Failure Go type is removed, with all in-repository callers migrated.

Rebuild all Go/Protocol/TS/SDK/installed packages together. Current backend description,
setup and generated feature contract identities use 2026-10-14.1; former frames and
routes are not converted. Retain former data directories separately for this breaking
pre-release change. The five CLI-consumed Schedule Core facades remain migration work.

## Protocol 2026-10-14.1

The optional [Schedules component](../plugins/schedules/plugin.json) binds its own
admitted installation/contribution and generated complete capability. Its canonical
backend results supply lists, exact revisions, captured workspace/model choices and
cron times. Add/edit/toggle/delete and complete manual run use that same public
installed entrance. Unknown commands retain their original key/retry closure; owner
notices reread accepted data without creating optimistic business outcomes.

Templates are Schedules-owned component source, copied into an explicit new draft.
They are not generic package metadata or persisted schedule references. The component
uses public model choices and captured optional workspace data. Failed pagination
retains the accepted page/cursor, and editing keeps the inspected revision until
submission. Concurrent edits remain business revision refusals.

The final Views declaration, Domain model, Protocol projection, readView/readSchedules
operations, generated contracts and HTML build path are removed. The generic loader
has no remaining business view cases or Schedule-domain import for templates. Rebuild
Runtime/CLI/TS/SDK/Host/packages together; backend .4 and private data formats remain.
Human action migration remains separate ongoing work. The current
background role still requires its backend-owned lifecycle cutover and full native
acceptance; component/build evidence does not close that requirement.

## Protocol 2026-10-10.3

The published Memory list returns its generated page/snapshot values, with target-bound
sort-position cursors and at most 100 items per page. CLI and shared-client collectors
traverse the complete bounded target. The former `{items}` response is removed.
The optional [Memory package](../plugins/memory/plugin.json) is an ordinary component
bound to its own installation/contribution and generated Memory capability. Reads,
add/edit/pin/delete and human review use the public installed operation entrance and
the backend's existing command/receipt owner. No plugin-specific readMemory wrapper,
Memory view kind, HTML carrier or private Desktop mutation control remains.

Client-scoped components capture optional Session/workspace targets from their existing
owners. Memory keeps user scope available without a workspace and captures the project
path before dispatch. Read failure keeps the accepted page/cursor; uncertain mutations
retain their original command and block new changes. Ledger/publication/private data
ownership remains with the Memory backend. Full cross-reload recovery stays queued.

Superseded HTML handshakes and resources are removed without aliases.

Trajectory components call the canonical `sessions.trajectory` request, including
`includeDescendants`, bounded page size and cursor scope. The existing query owns
capability admission and cursor validation. The component discovers negotiated
subagent capability and retains that scope with its accepted pagination chain.

The first-party [Session trajectory package](../plugins/trajectory/plugin.json) replaces
the compiled Desktop Timeline. It is optional and uses ordinary package installation,
explicit release selection and enablement. Its display, filters and current-page metrics consume
canonical Run, model-call and Item evidence. Runtime owns execution and recorded evidence;
the Host owns navigation, Run controls and complete Session trajectory export.

Rebuild Runtime, the shared client, CLI and Desktop together. Earlier protocol versions
are refused. Install the new package bytes from `plugins/trajectory` and explicitly select their digest;
previously admitted immutable releases do not change when their source directory moves.
There is no Timeline alias, fallback renderer or second trajectory query model.


## Protocol 2026-10-09.1

`UnresolvedEffect.output` preserves an observed Scope `chat.ToolOutput` from a
`CallError`. The terminal Run owns this evidence across reads, restart and
Session export/import. The wire and artifact carry its frozen JSON document as
text, so clients cannot round numbers or discard framework fields while parsing
and re-encoding it. Its presence establishes neither a final Tool result nor
retry safety; model history never receives it as a settled ToolResult.

The replay journal charges the complete unresolved-effect evidence, including
diagnostics and frozen output, once at publication. Its replay window and live
subscriber queues consume that same charge. An oversized event ends a subscriber
and makes the earlier cursor unavailable; clients recover from durable Run reads.

Rebuild the shared client, Desktop and CLI against this catalog. Earlier
protocol versions are refused. Session artifacts use version 31 so an earlier
importer cannot discard the new evidence. Effects without an observed output
remain valid; there is no alternate storage reader or migration path.

## Installed trajectory components

Trajectory is an ordinary prebuilt React component with declared Session scope.
The graphical navigation owner supplies its frozen Session ID through the public
SDK. Reads enter the canonical `sessions.trajectory` operation; no plugin-specific
trajectory query, HTML carrier or second history owner remains. Session replacement
retires its observer and joins reads without changing retained Run evidence.

The page owns filters and a cursor history for the last accepted bounded page.
A failed read keeps that page and cursor. Descendant scope comes from public Runtime
capability discovery and is captured for the complete pagination chain. The Runtime
still owns cursor admission, redaction, retention and source evidence.

## Protocol 2026-10-07.8

`ApprovalModeResult.modes` is required: every default mode with the gate
(`pass`, `prompt` or `deny`) it applies to write, exec and network tools, taken
from the Runtime's own policy. Clients describe a mode from it instead of
restating what each mode allows.

## Protocol 2026-10-07.7

`segment.finished` carries `run`, the RunRef the segment ended with, and
`interrupts` when that Run raised them. Its former `outcome`, `metrics` and
`contextTokens` are gone, and so is `SegmentOutcome`: the Run's own status,
outcome, metrics and context footprint state the boundary, so a client
replaces its Run instead of deriving one from why the segment stopped. A
waiting Run without `interrupts` was suspended by another Run in its tree.

## Protocol 2026-10-07.6

`SkillProposal.scope` and `SkillProposalRef.scope` are `SkillProposalScope`:
`project` or `user`. A proposal never joins an installed plugin's library, so
the wire no longer admits `installation` there; a reference naming it is
refused by request validation.

## Protocol 2026-10-07.5

Every problem that can end a Run, tool call or Item now publishes its default
recovery action: the manifest's `runChannelTypes` entries carry
`recoveryAction`, and the TypeScript contract exports `RUN_PROBLEM_RECOVERY`
beside `PROBLEM_RECOVERY` for failed requests. The two differ for a type that
rides both channels: an `internal_error` request stops, while a Run it ended
can be prompted again. Clients take whether to offer a retry from these tables
instead of keeping their own list.

## Protocol 2026-10-07.4

`SessionSnapshot.session` is required: the snapshot carries the Session it was
read from, with its status resolved from the snapshot's own Runs. Clients take
Session metadata from the snapshot instead of pairing it with a separate
`sessions.get`, which could observe another moment.

## Protocol 2026-10-07.3

A resume that answers Questions commits the answers before its continuation
opens, and the continuation now publishes each answered Question as an
`item.completed` immediately after `segment.started`. A Question Item may
therefore complete twice: once unanswered when its Run parks, and once with its
answers when the Run resumes. Clients fold the second completion like any other
Item; they no longer derive answers from the command they sent or re-read the
Session to learn them.
Resume acquires Session and working-tree admission before reading the open hand-off,
its Items, or its parked Run tree. Run identity only locates that admission. A competing
resume or cancellation therefore observes busy or a consumed hand-off, rather than
combining an old waiting record with answered Items or a completed Run.
Pending-interrupt pages read their waiting records, root Runs, and Items in one storage
snapshot. A page already in flight remains coherent when a resume consumes its hand-off;
the next page observes that consumption.

## Protocol 2026-10-07.2

`SkillProposal.origin` is required. The Runtime records why it created every
proposal, so a proposal without a valid origin is not listed and a client never
supplies a default. Older proposal files written without an origin are not
listed; resubmit them.

## Protocol 2026-10-07.1

A Run tree's root alone carries the facts it owns for the whole tree: only a
root `RunRef` has `protocolProfile`, and only a root `ArtifactRun` has
`messageMark`; both are absent on children. Portable artifact Tool results are
keyed by their owning Item. Older Runtimes and clients are refused by the exact
protocol-version check, and older artifacts by their version.

Rebuild and deploy Runtime, CLI, Desktop/Web, and generated contract
consumers together. Trajectory inspection requires `sessions.trajectory`, and evaluation
export requires `sessions.exportTrajectory`. Bundled clients reject an older
Runtime through the existing exact protocol-version checks; an updated Runtime
likewise refuses requests declaring an older version. There is no legacy endpoint
fallback. The trajectory document has its own `schemaVersion`
(`protocol.SessionTrajectoryVersion`), independent of the Runtime protocol and
importable Session artifacts.

Model invocation and Tool attempt `callId` values preserve the executor's exact
identity: 1–256 ASCII letters, digits, dots, underscores, colons, or hyphens.
Their wire validators are generated from the same rule used for persistence.
Rebuild Runtime and contract consumers together after updating these validators;
existing stored identities need no migration or rewriting.

The protocol includes bounded `WatchSpec.paths` with advertised subscription limits,
`checkpoint_conflict` for a safely refused file restore, and
`prompt_source_too_large` for an AGENTS.md cascade that cannot be included whole.
The existing Run, Segment, Item, and command identities keep their meanings.

Portable Session artifacts carry `protocol.SessionArtifactVersion` and preserve
unresolved-effect evidence for every terminal outcome. Any other artifact version
is rejected; export again from the updated Runtime. The SQLite history representation is unchanged.
Import retires the replaced Session's file checkpoints and isolated copy before
committing its history. Artifacts carry no working-tree snapshots, so imported Run
identities never grant access to a predecessor's file rollback boundaries. Use new
Runs to establish new file checkpoints. Retirement failure refuses the import;
a storage failure after retirement can leave the original history without those
optional resources. Workspace files are unchanged by import.
For a Session imported by an earlier build, submit a new import command to retire
any retained file checkpoints. Replaying its earlier receipt performs no mutation.
CLI attachment commands retain their prepared content with the original command
and replay guard; an older dispatched or ambiguous command whose attachment bytes
are unavailable remains unresolved and is never reconstructed from the current
path. A queued command that has never been dispatched can still prepare its input.

## Portable plugins

Current selection rows store contribution_id independently of server identity.
Original command routes retain that ID, named export and complete capability
contract alongside immutable provider/server/input evidence. Former host tables
without these bindings are refused by exact-schema admission without conversion.
Full public installed operations and component loading remain in the continuing
cutover; this backend binding does not claim their implementation.

Runtime owns plugin installations under its data directory. `plugin.json` uses Agent
Plugins 1.0. Flame's `io.github.tangerg.flame` namespace contains one `declaration`
path under that namespace. The referenced local JSON file declares the exact
current backend protocol requirement, optional inputs, contributions and current
presentation entries. Its own fields are strict; the former inline apiVersion
and services shapes have no interpretation. Portable MCP/Skill components remain
independent when Flame declaration admission fails. The complete operation schemas and error categories are generated in
[the API reference](contract/API_REFERENCE.md). `plugins` is an advertised capability.

Install from an absolute directory or ZIP path on the Runtime machine. Admission copies
regular files into a bounded, SHA-256-addressed release; it does not execute package code.
The shared filesystem input boundary owns regular-file kind, size admission and opened
identity for package sources, release scans, resources and packaged MCP commands.
Workspace, prompt, Skill, Hook and LSP adapters project their business error categories
without replacing the input boundary's kind, size or identity failure causes. Special
nodes are rejected before they can block a read. Copying and hashing use the opened file's
metadata and reject source replacement or mutation observed after reading; cancellation
preserves its standard category, custom cause and concurrent I/O failures, and cannot
complete a copy successfully. The package adapter owns
archive entry safety, extracted-byte quotas, release digests and the content index that
checks the exact resource bytes returned to a caller.
Staging owns one destination tree for files, declared directories and implied parents.
It rejects each duplicate, ambiguous spelling or file/directory conflict before writing
that destination. Implied parents count toward the entry limit; a later declaration of
the same directory is accepted once without changing the release's content identity.
The immutable release catalog owns the declaration accepted at first admission and is the
only owner of release content: an installation names its selected and staged releases by
digest. Installation reads hydrate selected and staged catalog declarations in the same
storage transaction as installation state. Lists, MCP definitions and Skills consume that
coherent snapshot, so concurrent release reclamation cannot break a later metadata join.
Byte availability remains a live filesystem observation after the transaction. A release
is admitted once by its constructor, which validates each contribution exactly once (package admission
isolates an invalid contribution against the ones admitted before it); persistence,
installations and projections trust the admitted value. Repeated installation and cold
loading use that declaration; integrity validation does not reinterpret package
contributions. Publish different package bytes to request a new admission.
The package adapter translates the portable `mcp.json` spelling (`streamable-http`, `cwd`)
once into the MCP registry vocabulary (`streamableHttp`, `dir`) that releases and
`PluginServerDeclaration` use; connection rules belong to the MCP server owner, and a
release adds only the rules that exist because the declaration comes from a portable
package (the reserved `PLUGIN_ROOT`/`PLUGIN_DATA` variables, case-ambiguous environment
names, package-relative commands and working directories, no user information in endpoint
URLs, HTTPS outside loopback). Package bytes are public content: static headers and
environment values are never treated as secrets, so a credential reaches a server only
through a declared input. Header and `authorization` inputs are admitted only as secrets;
an environment input uses its declared `secret` flag.
The Flame extension namespace supports `apiVersion`, `inputs` and `contributes`. Any other
member, including capability `requests`, which this API version does not support, is reported as an `unknownField`
diagnostic on its `extensionField` and ignored; it is never admitted or enforced.
Relative source paths, absent or malformed mandatory manifests and unsafe or malformed archives
return `invalid_params` without creating an installation. Filesystem and storage failures
retain their own causes; component diagnostics continue to preserve valid siblings.
Release diagnostics are typed admission findings: a closed `code` (`unknownField`,
`invalidDeclaration`, `unsupportedContribution`, `componentLimit`, `invalidDependencies`,
`unavailableComponent`) plus a closed `component` reference (`manifestField`,
`extensionField`, `contribution`, `mcpServer` and `skill` carry the authored `name`;
`flameExtension`, `mcp` and `skills` carry none). They describe what admission isolated,
travel with the immutable declaration, and never decide availability.
An installation owns selected release identity and desired `enabled` state.
Explicit installation and release selection trust those package bytes; there is no
separate release-approval operation. A new installation starts disabled. Configure
declared required inputs, then enable it. Disablement withdraws future dispatch
without discarding the selected release or standing tool decisions. Enabling the same
selected code may restore availability while execution or an unresolved command still holds that release;
configuration, release selection and uninstall still require quiescence. Package roots are
read-only. Admission seals the staged tree before digesting it, and admission and cold
loading scan an entire release once; concurrent cold loads of one digest share that scan
while other digests load independently. A scan canceled by its caller does not decide
another live caller's outcome; the surviving caller verifies under its own lifetime.
Cancellation retains both its standard category and owner-supplied cause, including
before staging, on a cached proof and while waiting for shared work.
Installation and MCP availability reads use the
same current byte proof as connection launches, so tampered bytes withdraw presentation
and availability on the next read, and repaired bytes restore both. Startup,
configuration, reconnect and OAuth read the current source owner before creating a
connection and revalidate its package bytes:
the cached scan is reused only while every entry of the release keeps the change stamp
(inode, size, mode, modification and inode-change time) recorded when its bytes were
digested, and any added, removed or changed entry or a replaced directory sends the release
through a fresh full scan that withdraws the cache for tampered bytes and restores it for
a repaired directory. A launch accepts only a scan that began after it observed a change,
and rechecks the stamps of the proof it receives, so a scan that read bytes before they
changed can never vouch for them. An entry whose inode-change time is not strictly older
than the scan that stamped it is unsettled (the racy-index rule): a proof holding one is
not reused, and the next inspection or launch rescans. Proofs are cached within a 32 MiB
budget derived from measured per-entry size; a proof that would exceed it is not cached,
and a proof an in-flight caller holds is never evicted. Platforms without an inode-change
time rescan on every inspection or connection. A stdio launch copies from the confined
verified root into private, sealed execution content and verifies that copy against the
admitted digest. Command,
arguments, environment and working directory are projected onto that content; replacing,
rewriting or reclaiming the published release cannot redirect an admitted process or its
later package reads. Connections of one digest share the content while any launch or
session holds it. A canceled preparation ends only its caller's claim; surviving callers
acquire content under their own context. This resource claim transfers once into the MCP
session ledger and is retired after process teardown, including failed handshakes, rejected or superseded
attempts, detach and shutdown. Retained source descriptors contain neither the claim nor
relocated paths and cannot launch installation stdio. Startup collects abandoned execution
copies using directory leases, preserving copies held by another Runtime. This protects
the launch from changes to published bytes; stdio still runs as trusted local code, without
an OS sandbox.

Advisory leases own their release attempt and settled cleanup result. Non-blocking
contention returns no lease and no error; descriptor or handle cleanup failures remain
errors and cannot be mistaken for contention. Owned Unix directory descriptors are closed
once after attempting unlock, with both failures retained. Borrowed-file unlocks and
Windows mutex releases remain retryable while their resource permits it. Repeated
release calls return the settled result without more OS calls. Data-directory setup
delegates release to its lease rather than maintaining a second release state.

The registry resolves each server by its origin, realizing an installation
server only as far as the read requires (desired definition, dispatch authority, or launch)
and never its siblings. Ordinary tool dispatch reads admitted declarations and
current installation authority for the called server alone. Resource and Skill reads verify their selected file
against the admitted content index through a confined directory capability. `${PLUGIN_DATA}` is a private per-installation directory
retained through updates and uninstall and is never collected automatically. A release
is retained while an installation selects or stages it or a live session, pending
checkpoint or unresolved installed command depends on it; every other release, its catalog row and its directory, is
reclaimed at the admission point when a change commits and at startup. A release whose
removal fails stays admitted and is retried by the next reclamation.

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
A secret input and an OAuth credential follow one rule: a credential follows its
recipient. For a Streamable HTTP server the recipient is the endpoint (transport, URL and
static headers, excluding configured input values), so its secrets and OAuth credential
survive a release that keeps that declaration and stop applying when it changes. For a
stdio server the recipient is the executable (release digest and server name), so its
secret inputs are dropped by every release change and must be entered again. Configuration commands require
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
never waits for, or holds back, an assembling Run. The live MCP tool catalog is a
projection of committed installation state, not a second copy that catches up: the same
critical section that commits a change cancels the affected sources' in-flight connection
attempts and detaches them, in memory and without waiting for I/O, so no Run assembled
after the commit can freeze the superseded release's tools. Launching or dialing the
committed release happens afterwards, outside the lock. The installation owner reports
whether its durable change affected selected execution capabilities; staging and
unchanged configuration do not. Quiescence controls existing executions,
while every committed execution change, including disable, invalidates
unpublished assemblies of that installation. A withdrawal failure after the durable
write preserves this invalidation; a rejected write cannot originate one.
Plugin and Skill read invalidations likewise follow the durable write, outside admission
and before realization. Withdrawal failure cannot suppress a committed change's notices;
a refused write publishes none.

Memory/Schedules human commands durably bind identity, method, admitted raw input and
original installation/release/server before network dispatch. The host records
prepared, dispatch-admitted and observed evidence, never a business result. A
dispatch admission is not proof that the backend received or committed the call.
After admission, retries query the original backend's private receipt without
resending the mutation. Missing, malformed or unavailable evidence remains unknown
and holds the release. A valid original success or business refusal releases the
hold; immutable routing remains retained so the same identity cannot adopt a new
provider after update or uninstall. History is not evicted or capped by an arbitrary
lifetime command count. The pure public jsoninput value normalizes object order
without rounding numbers or replacing original request evidence with decoded DTOs.
The host and private stores persist this input; fingerprints are derived projections.
The pure public `point` package owns consumer-defined value schema, scope,
applicability, optional absence and composition ordering. Namespace declarations
publish `points` and `pointValues` using local contract files. Installation admits
these without starting code. `ResolvePluginPoint` and `plugins point` return
immutable installation/release/value bindings through the ordinary Delivery
Endpoint; `ResolvePointValues[T]` checks the consumer's typed schema before
decoding them. Explicit order belongs to the consumer's composition owner.
This value projection does not execute decorators or persist scoped composition.
The Reviews analyzer example interprets a value as a same-release callable
contribution and invokes it through its declared optional capability dependency.

The pure public `capability` package owns local callable metadata, schemas and
canonical identity. It imports no MCP transport or Runtime internals. The public
`plugin` SDK, callbacks, actual feature bindings and generator consume that owner;
installation domain code can admit a contract without activating a backend.
Go API ownership changed without changing the backend protocol or wire identity.
This storage shape requires a fresh Runtime directory; former digest-only command
routing and private Memory/Schedules receipts are refused, as are bare-result receipts.
Retain old directories separately; opening them performs no conversion or deletion.

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
admission before asynchronous connection retirement, and leaves all retirement work in the
existing Connections shutdown ownership graph.
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
A stdio server that reports an exit status after its input closes or it is signaled has
exited, so its retirement succeeded whatever the status. Only a session that could not be
stopped, or a process-group cleanup that failed, is a retirement failure; it is logged when
it happens and still reported by Shutdown.
Canceling a wait for session retirement leaves unreported close failures with Connections
until shutdown captures them. MCP shutdown owns one terminal generation: all callers
that observe completion receive its retained result, including later calls after a
failed retirement. Canceling a waiter preserves its cancellation category and cause
without changing the cleanup generation. An already observable completion retains
precedence over that waiter's cancellation.
OAuth target fingerprints encode the owner's exact fields and header bytes through the
shared framed hash, independently of JSON serialization. Credentials recorded with the
former JSON hash require a new OAuth authorization; no dual fingerprint reader is retained.

Portable `mcp.json` admits stdio and Streamable HTTP through the existing MCP registry,
connection supervisor, deferred tools, exposure, approval and OAuth owners.
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
are inherited. HTTP authorization and secret headers use declared host inputs or the
existing OAuth owner. Credentials must not be embedded in portable files or UI assets.
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

The [first-party trajectory package](../plugins/trajectory/plugin.json) exercises a theme,
a Skill, an ordinary Session trajectory component and the current Session rename action. The public page API requires no code
execution in Runtime. Desktop's [component lifecycle](../desktop/README.md#installed-component-rendering)
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
[the repository data policy](../DEVELOPMENT.md#pre-release-data-policy). Continuation
payloads no longer carry dependency bindings. Completed historical Tool content remains generic and readable.

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
}, runtime.CommandOptions{IdempotencyKey: requestID + ":session"})
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
Startup and survivor recovery share one coordinator that acquires the sweep
lease, then reconciles Runs before Goals. It checks its lifetime before taking
ownership and before starting each phase, releases acquired ownership on every
exit, and retains a failed phase's cause alongside concurrent cancellation.
Cancellation stops unstarted phases; an already completed recovery retains its
committed result.
Before abandoned Running Runs are classified, startup acquires the ordinary
Run admission for each accepted Resume handoff and attempts its bounded revival.
An inconclusive storage or dependency failure stops startup with the binding
retained; an incompatible head follows the existing Run-lost policy.
File advisory leases consume their opened descriptor. Acquisition failure or
contention closes it immediately; an acquired lease owns both unlock and close,
retains their combined diagnostic, and never retries a retired descriptor.
Application ownership reports cleanup failures without discarding them or
replacing a completed use case's result.

## Attach without owning the Runtime

```go
client, err := runtime.Connect(ctx, runtime.RemoteConfig{
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

The standalone Runtime can publish the built `desktop/frontend/dist` directory on its own HTTP origin. Set `server.webDirectory` or `FLAME_SERVER_WEBDIRECTORY` to that directory's absolute path. Startup rejects a missing distribution instead of presenting a partially configured browser endpoint.

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
[`doc/ARCHITECTURE.md`](doc/ARCHITECTURE.md).

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

`make run` builds and runs the development server in the foreground. Use Ctrl-C
and wait for it to return before starting a replacement. For a fresh database,
run `FLAME_HOME="$(mktemp -d)" make run`; the new product root is isolated and is
retained after shutdown. `FLAME_HOME` is exported consistently to the server.
The former `start`, `stop`, `restart`, `status`, `logs`, `reset-db`, and `fresh`
targets are removed. Runtime processes can share a data directory, so a pidfile
or port owner cannot authorize deleting its database. `make clean` removes only
the compiled binary; data cleanup remains an explicit offline operation.

## Module instructions

Flame Runtime is the product backend and sole owner of durable agent semantics. It exposes one in-process Go binding and one Runtime Protocol for CLI, Desktop, and other hosts.

Read [`../AGENTS.md`](../AGENTS.md), [`../DEVELOPMENT.md`](../DEVELOPMENT.md), and the ordered Runtime baseline in [`doc/README.md`](doc/README.md) before changing this module.

- Runtime is a product backend, not another Agent Framework. Scope owns Process, strategy, tree, provider, and effect execution contracts; Runtime adapts them to Flame product semantics.
- The module root owns the public in-process `Runtime` lifecycle and typed operation methods. `protocol` owns public request, response, event, error, version, and validation values. Do not define synonymous public models or a forwarding binding package.
- [`plugin`](plugin/README.md) owns the public typed backend MCP wire binding, immutable Go-owned method contracts and logical Tool metadata translation. Memory, Schedules and independent Reviews use this same package. Private business mutation and receipt ownership stay with each backend; the binding never turns an unrecognized failure into a definite business refusal.
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

Failed attempts record the `resolve`, `call`, or `response` stage and distinguish cancellation from deadline expiry. These spans do not record prompt text, output text, or raw provider errors. They are diagnostic telemetry, not durable `modelInvocations.list` records or additional Run accounting. Telemetry retention depends on the configured exporter. Physical requests and known outcomes are separately recorded by the same endpoint observer used for Interaction, in the existing invocation journal. Session-associated records participate in trajectory/export and Session deletion; calls without that association are accessible by their original caller ancestry. Aggregate auxiliary accounting and unattached retention remain under development.

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

Git command observation retains simultaneous output-admission, process, cleanup
and cancellation failures. Output above 64 MiB is refused; cancellation does not
erase that capacity failure or its owner-supplied cause. Each invocation owns and
stops its process group, including after a normal root exit. Ordinary nonzero
exits remain observed data for Git's predicate and diff semantics.

Checkpoint predicates accept exit codes only from admitted Git command results.
A process failure carrying the same status remains a failed observation. Source
index and alternate-file admission preserves cancellation and I/O failures;
only opening an absent source permits an empty seed. Destination failures and
source changes never become successful absence.

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

Protocol `2026-10-14.1` makes keyed Steer acceptance Run-owned. Its original namespace, key, request fingerprint, reserved Item identity and immutable input commit with the Scope Signal admission head and execution context. `runs.getAcceptance` supports `steer` without redelivery or writer recovery. A consumed Signal receipt is not an applied input: only the exact completed User Item proves application. `sessions.snapshot.pendingSteers` retains accepted inputs until that Item commits, including after the Run finishes. Replaying a recorded command returns its original identity even when the original Run finished or its material was retired; importing the same Run ID never refills a retired receipt. Ordinary active execution still follows its existing crash policy.

Upgrade Runtime, Desktop and CLI together. The current SQLite command-record shape refuses older data directories without conversion. Retain the old directory separately and use a fresh directory. The SDK journal requires a concrete frozen Start, Resume or Steer command; other mutations still persist no request bodies. Older journal shapes are refused and preserved, not cleared or decoded through an adapter. Omitted original request bodies cannot be recovered from their hash.

## Command completion and configuration boundaries

Installed Schedule operations return the business owner's refusal through plugins.call; the former Core schedule_not_found/revision/workspace translations no longer exist. The publisher owns missing-item, stale-revision and workspace rules and atomically commits private receipts. Hook trust changes continue to declare workspace_unavailable when their directory cannot be resolved.

A successful start commits the Run, first Segment, and opening user Item. Resume commits the accepted interrupt responses and the new Segment opening. Neither promises a provider call has completed. Cancel is a settlement barrier: success proves a canceled Run; a natural completion that wins the race returns `run_finished`. Steer admission is described above. A missing receipt is an unknown command outcome, not permission to repeat it with a new identity.

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

The first-party `create_schedule` tool has one cold recovery class: a single
product root and its ordinary Tool processes. Before external dispatch, the
effective arguments, exact installation release and source authority, original
Process/Effect attribution, and complete execution context commit with the
canonical Tool Item. Active and waiting executions use the same context payload;
Scope's original JSON remains exclusively in the acknowledged tree head.

Startup acquires the original Session writer lease and reinstalls the original
Run and Segment pump before Scope restoration. It joins only the captured source's
already admitted connection attempt. Recovery reads the backend's original command
receipt, matching invocation identity, method, and the Tool owner's deterministic
input encoding. The receipt survives Schedule edits and deletion. Its normal Tool
output enters Scope through typed settlement and explicit Unknown resolution;
queries do not call the Tool, rerun hooks, or create another Schedule.

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

The MCP connection supervisor admits every attempt before configuration, execution
resource or credential I/O. Admission withdraws the previous session and tools and
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
Catalog publications include pending source identities atomically with executable tools.
Waiting checkpoints capture their frozen MCP tool references; recovery defers validation
only when a captured source is still connecting. This preserves waiting facts without
granting resumption or blocking startup on unrelated sources. Once dependencies settle,
resumption validates the exact configuration again. Checkpoints without these dependency
references are incompatible and are not decoded through a fallback.
Plugin realization describes current release and backend availability separately.
Server listing and startup enumerate desired definitions without realizing plugin resources;
these reads cannot overwrite the state of an admitted connection.
Unmapped connection or authorization states fail their read or projection with
an internal error; they never fabricate a server failure or panic during delivery.

Provider-reported token usage, durable conversation history, and model/Tool/execution telemetry are Scope contracts rather than Runtime restatements: `chat.Usage` survives whole to the protocol, the message store implements `history.Store`, and Scope's OpenTelemetry middleware instruments the provider, Tool, and execution-tree boundaries.

MCP identity follows Scope's capability chain through Tool decorators for discovery, exposure, and approval queries. Invalid MCP identity declarations reject the catalog instead of silently hiding entries or grouping them as built-ins.

`mcp.tools.list` reads the resolver's live MCP snapshot, the one each Run manifest freezes, and derives tool name conflicts from that same snapshot. Connections publish their current catalog when the resolver subscribes, then hand every changed tool set to that same sink inside the critical section that settles the connection. There is no separate startup catalog seed; connected Tool counts and the listing describe one state. Remote changes become visible after reconnect admits the replacement; listing no longer queries a separate live catalog. Invalid JSON Schemas, including unresolved references and oversized documents, now fail connection or probe admission through Scope rather than failing the next Run. Diagnostic and MCP schema projections preserve exact numeric literals and retain their existing protocol shape.

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


## Physical model pricing and interrupted accounting

Core 2026-10-14.26 adds optional `costUSD` to physical endpoint observations.
The endpoint records the exact provider and served model's known price beside
its response in the existing invocation journal. An absent price stays absent;
reported zero remains zero. Interaction consumes this source price and no longer
has an independent pricing callback. Auxiliary completions use the same recorder.

Restart recovery may find a known physical result behind an open logical call.
The existing recovery planner includes that immutable evidence, adds its previously
uncommitted usage/call count/cost to the original Run and settles the logical call
as failed with a known response. The logical settlement and Run replacement share
one transaction and source identity/value checks. Failure rolls both back; a later
pass cannot charge the settled call twice. No model request, Tool proposal or
semantic response continuation is reconstructed from that evidence. Calls with
no known result remain unknown. Missing reported usage and prices are not guessed
or recalculated from the current catalog.

The SQLite endpoint price column changes the pre-release schema. Rebuild clients
and use a fresh data directory; incompatible directories are refused and must be
retained separately. There is no conversion, old-schema reader or dual write. Stored
admission/response values use the existing canonical JSON input codec so exact
recovery checks retain numeric evidence instead of re-encoding a read DTO.

This covers current single-call Interaction accrual and boot loss recovery. Global/
unattached retention and aggregation, auxiliary attribution to usage totals,
multiple explicitly admitted requests, embedding metering and semantic model
selection/input transformation contracts remain under development.


## Retained physical usage reports

Core 2026-10-14.27 replaces Run-derived `usage.session`/`usage.summary` folds with
one snapshot of the existing physical model journal. Run accounting remains the
execution and Goal projection; these reports never add it to the same endpoint
consumption. Session reads include auxiliary calls bound to that Session without
a Run. Global reads also include unattached attempts and active/unknown outcomes.

Every report and bucket has `total`, `coverage` and optional `recordedCostUsd`.
Coverage includes attempts, reported usage, priced attempts and the four observed
states. Token fields are known reported subtotals. `recordedCostUsd` is the known
priced subtotal; `total.costUsd` is available only when every selected retained
attempt is priced. Empty, unreported and reported-zero evidence are distinguishable
through coverage. Missing/deleted historical records cannot establish zero spend.

Provider/model/day/origin/association buckets use the same source records. Model
keys retain exact provider plus served model; timestamps come from admission,
not Run completion. Recent windows subtract positive calendar days at one UTC
observation. Runs/Sessions are distinct associated owners, including active Runs;
bucket attempt counts reconcile, owner counts need not add across buckets.

The source scan, Session existence and ownership checks share one transaction;
only one source body is materialized at a time. Later corruption, invalid ownership
or overflow refuses the entire report. No cached report, aggregate writer, Run
fallback, legacy reader or inferred auxiliary Run association exists. Retention
follows existing source deletion: Run-associated evidence ends with its Run,
Session-only evidence ends with its Session, and unattached evidence remains in
the journal. Automatic historical pruning and embedding/multi-call metering remain
open. Rebuild Runtime/Go/TS/CLI and select rebuilt component bytes together.

Primary Interaction now captures public `plugin.ModelContextContract` contributors
before model-visible capacity admission. Their system material is measured and
sent for one call; it never becomes authored conversation. Exact captured
installation/release/order survives waiting recovery and holds the ordinary
installation dependency. A missing `model_contexts` checkpoint field is refused
as an incompatible pre-release payload. Auxiliary text use cases now capture the
same context/observer order with their actual role identity and input-text/output
budget; contributed context participates in the final byte check before dispatch.
Memory extraction/curation retain one capture and separate original call identities.
Release joins active calls before dropping their ordinary installation dependencies.
Fresh Run and auxiliary capture selection now use the public model-selection
boundary described below. Auxiliary capacity uses the captured selected-model
envelope and final composed request; request transformation remains in development.

The public `plugin.ModelSelectionContract` role chooses a route before a fresh
Run accepts its exact provider/model/reasoning identity. Media and output capacity
checks, executor construction and the original durable Run opening use that same
selected value. Session preference remains the user's proposal/default source.
Accepted Start retry and waiting Resume preserve the original Run/checkpoint and
do not call the selector again. An explicitly selected unavailable publisher is
an error; absence of a selected publisher preserves the proposal without discovery.

Auxiliary role selection enters the same public query before provider construction.
The input request is absent until the owning use case authors it; the captured
selected model then supplies subsequent context, observation and pricing identity.
Selection is per accepted execution/use-case capture; per-call routes for multiple
independent invocations remain open. Auxiliary token admission now freezes selected
catalog limits, reserves output and counts the fully composed request before
provider dispatch. Provider counting is used when available, with errors/cancellation/
non-positive results refusing; otherwise the shared request estimate is a local
preflight approximation. Unknown catalog limits remain unknown, with the caller's
existing byte/output envelope retained. Captured reasoning effort is applied before
context, counting and provider I/O; original request/selection cannot be replaced.

Auxiliary model-visible input can now be transformed through the public ordered
`plugin.ModelInputContract` point. The captured selection, Tool contracts/choice and
original output-envelope presence remain authoritative. Context precedes transforms;
final byte/output/token capacity admission precedes observer/physical execution.
Transform output never rewrites authored conversation, and transformer installations
share the existing captured dependency/cancel/join lifetime. Primary Interaction
now captures transform bindings with its configuration/checkpoint and hands the
complete admitted request through one per-call record into Call/Stream. Original
conversation/Options remain retained; final media/capacity and calibration see the
actual transformed input. Checkpoints require model_inputs with no old-shape fallback.
Core .29 moves selected-model media admission to the final transformed request.
Start/Resume/Steer still reject structurally invalid input, but retain original
well-formed source media even if the chosen model needs a converter. Unconverted
unsupported input becomes an accepted execution failure before provider generation,
with original transcript/command evidence retained. Canonical unsupported media is
not sent to a provider counter. Full native acceptance remains open.

## Protocol 2026-10-14.29

Model-media admission belongs to the final reduced/transformed request, not the
source input of Start/Resume/Steer. Source format/target/identity validation still
precedes command acceptance. Model capability refusal occurs through the existing
accepted execution failure path before the physical provider attempt; callers must
observe the Run outcome rather than interpreting opening acceptance as media/model
success. A converter may adapt the original image without changing its stored
user Item or retry identity. No new receipt, compatibility branch or error fallback
is introduced. Rebuild Runtime and clients together.

## Independent plugin model exchanges

Ordinary installed calls receive an RPC callback reference from their original
calling context. Its one Runtime endpoint now also serves the public selected-model
callbacks. `CallSelectedModel` and `StreamSelectedModel` accept a distinct invocation
ID, exact provider/model proposal, complete Scope request and `ModelCallBudget`.
Three or more separately admitted requests are allowed; Memory's private two-call
rule still belongs to Memory. A missing owning invocation is refused.

Each request resolves the explicitly installed selector before provider construction,
then captures the context, input and transparent observer chains. Its caller-owned
budget includes serialized request framing, options, Tools and media. Input conversion
may alter the provider projection; final byte/output/media/token admission precedes
the actual provider exchange. The original caller's request remains unchanged.
Unary and lazy streaming use the same capture, cancellation/join and held-dependency
owner. Streaming is advertised only when the exact provider instance supports it.

The existing observed resolver journals each physical dispatch with its own caller
ID, inherited origin/parent and final provider/model/request. Known response, usage
and price survive outer failure. Reusing the same identity cannot dispatch again,
even with changed input; transport loss remains uncertainty and does not authorize
an automatic retry. Core operations continue through canonical Delivery RPC; these
host model collaborators do not add a business receipt or a second provider journal.

The new callback methods have independent exact descriptors. Core .29, backend .2
and export schema 3 are unchanged: this is an additive host collaborator capability,
not a change to an existing command, stored record or wire method. Public Go API
inventory is regenerated. The variadic PublishRPC source signature requires external
method-value consumers to update and rebuild; no old-signature wrapper remains. Independently compiled consumers that need this capability
must rebuild from the current SDK and use a host that grants the selected methods.
Native installed-process acceptance, general remote composition, descendant Run/Goal
accounting, embedding evidence and full P01-P48 acceptance remain final-delivery work.

Background SDK model calls also carry the original retained invocation through the
existing activation-scoped projection. The shared unary/stream senders preserve
this header; Runtime validates it before selected-call child identity is derived.
The header cannot replace an ordinary captured call, continuation or actor.

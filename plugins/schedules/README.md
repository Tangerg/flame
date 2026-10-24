# Schedules

The backend owns Schedule definitions, revisions, cron cursors, durable Occurrence
claims, pending manual dispatch, captured execution input and firing acceptance.
Runtime alone owns Session, Run and Segment acceptance, execution, recovery and
model/provider configuration. A browser timer or a saved UI draft cannot advance
any scheduling fact.

The installed stdio process holds an exclusive lease and opens only
`PLUGIN_DATA/schedules.db`. Its local declaration binds `schedules-provider` to
the `schedules` server/export and exact generated consumer contract. Product
composition selects a provider for human Core commands; each enabled background
entry runs its own captured installation independently of that selection and UI.
Source-qualified MCP Tools retain their concrete installation identity. The backend
owns the minute wake-up, bounded scanner and original captured dispatch lifecycle.
It claims due work and rotates pending pages fairly. Failed pending work cannot
starve new due definitions.

The actual Schedules tool adapter invalidates owner reads after an admitted Tool
invocation, including a failed or lost reply. The notice contains no Schedule
facts or competing success. Generic MCP source authorization has no Schedules
type branch. Withdrawal during tool preparation is rechecked before invocation
and emits no dispatch-dependent notice.

Human definition commands and manual Run operations retain their original
installation/release/method/input binding in Runtime before dispatch. Unresolved
commands prevent replacement, configuration and uninstall, even after the host
database is reopened. Disable/re-enable restores that same owner. Redelivery after
dispatch admission queries the original private receipt; absence or unavailability
does not authorize another mutation or a call to a replacement provider. The
backend atomically stores successes and known refusals, rolling back refused
mutations. Runtime's routing evidence contains no competing business outcome.
The private receipt contains `business_input_json`, the original admitted JSON, and `reply`,
the typed value/refusal envelope. Fingerprints derive from original JSON rather
than decoded DTOs. The shared command format permits a pending admission with
no final reply and stores no invocation metadata; Schedules' database-only operations still commit their reply in
the mutation transaction. Former call-context input, completed-only, digest-only and bare-result databases are refused
without mutation; retain the old directory separately and use a fresh directory
for this breaking change.

The optional public Tool-result query projects the original `create_schedule`
receipt inside this backend, using the same normal output/failure codecs. Core
never decodes a private Schedule record. The query matches the original Tool name,
logical identity and admitted input digest; edits and deletion do not rewrite its
historical output. Unsupported Tool reads remain absent evidence. No replay or
additional persisted output representation is introduced.

Before dispatch, the backend persists the exact instructions, title, workspace,
model selection, stable Session/Run identities and original calling-owner
invocation (origin, parent and concrete target). Editing or deleting the
original definition does not rewrite that captured work. Manual dispatch enters
this durable queue before Runtime is called, so a lost reply or canceled caller
can be reconciled by the next dispatch. Pending scans return this original
evidence; a scanner cannot replace a human or model trigger with a new automatic
origin. The backend restores each captured invocation in its activation callback;
Runtime validates original human/model evidence against already dispatched routing
and the authenticated Actor. A new automatic trigger belongs only to this installation.

Cron occurrences retain `invocation_json`; manual work retains the complete
`captured_request_json`. Their immutable SQL guards prevent rewriting captured
caller evidence. Former tables are refused before schema creation or data
mutation; keep them separately and use a fresh Schedules directory. No callback
credential or mutable Run state is stored in these records.

Runtime rechecks installation authority inside the atomic Session/Run opening and
records acceptance in the same transaction. Its receipt names the accepted Run;
it is retained even after that Session or Run is deleted. Backend acceptance then
settles the private pending entry and updates a surviving definition once. A lost
acknowledgement redelivers the same identity and receives the existing acceptance,
including after terminal execution or Runtime restart. Selecting a new release
can reconcile captured work only through its original available current contract;
former incompatible routes are not converted or retargeted.

`plugins/schedules/backend/rpc/contracts.go` is the typed method
source used by actual server registration and query consumers. Its generated
canonical capability, Schema, method references and TypeScript data maps live
under `io.github.tangerg.flame/contracts` in this package. Regenerate from the workspace
with `go generate ./plugins/schedules/backend/rpc`; the existing contractgen
projects the same methods. The package builder copies this namespace into the
compiled package. Do not hand-edit generated artifacts or duplicate schemas.
The backend publishes the named `schedules` capability. Runtime validates the
complete contract and declared export, including runNow, and captures the
contribution/export and complete capability for original command recovery. Public
installed operation entrances retain that same original business owner.

The backend executable connects through the public `ConnectCallback` and ordinary
Runtime Client. `runs.start.newSession` carries the captured Session/Run identities;
`runs.getAcceptance` checks that same original request before submission or
reconciliation. A retired acceptance still proves the original opening. Core
observation detachment does not cancel an accepted Run. Each admission owns a
child observation context and releases it on return, including invalid or lost
acknowledgements, while retaining the original request for acceptance recovery.
The backend completes
pending work and its dispatch reply in one private transaction, with Runtime I/O
outside that transaction. A pending dispatch may reconcile again; a completed
receipt never invokes Runtime again.

Callback references travel as ephemeral MCP metadata, outside business input,
fingerprints and saved pending work. The actual managed connection establishes
the installed actor and its release/source authority. The private database binds
to one Runtime store namespace before capturing work and refuses another store.
Former private databases without this binding are refused without conversion;
retain them separately and use a fresh directory. The host's former automation
receipt table and special opening API are removed in the same cutover.

Disable and uninstall withdraw new Run admission and stop the process.
Already accepted Runs continue under Runtime. Pending private work and command
receipts remain in the installation directory. Reenable and restart resume that
same owner; uninstall does not delete data. Definition disable stops future cron
claims, while captured dispatch remains pending. There is no host Schedule store,
legacy worker or fallback to a built-in tool.

An admitted scheduling Tool dispatch and an unconfirmed human command invalidate
human reads even when the reply fails. The notice carries no facts; the next query reads the backend owner.

Use [the local build workflow](../README.md). Backend source lives in
[`backend`](backend) in the independent first-party Go module. Verify from `plugins` with the workspace
enabled: `go test ./...`, `go vet ./...`, and `go build ./...`.

## Public manual operation

`flame.schedules/runNow.v1` owns the complete manual use case. It captures the
current definition and original invocation with its pending command in one private
transaction, then invokes the existing Runtime Run admission outside that transaction.
Private acceptance and the original outcome reply commit together. Host human
commands and pending-manual recovery invoke this same operation; the separate
prepareManual export and host prepare-then-dispatch workflow are removed.

Session/Run identifiers derive from the admitted installation target and original
command identity, so recovery locates the retained private capture without another
route or mapping store. Edits/deletion cannot replace captured input. Success and
classified business refusal retain their private replies. Missing acknowledgement
remains uncertainty; receipt absence cannot originate another command or Run.

The Schedules capability identity changes; rebuild its generated contracts and
installed backend together. The current backend protocol and stored business formats are
unchanged by this operation cutover. The ordinary bound component and declared background contribution use
this public operation; no host prepare-then-dispatch path remains.

## Bound editing component

The client-scoped public component binds its own schedules-provider contribution
and generated capability. Reads use actual Rows/NextCursor/private Snapshot shapes;
NextRunAt is the existing enabled projection, not a separate flag. Add/edit/toggle/
delete and manual Run commands use the same private owner. Edits preserve the captured
revision, and model choices query each configured Provider through the public Runtime
catalog; no empty Provider stands for an aggregate model catalog. Same-key retry is
retained after unknown acknowledgement; new commands are blocked until reconciliation.

Each configured Provider's model catalog is read independently. Named failures
remain visible while successful catalogs stay selectable; a failed refresh retains
that Provider's last accepted catalog within the captured component context. A
refresh never replaces a saved selection. Missing and deprecated selections retain
their exact Provider/Model as labeled items until the user chooses another model.

Add/edit is a bounded dialog with scrolling field groups and persistent task
identity, Cancel and Save. Deletion is a separate named alert dialog and initially
focuses Cancel. The catalog remains mounted beneath either task. Explicit
cancellation, the close action and unhandled Escape return to the captured opener;
if a canonical refresh removed or disabled it, the retained page search is the fallback and
keeps its query. Accepted deletion returns directly to search because its record
will disappear; accepted edits likewise return to search while the record refresh
keeps its actions disabled. New-task Add/Templates remain available during a
catalog read and regain focus on creation or cancellation. A later refresh never steals that focus. Nested model
choices consume their own Escape first.

Command progress, rejection and original-outcome reconciliation render in the
active task's persistent action zone, so scrolling the fields cannot hide the
outcome or its next action. Pending or uncertain commands disable dismissal and new submission;
they retain the exact draft and original command rather than treating cancellation
as a business rollback. A rejected save keeps the draft editable unless the
existing revision-conflict rule retires it. The existing command owner alone
settles acceptance and refreshes the catalog.

Templates live in src/templates.ts as authoring content copied into a new draft.
The generic Domain/loader/Protocol owns no Schedule template or closed view kind.
The component consumes existing schedule notices and cancels/joins its I/O on
retirement. Failed pagination keeps the accepted page/cursor. Shared-client recovery
and backend background activation are covered separately from excluded graphical
window-reload interaction.

Catalog loading, failed reads and accepted empty pages remain distinct. Only an
accepted page supplies records, counts or an empty result; it stays visible while
another read is pending or fails. Read failures name Refresh as the recovery
action and claim retained data only when a page was accepted. Read and live
failures pair that complete localized status/recovery summary with a separate
literal publisher explanation, preserving its punctuation and line breaks rather
than interpolating it into the sentence. Refresh preserves
its keyboard position during reads while blocking repeated activation; unresolved
commands retain their independent native disablement.

## Declared background activation

The schedules-provider contribution declares background activation independently of
provider selection or an open component. Its setup uses the public activation callback
and verifies the attached Runtime against retained namespace evidence without writing
private state. Backend teardown joins that connection before closing SQLite.

Setup returns a provisional ActivationResource without scanning or changing business
facts. Runtime calls the public Activate handshake after setup succeeds; repeated
activation cannot start another loop. Disable/replacement/shutdown cancel and join the
backend's loop and callback connection; accepted Runs remain owned by Runtime.

The former host scan driver, Bootstrap worker/join and public scan/dispatch exports
are deleted. A pending manual entry resumes its existing private command and exact
original JSON, then commits private acceptance and the original runNow receipt in
one transaction. Recovery never rereads an edited/deleted definition or creates a
new command. Cron acceptance uses the existing private occurrence owner. The backend
publishes an authenticated contribution notice through plugins.notifyChange; current
readers receive plugins.dataChanged and reread rather than infer success.

Core .19 uses this generic plugin-data topic for command dispatch/recovery,
authenticated background notices and installed Tool calls. No first-party contract
match chooses a business topic, and no Schedule record IDs or results enter the
event. The ordinary component subscribes under its captured lifetime and refreshes
its bound owner; installation lifecycle remains plugins.changed. Private mutation
notices still follow actual commits. Rebuild current Go/TS/ESM consumers together;
the retired schedules.changed topic has no alias or double publication.
No second scheduler, receipt store or compatibility reader exists. Backend restart,
withdrawal and non-selected-entry activation use the same declared lifecycle.

The publisher's go:generate contract command also emits the CLI's typed Go schema
projection, alongside canonical capability JSON and TS. CLI catalog reads freeze
the selected binding and enter the same public installed list operation as the
ordinary component. CLI mutations use the public installed operations and original
receipt recovery; the former Core Schedule facades are removed without aliases.

## Feature boundary

Business changes expose a local `OnChange` signal within the caller's current
write-set. The transport owns publication after its private transaction commits;
no Core invalidation record or resource enum enters the feature application.
Failed writes and losing curation comparisons cannot emit a successful change.

Schedule identity parsing, the `sch_` namespace and occurrence framing belong to
this backend. Core no longer exports a Schedule identity type or Go/TS prefix;
external consumers obtain business identities from the published feature contract.
The opaque values and stored private format are unchanged.

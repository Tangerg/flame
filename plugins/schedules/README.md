# Schedules

The backend owns Schedule definitions, revisions, cron cursors, durable Occurrence
claims, pending manual dispatch, captured execution input and firing acceptance.
Runtime alone owns Session, Run and Segment acceptance, execution, recovery and
model/provider configuration. A browser timer or a saved UI draft cannot advance
any scheduling fact.

The installed stdio process holds an exclusive lease and opens only
`PLUGIN_DATA/schedules.db`. Its local declaration binds `schedules-provider` to
the `schedules` server/export and the exact generated consumer contract. Product composition explicitly selects an installation
and contribution for human commands and background scans; enabling another
candidate does not replace it. Source-qualified MCP Tools retain their concrete
installation identity. Runtime
owns the minute wake-up and dispatch lifecycle; the backend's bounded scanner
claims due work and rotates pending pages fairly. Failed pending work cannot
starve new due definitions.

The actual Schedules tool adapter invalidates owner reads after an admitted Tool
invocation, including a failed or lost reply. The notice contains no Schedule
facts or competing success. Generic MCP source authorization has no Schedules
type branch. Withdrawal during tool preparation is rechecked before invocation
and emits no dispatch-dependent notice.

Human definition commands and manual preparation retain their original
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

Before dispatch, the backend persists the exact instructions, title, workspace,
model selection, stable Session/Run identities and original calling-owner
invocation (origin, parent and concrete target). Editing or deleting the
original definition does not rewrite that captured work. Manual dispatch enters
this durable queue before Runtime is called, so a lost reply or canceled caller
can be reconciled by the next dispatch. Pending scans return this original
evidence; a scanner cannot replace a human or model trigger with a new automatic
origin. Runtime restores it before opening the callback, and the backend refuses
dispatch metadata that differs from the stored trigger.

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
can reconcile old captured work from the same installation without changing it.

`runtime/internal/plugins/schedules/rpc/contracts.go` is the typed method
source used by actual server registration and query consumers. Its generated
canonical capability, Schema, method references and TypeScript data maps live
under `io.github.tangerg.flame/contracts` in this package. Regenerate from Runtime
with `go generate ./internal/plugins/schedules/rpc`; the existing contractgen
projects the same methods. The package builder copies this namespace into the
compiled package. Do not hand-edit generated artifacts or duplicate schemas.
The backend publishes the named `schedules` capability. Runtime validates the
complete contract and declared export, including dispatch, and captures the
contribution/export and complete capability for original command recovery. Public
installed operation entrances, components and point dependency graphs remain open.

The backend executable connects through the public `ConnectCallback` and ordinary
Runtime Client. `runs.start.newSession` carries the captured Session/Run identities;
`runs.getAcceptance` checks that same original request before submission or
reconciliation. A retired acceptance still proves the original opening. Core
observation detachment does not cancel an accepted Run. The backend completes
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
[`runtime/internal/plugins/schedules`](../../runtime/internal/plugins/schedules) as part of the
local Runtime application module. Verify from `runtime` with the workspace
enabled: `go test ./...`, `go vet ./...`, and `go build ./...`.

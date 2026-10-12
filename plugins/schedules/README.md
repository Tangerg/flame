# Schedules

The backend owns Schedule definitions, revisions, cron cursors, durable Occurrence
claims, pending manual dispatch, captured execution input and firing acceptance.
Runtime alone owns Session, Run and Segment acceptance, execution, recovery and
model/provider configuration. A browser timer or a saved UI draft cannot advance
any scheduling fact.

The installed stdio process holds an exclusive lease and opens only
`PLUGIN_DATA/schedules.db`. Its closed `schedules` declaration binds human commands
and source-qualified list/create/delete MCP Tools to one enabled owner. Runtime
owns the minute wake-up and dispatch lifecycle; the backend's bounded scanner
claims due work and rotates pending pages fairly. Failed pending work cannot
starve new due definitions.

Human definition commands and manual preparation retain their original
installation/release/method/input binding in Runtime before dispatch. Unresolved
commands prevent replacement, configuration and uninstall, even after the host
database is reopened. Disable/re-enable restores that same owner. Redelivery after
dispatch admission queries the original private receipt; absence or unavailability
does not authorize another mutation or a call to a replacement provider. The
backend atomically stores successes and known refusals, rolling back refused
mutations. Runtime's routing evidence contains no competing business outcome.
The private receipt contains `input_json`, the original admitted JSON, and `reply`,
the typed value/refusal envelope. Fingerprints derive from original JSON rather
than decoded DTOs. The shared command format permits a pending admission with
no final reply; Schedules' database-only operations still commit their reply in
the mutation transaction. Former completed-only, digest-only and bare-result databases are refused
without mutation; retain the old directory separately and use a fresh directory
for this breaking change.

Before dispatch, the backend persists the exact instructions, title, workspace,
model selection and stable Session/Run identities. Editing or deleting the
original definition does not rewrite that captured work. Manual dispatch enters
this durable queue before Runtime is called, so a lost reply or canceled caller
can be reconciled by the next dispatch.

Runtime rechecks installation authority inside the atomic Session/Run opening and
records acceptance in the same transaction. Its receipt names the accepted Run;
it is retained even after that Session or Run is deleted. Backend acceptance then
settles the private pending entry and updates a surviving definition once. A lost
acknowledgement redelivers the same identity and receives the existing acceptance,
including after terminal execution or Runtime restart. Selecting a new release
can reconcile old captured work from the same installation without changing it.

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

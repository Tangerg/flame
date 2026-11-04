# Private SQLite command receipts

This public SDK package owns the existing atomic mutation/receipt mechanism used by
Memory and Schedules. A plugin supplies its own database, use case, typed operation
and business refusal classifier. The `commands` table belongs to that private
business database; Runtime routing evidence contains no competing business result.
Typed backend methods and receipt envelopes come from `packages/go-sdk/plugin/mcp`; this Flame
package adapts them to the plugin's existing private SQLite business owner.

`RegisterCommand` commits a known value or refusal with its mutation. A refusal
rolls the mutation back before storing its original refusal receipt. Unexpected
failure invents no receipt. `ReadReceipt` and its `RegisterReceipts` transport read the original method/input
binding without invoking the use case. Reusing an identity with changed original
JSON is a conflict; absent evidence never authorizes replay.

`Command` accepts only the original admitted JSON and decodes the handler's typed
input from that value. Callers cannot supply a separate mutation input under the
same receipt identity. Typed conversion never changes the stored numeric lexemes.

Identity conflicts retain the public `frame/process/mcp.ErrInvocationConflict`
category. Classify it with `errors.Is`; an independent author's refusal classifier
does not need private Runtime source or an error-string match.

For operations with external work, `AdmitCommand` commits original admission first.
Only `NewlyAdmitted` permits a first external attempt. `ResumeCommand` reads pending
work without creating it; `PreparedCommand.Complete` joins the private owner's short
publication transaction. Unknown outcomes remain pending, including after restart.
Use `github.com/Tangerg/frame/sqlite.OpenSchema` for schema admission. Local
`Command` owns one private database transaction and savepoint so a classified
refusal can roll back its mutation while retaining its reply in the same commit.
Repositories use this package's `Connection` and `WithinTransaction` to borrow that
original write-set; the borrow expires when the command ends and cannot enter a
different database. Outside a command, those functions delegate standalone
transaction scopes to Frame. Runtime's framework and product database continues
to use Frame's own scoped storage authority.

A failed commit returns `ErrCommandUnconfirmed` and discards its original database
connection. It never publishes a prepared success or refusal, even if the database
committed before losing its acknowledgement. The original retained receipt is the
only evidence that can subsequently establish that command's outcome.

After an external service has returned an authoritative acknowledgement, use
`PreparedCommand.SettleExternal` to persist it independently of caller cancellation.
An optional database write records associated business evidence in that same
bounded transaction. An interrupted exchange with no authoritative outcome stays
pending and cannot be dispatched again. Use `Complete` for work that belongs to
an existing local domain transaction.

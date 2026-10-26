# Private SQLite command receipts

This public SDK package owns the existing atomic mutation/receipt mechanism used by
Memory and Schedules. A plugin supplies its own database, use case, typed operation
and business refusal classifier. The `commands` table belongs to that private
business database; Runtime routing evidence contains no competing business result.

`RegisterCommand` commits a known value or refusal with its mutation. A refusal
rolls the mutation back before storing its original refusal receipt. Unexpected
failure invents no receipt. `ReadReceipt` and its `RegisterReceipts` transport read the original method/input
binding without invoking the use case. Reusing an identity with changed original
JSON is a conflict; absent evidence never authorizes replay.

Identity conflicts retain the public `packages/go-sdk/idempotency.ErrKeyConflict`
category. Classify it with `errors.Is`; an independent author's refusal classifier
does not need private Runtime source or an error-string match.

For operations with external work, `AdmitCommand` commits original admission first.
Only `NewlyAdmitted` permits a first external attempt. `ResumeCommand` reads pending
work without creating it; `PreparedCommand.Complete` joins the private owner's short
publication transaction. Unknown outcomes remain pending, including after restart.
Use `packages/go-sdk/storage/sqlite` for schema admission and transaction ownership.

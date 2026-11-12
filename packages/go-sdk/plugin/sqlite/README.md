# Private SQLite command receipts

This package binds a plugin's original command input, business decision and receipt
to its private database. Frame's `sqlite.Database` owns the physical connections,
transaction lifetime, commit acknowledgement and recovery fence. Runtime routing
evidence contains no competing business result.

## Local commands

`Command` and `RegisterCommand` accept a preparation callback that returns an
immutable `CommandPlan[R]`. They open one independent Frame write transaction and
run preparation under `RunReadSnapshot` within that transaction. The read lease
allows the original snapshot to be inspected but physically prohibits mutation
and promotion to a writer.

Preparation validates the input, reads the current business state and asks the
domain owner for its decision. A classified business refusal becomes the original
receipt without any business write. A successful preparation calls
`NewCommandPlan(result, apply)`; the constructor validates and encodes the result
before `apply` can run. A nil `apply` represents a successful decision requiring no
mutation. The plan does not retain a second mutable copy of the typed reply.

After the read lease ends, the command applies the decided mutation and inserts
its receipt in the same transaction. Repositories use
`github.com/Tangerg/frame/sqlite.Connection` inside that scope. Related standalone
repository operations compose their decision and write with Frame's `RunInTx`.
Every unscoped read enters `Database.Read` or `RunReadSnapshot`; rows and connection
capabilities must be consumed before that callback returns.

The classifier is used only during preparation. A mutation, cancellation, receipt
write or transaction failure returns `plugin/mcp.ErrOutcomeUnconfirmed` and cannot
be reclassified by the transport, even when its error chain contains a business
refusal. Frame rolls back the complete write-set or fences its database if commit
acknowledgement is indeterminate. An indeterminate command also returns
`ErrCommandUnconfirmed`. The original retained receipt is the only evidence that
can establish the outcome after reopening.

`Command` accepts the original admitted JSON and decodes its typed preparation
input from that value. Callers cannot supply a separate mutation input under the
same receipt identity. Stored numeric lexemes remain unchanged. A completed
same-key redelivery returns the original reply without preparing or applying
again. Reusing an identity with a changed method or original input is
`frame/process/mcp.ErrInvocationConflict`. A pending receipt remains unconfirmed;
its absence never authorizes an external retry.

`ReadReceipt` and `RegisterReceipts` project the original method/input binding
without entering the command. Use `errors.Is` for identity conflicts instead of
matching error strings or importing Runtime internals.

Registration captures the typed method by value. Replacing the caller's handle
cannot change the published method's receipt identity or output validation.

## Commands with external work

`AdmitCommand` retains the original input before external I/O. Only
`PreparedCommand.NewlyAdmitted` permits the first external attempt.
`ResumeCommand` reads an existing admission without creating work. A pending
admission after restart requires evidence from the original external owner; it
cannot be dispatched again merely because its final reply is absent.

Use `CompleteCommand` when external work produces a proposal that still needs a
local decision. Its preparation and mutation use the same read-then-write protocol
as `Command`, with the existing pending row as the receipt owner. A settled row
returns its original reply without repeating preparation. `CompleteCommand` owns
an independent transaction, so a returned result has passed the physical commit
boundary. Put related business writes in its plan instead of nesting this call in
another transaction.

`CompleteExternalCommand` uses that same protocol after an authoritative external
acknowledgement. It preserves the acknowledgement across cancellation of the
original request and bounds settlement with the package's single external
settlement timeout. The caller supplies the same preparation callback and does not
recreate the cancellation policy.

`PreparedCommand.Complete` attaches an already encoded, authoritative result to an
existing local transaction. It performs no business classification.
`PreparedCommand.SettleExternal` independently persists an already encoded external
acknowledgement, with optional associated business evidence, under the same bounded
cancellation policy. Repeated settlement must match the original reply byte for
byte and never repeats the evidence callback.

Private schemas include `CommandsSchema` and use Frame's `OpenSchema` for admission.
The package does not own a raw SQL pool, transaction or savepoint. Incompatible
private formats are refused without conversion; development builds use a fresh
data directory.

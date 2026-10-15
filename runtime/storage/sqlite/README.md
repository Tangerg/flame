# SQLite ownership

This public package supplies exact-schema admission and transaction settlement for
Runtime and independent plugin authors. Call `OpenSchema` with the owner's schema
and a private database path. Empty databases receive that schema; an incompatible
existing schema is refused before writes. The owner retains its old data separately.
The package configures WAL, foreign keys, bounded busy waits and one connection.

File-backed WAL is admitted explicitly before schema SQL; the explicit :memory:
database keeps SQLite's memory journal. SQLite can bypass its busy handler
while upgrading journal-mode locks, so connection-string WAL setup cannot promise
either concurrent admission or caller cancellation. The boundary waits only for
SQLITE_BUSY from that idempotent mode acquisition, within the existing five-second
budget and caller context, then requires the returned mode to be WAL. Ordinary
write contention retains SQLite's busy handler. WAL is persistent; replacement
pool connections do not reconfigure the journal. Schema and business callbacks
are never repeated by this wait.

Existing schema admission owns a read-only snapshot and never reexecutes
initialization statements. It cannot compete with a live recovery write-set or
recreate a missing owner fact. Empty databases are rechecked under the writer
slot before schema/data initialization commits atomically. Ordinary write-sets
acquire that slot before reading, so a deferred snapshot upgrade cannot fail
after the callback has already begun. No application retry is introduced.
`InstallSchema` uses the same boundary for callers supplying their own configured
SQLite database; writer transactions must use the immediate mode configured by
`OpenSchema`. Its statements describe empty-database initialization.

`Connection(ctx, db)` and `RunInTx(ctx, db, fn)` join only a transaction owned by
that exact `*sql.DB`. Nested work on another database keeps its own transaction
and outcome; this is not a transaction across plugin fact owners. An error, panic,
cancellation or goroutine exit settles the owned transaction before returning its
connection. Rollback diagnostics and cancellation causes are preserved, and a failed
settlement discards the connection.

`RunReadSnapshot` uses a deferred read snapshot so another WAL connection can
commit while the reader retains its original consistent view. It joins an ambient
read or write transaction for this database. Its callback must not mutate;
`RunInTx` refuses promotion of a read snapshot into a write-set. Query, export,
checkpoint and installation reads use this boundary; mutation paths retain
immediate writer admission. Neither boundary retries the callback.

`RunIndependentTx` deliberately starts a new write-set outside an ambient transaction
for that database. An attempt to enter it while the same database transaction is
active is refused rather than waiting on its own single connection. Schema admission
and Runtime workspace intent durability use this explicit boundary.

Private business command receipts are available through the public
`github.com/Tangerg/flame/runtime/plugin/sqlite` package. They remain in the
business database and commit with that owner's mutation. This package stores no
host success ledger, plugin data copy or replay authorization.

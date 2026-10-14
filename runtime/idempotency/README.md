# Opaque request identity and replay

This public mechanism owns the stable identity-conflict, lost-claim and
outcome-conflict categories shared by Runtime delivery and private command
receipts. Independent authors can use `errors.Is(err, idempotency.ErrKeyConflict)`
without private Runtime imports or error-text parsing.

`Record` and `Store` preserve opaque request fingerprints and result bytes. The
business owner chooses those values and atomically commits its mutation/receipt;
this package creates no store or competing result owner. Completed generic results
use the existing retention policy. Unresolved reservations never expire merely
because time passed. Private plugin receipts retain their own storage lifetime.

All former `internal/idempotency` callers migrate to this package. The private
entrypoint is deleted; no error alias, database conversion or replay path remains.

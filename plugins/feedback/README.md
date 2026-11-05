# Feedback

Feedback prepares an immutable quality observation from the user's rating or text. It preserves authored whitespace and retains the original decision under the provider command identity. The installed message action captures the session, run, item and selected release before dispatch. An uncertain command is retried with the original identity.

Both the first submission and an original retry enter `ComponentContext.track`
before starting I/O. The host retains the component use until that original
promise settles. A retired mount cannot start a retry or discard the unresolved
command merely because its lifetime no longer admits work.

Runtime appends the prepared observation and its acknowledgement atomically. The annotation ledger stores opaque JSON, the exact provider, its release and the original references. Session trajectory export reads these annotations and execution evidence at one SQLite cutoff, including after the originating installation is withdrawn.

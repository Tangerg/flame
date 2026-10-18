# Client diagnostics

This optional ordinary React component displays traces, metrics and logs from the
current graphical client's existing telemetry owner through public
`ComponentContext.telemetry`. All component publishers receive the same explicit,
lifetime-bound collaborator; author code imports no private store or application alias.

Read returns a detached snapshot of the current bounded buffer: newest traces/logs
first and metrics in stable key order. The former HTML transport's 256-record/128-KiB
cutoffs and omission DTO are removed. The telemetry owner still owns buffer capacity;
older evictions cannot be reconstructed. Displayed rows are labeled as the last read.
Log identities use the first nonblank line; expanded rows retain the complete
recorded body, including blank lines and stack indentation.

Refresh rereads that owner. Clear delegates to its existing local clear operation
and discards the displayed snapshot before reading again. The component does not
clear Runtime accounting or remote telemetry. Retirement invalidates both operations
without changing the retained buffer. Read failure preserves the last accepted read.

The package declares installation scope and a generic `diagnostics` material key for
the existing navigation consumer. It uses prebuilt ESM, shared React/UI and normal
component resources. No callable backend, HTML carrier or private RPC wrapper remains.

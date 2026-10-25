# Client diagnostics

This optional ordinary React component displays traces, metrics and logs from the
current graphical client's existing telemetry owner through public
`ComponentContext.telemetry`. All component publishers receive the same explicit,
lifetime-bound collaborator; author code imports no private store or application alias.

Read returns a detached snapshot of the current bounded buffer: newest traces/logs
first and metrics in stable key order. The former HTML transport's 256-record/128-KiB
cutoffs and omission DTO are removed. The telemetry owner still owns buffer capacity;
older evictions cannot be reconstructed. The existing signal controls carry their last-read counts, formatted for the active
locale. Counts come directly from the accepted snapshot; unknown counts stay absent
after an initial failure or failed reread following Clear. One status names the last
read and distinguishes retained accepted content after failure. Repeated
introductions and a separate count dashboard are removed. The About disclosure
retains the bounded-buffer limitation.
Log identities use the first nonblank line; expanded rows retain the complete
recorded body, including blank lines and stack indentation. Histogram sample counts
are dimensionless; measured sums, percentiles and averages retain their unit, as do
dimensional counter values. Raw evidence keeps every original field.

Refresh rereads that owner. Clear delegates to its existing local clear operation
and discards the displayed snapshot before reading again. The component does not
clear Runtime accounting or remote telemetry. Retirement invalidates both operations
without changing the retained buffer. Read failure preserves the last accepted read.

The package declares installation scope and a generic `diagnostics` material key for
the existing navigation consumer. It uses prebuilt ESM, shared React/UI and normal
component resources. No callable backend, HTML carrier or private RPC wrapper remains.

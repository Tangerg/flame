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
Raw-record code actions and accepted-copy feedback use the same active component
locale as the named reading region. Shared UI retains source, selection and
wrapping behavior; telemetry read and clear ownership remain unchanged.
Log identities use the first nonblank line; expanded rows retain the complete
recorded body, including blank lines and stack indentation. Histogram sample counts
are dimensionless; measured sums, percentiles and averages retain their unit, as do
dimensional counter values. Raw evidence keeps every original field.

Only the closed span status (`unset`, `ok`, `error`) and metric kind (`histogram`,
`counter`) badges use localized display words. An OpenTelemetry `unset` status means
that no status was set. Open span kinds, provider-defined log severities,
identifiers, bodies and raw JSON keep their exact recorded values.

Refresh rereads that owner. Clear delegates to its existing local clear operation
and discards the displayed snapshot before reading again. The component does not
clear Runtime accounting or remote telemetry. Retirement invalidates both operations
without changing the retained buffer. Read failure preserves the last accepted read.
The initial read and replacement of the telemetry collaborator or component signal
read that owner. Display-language changes preserve the accepted snapshot, selected
signal, expanded evidence and later focus without reading or clearing the buffer.
Read and Clear failures retain their original cause: literal details keep their
line breaks and indentation in the shared reader, while fallback guidance follows
the current display language. A language change does not dismiss a failure or
accept newer observations; explicit Refresh remains available for that read.

The package declares installation scope and a generic `diagnostics` material key for
the existing navigation consumer. It uses prebuilt ESM, shared React/UI and normal
component resources. No callable backend, HTML carrier or private RPC wrapper remains.

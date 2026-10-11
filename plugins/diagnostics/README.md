# Client diagnostics

This optional package displays traces, metrics and logs from the current graphical
client's existing telemetry buffer. It does not collect telemetry, read remote Runtime
observations, or calculate Runtime accounting. Enable the package explicitly through
Plugins; it is not compiled into the application.

Each read returns at most 256 whole records and 128 KiB of record JSON per signal.
Newest traces and logs are considered first. Oversized records are omitted, with the
omission count shown; earlier buffer eviction cannot be reconstructed from a read.
Refresh renews exact installation and resource admission before projecting the latest
local observations. It does not expose a cursor, choose a Runtime, or accept queries.

The trusted client owns the Clear button and calls the existing telemetry store's
clear operation. The isolated page can only read. Clearing remounts the page so its
display no longer retains the previous observations. Revocation, connection replacement
and withdrawal retire the page without clearing or otherwise changing telemetry.

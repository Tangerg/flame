# Saved usage reports

`flame.usagereports` owns a private, immutable archive of daily and weekly core
accounting responses. Its workspace component saves, lists, reads and removes
reports through the same declared typed contribution. It exposes no model Tool.

Core owns endpoint evidence, pricing, coverage and every total. A newly admitted
capture calls the public `GetUsageSummary` callback with a one-day or seven-day
relative period. Core returns the actual UTC observation time and lower bound
from its one clock reading. The archive copies that response without deriving
totals, filling missing values or changing unknown prices into zero. These are
rolling periods over retained evidence, not calendar-day settlement. Observation
time describes the core query; it is not a transaction cutoff for later writes.

The backend stores reports and original command receipts in `PLUGIN_DATA/reports.db`.
A capture command admits its original identity before the callback; publication
and settlement are one private transaction. A missing settlement never authorizes
another accounting observation under the same identity. Explicit recovery uses
the original command, and confirmed retirement does not delete an already saved
report. Capture receipts hold only report headers, so removing a report removes
its saved accounting payload. Core evidence and totals are unaffected.

Each accepted command has its own immutable report ID. Replaying a capture after
removal cannot recreate that report, and an old removal cannot target a later
capture. Capacity is 64 reports, each at most 1 MiB of canonical accounting JSON;
full archives refuse new captures until a report is explicitly removed. Stored
data that fails identity or summary validation is an error, never an empty
archive or a newly calculated replacement.

The component uses public SDK queries and original-command recovery. It cancels
retired reads, retains the last accepted report after a read failure, and displays
wide integers and exact JSON numbers without rounding them through JavaScript
`number`. Missing values remain missing; recorded zero remains zero.

Regenerate the typed contribution after changing its methods:

```sh
go generate ./plugins/usagereports/backend/rpc
npm run build --prefix plugins
```

Build and install the complete target package with the ordinary first-party
package builder. Source installation never compiles the backend. Reinstall and
select the new package digest when its declaration or component changes.

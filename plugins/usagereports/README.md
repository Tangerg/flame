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
go generate ./plugins/usagereports
npm run build --prefix plugins
```

The published source is `contracts/capability.json`. Go consumer codecs and the
TypeScript implementation bind that exact descriptor; old schema definition
names remain opaque contract data. The backend uses `@flame/backend-sdk` and
the distribution's selected Node companion, with `node:sqlite` as the private
archive owner. Exclusive SQLite ownership spans the process lifetime;
transactions never span host I/O. There is no replacement Go executable.

Build a portable package with the ordinary first-party builder:

```sh
node plugins/scripts/build-packages.mjs --plugin usagereports --output /absolute/new/packages
node plugins/scripts/build-author-kit.mjs --plugin usagereports --output /absolute/new/author --offline
```

The exported full-stack project has its own lock, packed SDKs and prebuilt
development tools. Its generate/typecheck/test/build/package commands require
no Go source or compiler. Runtime installation executes no build or npm install.

This pre-release cutover requires fresh private archive data. The previous Go
archive and installation remain for the original binary; the Node owner refuses
their different schema instead of converting or guessing at receipts.

Build and install the complete package. Reinstall and
select the new package digest when its declaration or component changes.

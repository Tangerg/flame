# Usage

The optional workspace component reads `usage.summary` through the public component
SDK's explicit Runtime client. Runtime remains the sole accounting owner. All-time,
seven-day and thirty-day ranges are query inputs; grouping/filtering are local React
state. Recorded zeros, absent prices and empty reports retain distinct meanings.
The report range precedes local source filtering and grouping. Refresh and failed
range changes keep the accepted period visibly identified in that persistent scope
zone, so the requested range cannot mislabel retained totals.

Changing a range cancels the previous request. Refresh or range-read failure retains
the last accepted snapshot and labels its actual range. Component retirement cancels
and joins queries; it does not cancel independently accepted Runs or erase accounting.
The component has no backend process or own callable contribution. Shared React, JSX
and UI controls come from the actual workbench graph. There is no HTML guest or
view-specific usage query.

Build icons and UI, install the portable toolchain, then run from the repository root:

```sh
npm run build --prefix plugins
npm run check:resources --prefix plugins
```

Edit `src/component.tsx` and rebuild the tracked prebuilt ESM. Installation admits
exact release resources without evaluating code. Install/enable the package and
explicitly select an updated release; an evaluated replacement requires window reload.

Reports fold retained physical model attempts once, including auxiliary, active,
Session-only and unattached calls. Run totals are not added again. Coverage counts
distinguish reported usage/prices from missing evidence and expose started/unknown
outcomes. Known price subtotal may be incomplete; total cost is exact only when
every retained attempt is priced. Grouping by provider, served model, admission UTC
day, calling origin or Session/Run association reconciles with the same sources.
Deleted or never recorded calls cannot be reconstructed from an empty report.

The repository's `plugins/build-author-kit.mjs` exports this complete feature with
packed public dependencies and a standalone locked toolchain. It typechecks and
builds outside the checkout and requires byte-identical release resources without
loading the UI. The resulting `usage` directory uses ordinary installation from
any origin; no publisher or source-path privilege is part of its declaration.

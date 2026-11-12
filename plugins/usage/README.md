# Usage

The optional workspace component reads `usage.summary` through the public component
SDK's explicit Runtime client. Runtime remains the sole accounting owner. All-time,
seven-day and thirty-day ranges are query inputs; grouping/filtering are local React
state. Recorded zeros, absent prices and empty reports retain distinct meanings.
Costs keep two to six decimal places; a nonzero cost below 0.000001 instead shows
up to six significant digits so it never reads as zero.
Attempt and token copy selects its locale plural form from the original count;
the displayed counts are unchanged.
The persistent report scope holds Period, Refresh and read feedback. Overview and
totals precede the breakdown, where local source filtering and grouping apply to
its rows. Those controls remain available for an accepted empty report and retain
their values through refresh. Refresh and failed range changes keep the accepted
period visibly identified in the persistent scope zone, so the requested range
cannot mislabel retained totals.

Changing a range cancels the previous request. Refresh or range-read failure retains
the last accepted snapshot and labels its actual range. Component retirement cancels
and joins queries; it does not cancel independently accepted Runs or erase accounting.
Display-language changes preserve the original pending read, accepted range and
totals, requested range, grouping, filter, expanded details and later focus. They
neither reread nor retry a failed requested range. Replacing the Runtime, tracking
function or component signal still cancels the previous read. Failures retain their
raw cause; literal details stay exact while fallback guidance uses the current language.
The component has no backend process or own callable contribution. Shared React, JSX
and UI controls come from the actual workbench graph. There is no HTML guest or
view-specific usage query.

Build icons and UI, install the portable toolchain, then run from the repository root:

```sh
npm run build --prefix plugins
npm run check:resources --prefix plugins
```

Edit `component/component.tsx` and rebuild the tracked prebuilt ESM. Installation admits
exact release resources without evaluating code. Install/enable the package and
explicitly select an updated release; an evaluated replacement requires window reload.

Reports fold retained physical model attempts once, including auxiliary, active,
Session-only and unattached calls. Run totals are not added again. Coverage counts
distinguish reported usage/prices from missing evidence and expose started/unknown
outcomes. Known price subtotal may be incomplete; total cost is exact only when
every retained attempt is priced. Grouping by provider, served model, admission UTC
day, calling origin or Session/Run association reconciles with the same sources.
Deleted or never recorded calls cannot be reconstructed from an empty report.

Only within calling-origin and association grouping, the known `human`, `model`,
`automatic`, `run`, `session` and `unattached` keys display localized labels;
`session` means a Session without a Run. Filtering matches both that label and
the original key. Other keys, including provider names that resemble them, stay
raw. Original keys, accounting and the read lifecycle remain authoritative.

The repository's `plugins/scripts/build-author-kit.mjs` exports this complete feature with
packed public dependencies and a standalone locked toolchain. It typechecks and
builds outside the checkout and requires byte-identical release resources without
loading the UI. Its built release uses ordinary installation from any origin;
no publisher or source-path privilege is part of its declaration.

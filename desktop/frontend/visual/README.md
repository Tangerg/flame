# Browser fixtures

This is a test-only Vite entry for isolated browser interaction checks and manual design review. It is intentionally separate from the production router and Wails
bootstrap.

Rules:

- import production components, styles, selectors, projections, and view models;
- freeze only test inputs such as viewport, locale, clock, theme, and canonical
  protocol snapshots;
- never add a production debug route, fixture-only business branch, or parallel
  presentation state model;
- future Agent fixtures must start from `AgentSessionSnapshot` or `RunEvent` and
  use the production fold/projection before seeding a test adapter;
- keep fixtures inert unless an interaction is the subject of the test.

The `foundation` fixture freezes geometry and primitive roles. The `shell`
fixture installs test-only data providers, then renders the production sidebar
plugins, application projections, workspace navigation port, and shell
primitives for populated, empty, loading, error, collapsed, resized, and Retina
states. The `agent` fixture is backed by canonical `AgentSessionSnapshot`
values and the production `projectAgentSessionSnapshot` fold; its state selector
covers empty, idle, Running, Waiting/HITL, terminal, error, delegated-tree, and
long-content cases. The `workspace` fixture starts from the canonical Agent
snapshot installer, registers the real workspace views and Settings pane
plugins, and supplies deterministic workspace data providers. Runtime-backed
Settings reads use the production adapters over an in-memory transport with
canonical protocol snapshots; they never register competing providers. Unmodelled
Runtime operations fail immediately, and the client owns transport and subscription
cleanup. It covers dock width and resizing, navigation identity,
diff loading/empty/error states, and the
production Settings surface without a parallel presentation model.

Portable workspace views use the production installed-component lifecycle and public
client over canonical, validated fixture reads. Session trajectory rereads the same
snapshot that the scripted Runtime response advances; fixture view models never write
an independent approval decision. Components render in the shared React host.

`*.browser.spec.ts` verifies observable interactions, keyboard and focus behavior,
accessibility, readable content, overflow and touch target geometry. The WebKit
smoke suite covers the engine closest to macOS WKWebView. Pixel comparisons,
golden-only scenarios, cosmetic token matrices, baseline images and snapshot-update
commands are removed.
Design review uses the actual rendered pages in both themes after a batch of
changes; passing these behavioral checks does not establish visual quality.

Run `npm run browser:dev` for manual inspection and `npm run browser:test` for
browser behavior checks. Run focused scenarios for the changed behavior rather
than repeating the complete matrix for each spacing or typography adjustment.

Useful routes:

- `/visual/?theme=light&sidebar=expanded`
- `/visual/?fixture=shell&theme=light&state=populated&sidebar=expanded`
- `/visual/?fixture=shell&theme=dark&state=error`
- `/visual/?fixture=agent&theme=dark&state=waiting`
- `/visual/?fixture=workspace&theme=light&state=dock-light`
- `/visual/?fixture=workspace&theme=dark&state=dock-review`
- `/visual/?fixture=workspace&theme=light&state=dock-loading`
- `/visual/?fixture=workspace&theme=dark&state=settings`
- `/visual/?fixture=agent&theme=light&state=long-content&font-size=18`
- `/visual/?fixture=shell&theme=light&state=populated&motion=full`

## Interaction baseline

`npm run perf:baseline` builds this entry for production, serves it with
`vite preview`, and runs `*.perf.spec.ts` in one Chromium worker with motion on.
Each scenario is repeated `FLAME_PERF_SAMPLES` times (default 5) on a fresh
navigation and reported as p50/p90/max, both on the console and as JSON under
`desktop/.cache/perf/`. Compare runs only from the same machine and build.

Measured per scenario: wall time, Chromium task/script/layout/style time from
`Performance.getMetrics`, long tasks, the slowest input-to-next-paint from Event
Timing, and JS heap. Loads report time from navigation start instead, because
engine counters do not survive a navigation.

Scenarios use the existing fixtures: a 200-file review
(`review-files=200` on the `dock-review` state) opened, folded, unfolded,
scrolled and resized; and the `long-content` transcript loaded and typed into.
Switching between long sessions is not covered — the fixtures render one
session.

Recorded 2026-09-24 on an Apple M4 (24 GB), Chromium from Playwright 1.63, five
samples. "Before" is the review without `content-visibility` on its file bodies;
"after" is the shipped code. p50 (p90):

| Scenario | Metric | Before | After |
| --- | --- | --- | --- |
| Review open | ready | 9014 ms (9704) | 1616 ms (1700) |
| Review collapse all | input to paint | 3320 ms (3848) | 232 ms (248) |
| Review expand all | input to paint | 1608 ms (1736) | 736 ms (848) |
| Review resize ×10 | wall | 76252 ms (112226) | 1664 ms (1914) |
| Review scroll ×40 | input to paint | 40 ms (72) | 32 ms (192) |
| Long transcript open | ready | 874 ms (1232) | 874 ms (976) |
| Long transcript typing | input to paint | 24 ms (80) | 32 ms (56) |

Scrolling the review now lays out file bodies as they enter the viewport, which
is the p90 cost above; everything that previously restyled all 200 bodies no
longer does. Expand all is still dominated by highlighting every row.

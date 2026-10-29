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

Hand-authored Tool invocations include the category supplied by Runtime delivery,
including approval payloads and streamed items. The fixture category helper mirrors
that presenter; it does not register or classify production Tools. Check resulting
file bodies, command output and change receipts through the actual Agent fold so
missing metadata cannot silently turn a specialized reading scene into generic JSON.

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

The workspace mounts the installed `app.main` contribution, so Composer focus
commands address the panel on screen, including when Settings hides it. Its
recorded default-Session port consumes the snapshot installer's existing actions;
mounting the workspace does not start live Run recovery or register a competing
driver. This fixture qualifies presentation and command routing, not a live
Runtime execution lifecycle. The smaller Agent fixture keeps its explicit,
inert shell controls and does not qualify sidebar resizing or collapse.

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
`npm run check --workspace @flame/web` runs the frontend and isolated Runtime
integration checks; `browser:test` is a separate browser gate. Qualify affected
interactions with `--browser chromium` and `--browser webkit`.

Code wrapping follows CodeCaption's next-action name and the shared `data-wrap`
preference. Schedules editors and deletion decisions are portaled, named `dialog`
and `alertdialog` surfaces. Background controls remain outside modal interaction.
The visual Runtime transport owns controlled Schedule removals and publishes the
real `plugins.dataChanged` notice; draft tests use that update to retire an opener
while its modal remains open, then check cancellation and fallback focus.

The `genui-report` Agent state loads the built GenUI package ESM and declared
Tool selector through the installed captured-result components. Its accepted
document combines a local scenario form, reactive calculations, conditional
content, a searchable and selectable table, charts and accessible media views.
Build plugin
resources first, then run `npm run browser:genui --workspace @flame/web`
from the repository root. This focused
suite reuses the production fixture build and preview, checks editing, validation,
reset, keyboard controls, overflow and both themes at narrow and wide widths,
and attaches screenshots for manual inspection. The sandbox cases exercise local
program execution while checking that scripts cannot access the parent or send
external requests. These fixtures do not contact a Runtime or load the OpenUI
development inspector; Runtime operation and conversation-input tests own those
execution contracts.

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
`out/web/perf/`. Compare runs only from the same machine and build.

Measured per scenario: wall time, Chromium task/script/layout/style time from
`Performance.getMetrics`, long tasks, the slowest input-to-next-paint from Event
Timing, and JS heap. Loads report time from navigation start instead, because
engine counters do not survive a navigation.

Scenarios use the existing fixtures: a 200-file review
(`review-files=200` on the `dock-review` state) opened, entered one file,
returned to its list, scrolled and resized; and the `long-content` transcript loaded and typed into.
Switching between long sessions is not covered — the fixtures render one
session.

Historical measurements below describe the former all-files-expanded reader,
not the current overview/detail navigation. No new performance baseline was run
for that interaction change.

Recorded 2026-09-24 on an Apple M4 (24 GB), Chromium from Playwright 1.63, five
samples. "Before" is the review without `content-visibility` on its file bodies;
"after" is the shipped code. p50 (p90):

| Scenario               | Metric         | Before            | After          |
| ---------------------- | -------------- | ----------------- | -------------- |
| Review open            | ready          | 9014 ms (9704)    | 1616 ms (1700) |
| Review collapse all    | input to paint | 3320 ms (3848)    | 232 ms (248)   |
| Review expand all      | input to paint | 1608 ms (1736)    | 736 ms (848)   |
| Review resize ×10      | wall           | 76252 ms (112226) | 1664 ms (1914) |
| Review scroll ×40      | input to paint | 40 ms (72)        | 32 ms (192)    |
| Long transcript open   | ready          | 874 ms (1232)     | 874 ms (976)   |
| Long transcript typing | input to paint | 24 ms (80)        | 32 ms (56)     |

At that checkpoint, scrolling laid out file bodies as they entered the viewport,
and expanding all files still highlighted every row. The current reader mounts
one selected file; those historical timings do not qualify its navigation cost.

# Component lab

The lab is the sole UI visual verification entry. It
renders production components, with two searchable catalogs: shared foundations
and product components. Product scenarios include the migrated foundation layout, Atoms, Agent, Workspace,
Settings, Sidebar, Computer and installed-composition fixtures. The production router does not import this entry.

## Run

Complete the repository's normal `make setup` prerequisites first. From the
repository root:

```sh
npm run lab --workspace @flame/web
```

Vite binds to `127.0.0.1:4174` and opens `/lab/`.

Direct entry points:

- `/lab/?panel=foundation`
- `/lab/?panel=business`
- `/lab/?story=text-field&variant=styles&mode=inspect`
- `/lab/?story=checkbox&variant=editable&mode=inspect&compare=on`
- `/lab/?panel=business&story=business-agent-scenarios&variant=waiting&mode=inspect`
- `/lab/?panel=business&story=business-reasoning&variant=running&mode=inspect`
- `/lab/?panel=business&story=business-references&variant=long&mode=inspect`

The catalog owns the valid story and variant IDs. An invalid deep link displays
its error instead of silently opening a different example. Copy current link
captures the selected story, variant, viewport, theme, language, font size, motion and view
mode. Selection is URL state; the lab persists each panel's camera. The shared
appearance provider retains its existing storage behavior.

Shared UI and icons are consumed through their normal compiled public packages.
Vite updates application source and lab examples. After editing a shared package,
rebuild its normal output and reload the preview:

```sh
npm run build --workspace @flame/icons
npm run build --workspace @flame/ui
```

The icons build is necessary only when its source changed or its output is
missing. The lab does not alias private component source around package exports.

To build the same lab as a static preview:

```sh
npm run lab:build --workspace @flame/web
```

This writes `out/web/lab`. It does not replace the production Web bundle.

## Use

Gallery pages mount at most six interactive previews. Components use their real
type and control sizes in the available width; complete product scenarios use
thumbnails. The single-story inspector appears only in the focused view.
Search matches component
names, groups, scene IDs, descriptions and source paths. Selecting a component
opens its focused preview. The right panel selects a named comparison or recorded
scenario and links to the actual component source.

Focused previews support component, phone, tablet and desktop widths. Theme
comparison mounts two independent documents with the same scenario. Changing
the scenario, theme, language, font size or motion reloads that document; changing its
viewport preserves the current input and component instance. Reload example is
an explicit, separate action.

The camera supports background drag, background touch pan/pinch and
Ctrl/Command-wheel zoom around the pointer. Focus the background to use arrow
keys, `+`, `-` and `Home`. Fit and 100% centering remain reachable outside the
transformed content. Window and viewport changes fit the existing document;
the camera toolbar has its own reserved space below it. Camera operations never reset component drafts. Each panel
stores one bounded, versioned camera value; malformed or unavailable storage
does not prevent previewing.

The observation panel captures real DOM activations, explicit sample callbacks,
rendering errors and unhandled failures. It retains only the latest bounded
in-memory log. Text drafts report their length; credential-shaped fields are
redacted. Callback enums, booleans and numbers remain inspectable. Clearing the
log does not change the component. The log is an interaction aid, not a Runtime
trace, profiler or command receipt store.

## Ownership and isolation

`foundationCatalog.ts` and `businessCatalog.ts` are the composition inputs to one
validated `StoryCatalog`. Navigation, search, variant selectors, preview
URLs, verification cases and source links are projections of those definitions. The catalog copies
and freezes its descriptions at construction. Production components do not
import the lab.

Each component example owns only its local input, selection or disclosure state.
Its variant vocabulary is checked against the production public API. Examples
compare one dimension at a time instead of generating every props combination.
Examples do not pretend that a recorded failure has recovered after a Retry click.

The reasoning reader example mounts the production `ReasoningBlock` directly.
Streaming, completed, interrupted and short variants supply fixed, localized
public props through the existing providers and preview document. They isolate
disclosure, text hierarchy and reading continuity without adding a Runtime
transition or requiring a complete workspace. The streaming sample does not
generate tokens or simulate provider execution.

The reference reading example mounts the production `MarkdownRenderer` with
fixed localized Markdown. Short and long notes repeat a non-Latin reference
label, preserving the renderer's native fragment destinations and distinct
return links. The same catalog capture and interaction suites check supporting
text, link cues and keyboard return without a workspace sidebar or another
navigation implementation.

`environment.tsx` composes Flame's actual appearance plugins, locale packages,
Theme, FontScale, Tooltip, Callout and code highlighting providers. It does not
define another palette or translated production dictionary. All lab examples and migrated fixtures consume the shared stylesheet and provider contracts.
Gallery and standalone-document resets are scoped by their HTML document identity;
the product fixture retains the production fixed, clipped application root.

`fixture.ts` observes and imports `scenarios/fixtureRuntime.tsx`, the migrated
protocol-fixture owner. The composition scenario shares that same root and feature contexts. It never rebuilds a
Session, Run, Goal or installed-component state machine. Fixture variants derive
from the fixture-owned `VISUAL_*` arrays, and their existing in-memory transport rejects
unmodelled Runtime operations. Clipboard, download and other browser interactions
retain their existing explicit behavior.

An iframe provides a separate document, module graph, portal root and focus
lifetime. It is not a security sandbox. The lab runs trusted repository examples;
it is not an editor for arbitrary HTML or JavaScript. Native component portals
stay in their preview document, so transforming the entire document keeps their
coordinates aligned without patching component DOM or browser APIs. Open preview
offers an ordinary untransformed document when native-size inspection is useful.

The observation bridge only sends bounded messages to its same-origin parent.
The parent binds each message to the exact active iframe, origin and instance ID
before accepting it. There is no inbound command channel. Listener, pointer,
animation-frame, observer and preview lifetimes retire with their owner.

Recorded business fixtures use one advancing calendar: `Date.now()`, `new Date()`
and `Date()` start from `VISUAL_NOW` and advance with elapsed `performance.now()`.
Explicit date arguments, parsing and native Date instances retain their normal
behavior. The fixture restores its captured Date constructor when its root
retires, including HMR. Acceptance times can still differ with interaction timing;
this contract does not make temporal text pixel-identical across captures.

The lab has no virtual animation clock. Timers, `performance.now()` and animation
frames remain native. Normal/reduced motion uses the production motion preference
and the platform's reduced-motion setting.

## Extend

Add a typed `ComponentStory` to the relevant catalog and a colocated example
component. A definition supplies its identity, localized description, group,
source path, natural viewport, nonempty variants and lazy loader. The loader
returns a component accepting `LabExampleProps`; use its `observe` callback for
meaningful public interactions. Free-text inputs must report length rather than
duplicating the draft in the inspector.

Keep samples cohesive by the component family they exercise. Reuse production
components directly. Do not add a wrapper, production prop or compatibility
branch merely to make a sample possible. Register complete business scenarios
with `FixtureStory` targets from the existing fixture vocabulary.

After changing a story, verify its real interactions, both themes, a narrow
viewport and any native popover/modal focus path. Public components may offer
more combinations than the named examples; these examples are purposeful
comparisons, not a claim of exhaustive combinatorial coverage.

## Verify

Run visual cases and migrated interaction coverage through one configuration:

```sh
npm run lab:verify --workspace @flame/web
npm run lab:verify --workspace @flame/web -- --grep text-field
npm run lab:verify --workspace @flame/web -- lab.browser.spec.ts
npm run lab:verify --workspace @flame/web -- scenarios/genui
npm run lab:verify --workspace @flame/web -- scenarios/computerConsole
```

`playwright.lab.config.ts` owns all browser verification. Its `light-en` and
`dark-zh` projects open built, independent preview documents at `127.0.0.1:4175`,
without gallery chrome or camera transforms. Each test projects every named
variant directly from the catalog, checks render readiness, extracted StyleX
rules, console errors, image decoding and horizontal overflow, then attaches a
full-page screenshot at native scale. Popover and dialog cases also open their
real controls, wait for their visible settled state and capture the overlay.
Captures finish finite browser animations; motion behavior has its own interaction
checks and native qualification. English/light/14 px and Chinese/dark/18 px
cover catalog widths and 390 px component widths. Complete layouts retain their
declared desktop dimensions.

Reasoning variants capture both the closed preview and the opened reader. Closing
must clip the retained body and exclude it from keyboard focus. The same Lab's
interaction tests check stable toggle names, readable text, keyboard reading and
return, both directly and within the production Agent fixture.

The `interactions` project retains all migrated `*.browser.spec.ts` tests,
including the WebKit suites. It uses the same lab's development server at
`127.0.0.1:4174` because scripted protocol transitions import fixture modules.
GenUI sandbox and Computer console tests use their existing isolated transports.
Both servers use `vite.lab.config.ts`, the same catalog, documents, providers and
fixture owners. There is no separate visual app, browser configuration or
compatibility route. Stop a manually running lab before starting verification.

`FLAME_LAB_STORY` can select one catalog story for the capture projects; unknown
IDs fail. Use file arguments or `--grep` to select interaction coverage. The normal
Web check runs colocated unit tests; it does not launch browser verification.

When a migrated check fails, trace the current component contract before changing
its expectation. Scope retained Dock content to its named visible region, measure
overflow at the actual reading viewport and verify literal captured results rather
than assuming a particular presentation. Keep keyboard entry, return focus and
draft continuity assertions. Add regression coverage to the existing scenario;
do not introduce another fixture application or verification configuration.

Focus checks capture the containing Search proxy or Composer surface when that
surface owns the keyboard cue. They compare its focused and blurred paint as well
as pixels, with carets hidden and screenshot animations disabled. Disclosure
alignment checks follow each toggle's own labelled region and summary text edge;
they do not infer the reading axis from an inner node's margin. Both checks cover
English/light/14 px and Chinese/dark/18 px.

Facts checks follow the shared equal-column contract, including common column
edges, source reading order and complete values inside each pair. They attach
geometry and reveal the long attribute path before capturing it. Browser
intersection rectangles use the same one-pixel tolerance as the other geometry
checks, so subpixel rounding does not require an exact intersection ratio of one.
Focus-rule checks inspect declared `outline` properties; a transition mentioning
`outline-color` does not paint a perimeter.

Transcript tail checks distinguish the retained, accessibility-hidden jump
control at the bottom from its visible, keyboard-reachable state while reading
earlier content. Import feedback checks exercise invalid server identity and
missing transport separately, preserving protocol validation order, drafts,
accessible errors, repeated attempts and focus return across both themes and
three viewport sizes.

Appearance interaction checks follow current theme, font and language values
through keyboard selection and focus return. Work-index and Brand audits use
their production rows and installed component resources. WCAG failures attach
the rule results and complete rendered button text/HTML to the same report,
preserving evidence when the audit's summarized markup is truncated.

Goal and Plan reading checks use the existing Agent and Workspace catalog
variants. The Workspace's unavailable captured Goal author permits reading;
the registered Agent author permits editing. Checks preserve that distinction,
cancelled edits, keyboard entry/return and each document's independent Composer
draft through narrow resizing. Captures wait for the whole reading surface to
be in view and fully opaque.

Installed-report checks also exercise Chinese selection through an empty search
in both themes at narrow and desktop widths. They capture the actual heading's
text rectangles and table before asserting that the short selection label stays
on one line. Clearing restores search focus and the selected row while retaining
the independent Composer draft. English authored headings keep their existing
whole-word and wide-table reading checks in the same scenario suite.
The same report supplies horizontal focus regressions for Chromium and WebKit.
Keyboard entry and narrowing must reveal the complete final sorting control;
manual horizontal reading and later Composer focus must retain their positions
through resizing. These checks use the existing Lab fixture, shared console
checks and browser configuration, without another visual runner or test app.

Largest-font transcript audits open inline disclosures within the transcript,
including newly mounted arguments and full output. Each click retains the
physical trigger while the remaining control list changes, then verifies its
expanded state and any controlled panel. The same report records every opened
control and captures both ends of the transcript. Session and Composer menus
are outside this expansion path.

Computer outcome checks reveal the original operation text, then the recorded
image figure and caption, asserting complete viewport visibility before capturing
the actual page viewport. A screenshot of a long panel clipped by its enclosing
ScrollArea is not evidence that the outcome is visible.

The HTML report is `out/web/lab-report/index.html`; screenshots and a `cases.json`
attachment record exact inputs and reproducible preview URLs. The verifier waits
for the production fixture's ready signal and never qualifies an errored document
as ready. There are no pixel goldens to approve automatically: inspect screenshots
for alignment, hierarchy, clipping and states. A passing automated check establishes
the listed rendering contracts, not complete visual acceptance or live Runtime
behavior. Native focus, gestures and zoom still need actual Wails qualification.

The current qualification results and unresolved migrated-suite failures are
recorded in [`UI_PROGRESS.md`](../UI_PROGRESS.md#component-lab-and-ui-visual-verification-2026-11-06).

```sh
npm run typecheck --workspace @flame/web
npm run test --workspace @flame/web -- lab
npm run lab:verify --workspace @flame/web -- lab.browser.spec.ts
npm run lab:build --workspace @flame/web
npm run check --workspace @flame/web
```

Install the browser matching the repository's Playwright version before running
browser tests. Browser tests use the lab server, real components
and its console/error guard. The normal Web check does not replace native Wails
verification on macOS.

## Fixture contracts

The lab is outside the production router and Wails bootstrap. Import production
components, styles, selectors and projections. Freeze test inputs such as viewport,
locale, clock, theme and canonical protocol snapshots. Do not add a production
debug route, fixture-only business branch or parallel presentation state model.
Agent scenarios start from `AgentSessionSnapshot` or `RunEvent` and use the
production fold before seeding a test adapter. Keep fixtures inert unless the
interaction is the subject of the test.

`scenarios/client.tsx` composes explicit feature instances. Runtime-backed reads
use production adapters over an in-memory transport; unmodelled operations fail
immediately. The workspace mounts the installed `app.main` contribution, so focus
commands address the on-screen Composer even when Settings hides it. Its recorded
Session port uses the snapshot installer's actions and does not start live Run
recovery. These scenarios qualify presentation and command routing, not live
Runtime execution.

Workspace scenarios mount the production working-tree action through its host
adapter in the conversation header. Its change counts and Diff reads project the
same captured report. A file-specific Diff entry requests that path; the Files
action explicitly reads the workspace overview before list tasks. Split-column
checks retain source identities and before/after order across paired and unpaired
changes instead of assuming one displayed row per source row.

The `dock-schedules-models` variant joins the same Workspace catalog and captures
its long model list before constructing the Runtime transport. The real Schedule
editor reads and refreshes that capture; tests do not mutate a retired seed after
the transport has cloned it. Its named Model popup retains keyboard selection,
scrolling and focus independently of the editor body. The fixture notifier forwards
the requested notification level through the production notification owner.

Tool invocations retain Runtime delivery categories, approvals and streamed
items. Recorded Web pages in `scenarios/web-pages.json` are offline captures;
offsets, limits and counts are Unicode codepoints. The lab never fetches those
pages. The `genui-report` state loads the built GenUI ESM and declared Tool selector
through the real installed captured-result lifecycle. Sandbox tests verify local
program execution cannot access the parent or make external requests. The
Computer fixture retains its isolated recorded surfaces and in-memory transport.
Its existing story exposes control, recorded images, unreadable image/audio,
embedded resources and resource links as typed named variants. The images variant
seeds landscape, portrait and small original records; expand a record and activate
its thumbnail to inspect the shared image reader. Canvas-authored synthetic PNGs
replace the old 1-by-1 carrier without capturing a desktop or fetching assets.
These variants use the same `state` links projected by the catalog.

Computer capacity remains unqualified: the fixture's zero usage does not account
for its recorded operations. The backend owns byte accounting, including command
metadata, pending result reservations and lifecycle commands; the visual records
do not retain all those facts. Do not derive substitute totals from image sizes
or displayed history, or treat these variants as capacity coverage.

Agent file suggestions select literal, case-insensitive path fragments from the
same five captured files and honor the requested limit. This keeps attachment
inspection faithful to its inputs; it does not qualify Runtime's ranked search.

Code wrapping uses the shared `data-wrap` preference. Schedule dialogs and
alertdialogs remain portaled and named, with background controls outside modal
interaction. The fixture transport owns controlled Schedule removal and sends
`plugins.dataChanged`; draft tests retire the opener and check cancellation and
fallback focus.

Direct fixture links are `/lab/fixture.html?fixture=workspace&state=dock-review`
and the other catalog-projected targets. Additional query parameters support
existing behavior probes, such as `review-files=200`, `sidebar=collapsed`, custom
accent colors and recorded long results. Motion defaults to `off`; only `off` and
`full` are accepted. New UI visual scenarios belong in this catalog and their
checks belong under `lab/`.

Agent reading scenes also honor `sidebar=collapsed`, leaving their full reading
frame available while viewport changes preserve local drafts. Recorded
state-sidebar navigation and resize handlers remain inert; the query selects the
initial fixture frame rather than simulating a Runtime transition.

## Optional measurements

The same Playwright configuration owns the `performance` project:

```sh
npm run lab:measure --workspace @flame/web
```

It runs `scenarios/*.perf.spec.ts` against the built lab in one Chromium worker,
with normal motion. `FLAME_PERF_SAMPLES` defaults to five. Results report
p50/p90/max and JSON under `out/web/lab/perf/`; compare runs on the same machine
and build. Existing review and long-transcript probes retain their measurements
of wall time, engine task/script/layout/style time, long tasks, Event Timing and
heap. They measure one Session, not switching between long Sessions.

The installed-composition story migrates the previous standalone benchmark's
one/eight/thirty-two-frame scenes into `fixture.html`. Those variants share
`COMPOSITION_SAMPLES` and the same fixture root, appearance, feature contexts and
teardown as the other product scenarios. Recorded Frame control messages drive
the real browser Realm, module evaluation, composition and close-proof code.
Only that document's fixed control endpoint is intercepted; the transport retires
after the module owner closes. This does not qualify a live Runtime authority.
The former unconsumed console timing API, separate HTML and Vite configuration
are removed. Timing is not a visual acceptance criterion.

## Design references

- [Cards](https://cards.uidesigncourse.com/) and its
  [source](https://github.com/oliur/cards): a navigable component gallery and
  consistent design-system presentation.
- [Component Canvas](https://www.ui-skills.com/skills/ibelick/component-canvas):
  real component behavior, single-dimension comparisons, bounded camera input
  and preserving native preview interactions.
- [Stageshow](https://github.com/benjitaylor/skills/tree/main/stageshow): focused
  scene inspection, document isolation and explicit reload controls. Its faux
  product UI and virtual-clock implementation are not used here.

The implementation is authored for Flame using its existing dependencies. No
third-party component collection, reference-project assets or skill runtime is
vendored.

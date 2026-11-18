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
Installed component fixtures also load their published ESM. After changing a
portable component, regenerate its installed resource through the normal
publisher build before qualification; editing its source alone does not update
what the fixture loads.

To build the same lab as a static preview:

```sh
npm run lab:build --workspace @flame/web
```

This writes `out/web/lab`. It does not replace the production Web bundle.

Native visual qualification uses this same Lab over loopback HTTP in the current
Wails host. Its observation bridge accepts HTTP/HTTPS origins only; embedding a
Lab document under `wails://` is unsupported. Use the development URL above, or
serve the unmodified `out/web/lab` build through a loopback HTTP server for static
qualification.

Use a disposable host with the selected Lab URL and isolated application identity
and data. Retain the production host, bindings and native zoom behavior. Qualify
production startup separately with its original `/` entry and normal staged Web
assets. A working HTTP Lab does not establish embedded product startup or live
Runtime execution. Keep all visual scenarios and checks in this Lab.

## Use

Gallery pages mount at most six interactive previews. Components use their real
type and control sizes in the available width; complete product scenarios use
thumbnails. The single-story inspector appears only in the focused view.
Search matches component
names, groups, scene IDs, descriptions and source paths. Selecting a component
opens its focused preview. The right panel selects a named comparison or recorded
scenario and links to the actual component source.

Paging and filtering start at the first result. Entering the inspector retains
one gallery reading position for the current catalog, query and page; changing
that range retires it, including while the gallery is unmounted. Returning through
the view control restores that position and keeps the control's focus. Paging and
retired card actions hand focus to the current, named main region; persistent
search, catalog and variant controls retain their own focus.

The clipped reading region uses the shared inset focus rule and reserves a
transparent frame before its preview content. Keyboard focus stays visible at
the viewport and gallery scroll edges. Independent preview documents retain
their own focus treatment.

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
The same 1–200% scale range applies to fitting, gestures and stored cameras.
Fit can use less than 20% to retain both complete product boards and the existing
inset in a narrow or short canvas. Existing valid stored poses remain valid.

The observation panel captures real DOM activations, explicit sample callbacks,
rendering errors and unhandled failures. It retains only the latest bounded
in-memory log. Text drafts report their length; credential-shaped fields are
redacted. Callback enums, booleans and numbers remain inspectable. Clearing the
log retains the component's local state. The clear action stays focusable while
unavailable through the shared button contract, preserving its keyboard context.
The log is an interaction aid, not a Runtime trace, profiler or command receipt store.

Invalid links and failed independent documents retain a complete, wrapping cause
and a normal link back to the catalog. Recovery preserves the admitted language,
theme, font size and motion while dropping the invalid scene selection. Fatal
readers project the existing appearance tokens locally; they do not write theme
preferences or create another document painter. Raw fixtures retain their own
English default and canonical custom-color parsing.

## Ownership and isolation

`foundationCatalog.ts` and `businessCatalog.ts` are the composition inputs to one
validated `StoryCatalog`. Navigation, search, variant selectors, preview
URLs and source links are projections of those definitions. The catalog copies
and freezes its descriptions at construction. Production components do not
import the lab.

Workspace scenario titles have one localized owner in
`scenarios/workspaceFixtureStates.ts`. Catalog search, the variant picker and
preview state rows project those same names and the existing state order.
English and Chinese use readable titles while reproducible links retain the
canonical scene IDs. Structural fixture controls keep their existing names.

Installed contribution names have a separate author: each package's public
`declaration.json` component entry. `installVisualWorkspaceFixture.ts` selects
that entry by component ID and forwards its fallback title and `localizedTitles`
to the existing Workspace and Settings contributions. Settings also retain the
authored fallback title and ID as search keywords. The normal locale projection
chooses the visible name; the Lab does not maintain a second translation map.
Fixture IDs, scopes and captured transport bindings remain fixture-owned. Icon
gallery is a full-view contribution and is not added to the Dock picker.

Workspace installs the production conversation Find plugin and its shortcuts.
Use a conversation with an unsent draft to inspect matches, focus return and
continued editing. Use the native Wails host to inspect macOS composition.
[Native UI evidence](../UI_PROGRESS.md#conversation-find-and-reading-return)

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

The attachment example mounts the production `ComposerAttachments` in its normal
Composer surface with a local draft. Fixed pending, failed and file-reference
props make narrow rendering and removal inspectable without racing a file read.
They never advance or retry an image read. The existing Composer image-intake
scenario checks delayed real FileReader completion, removal and draft retention.
The existing Workspace Composer also qualifies first paste into an empty input,
review, cancel, save, restoration and continued editing in English and Chinese
through Chromium and WebKit. At the small-window frame, attachment and action
bounds are checked against fractional clipping edges. The image-intake scenario
checks a complete focused thumbnail, preview return, readable input and removal
through 560 × 364 and 373 × 243 frames in both themes and browser engines.
The paste task also switches between its complete review action and input in
the smaller frame before continuing review, restoration and editing. These
small-window tasks collapse the fixture's control sidebar before measuring the
production area. These scenarios use the production draft and attachment
owners rather than adding a second implementation to the fixed component story.

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

The Agent file list honors literal, case-insensitive path fragments and the
requested limit over its five captured records. File suggestions reserve room
for a basename that fits by itself, allowing the parent directory to ellipsize
first. The complete path remains the option's accessible name and title. Existing
closure scenarios exercise both Go matches at narrow widths, dismissal, retained
selection, keyboard acceptance and continued input in the original Composer.
Opening or resizing at short heights reveals the current selection; explicit
scrolling can still browse the remaining options without being pulled back.
This recorded list does not qualify Runtime search ranking. Files reading uses
the Workspace fixture and its separate captured file providers.

The same Agent catalog offers `file-list-error` and `file-list-loading`. The
former rejects every file-list request with one captured error; the latter holds
the provider read unsettled. Production query state renders the loading result.
An attempt witness only records real calls and never decides their outcome.
Each independent preview seeds `@store`
and enters through the public Composer focus delivery, including an already
positioned, blurred native caret. Production suggestion state owns loading,
failure, retry and dismissal. Closure checks cover short-height error reading,
the complete Retry label, keyboard and pointer recovery, preserved padded drafts,
and the real file/command toolbar entries with immediate continued input. This
comparison does not qualify live filesystem discovery or successful recovery.

The same closure scenario checks reverse Tab across ready, empty, loading,
failed and projectless file suggestions. It compares return with the native
preceding target captured before opening the popup, then continues the exact
padded draft in the original input. Real command-toolbar entry shares that
return contract and retains forward Tab completion. All cases use the registered
Agent comparisons and existing documents, providers and runner.

The recorded `apps/web/package.json` read imports the current Web manifest as raw
text, so its displayed path and contents share that source instead of returning
the generic code sample. These recorded responses do not qualify live filesystem
reads.

The Workspace variants `dock-file-error` and `dock-file-unsupported` supply fixed
read failures and the production `unsupported_mime` classification. The latter
records an image entry, not image bytes. Every preselected file uses the existing
viewer owner after resetting the document's file tree memory, retaining its path
and parent folders for return. Listed file names derive from their captured paths.
Retry preserves the recorded failure; other files keep their normal captured reads.

The same Workspace document accepts `directory-state=loading|error|empty|refresh-error`.
It controls only the captured workspace's nonrecursive `apps` listing. Root entries,
other workspaces and recursive reads keep their normal captured responses. This is
separate from the Agent fixture's Composer `file-list` parameter.

- `/lab/fixture.html?fixture=workspace&state=dock-files&agent-state=idle&directory-state=loading`
  holds the expanded directory read until its query or provider lifetime is aborted.
  Collapse cancels that read; reopening starts another pending read.
- `/lab/fixture.html?fixture=workspace&state=dock-files&agent-state=idle&directory-state=error`
  rejects the initial query, including its automatic retries. The real Files Retry
  action then receives the captured child and ordinary file navigation continues.
- `/lab/fixture.html?fixture=workspace&state=dock-files&agent-state=idle&directory-state=empty`
  returns an accepted empty directory without emptying the root list.
- `/lab/fixture.html?fixture=workspace&state=dock-files&agent-state=idle&directory-state=refresh-error`
  first accepts the child. The fixture sidebar's **Notify apps change** action sends
  a fixed `files.changed` notification through the existing Runtime subscription.
  Production invalidation triggers a failed refresh while retaining that child;
  the real Retry action recovers it. The fixture's state rows remain read-only.

Each provider installation owns its directory phase. Settled reads own acceptance
and failure; only the fixed Notify event arms a refresh failure. Normal subscription
initialization and read-target changes can refresh successfully. Old cache values,
manual writes and cancellation do not advance the read phase. Its observer and
notification listener retire with the provider. No fixture timer,
query-default override, cache write or alternate FileBrowser renders these states.
The refresh scene installs the production Workspace event plugin with the real
Workspace mutation lifecycle. Its captured resolver accepts only the fixture root.
The transport owns the notification sequence and stops delivery when a subscription
aborts or closes. The control simulates a captured external filesystem notification,
not live filesystem access.

Use these states to inspect keyboard Retry, file and source return, retained
children, loading cancellation and root navigation. Check Wails accessibility
with the native host.

The existing Workspace catalog also owns `dock-skills-catalog`: 80 recorded
documents alternate project and user scopes, with 40 matching `runtime`.
Discovery and document reads project the same array captured when that fixture's
transport is constructed. The original two-document `dock-skills` scene remains
available. This is fixed provider data rendered by the installed Skills component,
not live filesystem discovery or another Skills implementation.

The same Skills scene retains two personal-library records, one active and one
archived. The existing Skills acceptance spec follows search, keyboard
Archive/Restore, refusal feedback, narrow reading, catalog changes and exact
continued Composer drafts in Chromium and WebKit, English and Chinese. Fitting
record feedback must remain completely visible with the action; an oversized
record retains its action and is read with a wheel gesture derived from the
actual text bounds. These commands deliberately retain their unmodelled fixture
failure and never manufacture a lifecycle change. The component-state test also
checks one original mutation key through unknown acknowledgement, query changes
and reconciliation. Neither coverage path adds a catalog, fixture or runner.

Schedule deletion cancellation uses the existing `dock-schedules` records in
the same Workspace spec. Chromium/WebKit and English/Chinese tasks follow
expanded details, keyboard action-menu entry, confirmation resizing, Cancel and
Escape return, retained query/disclosure and exact continued Composer drafts.
Read-only geometry checks complete object and consequence text, complete footer
button bounds and returned action visibility at 560 × 364 and 373 × 243. The
flow never activates deletion; the existing component-state test verifies that
Cancel issues no delete command. This scope does not certify Send containment
under native page zoom or live Schedule mutation.

The Workspace variant `dock-diagnostics-recorded` retains three span statuses,
two metrics and two multiline logs, including their original IDs,
source paths and raw JSON. It clones the typed snapshot once into the existing
client telemetry buffer before mounting the normal component. Refresh reads that
buffer and Clear empties it; neither operation reseeds the recorded data. Opening
a new document restores the sample. The original `dock-diagnostics` remains the
empty-buffer scene. These are local presentation inputs, not live Runtime
telemetry or another Diagnostics implementation.

The existing `dock-usage` sample projects the same 24 recorded attempts into all
five accounting groupings. Provider and model buckets retain their original
values; two UTC days and two calling origins reuse those captured aggregates.
All attempts belong to Runs, preserving the 24 associated Runs and seven Sessions.
Every grouping reconciles token totals, known and total cost, and coverage. The
same fixed report is deliberately returned for every period: this scene qualifies
presentation and retained reading state, not Runtime date filtering or accounting.

The Workspace fixture-state sidebar uses the production `AgentRow` overflow
label and Tooltip for complete names by keyboard and pointer. Its existing
resize observer keeps the focused row inside the actual scrollport, falling
back to the selected row when focus is elsewhere without moving focus. The
observation switch remains outside that scroller; redundant explanatory footer
copy is removed. These captured state rows remain readouts, not navigation.

The runtime retains its normal stylesheet load order. A caught boot failure loads
the shared reader styles in its failure branch; preloading them before runtime
admission changes the normal fixture's cascade.

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

After changing a story, verify the affected business or distribution contract.
Use the existing visual comparisons for manual inspection when relevant.

## Verify

Inspect visual and interaction changes in the lab with the relevant fixture, theme,
locale and viewport. The preview reports readiness and displays the first error
from a failed import or render. The observation log retains document errors
and unhandled rejections.

The normal Web check builds the production bundle and the lab. The Runtime HTTP
suite verifies service behavior with local binaries and installed packages.

```sh
npm run lab:build --workspace @flame/web
npm test --workspace @flame/web -- --project runtime
```

Lab development prebundles the linked `@flame/icons/react` entry. If rebuilding that
package leaves the previous version cached, restart the lab with:

```sh
npm run lab --workspace @flame/web -- --force
```

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
immediately. Agent and Workspace mount the installed `app.main` contribution, so
focus commands address the on-screen Composer even when Settings hides it. Their
shared Agent installer registers the existing Settings, shortcut and Workspace
Escape plugins once. The recorded default-Session port uses the snapshot
installer's actions and does not start live Run recovery. These scenarios qualify
presentation and command routing, not live Runtime execution.

Workspace scenarios mount the production working-tree action through its host
adapter in the conversation header. Its change counts and Diff reads project the
same captured report. A file-specific Diff entry requests that path; the Files
action explicitly reads the workspace overview before list tasks. Split-column
checks retain source identities and before/after order across paired and unpaired
changes instead of assuming one displayed row per source row.

The Schedule fixture admits its authored multiline instructions and long path
before the transport copies the snapshot. Search still matches its workspace
content, refresh returns those same admitted bytes, and reading checks distinguish
soft wrapping from authored blank lines. Tests never mutate an exported seed to
change a transport that has already captured it.

The Memory fixture selects its recorded Project or User page from the explicit
`flame.memory/list.v2` input scope. The two original Project records remain
separate from the User record, including their IDs, revisions and source facts.
Missing, unknown or malformed scopes refuse rather than displaying another
target's page. Scope, edit/cancel and conversation-return checks use the same
Workspace fixture and installed Memory component.

The `dock-agent-memory-recorded` variant supplies longer Project and User pages
through that same captured transport input. English and Chinese paragraphs,
authored blank lines, an unbroken path and a scrollable list exercise the actual
Memory reader. Existing short pages remain unchanged. The named variant feeds
the catalog, state picker and independent captures without another story list.
These are fixed reading inputs; unmodelled mutations still refuse and
cancellation leaves the recorded content unchanged.

The `dock-usage-reports` Workspace variant mounts the installed Saved usage
reports component over its declared public SDK binding. A single fixed report
array supplies both archive headers and exact report reads. Its Daily copy keeps
an integer above JavaScript's safe range, a recorded zero subtotal and an absent
total price distinct; its Weekly copy preserves the existing reconciled Usage
sample. Refresh reads the same captured evidence. Missing or unknown IDs refuse,
and capture, removal and original-command recovery are not simulated. Reading
checks may open the production removal confirmation and cancel it; they do not
qualify an archive mutation or live accounting.

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
through the real installed captured-result lifecycle. The
Computer fixture retains its isolated recorded surfaces and in-memory transport.
Its existing story exposes control, recorded images, unreadable image/audio,
embedded resources and resource links as typed named variants. The images variant
seeds landscape, portrait and small original records; expand a record and activate
its thumbnail to inspect the shared image reader. Canvas-authored synthetic PNGs
replace the old 1-by-1 carrier without capturing a desktop or fetching assets.
These variants use the same `state` links projected by the catalog.

Computer capacity derives from the same retained originals as history and result
reads. Seeded evidence includes canonical input, a real SHA-256 digest and complete
invocation metadata. Open, execute, permission reply, stop and close retain their
own originals and outcomes. The Node-free backend evidence owner supplies the
same fields and pending-result reservation rule as its SQLite metadata query;
there is no separate running usage total. Settled results replace their reservation
with exact canonical outcome bytes, while an interrupted execution stays pending.
Original-command recovery cannot readmit or replay it. These isolated tasks do not
simulate capacity exhaustion, live engine control or backend persistence.

Agent file suggestions select literal, case-insensitive path fragments from the
same five captured files and honor the requested limit. This keeps attachment
inspection faithful to its inputs; it does not qualify Runtime's ranked search.

Code wrapping uses the shared `data-wrap` preference. Schedule dialogs and
alertdialogs remain portaled and named, with background controls outside modal
interaction. The fixture transport owns controlled Schedule removal and sends
`plugins.dataChanged`.

Direct fixture links are `/lab/fixture.html?fixture=workspace&state=dock-review`
and the other catalog-projected targets. Additional query parameters support
existing behavior probes, such as `review-files=200`, `sidebar=collapsed`, custom
accent colors and recorded long results. Motion defaults to `off`; only `off` and
`full` are accepted. Add visual scenarios to this catalog for manual inspection.

Agent reading scenes also honor `sidebar=collapsed`, leaving their full reading
frame available while viewport changes preserve local drafts. The query only
sets the initial workspace layout; the existing workspace sidebar actions may then
collapse, expand and resize it without replacing the current Composer. Recorded
state-sidebar rows remain inert rather than simulating a Runtime transition.

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

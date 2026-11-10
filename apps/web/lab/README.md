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
The inspector's camera regression checks complete bounds, independent drafts,
keyboard fitting and restored local camera coordinates in both browser engines.
Each capture reveals the complete camera and its toolbar, waits for two
animation frames, and captures those visible bounds.

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
URLs, verification cases and source links are projections of those definitions. The catalog copies
and freezes its descriptions at construction. Production components do not
import the lab.

Workspace scenario titles have one localized owner in
`scenarios/workspaceFixtureStates.ts`. Catalog search, the variant picker and
preview state rows project those same names and the existing state order.
English and Chinese use readable titles while reproducible links retain the
canonical scene IDs. Structural fixture controls keep their existing names.
Dock containment checks run independently for each registered state, retaining
the existing assertions and per-test deadline with a specific failure name.

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
at that same frame. These scenarios use the production draft and attachment
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
This recorded list does not qualify Runtime search ranking.

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

Lab development prebundles the linked `@flame/icons/react` entry. Otherwise each
isolated preview loads the icon barrel's hundreds of modules independently.
This uses [Vite's dependency prebundling](https://vite.dev/guide/dep-pre-bundling.html)
and retains the same package exports and shared runtime deduplication.
After rebuilding the linked icon package, restart Lab with
`npm run lab --workspace @flame/web -- --force` to refresh its optimized dependency.

`FLAME_LAB_STORY` can select one catalog story for the capture projects; unknown
IDs fail. Use file arguments or `--grep` to select interaction coverage. The normal
Web check runs colocated unit tests; it does not launch browser verification.

When a migrated check fails, trace the current component contract before changing
its expectation. Scope retained Dock content to its named visible region, measure
overflow at the actual reading viewport and verify literal captured results rather
than assuming a particular presentation. Keep keyboard entry, return focus and
draft continuity assertions. Add regression coverage to the existing scenario;
do not introduce another fixture application or verification configuration.

Reuse these fixtures in Wails to qualify native window, zoom and menu behavior.
When a native menu is open, confirm that the input method actually operates that
menu before attributing a key to the page. A captured menu tree alone does not
establish keyboard routing. Check exposed menu dismissal and page Escape
separately, retaining the exact draft and return focus across both.

The existing inline-reference case follows the Workspace task from README source
through Preview, a narrow viewport, Go to line and keyboard reading, then Escape
back to the original reference. It also checks the package source's identity,
return focus, transcript position at an unchanged viewport and continued Composer
draft. Captures accompany visible-row and scroll observations in that same case.

The existing Subagent header scenario follows completed task 3 of 4 through
short-window reflow, keyboard and pointer source inspection, list return and an
exact continued Composer draft. It checks complete source fragments against the
actual reading viewport and Jump affordance in WebKit, English and Chinese.
The named title reader's Home/End path retains status, Back and result position;
its focus stop also belongs to the existing navigation contract. Wails uses the
same scene for actual native zoom, wheel and window-change qualification.

File failure checks distinguish first entry into the tree from returning to a
previously focused row. They retain reading focus after Retry, reference return and
unsent drafts in both languages. Narrow ordinary material uses its existing full-width opener
and Close action; reference inspection retains its own return action. Captures
await the admitted business outcome after the normal query retry, rather than
treating document readiness as a completed file read.
The same header spec samples the unsupported first read in Chromium and WebKit,
including its pending frames. It checks that line navigation is never offered
without text and that the admitted header, controls, Back and Copy path bounds
remain stable across settlement. Full layout-shift rectangles are retained in the
existing layout-shift spec; neither check changes query retry or document readiness.

Diff failure checks follow the first rejected scoped read into a pending overview.
The surviving Refresh control receives focus while its filter is unavailable;
success continues to the selected row or filter. Later outside focus and deliberate
blur retire that continuation. The fixed failure scene retains its business error
and unsent draft through narrow full-width reading and conversation return.

Long Composer checks retain the same input, literal draft and native selection
through constrained reading, wheel input, resizing and material return. Visibility
requires the complete first or last line's em box inside every clipping ancestor
and above the footer. A positive text fragment or retained value alone cannot
admit a capture. Earlier wheel reading remains independent of the retained caret.

Long Skills catalog checks use the existing Workspace scenario in Chromium and
WebKit, with English/light/14 px and Chinese/dark/18 px profiles. They follow
wheel reading, search reset to the first match, middle-row document entry, scoped
instruction selection, complete source values, keyboard return, retained query
and scroll position, refresh and continued conversation drafts. Row and source
alignment measurements accompany captures; selection checks read the exact DOM
Range because WebKit's rendered selection can add a paragraph newline. The
Chinese task uses a 640 × 400 full-width material view. These frames qualify
reflow; native page zoom remains a separate Wails check. Catalog captures await
the accepted 80-document result as well as document readiness.

Middle-row entry uses a real wheel gesture derived from the current reading
geometry and waits for eight stable animation frames. Keyboard focus cannot
reveal the target before its visibility assertion. After return, eight further
frames must retain the saved offset, row focus and the complete row below the
controls and inside the viewport. Both observations join the same case evidence.

Related cases keep the same catalog through keyboard Dock collapse/reopen and
the narrow-window full-width opener. An unchanged viewport retains its exact
offset. After document return, twenty gradual viewport changes take the actual
1440 × 900 native frame to 720 × 450. The selected `runtime-review-035` must stay
completely visible below the controls while the Dock fits and after the ordinary
full-width opener. Reflow and the different density may expose an earlier row.
The retained search, scrollport and Composer nodes, query, result count, focus
destination and continued draft are checked together. Geometry and rendered
frames remain attached on failure. These cases use the existing Workspace spec
and catalog; native page zoom is qualified separately in Wails.

The existing Workspace sidebar offers `Observe layout`, off by default. It reads
window, Dock, viewport, Composer and focus geometry without changing the material
or its draft. Composer observations include control bounds and secondary-label
visibility. Named screen-reader outputs expose one snapshot in full and a compact
reading projection for native accessibility tools with bounded text nodes. The
Composer projection includes its scrollport's offset, viewport and content height.
The transcript projection exposes its safe reading edges, measured capacity,
scroll position and focused reader bounds and offsets in a separate bounded output.
Editable values never enter these outputs. Disabling the option clears them and
disconnects its observers and event listeners; clean qualification keeps it off.
The same ResizeObserver tracks the current Panel body and Composer, so filtering,
expansion and input reflow update their geometry inside an unchanged viewport.
The existing Workspace regression compares the reported and actual heights over
eight rendered frames, checks growth and shrinkage, and changes content after
disabling observation to verify that the outputs stay empty. Resizing also checks
the Composer projection while retaining the same material, query and exact draft.

The existing toolbar suite checks actual 320/280 px viewports and restoration to
1120 px in Chromium and WebKit, with English/light/14 px and Chinese/dark/18 px.
Idle and running scenarios retain an explicit Auto policy, complete control and
glyph bounds, pointer hit areas, an aligned end-action group, menu focus return
and the same draft node. Native page zoom remains a separate Wails inspection of
these same Lab documents.

Short-window toolbar checks use the same Agent scenario with its drawer closed,
a standing Goal and an explicit Auto policy. Chromium and WebKit retain the exact
draft, node and selection through 373 × 243 px reflow. The task follows complete
keyboard traversal, model and Goal editor cancellation, deliberate wheel reading
and restoration with continued editing. Each focused control must fit every
clipping ancestor and admit five pointer hit locations over eight consecutive
frames, with another check after capture. Wheel reading settles before keyboard
return. These checks exercise the production Composer scrollport; they add no
alternate fixture, runner or layout measurement authority.

The existing Workspace chrome suite checks Composer reading continuity through
two full-width Schedules visits in Chromium/WebKit, English/light/14 px and
Chinese/dark/18 px. The first starts at a keyboard-revealed footer control; the
second follows deliberate wheel reading. At an unchanged 373 × 243 px viewport,
eight return frames must preserve the saved native offset, exact padded draft,
selection and input/scrollport identities, with focus on the conversation.
Hidden content retains layout without exposing the conversation to interaction
or accessibility. Evidence includes the Plan, Goal, editor and footer geometry;
actual Wails page zoom qualifies the native host separately.

The same chrome suite checks running-header capacity at 320, 373, 480 and
1120 px with the supported maximum 18 px font, Chromium/WebKit and English/Chinese.
The title must retain visible space, while More, Diff and the Dock opener retain
complete, distinct targets. Actual Tab/Enter opens the existing Session menu to
read the complete status. Hover reveals the same value. Keyboard and pointer Diff
visits retire the old status tip, return to the conversation when the captured
opener has been replaced, and preserve the exact padded draft for continued input.
These cases extend the existing Workspace scene and verification configuration.

The material-header task opens the real Diff inspection in the Dock and then
full width, retaining its source. At 18 px, both engines and languages check
320, 373, 480 and 1120 px frames: the name remains visible, fits completely from
373 px, and every header action retains a distinct 24 px target. Keyboard and
pointer return tips preserve the full destination. Both returns retain the
literal padded Composer draft for continued input after enlargement.

Use `expectLabReady` from `readiness.ts` for document admission, including frames.
It waits for visible readiness or the observation owner's terminal failure and
reports the failing document URL and first bounded cause. Existing caller action
timeouts remain in force. Business alerts do not imply a failed document, and
internal render-phase measurements keep their separate purpose. Failure-reader
regressions check actual rendered colors and text contrast as well as language,
wrapping, pointer and keyboard recovery; a theme class alone cannot prove paint.
Fault-injection routes retire before catalog recovery so normal caching resumes.
Recovery checks await the actual preview documents before ending the test, rather
than accepting a heading while its examples are still loading.

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

Lab navigation checks measure the painted outline against the viewport and
clipping ancestors, including scrollports. They check the visible edges of an
oversized reading region and the complete first card action after keyboard
continuation and Escape dismissal. Child clearance must also reserve the inset
perimeter; an unclipped outline can still be covered by preview content. These
measurements accompany screenshot review rather than establish pixel acceptance.
Responsive preview checks read the iframe width and its current sizing attribute
together, preserving draft and horizontal-containment assertions while the
ResizeObserver settles.

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

Composer choice captures wait for the same short draft to fit after narrowing,
inside its surface and clear of its footer. The existing TextArea owns width
measurement; its ResizeObserver can leave one pending frame before sizing runs.
The same report retains geometry from the width change and the settled draft,
then rechecks complete text after model selection changes the toolbar.

Empty-input contracts stay in the existing UI ownership scenarios for Chromium
and WebKit. The input scene changes only the placeholder on the same uncontrolled
TextArea, then follows width, font, typed-value precedence and form reset. A stable
style prop keeps a hint-only update independent from other sizing triggers.
The actual Workspace Composer checks English and Chinese hints at 18 px before
and after narrowing, using native scroll geometry and the footer boundary rather
than a copy of the sizing implementation. Captures accompany the assertions.

Math loading checks hold the actual KaTeX stylesheet request in the existing
Markdown scenario at desktop and 320 px widths. The lazy renderer must keep its
pending surface until that stylesheet applies, then expose the styled formulas.
The stylesheet belongs to the renderer module dependency rather than an
unawaited post-mount effect. Production still loads the renderer and its distinct
CSS chunk lazily; the static Lab build shares CSS across documents, so native
Lab review does not independently qualify production cold loading.

Paragraph checks follow the production type ladder's prose size rather than
assuming it equals UI type. Memory and Schedule checks select the production
Panel when a full-width host has the same accessible region name. Table checks
follow actual overflow and preserve keyboard reading at both ends; a natural
table can need scrolling after a Dock resize. Captures retain geometry alongside
paint, and valid resize-to-fit coverage remains in the same scenario file.

Recorded-result scroll checks wait for the preceding outer wheel and resize to
settle before recording the parent position. Its position must stay unchanged
over eight animation frames; the later inner gesture still requires exact equality
and scrolling at the result boundary must continue through the transcript.

Form catalog checks follow invalid submit, correction and successful local save
at component and phone widths. Feedback must stay within the Name field and
precede Review when stacked; adjacent Review controls must remain beside the
input. The exact localized accessible name and field description are checked
alongside the retained literal draft.
At a **240 px** viewport height, invalid Save focuses Name and reveals its label,
input and error together, including a later whitespace-only submission. Valid
Save retains its button focus; an initially invalid example does not take focus
on admission. Native qualification also checks the field clear of window controls.

The shared UI ownership scenarios also qualify keyboard reading at both edges
in Chromium and WebKit: wide formulas, inline tables and expanded tables.
They hold and repeat horizontal arrows, release the key even on failure, and
wait for elastic scrolling to settle before checking the content edge and DOM
position. They never assign the reading position. Focus and the enclosing
vertical position remain with their readers, and Escape returns from the table
preview to its trigger. Each direction retains a screenshot and geometry,
including failed attempts. The separate formula layout check uses explicit
positions to inspect notation and resizing; it does not qualify keyboard access.

The inline-table suite follows the recorded 80-row table in WebKit with English
light/14 px/full motion and Chinese dark/18 px/reduced motion. It retains the
Composer, its three-line draft, the table reader and its focused Expand action
through successive CSS viewport reductions from 1440 × 900 to 720 × 450 and
560 × 364. Every step checks focus, the reserved transcript frame and center hit
testing. The same task opens the preview, reads its final row and column, returns
to the original action and continues the original draft. These effective reading
budgets exercise reflow; actual page zoom remains a separate Wails qualification.

Goal and Plan reading checks use the existing Agent and Workspace catalog
variants. The Workspace's unavailable captured Goal author permits reading;
the registered Agent author permits editing. Checks preserve that distinction,
cancelled edits, keyboard entry/return and each document's independent Composer
draft through narrow resizing. Goal identity leads its supporting status in
both source and visual order; narrow summaries may wrap that status without
squeezing the objective into a few characters. Checks retain the full objective,
row containment, editor value and Escape focus return. Captures wait for the whole
reading surface to be in view and fully opaque.

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
attachment record exact inputs and reproducible preview URLs. Fixture readiness
requires a mounted surface with no unresolved lazy readers, then layout and the
fonts requested by that layout, followed by a rendered frame. An unresolved
surface fails at the existing arrival deadline instead of becoming ready. The
layout-shift canary moves the actual fixed application surface and verifies both
its displacement and the observer result. The verifier never qualifies an errored
document as ready. There are no pixel goldens to approve automatically: inspect screenshots
for alignment, hierarchy, clipping and states. A passing automated check establishes
the listed rendering contracts, not complete visual acceptance or live Runtime
behavior. Native focus, gestures and zoom still need actual Wails qualification.

The current qualification results, coverage limits and remaining visual findings are
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
Its existing Memory suite checks wrapping, local drafts, filtered-out openers,
scope identity and conversation return in Chromium and WebKit. At 720 × 450,
keyboard collapse to each draft endpoint must restore its reading inset without
changing the draft, selection or focus. Edge scroll allowance is bounded by the
computed half-leading, independently of the shared caret implementation.
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
frame available while viewport changes preserve local drafts. The query only
sets the initial workspace layout; the existing workspace sidebar actions may then
collapse, expand and resize it without replacing the current Composer. Recorded
state-sidebar rows remain inert rather than simulating a Runtime transition.
The existing WebKit checks measure short Composer drafts across 1120, 560 and
390 px frames: complete text must fit, the input must remain inside its Composer,
and sidebar changes must retain the input, draft and editing position. These
viewport checks complement native page-zoom acceptance.

The existing cross-browser UI-ownership scenarios also follow German Schedule
authoring through 560 × 364 and 373 × 243 frames at 18 px in both themes.
They retain the same task and conversation drafts while checking complete footer
text, wrapped model choices, first activation, nested Escape, model refresh and
keyboard cancellation. A focused Cron field must remain inside the body after
each resize with its exact value and backward selection. After manual wheel
reading, a scoped probe verifies that the component makes no further
`scrollIntoView` calls during resize; WebKit can still reveal the focused field
through its native scrolling. The probe forwards every native call and restores
the method in `finally`. Geometry is attached on success and failure. A selected
model can be taller than the smallest body; its first line must remain readable,
and its complete name remains in the scrollable body and menu. These effective
frames complement the native 200% and 300% review; they do not simulate page zoom
or qualify Save, Runtime dispatch or provider execution. Oversized text editors
also need a native caret-visibility check; fitting-field containment alone does
not qualify their editing position.

The MCP JSON import task retains the same focused textarea, exact draft and
backward selection through those frames in English/light and Chinese/dark.
It checks both the final line and a middle line after a wrapping mixed-language
prefix, using the known short suffix for independent line geometry. Wheel
reading remains at the prefix through another resize; later input resumes
editing. Local invalid-JSON feedback and Escape retain the draft and return to
the opener. The task loads the real installed component and shared `TextArea`;
there is no fixture editor or parallel visual entry.

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

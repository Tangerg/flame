# Flame UI

`@flame/ui` owns the shared presentation vocabulary used by Desktop and portable views: Base UI interaction primitives, token-dressed controls, typography, icon layout, document focus rules and compact panel composition, record disclosures, facts, metrics and readable tables. `@flame/icons` remains the owner of drawings, optical masters, SVG exports and discovery metadata. Product icon-name mappings remain with their consumer.

Appearance preferences and theme selection belong to the application. Installed
components use the actual shared React/UI provider context and controlled font scale;
the library never chooses or persists a theme. The former HTML guest projection,
message channel and bootstrap exports are removed.

`ButtonPrimitive` keeps command labels outside native text selection. Dragging a
navigation or action button cannot begin selecting the surrounding document.
Reading content and editable fields retain their native selection behavior.

```tsx
import { Button, IconButton, FontScaleProvider } from "@flame/ui";
import { Brain } from "@flame/icons/react";
import "@flame/ui/styles.css";

<FontScaleProvider fontSize={14}>
  <Button variant="primary">Review</Button>
  <IconButton icon={Brain} aria-label="Memory" />
</FontScaleProvider>;
```

The build emits JavaScript, declarations and one stylesheet. Ordinary consumers need no StyleX transform or application aliases. StyleX consumers may import the shared token declarations from `@flame/ui/tokens.stylex`; those declarations are compiled by their existing StyleX pipeline. Component overrides use the existing `styles` contract so each element merges its styles in one `stylex.props` call.

```sh
npm ci --prefix icons
npm run build --prefix icons
npm ci --prefix ui
npm run check --prefix ui
```

The package is private and is consumed through repository-local `file:` dependencies. Package verification installs locally packed UI and icon archives into an independent consumer. It never publishes to npm. Source owns styling; generated `dist` and token-name outputs are not committed. Changes to shared components require the Desktop gate and the portable resource build and component lifecycle acceptance.

`Panel` owns the content measure and container-responsive gutters.
It accepts the native section ref and events, so local navigation can keep its
focus and scroll path without adding another reading-plane wrapper.
Its explicit `header` slot holds introductory content, `controls` holds the one
sticky action and filter zone, and children hold the scrolling content. The host
keeps its existing scroll viewport. The panel does not shrink inside a column
host: its canvas and sticky-control boundary extend through the complete content.
`Panel` measures only its own controls,
projects their height on its root, and uses it to keep focused content and anchors
clear of the controls; wrapped controls update through `ResizeObserver`.
Body focus also respects the host's clipped viewport. A wrapping native label
and its focused control form one reading unit when they fit below the sticky
controls and disclosure header. Taller fields still reveal the focused control's
beginning after native autofocus or a control-zone resize.
`PanelHeader` owns the page title, description and contextual navigation.
Its title size can use the smaller existing display roles for workbench views.
Page actions belong to the persistent `Panel.controls` zone, alongside filters.
`PanelSection` groups one heading, supporting context, filters and content;
`PanelFooter` groups paging only when another page exists. `RecordGroup variant="list"` owns the shared resource surface, corners
and separators. Its `cards` variant provides the responsive schedule grid;
`ListRow variant="card"` owns each independent card surface.
`Disclosure variant="row"` owns resource summaries that expand directly. Hover
feedback belongs to its summary trigger; the expanded reading body remains
untinted while the pointer moves through it. Header and body keep the shared row
insets and existing sticky-header geometry. A consumer's `data-focused` record
marker uses the summary button's shared active state; it never tints the expanded
source or its semantic change colors.
`DiffStat` gives Tool summaries, workspace entry points and portable Diff files one
compact numeric voice. Positive additions and removals use their semantic ink;
known zeroes and unrecorded counts remain quiet and distinct. A count keeps its
sign with its digits, and the pair wraps between counts when constrained.
`RecordButton` is a whole-row resource that navigates rather than expands: a
ghost `Button` with the shared row insets, start alignment, no row corners or
edge, and a trailing chevron. It accepts the native button ref, events and
attributes; `aria-current` selects the row with the shared selected state.
Navigation history, origin focus and the destination belong to the consumer.
`DiffFileList` renders its files as `RecordButton` rows with a primary filename, complete
directory and compact state/count metadata. `DiffFileHeader` keeps that same
identity above a selected reader, while `DiffFilePath` allows complete directory
segments to wrap. Selection and source-return history belong to the consumer;
these components do not query files or own a second navigation model.
`RecordSummary` owns the optional identity mark, copy and state alignment.
`RecordBody` preserves recorded line breaks and allows selecting its complete text.
`PanelHost` marks an embedded panel whose surrounding chrome already shows the
view identity. Dock views use compact shared insets and quieter supporting copy;
their `PanelHeader` title stays in the accessible outline without repeating a
display heading. Context and descriptions remain complete and visible. Independent
pages keep their display title and page spacing. The host selects this composition;
portable components do not inspect viewport width or create another placement state.
`FilterBar` owns the control zone in three meanings, in reading order: the
growing search column, the page `actions` beside it at the inline end, and the
refining filters as `children`. Search and actions share a row while they fit and
the filters follow, joining that row when the width allows; each group wraps as a
unit, so actions never strand between filters. Multiple actions use one named
`Toolbar` in the `actions` slot. When a page's actions are creation commands, as in
Schedules, refresh stays with the filters it re-reads so the creation pair keeps
one row. Scope and catalog navigation keep a separate
preceding `Toolbar`.
`Toolbar.actions` aligns a related trailing action group and lets it wrap with
the available width. The group shares a row when it fits and preserves each
control's size role.
Controls wrap naturally rather than shrinking their labels to fit one row.
Section headings and form legends use the smaller, semibold supporting role;
record identities retain the main text role within each group.
Management filters use the shared `lg` size for both search and selection; compact
workbench controls keep their own smaller roles.
`RecordHeading` groups identity, state and the main action. `Disclosure` owns a
full-width expanding panel with an optional adjacent action slot. Its optional
`stickyHeader` keeps that same complete summary beneath the enclosing Panel's
controls in a bounded scroll viewport. It returns to normal flow when fewer than
two body lines and their leading inset would remain below the header.
The measured header height also keeps focused body content clear; the header does
not add its own height to its focus offset. Header, viewport and controls resizing
update this local presentation without tracking the current file or scroll position.
`ActionMenu` owns secondary action presentation, keyboard behavior and focus return; each command
receives its actual trigger as the opener for an inline editor. `triggerContent`
replaces the default ellipsis with a text trigger for named action menus.
`Facts` keeps labels with their values in equal, container-responsive columns.
Column edges remain shared across rows; narrow containers reduce the column count.
Long identities and paths wrap completely inside each pair, and visual reading
order follows the source order.
`TileGrid` reserves text space at the tile's font scale plus the shared card
inset. Filtering retains the column geometry, so a lone result keeps the
catalog's card width; narrow containers retain complete, wrapping identities.
`Stats` groups metrics in one shared card with a single inset and compact,
content-driven columns that wrap with the available width;
`MetricValue` and `ShareBar` render existing values without calculating business totals. `Segmented` owns the controlled catalog selector in
Desktop and portable views. `Form`, `RecordForm`, `FormSection`, `FormRow`, `FormField` and `FormGroup` provide responsive form grouping and visible
native labels; `DocumentBody` styles already-parsed semantic document content.
These components own presentation only and do not query or advance product facts.
`CodeBlock` frames its single literal string child with the same `ARTIFACT_SURFACE` skin
as the transcript's code and diagram artifacts. Its named `pre` remains the
bounded keyboard scroll region. `CodeBody` owns that stable literal source region,
its code typography, shared wrap preference and host-supplied highlighting. It
limits long code to the smaller of 28rem and 60vh, keeping the caption outside
the scrolling source. A consumer's `styles` override merges on that same `pre`;
compact Tool inspectors retain their smaller existing bound. Highlighting changes
only the child `code`, preserving the reading region, scroll position and focus.
The literal remains natively selectable even inside an otherwise non-selectable
host. With the region focused, Ctrl/Cmd+A selects only its complete source; the
browser owns the range and ordinary copy behavior.
`CodeFrame` is that frame alone, for portable views
that own their own body, such as recorded Tool results. `CodeCaption` is the one code
header for every renderer, including the transcript's highlighted blocks: the recorded
label (or `text`), any consumer actions passed as children, a wrap toggle bound to
the single page-wide `useCodeWrap` preference, and a copy action that writes the exact
literal and reports success only after the clipboard accepts it. Callers supply
localized `CodeActionLabels`; portable packages use English labels. Syntax color is a host capability:
`CodeHighlightProvider` supplies one `CodeSourceComponent`, which React renders
inside the stable `pre` region as `<Source source language />` and which returns
the `code` element for exactly that source: escaped highlighted markup when ready,
the plain literal until then. The copied text is always the original literal;
without a provider `CodeBlock` renders it plain. The Desktop installs its single Shiki pipeline
here, so transcript code, captured file previews and recorded evidence share one
highlighter, cache and scheme-following theme. `copyText` and `useCopyFeedback` are the shared
plain-text clipboard owner. Input and output wells retain their own sunken surface.
`DocumentBody` opts into the shared `data-prose` reading grammar: paragraph and
heading rhythm, nested list markers, quotes, rules, emphasis, inline code and
tables. Other semantic renderers use that same root marker while retaining their
host's content measure and font size. Paragraphs, lists and quotations cap their
reading measure at `65ch` within that space, aligned to their text direction;
media paragraphs can use the host's full width. `data-prose-level` records an authored
heading level when the host nests its physical heading below a surrounding
outline. The `inline-code` and `prose-table` data slots also supply those reading
roles outside a complete prose root, including table lightboxes.
Paragraphs share one block gap. Headings use unitless leading and keep supporting
content visibly separated even at the deepest levels; weight and ink distinguish
levels that share the body size. The host owns the prose size, independently of
its interface and code roles.
Generated footnotes use a quiet supporting-text section and retain their native
reference links. The renderer owns document-local identities and navigation.
The link appearance marks content that navigates: anchors with a destination
and actionable workspace file references, which mark themselves as the
`file-reference` slot. Both read as accent text with a tinted underline that
strengthens on hover; an anchor without a destination remains ordinary text.
Emphasis and inline code inside a link anchor inherit the link color. A file
reference also uses inline code's mono family and code size, without the code
tint, so a path never reads larger than the code beside it. Its text remains
selectable within the surrounding prose while retaining button activation.
Captured documents
do not navigate: their link labels remain ordinary text, and a destination shown
beside a label uses the muted `link-destination` slot.
Prose color comes only from the host's accent roles: headings and inline code use
`--color-accent-strong`, links and list markers `--color-accent-text`, quote rules
`--color-accent-line`; body and strong text keep the neutral ink. Panel titles use
the same heading role.
`Callout` owns the GitHub alert presentation for every Markdown adapter: the
marker grammar (`readCalloutMarker`), the natural-case title and icon per kind,
and the status rule, wash and on-tint ink. `ImageNotice` is the one inert image
description: an alternative text and a muted detail, used for unavailable
transcript images and for captured file images that are never loaded.
Quotes and loose lists retain paragraph boundaries around nested blocks; quote
spacing applies to direct content without overriding a nested list's own flow.
`TaskListItem` renders one recorded task as a list item with an immutable,
named checkbox marker. Its caller supplies the captured checked state and primary
visible label; paragraphs and nested lists remain selectable document content.
The marker follows the reading font size and does not enter the keyboard path.
Its primitive projects document semantics without registering a form control.
`IconMark` owns the decorative identity frame used by record summaries, empty
states and modal headers. Its emphasis surface derives from the host accent;
changing the accent recolors these marks and selected rows together.

## Review preview

The Desktop visual fixture renders the library with the real Flame appearance owner. Run `npm run browser:dev --prefix desktop/frontend` from the repository root and open `/visual/?fixture=atoms&theme=light` or the dark counterpart. The shared-panel specimen includes the Memory toolbar, status filter and record row. This preview uses an isolated in-memory transport and does not contact a user Runtime.

Field `invalid` owns both error styling and the accessible invalid state. Search
clear actions require an explicit localized label. Compact controls retain a
24px minimum target; hover effects run only on hover-capable pointers.
Text buttons wrap long labels and grow vertically within their available width;
size tokens define their minimum height. Standalone icon controls retain square
geometry. Within a flex row, joined controls share the row's actual height and
their default pressed feedback keeps the seam intact.
The shared stylesheet owns the 44px coarse-pointer floor; controls and their
layout hosts include that floor in their geometry.

`SearchField` requires a controlled string value. Its input and clear action obey
the same disabled, read-only and pending constraints. Its explicit clear action
returns focus to the input and replaces the browser's native cancel action.
The query is literal text: capitalization, autocorrection and spellcheck are off
unless the caller passes those native props. The caller's `onKeyDown` runs first
and keeps any key it handles. An unhandled Escape over an editable, nonempty query
then belongs to the field, from the input or its clear action: it clears through
`onClear`, keeps input focus and does not reach enclosing navigation. Without
`onClear`, the field still keeps that Escape local and leaves the browser's native
search clearing in place. Escape in an empty or immutable field, and any key during
IME composition, reaches the host. A bounded task that closes on Escape claims it
in capture on its own owner. `isImeKey` is the shared composition test for
`isComposing` and the legacy `keyCode` 229.
`IconButton` requires either
`aria-label` or `aria-labelledby`; `title` only overrides the tooltip. `DiffTable`
requires consumer-supplied region and column labels, including assistive text.
Every code cell reserves the same nonselectable marker width so context, added
and deleted source retain a common code column in both layouts. Source text and
its original indentation remain separate from the decorative marker.
`ScrollRegion` contains horizontal overscroll while allowing vertical scrolling
to continue through the enclosing reading viewport. Bounded image and diagram
viewers explicitly contain both axes in their own viewport styles.
Read-only code regions follow the same horizontal-only containment policy.

The shared stylesheet owns search emphasis: `search-match` and
`search-match-active` CSS highlights use the host's selected wash and paired accent
fill and ink. A `search-line` slot with `data-current-match` marks a line-based
result with the same wash and a leading accent rule, preserving its ANSI ink and
fixed row height. Each consumer retains its own query, matching and navigation.

`TextArea autosize` measures its content across supported hosts. The component owns
height updates for input, controlled values, form reset, width and font changes;
CSS still owns minimum and maximum height. Consumers do not add another sizing loop.

The shared stylesheet also applies the application's `data-motion="off"` projection
and the system reduced-motion preference. Both disable motion with zero duration;
they do not manufacture transitions on otherwise static components.

The default host theme owns the semantic roles (neutral planes, one accent scale
generated from the user's configurable accent, and status colors) and follows HeroUI
3.2.6 for rounded buttons, fields and quiet contact shadows. Button and field radii follow
the user's radius scale at every size; text chips and status labels share the
button role. Fixed circular marks and explicitly round buttons keep their geometry.
Desktop size roles are
24/28/32/36px, independent of navigation density. Controls retain native cursors,
keyboard focus, wrapping labels and the coarse-pointer target floor. Theme values
remain with the host; the library renders them without a second appearance store.
Settings and portable pages share `Panel` as the sole content-measure owner.

Tonal `Badge` labels consume the host's on-tint text roles, derived from each
semantic ink and the main text ink. The shared component keeps the state label
visible; consumers select its tone from the product owner's state. Neutral labels
retain muted ink, while icons, links and actions retain the ordinary semantic ink.

`ModalDialog` owns bounded modal geometry, accessible titles and descriptions,
header and footer alignment, and internal body scrolling. Its optional submit
handler makes the popup itself a native form; installed component content keeps
its own form when no handler is supplied. Desktop dialogs supply their existing
command handlers and captured targets. `Form` preserves native submission and
shares its section spacing with `RecordForm`; only `RecordForm` adds a card
surface and inset. Inline editors pass their captured `opener` to `RecordForm`.
On unmount it returns focus only when that form owned focus and removal left
focus on the document body. Restoration follows the DOM commit so the opener
can be enabled again; another control's focus is preserved. The opener may be an
element or a resolver returning the current target or null. The resolver runs only
when restoration is eligible, after removal, so a filtered-out trigger can return
to its consumer's persistent search field. `FormActions` groups
inline editor actions, with `auxiliaryAction` leading and children trailing;
cancellation precedes the primary action. Both groups wrap within the available
width, using the shared control gap internally and content gap between groups.
The parent form or record composition owns spacing before the actions;
`FormActions` adds no outer inset or separator.
`modalPanel()` owns the elevated modal face, corners, shadow and motion. Document
lightboxes own their close control and reserve its space alongside their content;
callers supply the localized close label. Top-aligned overlays derive their
available height from the same inset that positions them, preserving the compact
bottom gutter. Search overlays keep the query row fixed while their results scroll.

## Shared visual ownership

The host visual-style contribution owns colors, shape, layout, and timing tokens.
`FIELD_SURFACE`, `MENU_SURFACE`, `CARD_SURFACE`, and `CHOICE_SURFACE` own their
respective control skins. Desktop atoms and portable components consume these
same surfaces. `layout` tokens own page measure, gutters, section and field gaps,
card padding, row padding, label and content gaps, compact viewport gutters,
control spacing, and menu insets; consumers keep only structural
layout and content-specific geometry.

`HoverTrack` and `ItemHighlight` own the moving item backplate, also used by
`Segmented`. They share the host's Motion runtime. The spring reads the slow
motion rung and motion scale from the current CSS projection, so a theme change
affects the next movement without a second animation preference. No animation
runs before styles are available, during server rendering, or when motion is
disabled. System reduced motion is honored separately.

The former large-control radius and resource-surface tokens are removed. All
field sizes use `field-radius`; cards and resource lists use `app-card-surface`.
Repository consumers migrate together; external authored components must use
these shared roles when rebuilt.

`FIELD_SIZE` owns the minimum height and inline inset for single-line text,
search, and select controls. Large fields have the same skin as compact ones.
For interactive preview, append `&motion=full` to the fixture URL. Geometry
fixtures disable motion by default; they do not represent the application
motion preference.

`FormRow`, `FormField` and `FormGroup` keep intrinsic field heights when translated
labels wrap. `FormField` uses a native label for one labelable control;
`FormGroup` names a composite control group without replacing its option names.
Both share field spacing and label typography.
Boxed single-line and multiline fields share `FIELD_SIZE` text insets and reset
native WebKit appearance; focus and invalid borders remain with `FIELD_SURFACE`.
Provider, MCP, and hook lists consume `RecordGroup` and `ListRow`, while inline
editors consume `RecordForm`, `FormField`, and `FormActions` rather than defining
another set of row insets, field spacing, and footer alignment.

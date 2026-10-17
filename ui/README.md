# Flame UI

`@flame/ui` owns the shared presentation vocabulary used by Desktop and portable views: Base UI interaction primitives, token-dressed controls, typography, icon layout, document focus rules and compact panel composition, record disclosures, facts, metrics and readable tables. `@flame/icons` remains the owner of drawings, optical masters, SVG exports and discovery metadata. Product icon-name mappings remain with their consumer.

Appearance preferences and theme selection belong to the application. Installed
components use the actual shared React/UI provider context and controlled font scale;
the library never chooses or persists a theme. The former HTML guest projection,
message channel and bootstrap exports are removed.

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
Its explicit `header` slot holds introductory content, `controls` holds the one
sticky action and filter zone, and children hold the scrolling content. The host
keeps its existing scroll viewport. `Panel` measures only its own controls,
projects their height on its root, and uses it to keep focused content and anchors
clear of the controls; wrapped controls update through `ResizeObserver`.
`PanelHeader` owns the page title, description and contextual navigation.
Page actions belong to the persistent `Panel.controls` zone, alongside filters.
`PanelSection` groups one heading, supporting context, filters and content;
`PanelFooter` groups paging only when another page exists. `RecordGroup variant="list"` owns the shared resource surface, corners
and separators. Its `cards` variant provides the responsive schedule grid;
`ListRow variant="card"` owns each independent card surface.
`Disclosure variant="row"` owns resource summaries that expand directly.
`RecordSummary` owns the optional identity mark, copy and state alignment.
`FilterBar` owns the growing search column and wrapping controls. Page actions
follow the filters in that same zone; multiple actions use a named `Toolbar`
within it. Scope and catalog navigation keep a separate preceding `Toolbar`.
Controls wrap naturally rather than shrinking their labels to fit one row.
Section headings and form legends use the smaller, semibold supporting role;
record identities retain the main text role within each group.
Management filters use the shared `lg` size for both search and selection; compact
workbench controls keep their own smaller roles.
`RecordHeading` groups identity, state and the main action. `Disclosure` owns a
full-width expanding panel with an optional adjacent action slot. `ActionMenu` owns
secondary action presentation, keyboard behavior and focus return; each command
receives its actual trigger as the opener for an inline editor. `triggerContent`
replaces the default ellipsis with a text trigger for named action menus.
`Facts` keeps labels with their values and wraps complete pairs according to their
content. Short pairs share rows; long identities and paths use the available width
and remain complete. Visual reading order follows the source order.
`Stats` groups metrics in one shared card with a single inset and compact,
content-driven columns that wrap with the available width;
`MetricValue` and `ShareBar` render existing values without calculating business totals. `Segmented` owns the controlled catalog selector in
Desktop and portable views. `Form`, `RecordForm`, `FormSection`, `FormRow`, `FormField` and `FormGroup` provide responsive form grouping and visible
native labels; `DocumentBody` styles already-parsed semantic document content.
These components own presentation only and do not query or advance product facts.
`IconMark` owns the decorative identity frame used by record summaries, empty
states and modal headers. Its emphasis surface derives from the host accent;
changing the accent recolors these marks and selected rows together.

## Review preview

The Desktop visual fixture renders the library with the real Flame appearance owner. Run `npm run browser:dev --prefix desktop/frontend` from the repository root and open `/visual/?fixture=atoms&theme=light` or the dark counterpart. The shared-panel specimen includes the Memory toolbar, status filter and record row. This preview uses an isolated in-memory transport and does not contact a user Runtime.

Field `invalid` owns both error styling and the accessible invalid state. Search
clear actions require an explicit localized label. Compact controls retain a
24px minimum target; hover effects run only on hover-capable pointers.
Text buttons wrap long labels and grow vertically within their available width;
size tokens define their minimum height. Icon-only controls retain square geometry.
The shared stylesheet owns the 44px coarse-pointer floor; controls and their
layout hosts include that floor in their geometry.

`SearchField` requires a controlled string value. Its input and clear action obey
the same disabled, read-only and pending constraints. `IconButton` requires either
`aria-label` or `aria-labelledby`; `title` only overrides the tooltip. `DiffTable`
requires consumer-supplied region and column labels, including assistive text.

`TextArea autosize` measures its content across supported hosts. The component owns
height updates for input, controlled values, form reset, width and font changes;
CSS still owns minimum and maximum height. Consumers do not add another sizing loop.

The shared stylesheet also applies the application's `data-motion="off"` projection
and the system reduced-motion preference. Both disable motion with zero duration;
they do not manufacture transitions on otherwise static components.

The default host theme follows HeroUI 3.2.6: cool neutrals, blue primary actions,
capsule buttons, rounded fields and quiet contact shadows. Desktop size roles are
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
can be enabled again; another control's focus is preserved. `FormActions` groups
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

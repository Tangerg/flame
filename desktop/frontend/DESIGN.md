# Flame visual design

The visual specification for the desktop app: what each surface, colour, type
step, shape and motion token is FOR.

**It does not restate values.** Shared vocabulary and document rules live in `../../ui`; application palette values live in `src/styles/globals.css` and
`src/plugins/builtin/theme/themes/*.ts`, and a document that copies them is a
document that goes stale — this one carried a 339-line palette snapshot of a
design three revisions old, and five of its sections cited that snapshot as
canonical while its own opening line told the reader not to trust it. Where a
value matters here, the token is named and the reader looks it up.

Companion documents: `DESKTOP_UI_POLISH.md` is the desktop-feel regression list,
`ARCHITECTURE.md` the structure, `CONTENT_RENDERING.md` the wire → render
grammar. Where this document and the shipped design disagree, the shipped design
wins; where the shipped design and the ChatGPT desktop reference disagree, that
is a decision to make, not a rule to apply.

## 0. Design language (the five pillars)

The whole system reduces to five decisions. Everything below elaborates them.

1. **Tool windows around one reading plane.** Permanent regions separate by
   material value and a half-pixel edge whose shape the visual style owns (see
   Hairlines). Cards group independently readable
   content through fill; ordinary tool activity stays linear. Only floating
   menus and popovers receive an optical ring and elevation. The composer is
   an input surface with one real border, accented on focus. Boundary mechanisms
   belong to the visual style, never individual call sites.
2. **Near-monochrome, one restrained accent** — overall black/white/grey; the
   accent (a calm **blue**, user-selectable) marks live state, progress, focus,
   and links. Primary actions invert neutral ink against their surface: dark
   on light, light on dark. Their fill and text belong to the theme's CTA tokens.
3. **Dual-theme parity, follows the OS** — light and dark are both first-class
   and polished; the default theme is "system" and tracks `prefers-color-scheme`
   live. The two are mirrors of one region algorithm, not two hand-built skins.
4. **Native system font, mono as the technical voice** — SF Pro / PingFang on
   macOS (the OS UI face) for language; mono for everything that is data — paths,
   counts, durations, tokens, shortcuts, code. That split is what makes a dense
   agent transcript scannable.
5. **Dense, not cramped** — a workbench rhythm: 46px chrome bars, short rows, one
   centred reading measure flanked by navigation rails. Features are first-class
   grouped entries in the sidebar + settings, not buried in the command palette.
   **No tab strip** — one active session; workspace views open full-pane or in the
   dock.

Constant across all of it: the `@theme` token bridge, plugin-contributed chrome,
accent _scarcity_, tabular numerals, keyboard-focus discipline, reduced-motion.

---

## 1. Overview

Flame is an agent client — a desktop application (Wails / React) that streams Flame Runtime Protocol events from a Go runtime and renders them as a chat surface with inline tool calls, code, diagrams, and approval flows. The frontend is a **view onto a runtime**, not the runtime itself — but it presents as a refined, calm product surface, not a dense console.

Light and dark are **equal first-class themes**; the default follows the OS (`prefers-color-scheme`) and tracks it live. Neither scheme is second-class.

**Reference** — ZCode's resource management pages guide plugin layout and control
proportions: a bounded content column, clear page identity, grouped resources,
quiet supporting copy and aligned actions. The local reference is read-only;
Flame keeps StyleX, Base UI, its own tokens and Runtime ownership.

- Plugin pages share `Panel` and `PanelHeader`, with one content measure and
  container-responsive gutters. Search, filters and content share alignment edges.
- Settings, plugin resources and inline editors share the semantic layout roles
  for card insets, row padding, field labels and content gaps. Compact page gutters
  also bound modals. Translated field labels keep their intrinsic height; they do
  not stretch neighboring controls. Field labels contain their control only;
  related commands belong in the shared action zone. Auxiliary actions lead;
  cancellation and the primary action trail. The parent composition owns the
  spacing before this zone.
- Resource lists use direct summary disclosures: identity and description lead,
  state and the main action trail, and details expand across the full row width.
  Secondary commands use the shared action menu. Technical provenance belongs in
  details; current user-facing state remains visible.
- Schedules are independent tasks and use a responsive card grid. Dense evidence
  feeds remain linear lists. Neither layout introduces its own business state.
- Management search and selection share the large control size and radius tokens.
  Compact workbench controls retain the smaller size roles; all sizes respect the
  coarse-pointer floor and grow with translated labels.
- Metric summaries use one shared card with tabular values. Columns wrap by
  their content width; labels stay close to their values. Resource lists group
  through fill and subtle separators; summaries, independently actionable cards
  and editor forms share the card surface and quiet contact shadow.

## 2. Color

### Philosophy

Color carries information, not decoration. The system uses **one chromatic accent**, **four greys for surfaces**, **three greys for hairlines**, and **four semantic colors used sparingly**. Decoration comes from the surface ladder, not from color variation.

### Surface anchors

Values live in `themes/flame-*.ts` and are restated for first paint in
`globals.css`. This table says what each anchor is FOR; it deliberately does not
repeat the hexes, which is how the previous version of it went stale.

| Token                                              | Role                                                                                                                                                                             |
| -------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `canvas` (`--color-bg`)                            | The reading plane — transcript and view bodies, slightly off-white in light and near-black in dark.                                                                              |
| `surface`                                          | Region chrome — the drawer, the dock, the bars that frame the plane.                                                                                                             |
| `card` (`--app-card-surface` → `--color-elevated`) | An object placed on a region: a message, a tool card, the composer.                                                                                                              |
| `sunken` (`--color-sunken`)                        | A well cut into a surface: code bodies, terminals, diff hunks, text fields, progress tracks, and inline code in prose.                                                           |
| `surface-2` / `-3`                                 | Derived chip rungs above `surface` — badges, kbd, selected rows, resting control fills. Mixed out of the CHROME grey, so they belong on chrome; on the plane they read as grime. |

The light canvas is off-white; editable fields and summary cards are white.
The dark canvas is near-black; cards lift gently above it. Resource lists use the elevated content surface and one quiet separator between
adjacent records. Wells remain distinct from cards and list groups.

`surface-2/3/4` derive from `surface` via `color-mix(--depth-step)` so the
contrast preference moves the chip rungs per scheme — they are never pinned
inline. **The step is scheme-aware**: dark doubles it, because 4% of a near-white
ink over a near-black surface moves it a third as far, in perceived lightness, as
4% of a near-black ink over a near-white one.

### Hairlines

A hairline is the edge of a **control** — a text field, a chip, the composer — and
of a **region**: the two are the same primitive at two weights. A change of region
takes `border-soft`, a bar inside one takes `border`, and the reference weights them
apart by the same ratio (207 against 225 on a 255 plane). Regions get theirs from
`--app-card-edge` / `--app-pane-split` / `--app-header-edge`, whose SHAPE the visual
style owns; a callsite never draws a region boundary itself.

Half a pixel, not one. On a 2x panel that is exactly one device pixel, which is what
makes an edge crisp without giving it weight — at 1px the composer carried the
heaviest line on the screen. The earlier revision of this section spread a
directional cast at each seam instead; a cast lands ON the reading plane, so all
three seams read as pressing down on the document.

There is one other case, and only one: an object that **demands attention** — the
failed-run banner. It is neither a region nor a control, and it takes a 1px
`--color-negative-edge` over a neutral fill. The fill stays neutral because it runs
200px tall and a wash at that size is a lot of colour for "please look"; the small
inline notices are the inverse, and tint instead. There is exactly one tone token
for this, because there is exactly one such object: a pending approval is a card
like any other, and the four other `-edge` tones this once spelled out were a
palette nothing asked for. This is the only place the language spends a border on
meaning rather than on affordance. The three-step ramp
(`border` / `border-soft` / `divider`) uses literal hex per theme, because a
semi-transparent border shifts across surface lifts and reads as approximate.

**Ink, by contrast, may derive.** Unlike hairlines, the ink ramp (`text-soft` / `text-muted` / `text-faint`) _should_ adapt to the surface behind it — that's the Apple label model. A theme can ship just `text` + `text-bright` and let the soft/muted/faint steps derive as `text` at ~82% / ~56% / ~38% alpha over transparent (so they composite against whatever surface they sit on). Palette themes (Solarized, Catppuccin, Tokyo Night, One Dark) instead pin explicit ink hues — their ramp is part of the palette identity, not a single hue at falling opacity. The first-party Flame themes keep explicit values too; the derivation is the low-friction default for third-party themes.

### Accent policy

The single accent (`--color-accent`, a calm blue by default, user-selectable with
green / pink / orange as alternates) is reserved for these uses:

1. Selected navigation and dock tab state
2. Focus ring (`:focus-visible` — a single thin stroke, **no halo / glow**; one global rule, never drawn at a callsite)
3. Live indicator (streaming dot, running pill, `tab-dot.running`)

Forbidden surfaces for accent: section background, card fill, avatar background, decorative borders, status icons that are not "live". And **no bright accent ring on input focus or click** — inputs/composer strengthen their border quietly instead (the loud halo read as cheap).

### Semantic palette

| Token              | Use                                                                                              |
| ------------------ | ------------------------------------------------------------------------------------------------ |
| `--color-success`  | Run finished cleanly, action confirmed. Allowed in: run pill (idle/done), tab dot after success. |
| `--color-warning`  | User attention required. Allowed in: the approval card, the waiting tab dot.                     |
| `--color-negative` | Error. Allowed in: the run error banner, a tool call's failed status.                            |
| `--color-info`     | Information / link. Allowed in: inline links, info badges.                                       |

**Each semantic is two colours, and only one of them is pinned.** The theme
spells the INK — the tone a status word, an icon or a 6px dot is drawn in, whose
luminance is pulled until it clears 4.5:1 on the darkest surface it can sit on.
Every TINT (`-wash` / `-badge` / `-edge`, and the diff row and word tints) mixes
instead from `--tone-*`, the same hue lifted to L68 at 1.4× chroma. One token
cannot do both jobs: light `warning` is pinned at L51 C.10, an olive, and a tint
mixed from an olive reads as dirt rather than as amber. The fill tone is derived,
not shipped, so the hue is still said once — by the theme, or by the user's accent
pick — and no palette has to carry a second set. Anything that must be legible on
its own keeps the ink: these tones are near 1.9:1 on white and would fail 1.4.11
as a mark.

Badge labels consume the host's `--color-*-badge-text` roles, derived from each
semantic ink and the main text ink. This keeps the semantic hue readable on the
badge tint across content and chrome surfaces. These on-tint roles do not replace
the ordinary semantic ink used by icons, links and actions. Pending Memory
reviews and Skills proposals use warning because they require user attention;
their product owners still supply the state and commands.

**Semantic colours are scheme-tuned**, and in both directions. Dark lifts them in
luminance and pulls the chroma down so they neither vibrate nor edge-bleed on a
near-black plane; light pushes them the other way, DARKER and deeper than the
saturated web reds and blues, because on white it is contrast against the plane
that decides legibility, not brightness. Palette themes (Catppuccin / Tokyo Night
/ Solarized / One Dark) ship their own canonical semantic tones and are left
untouched.

## 3. Typography

### Font families

The interface uses the operating system's own face; code uses one bundled mono:

- **Sans** (`--font-sans`) — the native stack: `-apple-system` / `BlinkMacSystemFont`, **PingFang SC** / Hiragino for CJK, then `system-ui` / Segoe UI / Microsoft YaHei. No face is bundled for language. Display and body share it; weight does the hierarchy.
- **Mono** (`--font-mono`) — **JetBrains Mono**, then `ui-monospace` / SF Mono / Menlo. Genuine data only: code, IDs, timestamps, file paths, tool signatures.
- A single `--font-sans` / `--font-mono` token (no `--font-ui` split); the user can override either in Settings → Appearance, and code has its own size preference that moves only `--fs-code`.

Same shape as the reference, which bundles `OpenAI Sans` in front of
`-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`. It bundles **no**
mono, though: its code voice is `ui-monospace, SFMono-Regular, SF Mono, Menlo,
Consolas, monospace`, and Flame's is JetBrains Mono.

### Scale

The scale has nine roles: four UI steps, prose, code, and three display steps. At the default 14 px base, UI steps are 10 / 12 / 13 / 14 px and display steps are 16 / 18 / 24 px. Display sizes are smaller than typical marketing systems because Flame is a product UI, not a hero page.

Sizes, weights and tracking are the `--fs-*`, `--fw-*` and `--tracking-*` tokens
in `globals.css`; `check:tokens` fails the build on a per-callsite pixel value.

### Principles

1. **Sans-first; mono is the _data_ voice only.** Labels, section headings, nav, speaker names, view titles + subtitles are **sans**. Mono (`caption-mono` / `code`) is reserved for genuine data — IDs, durations, timestamps, file paths, tool-call signatures. (The earlier "mono as eyebrow everywhere" read as an engineering console; pulled back.)

2. **Use weight for hierarchy without making every control bold.** Regular body
   and action labels use 400, resource names use 500, and page titles use 600.
   Supporting descriptions use the 13 px step, not micro text.

3. **Keep display restrained.** Plugin page headings use the 24 px step; section
   and resource headings use the UI ladder with distinct weights. Sizes derive from the single appearance-owned ladder and
   retain the user's font-size preference.

4. **Sentence-case headlines.** Never ALL-CAPS. Welcome screen, settings sections, view headers — all sentence-case. Optional period termination is allowed (Vercel signature) but not required.

5. **Tabular numerals everywhere numeric.** `font-feature-settings: "tnum"` is set on `body`, so it inherits everywhere rather than being asked for per call site, and the reset re-inherits it for `button`/`input`/`textarea` because the `font` shorthand resets it. Measured on the bundled face: swapping every digit in a numeric run changes no width anywhere (26 runs across five routes; with the feature forced off, 17 of them move by up to 10.7px).

   **This holds for the face the product ships, not for every face it offers.** A font can only do tabular figures if it has them, and the UI-font picker offers one that does not: on a ten-digit run at 32px, `Arial` moves 21.4px while `Helvetica Neue` is exact. Asking through `font-variant-numeric: tabular-nums` or `lining-nums tabular-nums` instead changes that number by nothing — there is no CSS answer, only the font's own glyph widths. Every code candidate measured is exact, which monospace guarantees by construction. (Only families installed on the measuring machine can be judged; the rest of the curated list is unverified rather than known good.) The promise above is therefore a property of the default and of any chosen family that provides the feature.

6. **CJK safety.** Letter-spacing > 0.02em should be scoped to `:lang(en)` — CJK characters are pulled visually apart by positive tracking.

## 4. Layout

### App shell

Three opaque tool windows and no line between any of them. No bottom status bar:

```
 drawer (fixed, slides)      reading plane (z-15)                    dock
┌─────────────┐┌──────────────────────────────────────────────┬─────────┐
│ 46px header ││ 46px header: project / title · state · meta   │ 46px    │
│ project     │├──────────────────────────────────────────────┤ tabs +  │
│ ⌘N ⌘K …     ││ ·                                       In   │─────────│
│ Projects    ││ ·      Message stream (--content-max)   this │         │
│   session   ││ ·                                       answ │  views  │
│   session   ││                                              │         │
│             ││    ┌──────────────────────────────────────┐  │         │
│ ⚙ settings  ││    │ Composer                             │  │         │
└─────────────┘└────┴──────────────────────────────────────┴──┴─────────┘
       ↑          ↑ turn rail (44)          outline rail (186) ↑ dock seam
   --sidebar-width  --app-card-edge: the drawer seam,          0.5px edge
   (275, 240–520) drawn inside the plane                  (--app-pane-split)
```

Both rails are container-query gated on the width of the reading column — not the
window — because the drawer and the dock change it without the window changing at
all. Banners and composer take the same gutters, so the three stay on one axis.

### Sidebar

- **Default state: expanded** (`--sidebar-width`, 275px, user-resizable by dragging
  the seam rail; floor 240px, ceiling 520px). Both flanks clamp against one reading
  floor, `CONVERSATION_READING_MIN_PX` (440px): the sidebar never widens past it,
  and a dock that cannot fit beside it opens its material full width instead.
- **Pinned identity** above the scrolling index: the active session's workspace,
  because the one fact you must be able to read without scrolling is where the next
  command will run.
- **Single-line session rows**: title, then one trailing mark (pin, running or
  waiting state, or time). A "…" button on hover or focus opens the same actions as
  the context menu; a collapsed project reports how many of its sessions wait.
- **Collapsed** (`⌘B`) slides the drawer fully off-canvas under the card — there is
  no icon rail. The card then reaches the window edge, squares its seam corner, and
  its header widens its leading inset to clear the macOS traffic lights.
- **One visible collapse control.** Expanded, the toggle lives in the drawer's
  46px header after the traffic-light gutter; collapsed, ownership moves to the
  content header. Keyboard focus follows that handoff instead of falling onto the
  document.
- The seam rail is a focusable vertical separator: pointer movement writes only
  `--sidebar-width`, pointer release commits once, and Arrow/Home/End provide the
  same bounded resize path for keyboard users.
- The drawer is opaque region chrome (`--app-drawer-surface`), never a translucent
  sheet. It carries no border and casts no shadow of its own: the plane draws the
  seam as an inset 0.5px edge, because the plane outranks the drawer on z-index so
  the drawer can slide underneath it.

### Chat measure

- Message stream + composer both cap at **`--content-max`**, centered between the
  rails, with a `--reading-gutter` inset.
- A turn is a caption line over a full-width body, not an avatar gutter beside a
  narrowed one: who is speaking is read once, the measure is inhabited for the
  whole turn, and a 38px gutter was taking it from every code block and table.
- Long code blocks and tables can exceed the measure — they scroll horizontally
  inside their own wrapper; the prose column does not move.

### Tabs

- **No session tab strip.** One active session at a time (ChatGPT-style);
  switching is via the sidebar session list. A main workspace view opens
  full-pane, and closes by clicking its sidebar nav row again, pressing `Esc`
  (which yields to palette/dialog/input first), or the split-view control.
- **The dock has one.** Views opened beside the conversation share a strip of
  tabs: drag to reorder, a fade at each end that says the strip runs on, and
  close with the ×, with `Delete`/`Backspace` on the focused tab, or with a
  middle click. The × is not in the tab order — a focusable sibling inside a
  `tablist` is an unallowed child, and Delete on the focused tab is the ARIA
  practice for a closable tab.

### Spacing rhythm

Flame is a **product UI**, not a marketing site. Spacing comes from the `space` steps in
`tokens.stylex.ts` — every one a multiple of `--spacing` — and the `--density-*` tokens, but:

- Section breaks inside a panel: 16px to 24px.
- Card interior padding: 16px, 24px where the card is the point of the view.
- Inline gaps: 8px to 12px.
- Nothing reaches marketing-band spacing — no 96px section break, no 192px band —
  outside the welcome screen, where the page IS the band.
- Density controls navigation row heights, gaps and gutters only. Reading gutters
  and composer insets have independent static tokens. Conversation container
  queries determine reading insets, including when the dock narrows a wide window.
- New turns, final answers and same-run continuations have distinct spacing.
- The automatic dock starts at an auxiliary width and never persists a preference.
  Only user resizing stores a ratio. When the window cannot hold the dock beside
  the reading floor, the dock toggle opens the current material full width;
  navigation retains its target and the dock returns when space does.
- The composer prioritizes input, model and send. Reasoning effort lives on the
  selected model's row inside the model picker. Normal approval labels yield on narrow containers; high-risk approval
  remains explicit. Label length never controls a JavaScript layout mode.

## 5. Elevation & Depth

**Depth is value plus a half-pixel edge. Flush chrome casts nothing.**
Every seam between regions is carried by an inset 0.5px edge whose shape the visual
style owns — `--app-card-edge` at the drawer seam (drawn INSIDE the plane, because
the plane outranks the drawer on z-index so the drawer can slide under it),
`--app-pane-split` where the dock meets the conversation, `--app-header-edge` under
a bar. A call site never draws a region boundary. Resource surfaces use the shared contact shadow. Floating overlays (menus, popovers, tooltips, command palette, lightbox) use greater depth because they can land over any surface.

| Level | Treatment                                              | Use                                                                            |
| ----- | ------------------------------------------------------ | ------------------------------------------------------------------------------ |
| 0     | Region fill only                                       | The reading plane, prose, a message body                                       |
| 1     | `bg-card`                                              | Message card, tool card, composer, plan card, table                            |
| 2     | `bg-sunken`                                            | Code body, terminal, diff hunk, text field, progress track                     |
| 3     | `surface-2` / `-3`                                     | Chips, badges, kbd, selected rows — on chrome, not on the plane                |
| 4     | `--shadow-raised` / `-overlay` / `-popover` / `-modal` | Floating overlays only — one ladder, each rung `--shadow-ring` plus more depth |

Each role owns exactly ONE edge mechanism. A border and a shadow ring on the same
surface is a double edge; two 1px semi-transparent lines sharing a pixel double
their alpha and read as a bright dot.

**Row state is not on this ladder.** Hover and selection are `bg-hover` /
`bg-selected` — an ink wash (`--color-hover` / `--color-selected`), so a row
lights up the same over a card, a menu or the drawer, and selection stays legible
while the pointer sits on its neighbour. A surface step as a hover paints a slab
where there was none; `check-interactive-chrome` fails the build on both that and
a hand-picked `hover:bg-fg/[…]` alpha.

This holds identically in **both schemes**; the edge takes the scheme's
`border-soft`, while elevation uses the scheme's `--shadow-cast`.
Resource lists use one shared surface with separators. Forms and metric summaries
use bounded cards with a quiet contact shadow; workbench regions retain their seam treatment.

## 6. Shapes

### Radius scale

The visual style owns the ladder (`style-shape-*`); the user's radius preference
multiplies through.

| Semantic role           | Default                       | Use                                                     |
| ----------------------- | ----------------------------- | ------------------------------------------------------- |
| `button-radius`         | Pill                          | Buttons and icon buttons                                |
| `field-radius`          | 12px                          | Text, search and select controls at every size          |
| `mark-radius`           | 6px                           | Checkbox marks                                          |
| `row-radius`            | 10px                          | Navigation rows                                         |
| `surface-card-radius`   | 16px                          | Settings groups, resource lists, cards and inline forms |
| `floating-panel-radius` | 16px                          | Popovers, menus and dialogs                             |
| `floating-tip-radius`   | 8px                           | Tooltips                                                |
| `segmented-radius`      | 20px                          | Segmented tracks                                        |
| `menu-item-radius`      | Panel radius minus menu inset | Nested menu rows                                        |

Components consume semantic roles. The visual style maps those roles to the
shape ladder; resizing a field does not choose another radius. Card padding,
page gutters and grouping use the shared layout tokens. Nested menu corners
are derived from their panel and inset rather than authored independently.

### Corner curve

Every corner that is not a circle is a **superellipse**, not a circular arc —
`corner-shape: superellipse(1.5)`, which in the CSS parameterisation sits halfway
between `round` and `squircle`. This is the Codex corner, and it is what makes a
surface read as drawn rather than clipped.

The curve holds more material near the corner than an arc does, so the same
radius reads tighter. `--corner-scale: 1.25` buys that back on `md` and up; below
`md` the two curves differ by a third of a pixel, so those steps take the shape
and skip the scale. Curve and scale share one `@supports` — each engine gets a
ladder that agrees with itself.

`pill` opts out. A superellipse at 50% is a rounded square, and an avatar, a
status dot or the type caret is a circle.

WebKit has no `corner-shape` as of Safari 26.5, so the WKWebView the desktop app
ships in draws the arc at the base radius today.

### NEVER

- **No call-site corner values.** Buttons use the shared capsule role; fields use the 12px field role and resource groups use the 16px card role.
- **No mixed scales on one screen.** One corner language, one ladder, no step
  invented at a call site.

## 7. Motion

Durations and easings are the `--dur-*` / `--ease-*` tokens, all scaled by
`--motion-scale` so the Motion setting and `prefers-reduced-motion` reach every one.

One curve for everything that eases, `ease-out`, plus the sampled spring the
structural panels travel on. Two curves that nothing read were deleted: a ladder
rung a visual style must fill and no surface consumes is a rung that can only
ever be wrong.

- **Colour only** (hover wash, ink): `dur-color` 100ms.
- **Hover / press feedback, and most transitions**: `dur-fast` 150ms with `ease-out`.
- **A surface opening, closing or arriving** (modal, toast, palette, disclosure):
  `dur-med` 200ms. There is no separate disclosure rung — it sat 20ms from this
  one, a fifth of a frame, which is a distinction the ladder cannot express.
- **An exit**, which should be quicker than the enter that earned it: `dur-instant` 80ms.
- **Heavy transitions** (panel slide): `dur-slow` 300ms, and the drawer itself
  `dur-drawer` 500ms on `ease-drawer`.
- **Active press scale**: `active:scale-[var(--press-scale)]`. One value for the
  whole app — a control that sinks 0.90 next to one that sinks 0.98 reads as two
  different apps. Per-element amounts were tried and drifted to four of them.
- **`prefers-reduced-motion`**: all transitions degrade to ≤80ms, all scale animations disabled.

## 8. Components

A component's canonical spec is its own file in `ui/atoms` / `ui/agent`; the visual
vocabulary they share is what follows.

### Tool-call row — one line on the work narrative

A tool call is **a line, not a card**. It renders through
`AgentActivityDisclosure` at `shell="line"`: an identity glyph, the intent, an
optional mono detail, and trailing meta — a diff stat, a duration, a running dot.
No fill, no hairline, no radius, because it has no surface of its own.

Every invocation takes the neutral tone whatever its safety class or outcome. The
material result earns a surface only once the row is opened; colouring the
identity glyph would turn a failure or a refusal back into a status card, which is
the thing this shape exists to avoid.

An earlier revision spelled this as an RPC log entry inside card chrome. The
narrative line is what replaced it.

### No bottom status bar

There is no dense bottom data row. Run telemetry (tokens / cost / rate) lives in the **composer footer**; global status + notifications live in the **sidebar footer**. A persistent mono data strip read as "console" — the chrome stays calm.

### Reasoning block — mono header, no caps

Header was `THOUGHT FOR 1S` ALL-CAPS — now `thought · 1.2s` in `caption-mono` lowercase. Body italic stays.

### Shortcuts pane — auto-derived

There is no composer cheatsheet. A command that carries a combo is projected into
the `SHORTCUT` point, and `ShortcutsPane` renders that point — so a shortcut is
listed because a command declared one, never because a table was kept in step.

## 9. Accent Usage Policy (strict)

Accent (`--color-accent`, user-selectable) appears in:

2. **Focus ring** — `:focus-visible`, a single thin accent stroke (**no halo / glow**, and never on plain mouse-focus of inputs). One global rule in globals.css draws it for everything; mark `data-focus-inset` where it would land outside the box, `data-chrome-focus` where a row fills instead. A theme retunes it through `--color-focus-ring`. Never drawn at a callsite — `check-interactive-chrome` fails the build
3. **Live indicator** — the running `StatusDot`, the status pill while a run is
   live, the reasoning block's pulse dot
4. **Selection and control fills** — the check on a chosen menu item, a slider or
   switch that is on, a progress indicator, an active step marker

That's the entire list. Accent does **not** appear in:

- Avatar backgrounds (use `surface-2/3` + `ink-muted`)
- Section headers (use `ink`)
- Active-state list rows (use `surface-2/3` + `ink`)
- Ordinary iconography — an icon that is only an affordance is `ink-muted` → `ink` on hover. An icon that carries STATE is item 4.
- Tool-call success status (use `success`, not the accent)
- Input / composer focus (a quiet border strengthen — no accent ring)

When in doubt: **does this surface convey "the agent is alive and live"?** If yes, accent. If no, grey.

## 10. Do's and Don'ts

### Do

- Render IDs / durations / file paths / tool signatures in mono; labels, headings + names in **sans**.
- Cap chat content (message stream + composer) at `chat-measure: 760px`, centered.
- Use semantic border and divider tokens; resource separators stay quieter than control boundaries.
- Set every interactive element with `:hover`, `:active`, `:focus-visible`.
- Use `font-feature-settings: "tnum"` on every numeric display.
- Default the Work Index to **expanded** (275px); collapse it fully off-canvas on
  demand (⌘B), while keeping one recovery control in the content header.
- Render tool calls as RPC logs (mono signature + duration line — the one place mono stays).
- Pair display weight 600 with body weight 400. Hierarchy via size + weight contrast, never weight 700+.

### Don't

- **Don't use ALL-CAPS labels with letter-spacing.** Section labels / eyebrows / table heads are **sentence-case** (mono for dense technical labels like `args` / `attrs`); the ALL-CAPS + wide-tracking eyebrow is the rejected Sonance vocabulary.
- **Don't assign button geometry at call sites.** Shared controls own the capsule treatment through `--button-radius`.
- **Don't use weight 700+ for display.** 600 is the ceiling, Linear and Vercel both forbid this.
- **Don't add call-site shadows.** Workbench regions stay flush. Resource groups and forms use the shared contact-shadow token; overlays use their elevation role.
- **Don't hardcode canvas colors at call sites.** The default cool neutral canvas follows the HeroUI palette through `--color-bg`.
- **Don't flash a bright accent ring/halo on focus or click.** Keyboard focus is one thin stroke; inputs/composer just strengthen their border. The loud glow read as cheap.
- **Don't introduce a second chromatic accent.** Flame has one accent + four semantic colors. No more.
- **Don't use accent decoratively.** Focus ring / selected control / live indicator — that's the entire allowed list.
- **Don't set body paragraphs in mono.** Mono is for the technical layer only.
- **Don't apply atmospheric gradients, mesh backdrops, or dot grids** (the latter was discussed and rejected — Linear explicitly forbids "atmospheric gradients or spotlight cards").
- **Don't add a fourth glass surface.** Blur is material, not decoration, and it belongs to exactly three tokens — `--floating-backdrop`, `--composer-backdrop`, `--composer-tray-backdrop` (see `DESKTOP_UI_POLISH.md` §Glass). ChatGPT blurs its own composer surface the same way. A raw `backdrop-blur-*` at a call site is how a fourth appears.

## 11. Light theme

Light is full parity, not second-class — and the **default theme follows the OS**
(`prefers-color-scheme`, live). It runs the same region algorithm mirrored: the
canvas is off-white, cards, fields and resource groups are white with quiet semantic boundaries. Dark cards lift above their near-black canvas. Values live in `themes/flame-light.ts`.

Two places where light is not a mechanical inversion, both for the same reason —
ink cannot be:

- **Semantic hues sit one step deeper** than the reference language's. Its greens
  and ambers land at 3.4–3.9:1 as text on this chrome, and a status word nobody
  can read is not a status. Hue family preserved, luminance pulled until each
  clears 4.5:1 on the darkest surface it can sit on.
- **The accent is the deeper blue.** Same reason; the accent carries link text.

## 12. References

- **ZCode** — plugin page composition, grouped resource rows, control proportions,
  supporting text hierarchy and desktop resource density.
- **HeroUI 3.2.6** — default cool neutral palette, blue primary actions, capsule buttons, rounded fields, layered fills and contact shadows. Adaptations retain 24/28/32/36px desktop control roles, native cursors, 3:1 field boundaries and AA text contrast. The application theme owns token values; `@flame/ui` owns their rendering.
- **ChatGPT / Codex** — conversation reading measure and tool disclosure behavior.
- **JetBrains tool windows** — the surrounding workbench region model.
- Flame Runtime Protocol — `src/rpc/` — drives the shape of the data this UI
  renders.

## 13. Iteration guide

1. When adding a new surface, build it from the rings in `ui/` rather than at the
   call site; if the vocabulary has no rung for it, add the rung and say so here.
2. Verify BOTH schemes (the default follows the OS) before merging visual changes.
3. Batch related layout and token changes, then run the repository gates once.
4. Inspect actual rendered pages in both themes, at narrow widths and increased text size. Screenshot baselines are not an acceptance criterion. Use the inert browser fixtures for review; native packaging needs separate host verification.
5. Reserve accent for primary actions, active controls and meaningful status; use neutral fills for supporting actions.

## Responsive document and shared renderers

Native window geometry belongs to the Wails host. The HTML document does not
enforce those physical minimum dimensions: browser resizing and text zoom can
reflow content independently. At narrow widths, work navigation precedes the
reading pane and Settings navigation precedes its content; each scroll region
retains keyboard access. Setting rows and segmented choices reflow inside their
actual container without changing saved appearance or navigation preferences.

First-party Tool results and Markdown renderer controls are compiled from
`@flame/ui`, including document styles and the Host's appearance projection.
Recorded-content and Markdown grammar remain local renderer responsibilities.
Hover icon changes use a short opacity crossfade; they do not blur or shrink
on routine pointer movement. Reduced motion removes control and overlay movement.

Shared field, menu, card, and choice surfaces are authored in `@flame/ui`; host
atoms consume those skins instead of restating colors, radii, and edges. The
visual-style owner supplies semantic `layout-*` tokens for page measure, gutters,
card padding, and grouping. Segments and navigation hover tracks use the same
shared spring backplate, with current timing and motion scale projected from
the theme owner.

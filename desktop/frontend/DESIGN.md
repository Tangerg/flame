# Flame visual design

The visual specification for the desktop app: what each surface, colour, type
step, shape and motion token is FOR.

**It does not restate values.** Those live in `src/styles/globals.css` and
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

1. **Tool windows around one reading plane** (revised 2026-08; supersedes both
   "flush background delta" and "card over drawer"). Three opaque materials — the
   plane you read on, the chrome columns that frame it, the cards placed on it —
   separated by VALUE, with **a single device pixel** over that step at each seam.
   The plane is the darkest surface on dark and the brightest on light; the chrome
   steps the other way. Both halves are load-bearing: the step alone measured too
   small to read, and a line alone draws the columns as a wireframe of pasted
   rectangles. Which mechanism draws which boundary belongs to the active visual
   style, not to any call site — this one spells its three seam tokens as hairlines;
   a spatial style spells the same three as casts and nothing else changes.
2. **Near-monochrome, one restrained accent** — overall black/white/grey; the
   accent (a calm **blue**, user-selectable) marks live state, progress, focus,
   links, and the one primary action per surface. It is the CTA fill too: this
   language spends its single colour on the action that matters and leaves
   everything else grey.
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

**Reference** — the direction is the JetBrains tool-window language: an editor you
are *inside*, framed by opaque panels, with the technical layer set in mono.

- **Region model**: three materials, each seam a half-pixel hairline over a value
  step. The reading plane is the one surface that is not chrome.
- **Density**: short chrome bars, two-line index rows, borderless cards.
- **Voice**: sans for language, mono for data — and the mono is load-bearing, not
  decorative, because most of what an agent transcript reports IS data.

**Explicitly rejected** (both prior passes):
- Region hairlines and seam rings (regions separate by value + cast now)
- Cards-on-canvas gutters, panel drop shadows, and glass blur outside floating panels
- An inverting ink CTA that kept the accent unused (the accent IS the CTA)
- Pill-radius CTAs, ALL-CAPS letter-spaced labels, 700+ display weight
- Bright focus halos/glows that flash on click (focus is a single quiet stroke)

## 2. Color

### Philosophy

Color carries information, not decoration. The system uses **one chromatic accent**, **four greys for surfaces**, **three greys for hairlines**, and **four semantic colors used sparingly**. Decoration comes from the surface ladder, not from color variation.

### Surface anchors

Values live in `themes/flame-*.ts` and are restated for first paint in
`globals.css`. This table says what each anchor is FOR; it deliberately does not
repeat the hexes, which is how the previous version of it went stale.

| Token | Role |
|---|---|
| `canvas` (`--color-bg`) | The reading plane — transcript, view bodies. Darkest surface on dark, brightest on light. |
| `surface` | Region chrome — the drawer, the dock, the bars that frame the plane. |
| `card` (`--app-card-surface` → `--color-elevated`) | An object placed on a region: a message, a tool card, the composer. |
| `sunken` (`--color-sunken`) | A well cut into a surface: code bodies, terminals, diff hunks, text fields, progress tracks, and inline code in prose. |
| `surface-2` / `-3` / `-4` | Derived chip rungs above `surface` — badges, kbd, selected rows, resting control fills. Mixed out of the CHROME grey, so they belong on chrome; on the plane they read as grime. |

**Why four anchors and not one ladder.** The reading plane is the extreme of its
scheme — pure white on light, near-black on dark — and an object on it steps IN,
toward the chrome. On light that reads as a ladder: every region steps down from
white. On dark it cannot, because the WELL still goes the other way — a card lifts
UP off the plane while a code body recedes BELOW it. One monotonic mix cannot say
both, so `elevated` and `sunken` stay anchors.

A card used to be spelled `#ffffff` over an off-white plane, which is the same
delta pointing the other way — 1.2 L with zero chroma, so the object was held up
entirely by its cast. Stepping in instead gives it a value AND a hue of its own.

**One hue, chroma by area.** Every neutral sits on the accent's hue, and carries
chroma in inverse proportion to the area it covers: the plane none, the chrome
~0.006, a card ~0.008, a well ~0.016 (dark: 0.008 / 0.010 / 0.015). Under roughly
C 0.005 a grey's hue is not addressable in 8-bit sRGB — one byte swings it 20–40° —
so a near-neutral ramp cannot pick its own hue, and a hue nobody chose is what
"dirty grey" means. Chroma is what makes the set read as one material family;
keeping it low on the large areas is what keeps the same decision from reading as
a blue tint.

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

**Ink, by contrast, may derive.** Unlike hairlines, the ink ramp (`text-soft` / `text-muted` / `text-faint`) *should* adapt to the surface behind it — that's the Apple label model. A theme can ship just `text` + `text-bright` and let the soft/muted/faint steps derive as `text` at ~82% / ~56% / ~38% alpha over transparent (so they composite against whatever surface they sit on). Palette themes (Solarized, Catppuccin, Tokyo Night, One Dark) instead pin explicit ink hues — their ramp is part of the palette identity, not a single hue at falling opacity. The first-party Flame themes keep explicit values too; the derivation is the low-friction default for third-party themes.

### Accent policy

The single accent (`--color-accent`, a calm blue by default, user-selectable with
green / pink / orange as alternates) is reserved for **exactly four surfaces**:

1. Active tab indicator (2px underline on `chat-tab.active`)
2. Primary CTA fill (`button-primary`, Send button)
3. Focus ring (`:focus-visible` — a single thin stroke, **no halo / glow**; one global rule, never drawn at a callsite)
4. Live indicator (streaming dot, running pill, `tab-dot.running`)

Forbidden surfaces for accent: section background, card fill, avatar background, decorative borders, status icons that are not "live". And **no bright accent ring on input focus or click** — inputs/composer strengthen their border quietly instead (the loud halo read as cheap).

"Card fill" here means the accent as a **colour**. The surface anchors sitting on
the accent's *hue* at C ≤ 0.016 is the neutral algorithm above, not accent usage:
at that chroma nothing reads as blue, and the alternative is not a purer neutral
but an unchosen one.

### Semantic palette

| Token | Use |
|---|---|
| `--color-success` | Run finished cleanly, action confirmed. Allowed in: run pill (idle/done), tab dot after success. |
| `--color-warning` | User attention required. Allowed in: the approval card, the waiting tab dot. |
| `--color-negative` | Error. Allowed in: the run error banner, a tool call's failed status. |
| `--color-info` | Information / link. Allowed in: inline links, info badges. |

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

**Semantic colours are scheme-tuned**, and in both directions. Dark lifts them in
luminance and pulls the chroma down so they neither vibrate nor edge-bleed on a
near-black plane; light pushes them the other way, DARKER and deeper than the
saturated web reds and blues, because on white it is contrast against the plane
that decides legibility, not brightness. Palette themes (Catppuccin / Tokyo Night
/ Solarized / One Dark) ship their own canonical semantic tones and are left
untouched.

## 3. Typography

### Font families

Two bundled variable faces, each in front of a native fallback chain so the app
has one shape on every machine and still renders mixed CJK with the OS:

- **Sans** (`--font-sans`) — **Geist**, then `-apple-system` / `BlinkMacSystemFont` / `system-ui`, with **PingFang SC** (+ Hiragino / Microsoft YaHei) for CJK. The primary UI face; display + body share it, weight does the hierarchy.
- **Mono** (`--font-mono`) — **JetBrains Mono**, then `ui-monospace` / SF Mono / Menlo. Genuine data only: code, IDs, timestamps, file paths, tool signatures.
- A single `--font-sans` / `--font-mono` token (no `--font-ui` split); the user can override either in Settings → Appearance.

Same shape as the reference, which bundles `OpenAI Sans` in front of
`-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`. It bundles **no**
mono, though: its code voice is `ui-monospace, SFMono-Regular, SF Mono, Menlo,
Consolas, monospace`, and Flame's is JetBrains Mono.

### Scale

The full scale is nine sizes — four UI steps, prose, code, and three display steps. Display sizes are smaller than typical marketing systems because Flame is a product UI, not a hero page.

Sizes, weights and tracking are the `--fs-*`, `--fw-*` and `--tracking-*` tokens
in `globals.css`; `check:tokens` fails the build on a per-callsite pixel value.

### Principles

1. **Sans-first; mono is the _data_ voice only.** Labels, section headings, nav, speaker names, view titles + subtitles are **sans**. Mono (`caption-mono` / `code`) is reserved for genuine data — IDs, durations, timestamps, file paths, tool-call signatures. (The earlier "mono as eyebrow everywhere" read as an engineering console; pulled back.)

2. **600 is the ceiling, and it is the heading weight.** The ladder is three
   rungs — `--fw-regular` 430, `--fw-medium` 500, `--fw-semibold` 600. 700 and up
   are out, and nothing in the app reaches for one. 600 carries titles and section
   headings and the HITL action button; below that, hierarchy comes from size and
   the 430/500 contrast rather than more weight. Both references land the same
   way: Codex sets its heading at 600 over a 430 body, and 600 is the single most
   common weight in ChatGPT's stylesheet.

3. **Display gets negative tracking.** Three editorial steps — 18 / 20 / 24 — matching Codex's heading ladder; the app has no fourth, so there is no token for one. Body holds at -0.05 to 0.

4. **Sentence-case headlines.** Never ALL-CAPS. Welcome screen, settings sections, view headers — all sentence-case. Optional period termination is allowed (Vercel signature) but not required.

5. **Tabular numerals everywhere numeric.** `font-feature-settings: "tnum"` on caption-mono and code by default. Numbers don't jitter when counters update.

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
       ↑          ↑ turn rail (44)          outline rail (186) ↑ dock casts
   --sidebar-width  --app-card-edge: the drawer's cast,          leftward
   (275, 240–520) drawn inside the plane                  (--app-pane-split)
```

Both rails are container-query gated on the width of the reading column — not the
window — because the drawer and the dock change it without the window changing at
all. Banners and composer take the same gutters, so the three stay on one axis.

### Sidebar

- **Default state: expanded** (`--sidebar-width`, 275px, user-resizable by dragging
  the seam rail; floor 240px, ceiling 520px, and the live clamp always leaves at
  least 240px for the reading plane). The Context Dock keeps its separate 640px
  conversation floor; the two flanks do not share one clamp.
- **Pinned identity** above the scrolling index: the active session's workspace,
  because the one fact you must be able to read without scrolling is where the next
  command will run.
- **Two-line session rows**: title, then state and time — the index is something you
  triage from, not just a list of names.
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
  seam as an inset cast, because the plane outranks the drawer on z-index so the
  drawer can slide underneath it.

### Chat measure

- Message stream + composer both cap at **`--content-max`**, centered between the
  rails, with a `--density-column-gutter` inset.
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

Flame is a **product UI**, not a marketing site. Spacing comes from the Tailwind scale and the `--density-*` tokens, but:

- Section breaks inside a panel: 16px to 24px.
- Card interior padding: 16px, 24px where the card is the point of the view.
- Inline gaps: 8px to 12px.
- Nothing reaches marketing-band spacing — no 96px section break, no 192px band —
  outside the welcome screen, where the page IS the band.
- The chrome's own rhythm is not free-hand: row height, gutters and composer
  insets are `--density-*`, a third axis beside type and shape, so the Density
  setting moves all of them together.

## 5. Elevation & Depth

**Depth is value plus a directional cast. Flush chrome casts nothing.**
Every seam between regions is carried by a short, tight cast from the panel that
overlaps — `--app-card-edge` at the drawer seam (drawn INSIDE the plane, because
the plane outranks the drawer on z-index so the drawer can slide under it),
`--app-pane-split` where the dock meets the conversation. No region anywhere
carries a border. The only elements with a real drop shadow are **truly-floating
overlays** (menus, popovers, tooltips, command palette, lightbox), which have no
value delta to lean on because they can land over anything.

| Level | Treatment | Use |
|---|---|---|
| 0 | Region fill only | The reading plane, prose, a message body |
| 1 | `bg-card` | Message card, tool card, composer, plan card, table |
| 2 | `bg-sunken` | Code body, terminal, diff hunk, text field, progress track |
| 3 | `surface-2` / `-3` | Chips, badges, kbd, selected rows — on chrome, not on the plane |
| 4 | `--shadow-raised` / `-overlay` / `-popover` / `-modal` | Floating overlays only — one ladder, each rung `--shadow-ring` plus more depth |

Each role owns exactly ONE edge mechanism. A border and a shadow ring on the same
surface is a double edge; two 1px semi-transparent lines sharing a pixel double
their alpha and read as a bright dot.

**Row state is not on this ladder.** Hover and selection are `bg-hover` /
`bg-selected` — an ink wash (`--color-hover` / `--color-selected`), so a row
lights up the same over a card, a menu or the drawer, and selection stays legible
while the pointer sits on its neighbour. A surface step as a hover paints a slab
where there was none; `check-interactive-chrome` fails the build on both that and
a hand-picked `hover:bg-fg/[…]` alpha.

This holds identically in **both schemes**; only the cast's strength differs, and
that is one palette value (`--shadow-cast`) rather than a per-component decision.
(Both earlier models are gone: cards-on-canvas with gutters and multi-layer drops,
and the 2026-07 seam-ring pass that gave every boundary a hairline.)

## 6. Shapes

### Radius scale

The visual style owns the ladder (`style-shape-*`); the user's radius preference
multiplies through.

| Token | Value | Use |
|---|---|---|
| `none` | 0px | Full-bleed bars |
| `2xs` | 2px | Small marks inside a control — key caps, checkboxes, swatches |
| `xs` | 4px | Anything that is really a tag — badges, inline code |
| `sm` | 6px | Controls: buttons, chips, index rows, dock tabs |
| `md` | 8px | Cards, text fields, segmented tracks |
| `lg` | 10px | Blocks inside the conversation: code, diagrams, images, banners |
| `xl` | 12px | Every floating panel and modal, through `--floating-panel-radius` |
| `bubble` | 16px | The user's own message, and the cards that answer it |
| `composer` | 20px | The composer and the surfaces that echo it |
| `pill` | 9999px | Circles and lozenges: dots, tracks and thumbs, the status pill, circular icon wells, the count badge, a selected choice row |

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

- **No pill-radius CTAs.** The action button is a rounded square on the control
  ladder — a lone disc beside a row of rectangles reads as a different kit.
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

1. **Primary CTA fill** — `bg-cta`: the send button, an accent `PillButton`
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
- Use literal hex hairlines — not `color-mix(text X%, transparent)`.
- Set every interactive element with `:hover`, `:active`, `:focus-visible`.
- Use `font-feature-settings: "tnum"` on every numeric display.
- Default the Work Index to **expanded** (275px); collapse it fully off-canvas on
  demand (⌘B), while keeping one recovery control in the content header.
- Render tool calls as RPC logs (mono signature + duration line — the one place mono stays).
- Pair display weight 600 with body weight 400. Hierarchy via size + weight contrast, never weight 700+.

### Don't

- **Don't use ALL-CAPS labels with letter-spacing.** Section labels / eyebrows / table heads are **sentence-case** (mono for dense technical labels like `args` / `attrs`); the ALL-CAPS + wide-tracking eyebrow is the rejected Sonance vocabulary.
- **Don't use pill-radius CTAs** (`9999px`, `500px`, `100px` on a button). Buttons are `sm`, through `--button-radius`.
- **Don't use weight 700+ for display.** 600 is the ceiling, Linear and Vercel both forbid this.
- **Don't add panel / card drop shadows.** The layout is flush — depth is the surface step + hairlines. Stacked-subtle shadow is for truly-floating overlays (Level 4) only, in BOTH schemes. No cards-on-canvas, no gutters.
- **Don't use pure `#000000` or a harsh near-black canvas.** Dark canvas is `--color-bg`, a soft grey, not a black.
- **Don't flash a bright accent ring/halo on focus or click.** Keyboard focus is one thin stroke; inputs/composer just strengthen their border. The loud glow read as cheap.
- **Don't introduce a second chromatic accent.** Flame has one accent + four semantic colors. No more.
- **Don't use accent decoratively.** Active tab / primary CTA / focus ring / live indicator — that's the entire allowed list.
- **Don't set body paragraphs in mono.** Mono is for the technical layer only.
- **Don't apply atmospheric gradients, mesh backdrops, or dot grids** (the latter was discussed and rejected — Linear explicitly forbids "atmospheric gradients or spotlight cards").
- **Don't add a fourth glass surface.** Blur is material, not decoration, and it belongs to exactly three tokens — `--floating-backdrop`, `--composer-backdrop`, `--composer-tray-backdrop` (see `DESKTOP_UI_POLISH.md` §Glass). ChatGPT blurs its own composer surface the same way. A raw `backdrop-blur-*` at a call site is how a fourth appears.

## 11. Light theme

Light is full parity, not second-class — and the **default theme follows the OS**
(`prefers-color-scheme`, live). It runs the same region algorithm mirrored: the
plane is the brightest surface, the chrome steps down from it, cards lift to
white, wells recede. Values live in `themes/flame-light.ts`.

Two places where light is not a mechanical inversion, both for the same reason —
ink cannot be:

- **Semantic hues sit one step deeper** than the reference language's. Its greens
  and ambers land at 3.4–3.9:1 as text on this chrome, and a status word nobody
  can read is not a status. Hue family preserved, luminance pulled until each
  clears 4.5:1 on the darkest surface it can sit on.
- **The accent is the deeper blue.** Same reason; the accent carries link text.

## 12. References

- **ChatGPT desktop** — the authority. Where its answer and this document's
  disagree, its answer is the one to explain away, not the other. Its own
  stylesheet is readable evidence: heading weight, thread measure, composer
  material, glass.
- **Codex** — the corner shape, the heading ladder, the body weight.
- **JetBrains tool windows** — the region model: an editor you are inside, framed
  by opaque panels, separated by value rather than by lines.
- **Linear-app** — the scarce single-accent policy and sentence-case labels.
- Flame Runtime Protocol — `src/rpc/` — drives the shape of the data this UI
  renders.

## 13. Iteration guide

1. When adding a new surface, build it from the rings in `ui/` rather than at the
   call site; if the vocabulary has no rung for it, add the rung and say so here.
2. Verify BOTH schemes (the default follows the OS) before merging visual changes.
3. Run `npx tsc --noEmit && npx vitest run` after any token change.
4. Visually verify in `wails3 dev` — type/spacing changes especially.
5. Treat the accent as scarce: ask "is this live?" — if no, use grey.

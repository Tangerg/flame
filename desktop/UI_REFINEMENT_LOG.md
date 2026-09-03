# UI refinement log

Rounds of the audit → plan → implement → verify → reclaim loop described in
`refactor-prompt.md`. Each round records the evidence that justified a change,
not a summary of the change itself: the diff is in git, the reason is here.

Evidence is gathered against the production components rendered by the
`visual/` fixtures at `http://127.0.0.1:4174/visual/`, 1120×720, DPR 1,
`en-US`, reduced motion — the same conditions the Playwright suite photographs
in, so a screenshot taken here and a golden taken there are comparable.

---

## Round 1 — the transcript outline, the composer's narrow floor, three
## presentation slips

Status: **complete**

### Audit scope

- `axe-core` (WCAG 2.0/2.1/2.2 A + AA + best-practice) across every fixture
  state of the `agent`, `shell` and `workspace` fixtures, in both themes —
  62 renders — plus a second pass over the interactive surfaces axe cannot
  reach from a static render: the four composer menus, the settings theme
  menu, the question card, and the HITL request.
- Hit-area and computed-style probes of every focusable element on the agent
  surface.
- Geometry probes of the sidebar rows, the settings font row, and the composer
  footer at the narrowest content pane the shell allows.
- Screenshots of `running`, `waiting`, `error`, `question`, `tool-shells`,
  `long-content`, `terminal` (dark), `shell/populated`, `workspace/dock-review`
  and `workspace/settings`.

### Findings

| # | Problem | Evidence | Root cause |
| --- | --- | --- | --- |
| 1 | The transcript's heading outline is invalid: a turn is labelled `<h4>` while the model's own markdown emits `<h1>`–`<h6>`, so content headings outrank the turn that contains them. | axe `heading-order`, moderate, `agent/narrative`: `div[data-turn-id="item_n_ask3"] > … > h4`. DOM outline reads h4, h4, h4, **h2**, h2, h4, h4, h4, h2. | Two owners pick heading levels independently. Nothing maps the authored level of a message body onto the level of the turn that holds it. |
| 2 | At the narrowest content pane the model chip collapses to an 18 px sliver whose icon and chevron overflow their own button onto the neighbouring chip. | Footer at its 312 px floor: model chip `width: 18`, label `0`, its own contents ≈ 56. Screenshot `composer-min.png`. | `AgentComposerChip` sets `min-w-0`, which lets flex shrink the button below its own controls; the leading glyph carries no `shrink-0` of its own. |
| 3 | Two of the three composer chips give the user no way to read a truncated value: the approval chip has no `title` at all, and the reasoning chip's `title` names the action ("Switch reasoning effort") rather than the value. | DOM probe of the three chips at 423 px. | The label→tooltip relationship is a per-callsite habit, not a contract of the chip. |
| 4 | Settings → Font → Size renders the word **Default** in JetBrains Mono. | Computed `font-family` on the segment: `"JetBrains Mono", ui-monospace, …`. | `Segmented`'s `mono` prop, used at exactly one callsite, applies a code typeface to a whole control so that its *digits* line up — digits `body { font-feature-settings: "tnum" 1 }` already lines up. |
| 5 | The run error banner puts a raw protocol enum in its localized headline at title weight and title colour: "Agent error · provider_rejected", "Erreur d'agent · provider_rejected". | `RunErrorBanner.tsx` concatenating `error.code` into `t("runError.title")`. Screenshot `agent-error.png`. | The banner has no representation for a machine identifier, so the code borrowed the headline's. Everywhere else in the app a machine id is muted mono. |
| 6 | A goal the Runtime will not resume shows no reason: the resume control silently disappears and the row still reads "Paused goal". | `goal.stop` is read in exactly one place in the whole tree — the predicate that removes the button. The reason itself reaches no pixel. Screenshot `agent-terminal.png`. | The fact that explains the missing control is consumed as a boolean and discarded. |

### Deliberately not changed

- **`page-has-heading-one`** (axe, best-practice, every state). The shell has no
  `h1` because it has no page title — the header carries a breadcrumb. Giving
  the window a document title is a shell ownership question, not a transcript
  one; queued for a later round rather than answered with an `sr-only` string
  bolted to the nearest component.
- **`region`** (axe, best-practice) on `.agent-seam-rail`. The resize rail is a
  control that belongs to the seam between two landmarks; putting it inside
  either one misdescribes it.
- **22–28 px controls.** `refactor-prompt.md` rule 6 asks for 40×40 px hit
  areas. The composer footer holds five controls in 312 px at its floor;
  40 px targets there would overlap, which the same rule forbids. The desktop
  ladder (`--control-height-xs/sm/md/lg` = 22/26/30/34) is the shipped answer
  for a fine pointer, and `globals.css` already promotes every control to 44 px
  under `@media (pointer: coarse)`. Recorded as a resolved conflict, not an
  oversight.
- **The sidebar row's fade against its trailing dot.** Measured: label box ends
  at 240, the attention dot's box starts at 240, and the mask reaches
  transparent exactly at the seam. No overlap.

### What changed

| | Before | After |
| --- | --- | --- |
| Turn heading | `<h4 class="sr-only">Assistant</h4>` | `<h2 …>` — the shallowest rung of the transcript |
| Message markdown | authored `#`…`######` rendered as `h1`…`h6` | rendered `h3, h3, h4, h5, h6, h6`, each carrying `data-md-level` with the authored level |
| Markdown heading type scale | keyed on the tag (`.md h1 { … }`) | keyed on `data-md-level`, so the size still follows what the model wrote |
| Chip layout | `inline-flex` + `min-w-0`, leading glyph shrinkable | 3-track grid `auto minmax(0,auto) auto`, so only the label may shrink and the chip's floor is its own glyph + chevron |
| Chip tooltip | model chip only | every chip; `title` defaults to the label, the model chip still appends its provider |
| Font size segments | `font-mono` on the whole control | none — the digits were already tabular from `body { font-feature-settings: "tnum" }`, and "Default" is a word |
| `Segmented` | `mono?: boolean` | prop removed (its only caller was the one above) |
| Error headline | `Agent error · provider_rejected`, all semibold red | `Agent error` semibold red + `provider_rejected` in muted mono, selectable |
| Goal row, refused resume | `Paused goal` and a control that quietly vanished | the refusal itself: `Cost budget reached` |

Breaking change: `Segmented`'s `mono` prop is gone. One caller, migrated in the
same commit.

### Verification

Commands, from `desktop/frontend`:

- `npm run typecheck`, `lint`, `format:check` — clean.
- `npm run knip`, `check:circular`, `check:contexts`,
  `check:published-boundaries`, `check:layers`, `check:port-surface`,
  `check:style-invalidation`, `check:design-system`, `check:tokens`,
  `check:styles`, `check:chrome`, `check:locales` (1064 keys × 8 locales),
  `check:lookup-tables`, `check:bootstrap`, `check:utilities`,
  `check:bundle` — all clean.
- `npm run test` — 2324 passed, 8 failed. All eight are outside this scope and
  predate the round: two are `runtime/contract`'s own
  `segment.finished.json` sample failing its own generated validator
  (`contextTokens` missing), six are `runtime-http.e2e` against the live Go
  runtime.
- `npm run visual:test` — 387 passed, 4.5 min. One golden pair
  (`agent-{light,dark}-terminal`) regenerated for the goal row's new text;
  every other golden matched unchanged, which is the evidence that the chip's
  grid rewrite is pixel-identical at normal widths.
  `closing tabs selects a neighbor` failed once under two workers and passed on
  a clean re-run of the whole suite — a flake, not a regression.
- axe re-run over the same 62 renders: `heading-order` gone; only `region` and
  `page-has-heading-one` remain, both recorded above as deliberate.
- Browser checks at 1120×720, light and dark, with the dock dragged across its
  full travel (content pane 460 → 400 → 352 px): footer stays one row
  (`clientHeight` 34) and never overflows (`scrollWidth == clientWidth`) at
  every width.

Screenshots in `/tmp/uiaudit/`: `composer-min.png` → `after-composer-min.png`,
`agent-error.png` → `after-error.png`, `after-goal-tray.png`,
`after-settings-size.png`.

### Reclaimed

Visual dev server on 4174 stopped; the round's probe scripts
(`_audit_*.mjs`, twelve of them) deleted from the package root; no test-results
or cache directories added.

### Open, for the next round

1. **A chip in mid-shrink still shows a two-character stub** (`Balanc…`).
   Measured cause: the label box is sized exactly to its text, so a 0.05 px
   loss is enough for the ellipsis to eat two characters. Neither of the two
   pure-CSS answers works — `flex-shrink: 0` on the holders overflows the
   312 px floor by 78 px, and letting the row wrap puts the send button alone
   on a second line at the *default* dock width. A real fix needs the footer to
   drop labels below a measured threshold; a px breakpoint is locale-fragile,
   so it wants measurement, not a container query.
2. **`page-has-heading-one`** — the shell has no document title.
   `SettingsPage` and the empty `ChatStream` each render an `h1`; a populated
   transcript renders none. Decide what names the window.
3. Prose claims in `ARCHITECTURE.md` §7.3 and §5.5 are still unverified against
   the code.

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

---

## Round 2 — twenty-one error states with no way out

Status: **complete**

### Audit scope

Loading, empty and error states of the workspace views, plus the interaction
states around them. Screenshots of `dock-loading`, `dock-error`, `dock-empty`,
`dock-inbox`, `dock-stats`, `dock-catalog` at the canonical 1472×900. DOM
probes of the dock's live regions, hidden panels, and toolbar geometry across
all four diff states.

### Findings

| # | Problem | Evidence | Root cause |
| --- | --- | --- | --- |
| 1 | Every error state in the app is terminal. Nothing retries and nothing offers to. | `DataView` is the single loading/empty/error owner and has **21 callsites**; none can render a recovery action. `queryClient` sets `retry: 1`, `refetchOnWindowFocus: false`, `staleTime: 60_000` — after two failures nothing refetches, so the only way back is to unmount the view and return to it. | `EmptyState` has had an `action` slot since it was written, used by **zero** callsites, and `DataView`'s `EmptyConfig` never exposed it. |
| 2 | The diff and file views draw a failure with the same glyph as their own empty result, so error and empty are the same picture and differ only in wording. | Screenshots `r2-error.png` / `r2-empty.png` — identical layout, identical neutral circular glyph. | `DataView` defaults the error glyph to `alert` and then spreads the caller's config over it. Four of its callsites overrode it; two of those were errors wearing the empty icon. |
| 3 | "The Runtime does not implement this" was being smuggled through the error slot with a hand-picked glyph. | `RulesRow` and `filetree` both pass `isUnsupportedMethod(error) ? {icon: …} : undefined` as `error`. | The triad had no name for a capability gap, so callsites disguised one as a failure — and a retry would have been offered for something retrying cannot fix. |
| 4 | The diff's failure copy named an RPC method and pointed at a destination it gave no way to reach: "The runtime rejected `workspace.diff.get` — see Diagnostics." | Locale catalogues, all eight. | Same shape as round 1's error headline: a protocol identifier used as prose, in a string no locale can translate. |
| 5 | "Retry" was written twice. | `runError.action.retry` beside no `common.retry`. | — |

### What changed

| | Before | After |
| --- | --- | --- |
| `DataView` error | message only | message + one standard `Retry`, wired at **all 21 callsites** |
| `DataView` error glyph | `icon` overridable per callsite | `error?: Omit<EmptyConfig, "icon">` — the alert glyph is the owner's |
| Capability gap | disguised as an error with a custom icon | its own `unsupported?: EmptyConfig`, checked before `isError`, with no retry |
| Diff failure copy | `The runtime rejected workspace.diff.get — see Diagnostics.` | `The runtime rejected the request.` |
| Retry label | `runError.action.retry` + nothing shared | one `common.retry`; the run banner uses it too |
| `WorkIndex` / `useWorkspaceDiffView` | state only | plus `retry`, so the sidebar and the review panel can serve the button |

Breaking change: `DataView`'s `error` no longer accepts `icon`; the two
callsites that were using it to draw a failure as an empty result are corrected
and the two that were describing a capability gap moved to `unsupported`.

### Deliberately not changed

- **`HooksPane`'s early return** for an unsupported Runtime. It replaces the
  whole pane rather than the list, which is right: rendering the `unsupported`
  notice inside `DataView` would leave a trust toggle on screen for a capability
  that does not exist.
- **The 26 px retry button.** Same resolved conflict as round 1: it is
  `--control-height-sm`, the ladder every other `size="sm"` action uses, and
  `@media (pointer: coarse)` still promotes it to 44 px.

### Verification

- `npm run typecheck`, `lint`, `format:check` — clean.
- `knip` and the fifteen architecture/style gates — clean.
- `npm run test` — 2329 passed, the same 8 out-of-scope failures as round 1.
  Five new `DataView` tests pin the contract: the retry fires, the failure glyph
  survives a caller's own title, `unsupported` wins over `isError` and offers no
  retry, no retry is invented without `onRetry`, and an empty result keeps its
  own icon and no action.
- `npm run visual:test` — 387 passed. **No golden changed**, which is itself a
  finding: at 1472×900 the suite's `maxDiffPixelRatio: 0.002` budget absorbed a
  swapped glyph, a rewritten sentence and a new button. The unit tests are what
  guard this contract, not the goldens.
- Browser: the retry button focuses from the keyboard, takes the one global
  accent ring (`outline: 1px solid oklab(… / 0.5)`, offset 1), and `Tab` leaves
  it for the dock's collapse control. Screenshot `r2-after-error.png`.

### Reclaimed

Visual dev server stopped; five probe scripts deleted; `knip` clean.

### Open, for the next round

1. Round 1's chip-stub item is unchanged.
2. `page-has-heading-one` is unchanged.
3. **The golden budget hid a real visual change.** Worth deciding whether the
   workspace suite wants a tighter budget at its larger viewport, or whether
   region-scoped assertions are the right answer for surfaces this small
   relative to the frame.

---

## Round 3 — the suite had been photographing an app that no longer exists

Status: **complete**

### Audit scope

Round 2's third open item: the visual suite reported green while absorbing a
real change. This round measured what it was actually absorbing.

### Findings

| # | Problem | Evidence | Root cause |
| --- | --- | --- | --- |
| 1 | **96 of 99 goldens were stale by an entire icon set.** The suite had been comparing pre-Lucide glyphs against Lucide ones and reporting green for months. | At `maxDiffPixels: 0`, 83 goldens differ. The diff image for `agent-light-empty` shows the differing pixels are **only the icons** — every sidebar glyph, every composer glyph, every chevron — with the text untouched. `agent-light-empty-darwin.png` last written 18:44 Sep 2 in `b5f0119`; `package-lock.json` last written 22:23 Sep 2 in `1e8ec7a`, *"feat(desktop): draw the glyph set with Lucide"*. That commit regenerated **3** goldens. | `maxDiffPixelRatio: 0.002` scales the tolerance with the frame, so the largest goldens forgive the most: 2650 px at 1472×900. A whole glyph swap fits inside that. |
| 2 | The wait that was supposed to settle a running turn's elapsed label could not do what its comment said. | The label re-reads on a `setInterval(…, 1000)`; the wait compared `body.innerText` across **60 ms**. `workspace golden dark dock-light` failed on `390m 2s` vs `390m 1s`, 34 px. | A stability window sixteen times shorter than the tick it is waiting for. Duplicated verbatim in both spec files. |
| 3 | Mermaid does not place its own SVG label glyphs at the same subpixel offset twice. | ~196 px of text-edge difference between two runs of `markdown-mermaid-dark`, diff image is entirely label text. | Third-party renderer; not the app's to fix. |

### What changed

| | Before | After |
| --- | --- | --- |
| Budget | `maxDiffPixelRatio: 0.002` — 1613 px at 1120×720, **2650 px** at 1472×900 | `maxDiffPixels: 40`, one absolute count at every viewport |
| Mermaid golden | same budget as everything else | its own `maxDiffPixels: 400` at the call site, with the reason |
| Elapsed-label settle | 60 ms `body.innerText` comparison, written twice | `freezeVisualClock(page)` — waits out the full second the label ticks on, and only on states that show it |
| Goldens | 96 of 99 predating the Lucide swap | all regenerated against the current UI |

### Verification

- Three consecutive full runs of `npm run visual:test`: **387 passed** each
  time, no flakes.
- `typecheck`, `lint`, `format:check`, `knip` clean.
- **Sensitivity proved, not assumed.** Temporarily disabling round 2's retry
  button makes `workspace golden light dock-error` fail with 90 differing
  pixels — the change the old 2650-px budget had absorbed in silence. Reverted
  immediately; `data-view` tests still 5/5.
- Noise floor measured two ways: a scratch baseline in `.cache` compared across
  separate browser processes (0 px), and repeated full runs at zero tolerance
  (0 px on almost every golden, 2–34 px on a few, ~196 px on Mermaid alone).

### Deliberately not changed

- **`threshold: 0.05`.** It is the knob that absorbs per-pixel antialiasing, and
  tightening it would raise the noise floor as fast as it raised sensitivity.
  It is also why a whole button only registers as 90 px: most of its pixels sit
  within 5% of the background they replaced.

### Reclaimed

`.cache/noise-snapshots` and `.cache/playwright-results` removed; the temporary
`_noise.config.ts` deleted; no dev server left running.

### Open, for the next round

1. Round 1's chip-stub item, unchanged.
2. `page-has-heading-one`, unchanged.
3. **The margin is thin.** A whole button is 90 px against a 40 px budget — 2.25×.
   A single small glyph on a small control could still pass. If that matters,
   the answer is component-scoped goldens rather than a tighter global count.

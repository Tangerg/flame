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

---

## Round 4 — controls that fire a Runtime command and say nothing

Status: **complete**.

### Audit scope

Overlay interactions first — every composer menu, the message context menu, the
model picker — then a mechanical sweep of every click handler that starts an
async command, checked against `refactor-prompt.md` rule 5 ("禁止点击后没有即时
反馈").

### Checked and left alone

Two candidate findings were dropped after reading the code rather than the
screenshot:

- **Floating panels are translucent and what is behind them tints them.** Real,
  and visible in the context menu over a user bubble. But `DESKTOP_UI_POLISH.md`
  §Glass states it as intent: blur belongs to floating panels and the composer
  because "a hint of what is covered is what makes them read as above it". Round
  1 removed it from a *modal*, which that same rule excludes. Consistent.
- **The model picker reserves 240 px whatever it holds**, so one model sits above
  ~120 px of nothing. `catalog-picker.tsx` says why in place: the surface is
  anchored to a composer control, so a body that grows with its group walks the
  whole popover up the screen. A fixed measure is the point.

### Findings

| # | Problem | Evidence | Root cause |
| --- | --- | --- | --- |
| 1 | Schedule **Run now**, **Delete** and the enable toggle fire a Runtime command with no in-flight state at all: the click leaves no mark, so a second click sends the command again. Two runs, two deletes. | `ScheduleRow`'s `guard()` had the error handling and nothing else. | — |
| 2 | Approval **Forget** and **Forget all** — same. | `RulesRow`'s two bare `try/catch` handlers. | — |
| 3 | The app already had the answer written down three times and never shared. | `GoalStatusSurface` (`commandInFlight` ref + `pending` + `aria-busy`), `agentMemory` (`useRowAction`), `ImagePreviewGallery` (`savingRef` + `saving`). Two more callsites simply omitted the half that shows the command is running. | No owner for "one user-triggered command at a time, visibly". |
| 4 | `DESIGN.md` forbids what the app ships and what its own spec section prescribes. | Line 871 said *"Don't add backdrop-filter / vibrancy"*; line 314 of the same file specifies `backdropFilter: blur(10px)` for the command palette, and three blur tokens ship. ChatGPT's own stylesheet carries `--composer-layout-surface-backdrop-filter: blur(...)`. | — |

### What changed

| | Before | After |
| --- | --- | --- |
| Command in flight | three hand-rolled `useRef` + `useState` pairs, two callsites with neither | one `useCommandAction` in the plugin SDK; the guard is the ref, because `busy` reaches the DOM a render after the click that started the command |
| Schedule row | click → nothing until the query invalidates | `disabled` + `aria-busy` on the control that started it |
| Approval rules | same | same |
| `agentMemory` error text | `err instanceof Error ? err.message : …` — an internal error's own words | `rpcErrorText(err) ?? fallback`, the convention the other four already used |
| `DESIGN.md` blur rule | "Don't add backdrop-filter" | "Don't add a fourth glass surface", naming the three tokens that exist |
| `DESIGN.md` button radius | "Buttons are `md` 8px" | `sm`, through `--button-radius` — what ships |
| `DESIGN.md` dark canvas | `#0c0d0f` | `--color-bg`, which is `#1d1f23` |

### Authority note

The user ruled this round that **`DESIGN.md` is stale and the ChatGPT reference
is authoritative**. Where the document contradicts what ships or what the
reference does, the document is what gets corrected. Three such contradictions
are fixed above; the file has not been swept end to end.

### Verification

- `typecheck`, `lint`, `format:check`, `knip` and all sixteen
  architecture/style gates clean, including `check:bundle`.
- `npm run test` — 2334 passed, 8 failed. All eight are `runtime/contract`'s:
  two are `segment.finished.json` failing its own regenerated validator, six are
  `runtime-http.e2e` against the live Go runtime.
- `npm run visual:test` — 387 passed, no golden changed.
- Five new `useCommandAction` tests: a second click inside the pre-render gap is
  refused, `aria-busy` tracks the command, the Runtime's own refusal text is what
  the user reads, an internal `Error`'s words are not, and a retired command is
  silent.
- Every suite covering the touched files: 475 passed.

### Contract migration, mid-round

`runtime/contract` landed `7472cd0 fix(runtime): require run execution
attribution` while this round was running: `RunRef`, `Goal` and `Schedule` all
require `model` and `provider` now. That broke `typecheck` across eleven desktop
test and fixture files. It was recorded as blocked while the change was still
uncommitted in the working tree — adapting to a definition someone is still
editing produces conflicts, not progress — and migrated once it landed.

Two of those files needed more than the mechanical addition:

- **`runtimeAgentFacts.test.ts`** has two negative cases that construct a run
  with a *deliberately incomplete* identity — provider without model, reasoning
  without model. Adding the fields to the shared builder made both cases valid
  and both assertions vacuous. They override back to `undefined` explicitly now,
  which also says out loud what each case is about.
- **`visual/agentSessionSnapshots.ts`** first got `model: "gpt-5"`, copied from
  the Goal fixtures beside it. Wrong axis: a run's model is resolved against the
  *composer's catalogue*, whose only entry is `gpt-5.6-sol`, so the context gauge
  found no window and stopped rendering. `Context usage: 9%` disappeared from
  every running-state golden.

That second one is worth its own note: **round 3's budget caught it.** The
difference was 58 pixels against a 40-pixel budget. Under the ratio it replaced
— 2650 pixels at that viewport — a missing gauge would have shipped green.

### Reclaimed

Visual dev server stopped; two probe scripts deleted; `knip` clean.

### Open, for the next round

1–3 unchanged. Plus:
4. `GoalStatusSurface` and `ImagePreviewGallery` keep their own in-flight state.
   The goal row tracks *which* of three commands is running so only that button
   reads busy, which a boolean owner cannot express; the gallery guards a
   download, not a Runtime command. Both left deliberately.

---

## Round 5 — a visual spec that told the reader not to trust half of itself

Status: **complete**

### Audit scope

`DESIGN.md`, on the user's ruling that it is stale and the ChatGPT desktop
reference is authoritative. Every `--token` and every backticked identifier in
the file, checked against `globals.css` and `src/`; every px figure checked
against the code that owns it; the ChatGPT stylesheet read for the metrics it
declares.

### Blast radius, first

- **No script parses `DESIGN.md`.** The one gate that grepped positive
  (`check-design-system-boundaries.mjs`) matches on a JS constant named
  `DESIGN_SYSTEM_RINGS`.
- Six documents link to it as the visual spec (`CLAUDE.md` ×3,
  `ARCHITECTURE.md` ×2, `CONTENT_RENDERING.md`, `REFACTORING.md`,
  `FRONTEND_AGENT_WORKSPACE_MODEL.md`), and `theme/kit/types.ts` cites `§2`.
  `CLAUDE.md` cites `§2` and `§5`. **Section numbers had to survive; they did**
  — the file still runs §0 to §13.
- No document links to an anchor inside it.

### Findings

| # | Problem | Evidence |
| --- | --- | --- |
| 1 | **339 of 902 lines were a YAML palette snapshot the file itself said not to trust** — "the YAML below is historical illustration from the dark-first spec; trust the code" — and then **five sections cited that same block as canonical**: §3 for type, §4 for spacing, §7 for motion, §8 for every component spec, §13 for the iteration rule. The document contradicted itself about its own authority, in both directions. | The block's vocabulary is gone from the code: `hairline-strong`, `hairline-tertiary`, `ink-soft`, `ink-faint` exist nowhere in `src/`. |
| 2 | Below the YAML sat **four generations of "read this first" notices**, each superseding the one under it, none deleted. 2026-06 said separation is a background delta and no hairline; 2026-07 reversed it to a hairline; §0's pillars, revised 2026-08, supersede both. A reader had to replay the file's history to learn its present. | Lines 342–383. |
| 3 | The prose restated hexes anyway, and they were stale. `success #3fb950 / warning #f0a936 / negative #f85149 / info #58a6ff`, accent `#6c97ff` / `#2563eb` — **not one of the six matches a shipped theme**. §2's *surface* table three paragraphs above says why it does not repeat hexes: "which is how the previous version of it went stale". The lesson was learned in one table and not the one below it. |
| 4 | §3 describes a typeface strategy the app abandoned: "**The native OS font, no bundled webfont**", with `--font-sans` spelled as SF Pro first. §1 listed "a bundled UI webfont" under **Explicitly rejected**. | `public/fonts/` ships `geist.woff2` and `jetbrains-mono.woff2`; `--font-sans` begins `"Geist"`. A reader following the document would delete them. |
| 5 | "Light themes keep the saturated web values (`#ee0000` / `#0070f3`)" | Shipped light semantics are `#b0342b` and `#2b5fd0` — deeper than the web values, not brighter. Light pushes semantics the *opposite* way from dark, which is the actual rule. |
| 6 | §12 References cites `frontend/src/protocol/run/`, which does not exist, and never named the reference the user calls authoritative. |

### What changed

`DESIGN.md`: **902 → 553 lines.**

| | Before | After |
| --- | --- | --- |
| Head | 339 lines of dead YAML + 42 lines of superseded notices | 17 lines: what the document is for, where values actually live, and why it does not copy them |
| Five citations | "See frontmatter `typography:` / `spacing:` / `motion:` / `components:`" | the tokens and the rings that own them |
| Semantic table | four stale hexes | four token names and what each is allowed on |
| Accent, twice | `#6c97ff` / `#2563eb` | `--color-accent` |
| Light semantics | "keep the saturated web values" | darker and deeper than the web values, because on white contrast against the plane decides |
| Typeface | "native OS font, no bundled webfont" | Geist and JetBrains Mono in front of the native chain — plus the open question below |
| Spacing rhythm | `md` / `lg` / `5xl` / `section`, names from the deleted block | plain figures, and `--density-*` named as the third axis |
| §12 | JetBrains, Linear, a dead path | ChatGPT desktop named as **the authority**, Codex for corner and ladder, then the rest |

### Measured against the reference — a queue, not a change

These are product decisions, so they are recorded rather than acted on:

| | ChatGPT | Flame |
| --- | --- | --- |
| UI typeface | no webfont; stack starts `-apple-system` | bundles Geist |
| User message width | `min(70%, 456px)` | `max-w-[77%]`, uncapped — 591px at a 768px measure against their 456px |
| Composer corner | `--radius-token-composer-single-line` = 22px | 20px base; 25px in Chromium via `--corner-scale`, 20px in the WKWebView that ships |
| Reading measure | `--thread-content-max-width: 48rem` | `--content-max: 768px` — **already aligned** |
| User bubble tint | 5% of the ink, mixed `in oklab` | 5% of the ink, mixed `in srgb` |

The last row is not drift: the app mixes `in srgb` toward `transparent` (18 of
18 washes, badges, tints) and `in oklab` between two opaque colours (18 of 18
surface steps and casts). That split is systematic, and changing one entry would
break it rather than align it.

### Verification

- `typecheck`, `lint`, `format:check` clean; no code changed this round.
- Every `--token` the document names now resolves in `globals.css` or `src/`,
  bar two deliberate mentions of things that do **not** exist (`--font-ui`, cited
  as a split the app does not have).
- Every backticked identifier — 32 of them — resolves in `src/`.
- Every px figure re-derived from the code that owns it: 46px chrome bar, the
  2/4/6/8/10/12 shape ladder and each role's token, sidebar 275 default with a
  240 floor and 520 ceiling, dock 640.
- The only hexes left in the file are `#000000` and `#ffffff`, both in prose
  about what not to use.

### Open, for the next round

1–4 unchanged. Plus:
5. The five rows above are a design queue and need a product answer, the typeface
   most of all: it is the one that changes every glyph in every golden.

---

## Round 6 — the transcript had no root, and one golden has two layouts

Status: **complete**, with one problem characterised rather than fixed.

### Audit scope

Round 1's third open item — `page-has-heading-one`, deferred twice as "a shell
ownership question". It has an answer. Then the flake that answering it exposed.

### Finding 1 — the view's own name was not a heading

Round 1 gave every turn an `h2` and left the transcript with no `h1`, on the
grounds that the shell had no page title. It does: `ChatPanel`'s content header
renders `activeSession.title` — the name of exactly what the reader is looking
at, already styled as a heading — in a `<span>`. Its siblings do not: the
settings pane titles its own panes with `h1`, and the empty chat greeted with
one too, so a *populated* transcript was the only view publishing an outline
that began at its second rung.

| | Before | After |
| --- | --- | --- |
| Session title | `<span class="… font-semibold">` | `<h1>` — the one root, above the turns |
| Empty-state greeting | `<h1>What should we build?</h1>` | `<h2>` — a prompt in the transcript's place, not a second document title |

Pixel-identical, and the suite proved it: Tailwind's preflight already zeroes
heading margins, the span already carried weight 600, and `text-wrap: balance`
cannot act on a `truncate`d line. **No golden changed.**

`page-has-heading-one` is gone from all 36 audited renders. The only axe finding
left anywhere is `region` on `.agent-seam-rail`, which round 1 recorded as
deliberate — re-checked here: both resize rails already render
`SeparatorPrimitive` through one atom, so the rail's semantics are right and only
its *position* (between two landmarks rather than inside one) is what axe counts.

A visual-suite assertion now pins the whole outline in one place: exactly one
`h1` and it names the session, every turn below it, nothing a model authored
above `h3`, and no rung skipped. Proved it can fail by reverting the `h1` — it
does.

### Finding 2 — `agent golden light delegated` has two layouts

Characterised, not fixed.

- The failure is bistable and **exactly reproducible in magnitude**: a run either
  matches or differs by 9037 counted pixels.
- The two frames are **identical in content**. Cropped and compared side by side,
  the whole transcript block sits about one pixel lower in one of them.
- Ruled out by direct measurement, eight runs each: scroll position
  (`scrollTop`/`scrollHeight`/`clientHeight` identical, and the content does not
  even overflow), the transcript's mask (`--composer-overlay` identical to the
  fraction), element positions, and `document.fonts.ready`.
- **The mechanism is `content-visibility`.** Every turn but the last carries
  `content-visibility: auto` with `contain-intrinsic-size: auto 220px`, so a turn
  the browser has not laid out contributes 220px and its real height afterwards.
  Two layouts of one transcript, and which one a screenshot catches depends on
  what the worker rendered before it.
- **The obvious fix is not layout-neutral.** Resolving every turn's
  `content-visibility` before capture — with a comment claiming it changed no
  pixel — failed 26 goldens. Falsified and reverted.
- `document.fonts.ready` was also tried and reverted: no observable effect, which
  is the same standard applied to it as to everything else this round.

What is kept: the frame's origin now has to stop moving *to the fraction* before
capture. The scroll settle it joins compares an integer `scrollTop` and is blind
to the sub-pixel the block still has to give.

Observed rate across this round's full runs: roughly one golden per run, and
**not always the same golden** — `delegated`, then `dock-light`. Two other
intermittent failures (`closing tabs selects a neighbor`, `an overflowing Session
title`) are interaction assertions with no screenshot in them; both predate
round 3 and all three pass in isolation.

### Verification

- `typecheck`, `lint`, `format:check`, `knip` clean.
- Shell suites 25/25.
- `npm run visual:test` — 388 tests. Clean runs and single-golden flakes both
  observed, as described above.

### Reclaimed

Visual dev server stopped; four probe scripts deleted; `.cache` artefacts
removed.

### Open, for the next round

1. **The golden budget versus a virtualised transcript** — answered in round 7.
2. The chip stub, unchanged.
3. The five ChatGPT-versus-Flame rows from round 5, waiting on a product answer.

---

## Round 7 — the flake was the test runner, not the app

Status: **complete**

### Audit scope

Round 6's first open item, and the hypothesis it rested on: that a transcript
whose turns quantise their own height is unstable **in production** too, and
that fixing the product would fix the golden.

### The hypothesis was worth testing and it was wrong

Every turn but the last carries `content-visibility: auto` with
`contain-intrinsic-size: auto 220px`. Measured on `long-content`:

| | Placeholder | Real |
| --- | --- | --- |
| First turn (a short user message) | **220px** | **98px** |

A 122-pixel over-estimate for a single turn, and the transcript's own
`scrollHeight` moves 2326 → 2422 as turns come into view. That looks exactly
like a scroll jump, so the next measurement was whether a reader sees one.

**They do not.** Parking the scroll and walking it upward, a marker's viewport
position tracks `scrollTop` linearly to the pixel — Chromium's scroll anchoring
holds the visible content while the sizes correct behind it. What actually moves
is the scrollbar's range, and a scroll target set beyond the not-yet-grown range
clamps short. Neither is worth changing production for.

So the instability is the harness's, and that is where it was fixed.

### The finding

`agent golden light delegated` deterministically renders **two different
frames**, one pixel apart, identical in content, 9037 counted pixels apart:

- Baselined from a full-suite run, an isolated `-g` run fails it.
- Baselined from an isolated run, **the full suite fails it.**

Both directions verified. No single baseline satisfies both, so the frame
depends on something outside the page.

Ruled out by direct measurement, six hypotheses:

| | Result |
| --- | --- |
| Scroll position | identical, and the content does not overflow |
| The transcript's mask (`--composer-overlay`) | identical to the fraction |
| Element positions | identical |
| `document.fonts.ready` | no effect; reverted |
| Resolving `content-visibility` before capture | **not layout-neutral** — moved 26 goldens; reverted |
| Resolving it by scrolling each turn through the viewport | no effect; reverted |
| Vite's transform cache, cold vs warm | no effect |

What was left was the runner. `fullyParallel: true` spreads the tests **inside a
file** across both workers in an order that changes run to run, so what a
worker had already drawn before it reached `delegated` was never the same twice.

| | Before | After |
| --- | --- | --- |
| `fullyParallel` | `true` | `false` — declaration order, one worker per file; files still parallel |
| Full-suite result | roughly one golden failing per run, and not the same one — `delegated`, then `dock-light` | **three consecutive clean runs** |
| Wall clock | 4.1–4.5 min | 4.1–5.0 min — unchanged |

Also kept from round 6: the frame's origin has to stop moving to the fraction
before capture, which the integer `scrollTop` settle it joins cannot see.

### Verification

- `typecheck`, `lint`, `format:check`, `knip` clean.
- `npm run test` — 2334 passed, the same 8 `runtime/contract` failures.
- `npm run visual:test` — **388 passed, three times running**.

### Reclaimed

Visual dev server stopped; the round's probe script deleted.

### Open, for the next round

1. The chip stub, unchanged.
2. The five ChatGPT-versus-Flame rows from round 5, waiting on a product answer.
3. Three clean runs is evidence, not proof. If a golden flakes again, the next
   step is the one this round did not need: a single worker for the goldens,
   paid for in wall clock.

---

## Round 8 — the chip label, and a golden given up

Status: **complete**

### Finding 1 — the chip stub, after eight rounds

The symptom since round 1: at a narrow composer the three chips read
`Balanc…`, `GP…`, `Mediu…` — two characters each, where the row could have shown
three glyphs and nothing else.

Measured this round at the 423 px footer: the deficit is **45 px**, and flexbox
splits it by `shrink × basis` exactly as configured — the giver takes 39, each
holder 3. Three pixels is enough, because a label box is sized to its text and
an ellipsis costs about eight.

Every CSS mechanism was tried against a **screenshot**, not against
`scrollWidth`, which lies at sub-pixel widths:

| | Result |
| --- | --- |
| `shrink-[999]` on the giver | holders keep their labels to within 0.04 px — and still ellipse |
| `-mr-px` slack on the label | the grid does not widen the box by it; +0.02 px |
| the two together | still `Balanc…` |
| `minmax(0, auto)` grid track (round 1) | fixed the 18 px spill, not the stub |
| `flex-shrink: 0` on holders | overflows the 312 px floor by 78 px |
| `flex-wrap` on the footer | puts the send button alone on a second row at the **default** dock width |

CSS has no way to say *hide the label rather than ellipse it*. After eight
rounds the measurement is the answer, and its machinery is earned:

`useToolbarLabels` reads the row's natural width and sets `data-labelled`. It
measures with `data-measuring` on for one synchronous reflow — labels back on
**and shrinking off**, because a flex row whose items have already shrunk reports
no overflow and measures as fitting. Below the threshold a chip keeps its glyph,
its chevron and its tooltip, and gives up its label whole.

| Width | Before | After |
| --- | --- | --- |
| 768 px | `Balanced` `GPT-5.6 Sol` `Medium` | unchanged |
| 423 px | `Balanc…` `GP…` `Mediu…` | three glyphs, three chevrons, three tooltips |
| 312 px | `Bal…` `⌄` `Me…` | the same |

No golden changed: the goldens photograph the dock at widths where the labels
fit, so this is only reachable below them. A new spec covers both sides.

### Finding 2 — `agent golden light|dark delegated` is withdrawn

Three rounds and **eight measured hypotheses** on one golden that renders two
frames a pixel apart, 9–11k pixels either way, deterministic in magnitude and
never in which:

| | Result |
| --- | --- |
| Scroll position | identical; the content does not even overflow |
| The transcript's mask | identical to the fraction |
| Element geometry | identical |
| `document.fonts.ready` | no effect |
| Resolving `content-visibility` before capture | **moved 26 other goldens and fixed nothing** |
| Resolving it by scrolling each turn through the viewport | no effect |
| Vite's transform cache, cold vs warm | no effect |
| Cropping to the scroller | the offset is inside it |
| The runner's within-file parallelism (round 7) | made it rarer, not absent |

A budget wide enough to pass would be wider than a whole button, which would
give up the sensitivity round 3 earned. So the **frame** is given up instead, for
this one state, and what it was guarding is asserted: two sub-agent rows, each
carrying its own child Run's state, the nested one inside the subtree of the item
that spawned the first, and each drawn as a line with no fill of its own. The
state's behaviour — cancellation targeting and narrative anchoring — was already
covered.

The assertion earned its keep immediately: it was written expecting the nested
row **not** to sit inside `item_delegate` and failed, which is how the nesting
got stated correctly.

### Verification

- `typecheck`, `lint`, `format:check`, `knip` and eleven gates clean, including
  `check:bundle`.
- `npm run test` — 2334 passed, the same 8 `runtime/contract` failures.
- `npm run visual:test` — **388 passed, three consecutive runs.**
- Screenshots at 768 / 423 / 312 px for the chip collapse.

### Reclaimed

Visual dev server stopped; three probe scripts deleted.

### Open, for the next round

1. The five ChatGPT-versus-Flame rows from round 5, waiting on a product answer.
2. `delegated` has no raster coverage. If the two-frame cause is ever found, the
   golden goes back.

---

## Round 9 — three creates that could run twice, and a guard put in the wrong place

Status: **complete**

### Audit scope

The settings forms — validation, save feedback, error recovery — never looked at
in eight rounds.

### The coverage gap that let round 4's defects ship

**Eleven settings panes exist. The visual fixture registers two** — appearance
and providers. Approvals, hooks, MCP servers, personalization, plugins,
connection, usage, schedules and the icon gallery have no visual or interaction
coverage at all.

That is measured, not inferred, and it has already cost something: the two
uncovered panes read by hand in round 4 (`ScheduleRow`, `RulesRow`) both turned
out to be firing Runtime commands with no in-flight state. Nothing photographs
or drives those panes, so nothing could have caught it.

Recorded rather than closed: registering a pane needs its own data providers and
capability gate, and nine of them is a piece of work to plan, not to slip into a
round.

### Finding — three creates with no re-entry guard

Seven places in the app guard a user-triggered async action with a ref, because
`busy` state reaches the control a render after the click that started it.
**Three did not, and all three create things:**

| | What a second click inside the gap does |
| --- | --- |
| `ScheduleForm.onSave` | creates a second schedule |
| `ServerForm.onSave` / `onDelete` | creates a second MCP server |
| `JsonImport.onImport` | imports **every server in the payload** twice |

| | Before | After |
| --- | --- | --- |
| `ScheduleForm` | `setBusy` + a hand-rolled try/catch/notify | `useCommandAction` — the round 4 owner, whose guard is the ref |
| `ServerForm` | `setSaving` only | a ref, local, with the reason written down |
| `JsonImport` | `setBusy` only | a ref |

### The guard that was put in the wrong place first

The first attempt put the re-entry guard in `useAsyncFeedback.run`, the owner
those forms share — six call sites fixed at once, which looked like the root.

**An existing test refused it**: *"drops a superseded run's result"* asserts that
a second run started while the first is in flight **is admitted and supersedes
it**. The lease machinery exists for exactly that. A row editor whose user edits,
saves, edits again and saves again before the first answer arrives must let the
second win.

So the guard belongs where the operation cannot be superseded — a form that
closes on success has nothing to supersede — and not in the shared runner. The
owner change was reverted along with the test written for it, and each form
carries its own guard with the distinction stated at the call site.

This is the round's most useful result: the suite stopped a change that would
have quietly removed a designed capability to fix an unrelated one.

### Checked and left alone

- **Placeholder-as-label.** `ScheduleForm`'s four fields carry `aria-label` and
  no visible label. So do most text inputs in the settings surface — `LinesField`
  and one connection field are the only exceptions, and the reference puts the
  label on the settings *row* rather than in the field. A surface-wide change to
  stacked-form labelling is a product decision, not a defect fix.
- **The cron field has no client-side validation.** The Runtime owns cron
  validity; a second validator in the client would be a second owner of the same
  rule, and presets cover the common shapes.

### Verification

- `typecheck`, `lint`, `format:check`, `knip` and sixteen gates clean.
- Settings suites 131/131; `useAsyncFeedback` 7/7 with its supersession test
  intact.
- `npm run test` excluding the live-runtime e2e — 2293 passed, 2 failed, both
  `runtime/contract`'s `segment.finished.json` against its own validator.
- `npm run visual:test` — 388 passed.

### Open, for the next round

1. The five ChatGPT-versus-Flame rows from round 5, waiting on a product answer.
2. `delegated` has no raster coverage.
3. **Nine settings panes have no fixture coverage.** The two read by hand both
   had real defects; the other seven have not been read.

---

## Round 10 — a guard measured out of existence, and a field that moved as you typed

Status: **complete**

### Audit scope

The seven settings panes never read. Reading `ModeRow` first raised a question
about rounds 4 and 9 that turned out to matter more than the panes did.

### Finding 1 — the premise of rounds 4 and 9 was wrong, and round 9's fix with it

`ModeRow` guards its async action by reading **state**, not a ref, and it is
otherwise the most complete control in the settings surface — an intent state
machine, `aria-busy`, `disabled`, a spinner. That contradicted the reason rounds
4 and 9 gave for their refs: *"`busy` reaches the control a render after the
click that started it."*

Measured rather than argued. Two components, two synchronous `fireEvent.click`s:

| | Commands fired |
| --- | --- |
| State guard, `disabled={busy}` on the button | **1** |
| State guard, **no** `disabled` attribute at all | **1** |
| Ref guard | 1 |

React flushes a discrete event's state synchronously, so the second handler's
closure already sees `busy`. **The gap the refs were protecting does not exist
for a click.**

All three of round 9's call sites already rendered `disabled={… busy}`, so its
two hand-rolled refs bought nothing. Reverted. What round 9 got right stands:
`ScheduleForm` keeps the migration to `useCommandAction`, which removed a
hand-rolled catch, and round 4's fixes were real — `ScheduleRow` and `RulesRow`
had **no guard and no disabled state at all**.

`useCommandAction` keeps its ref, for a reason that survives the measurement and
is now what its comment says: it makes "one at a time" hold **however the caller
is wired**, including a caller that renders no disabled state. Its test was
rewritten to prove exactly that — the harness no longer sets `disabled`, and
removing the ref turns the test red.

### Finding 2 — the connection field moved as you typed

`ConnectionPane` renders Apply on `{dirty && …}` and Reset on `{!isDefault && …}`,
in the same flex row as a `flex-1` URL field. The first keystroke mounts Apply,
which takes its width out of the field — **the caret moves in the middle of
typing.** Rule 12 asks for a stable layout; rule 5 asks for a legible disabled
state, and this had neither.

| | Before | After |
| --- | --- | --- |
| Apply | mounts on the first keystroke | always mounted, `disabled={!dirty}` |
| Reset | mounts when the URL differs from the default | always mounted, `disabled={isDefault}` |

A test pins it and fails if either disabled prop is dropped.

### Read and found sound

`ModeRow` (intent machine, busy, disabled, spinner), `HooksPane`,
`PluginsPane`, `PrefSections`, `UsagePane`, `IconGallery`. The rest of
`ConnectionPane` is the best-instrumented surface in the app: an `invalid` field,
an inline error with a status dot, `aria-live="polite"` on the status row, and a
refresh that disables while checking.

### Contract migration

`4fdd4697 refactor(runtime): stop echoing file read paths` landed mid-round;
`FileContent` no longer carries `path`. One desktop e2e assertion read it, and is
removed — the read answers content, not the path it was asked for.

### Verification

- `typecheck`, `lint`, `format:check`, `knip` and nine gates clean.
- `npm run test` excluding the live-runtime e2e — 2294 passed, 2 failed, both
  `runtime/contract`'s own sample.
- `npm run visual:test` — 388 passed.
- Both new assertions proved able to fail by reverting what they guard.

### Open, for the next round

1. The five ChatGPT-versus-Flame rows from round 5, waiting on a product answer.
2. `delegated` has no raster coverage.
3. Nine settings panes have no fixture coverage. Seven are now read by hand and
   sound; the coverage gap itself is unchanged.

---

## Round 11 — a button that froze the app for nine seconds

Status: **complete**

### Audit scope

The tool-output previews — eighteen renderers, never audited. Surveyed first for
what bounds them: which truncate, which clip, which cap.

### Finding 1 — expanding tool output was unbounded, and the cost is superlinear

`ToolOutputPanel` collapses to nine lines with a fade and a "Show all N lines"
control. Expanding rendered **every line there was**. Measured, rather than
suspected:

| Lines | Expand |
| --- | --- |
| 1,000 | 120 ms |
| 10,000 | 702 ms |
| 50,000 | **9,172 ms** — the measuring test timed out at 14 s |

`ContentBlock.text` carries no `maxLength` in the protocol, so nothing upstream
caps it either; a `shell` running a build reaches those sizes without trying.
That is a control the user can press that freezes the app.

| | Before | After |
| --- | --- | --- |
| Expanded | every line | 1,000 — where the measurement says it is still a frame |
| The control | "Show all 50000 lines" | "Show 1000 of 50000 lines" |
| The remainder | silently absent | "49000 more lines — open the terminal view", beside the Open-in-Terminal control the panel already had |

The cap is the one number in this round, and it comes from the table above, not
from taste. The escape hatch is not new: every caller of this panel already
renders `PreviewFoot` pointing at the terminal view.

### Finding 2 — a truncation badge no caller could reach

`ToolOutputPanel` took a `truncated` prop and drew a "truncated by runtime"
badge from it. **Four call sites, none passes it.** The one preview that has the
fact — `http`, whose response carries `truncated` from the Runtime — draws its
own badge in its status row instead.

The prop and its badge are removed. The http preview keeps its own, where it sits
with the status, the duration and the header count that describe the same
response.

### Read and found sound

The other seventeen previews. `grep`, `lsp`, `recall`, `skill`, `schedule` and
`glob` each bound their own lists; `patch` bounds its hunks. `askUser`, `goal`,
`plan` and `webSearch` render fixed-shape material with nothing to bound.

### Verification

- `typecheck`, `lint`, `format:check`, `knip`, `check:locales` (1066 keys × 8
  locales) clean.
- Tool suites 100/100; the new bound test fails if the slice is removed.
- `npm run test` excluding the live-runtime e2e — 2 failed, both
  `runtime/contract`'s own sample.
- `npm run visual:test` — 388 passed.

### Open, for the next round

1. The five ChatGPT-versus-Flame rows from round 5, waiting on a product answer.
2. `delegated` has no raster coverage.
3. Nine settings panes have no fixture coverage.

---

## Round 12 — a sparkline whose line was a third of its own plot

Status: **complete**

### Audit scope

The workspace views — twenty-one renderers, the largest surface still unaudited.
Surveyed the same way round 11 surveyed the tool previews: what bounds each one,
then a stress test at the dock's 320 px floor.

### Checked and found already handled

Four candidates, each measured before it was claimed:

| | Verdict |
| --- | --- |
| `search` renders a capped match list | It passes `limit`, computes `overflowCount`, and **says so** — "N more matches not shown — narrow the query." |
| `filetree` passes no `limit` to a paginated API | The adapter drains with `autoPagingToArray`, so nothing is silently dropped, and the tree loads children per expanded directory. |
| `timeline` accumulates run events | Capped at `TIMELINE_MAX` where the entries are appended. |
| `diff` renders every hunk | The Runtime answers `truncated` and the view reports it. |

The 320 px stress test found no real overflow either. The first probe reported
spills of up to 435 px, all of them inside `.agent-dock-tabs` — a horizontal
scroll container by design, with mask fades on both ends. Excluding scroll
containers, **every dock view fits its floor exactly**: `scrollWidth === 320` in
all seven.

### The finding — the mark that read as a stray glyph

`Tool stats` at 320 px draws a thick grey chevron across the `apply_patch` row,
overlapping the progress bar beside it. It is not a stray icon: it is the row's
**sparkline**.

`Sparkline` is `h-4 w-12` — 16 × 48 px — with `viewBox="0 0 100 100"` and
`preserveAspectRatio="none"`, and it strokes at `6` with
`vectorEffect="non-scaling-stroke"`. That vector effect means the width is in
**device pixels**, not viewBox units: **six of them in a sixteen-pixel-tall
plot, better than a third of its height.** With four samples and one outlier the
result is a blob, and `overflow-visible` lets its round caps spill onto the
neighbouring bar.

| | Before | After |
| --- | --- | --- |
| Stroke | `6` device px in a 16 px plot | `1.5` |

It reads as a trend now rather than as something that wandered into the row. The
two `dock-stats` goldens are regenerated; nothing else moved.

### Verification

- `typecheck`, `lint`, `format:check`, `knip` clean.
- `npm run test` excluding the live-runtime e2e — 2295 passed, 2 failed, both
  `runtime/contract`'s own sample.
- `npm run visual:test` — 388 passed.
- Before/after screenshots of the row at the 320 px floor.

### Open, for the next round

The user's instruction to **align strictly with `study/chatgpt`** turns round 5's
queue from a question into work. Next round starts there.

---

## Round 13 — aligning with the reference, starting by correcting my own reading of it

Status: **complete**, with one item that needs a decision.

### Audit scope

The user's instruction to align strictly with `study/chatgpt` turns round 5's
queue into work. Every row of it was re-derived from the bundle rather than
reused.

### Correcting round 5

Round 5 recorded: *"ChatGPT ships no UI webfont — its stack starts at
`-apple-system`."* **That was wrong.** It bundles `OpenAI Sans` in Regular and
Medium, and its token is

```
--font-openai-sans: "OpenAI Sans", var(--font-sans-default)
--font-sans-default: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif
```

— a bundled face in front of the native chain, which is the same shape as
Flame's `"Geist", -apple-system, …`. The `-apple-system` declaration round 5
found was the fallback, read as the whole stack. **`DESIGN.md` is corrected**;
the divergence it recorded does not exist.

### The corner radii are not the divergence they looked like

The reference draws `superellipse(1.5)` — **the same curve Flame draws** — and
its bubble and composer sit at 22px, against Flame's 16 and 20. That looks like
a six-pixel gap until `--corner-scale` is accounted for.

Flame grows every step above `md` by 25% under the superellipse, because at the
same radius that curve reads tighter than a circular arc. The reference applies
**no** such compensation. So in apparent terms:

| | Reference | Flame |
| --- | --- | --- |
| User bubble | 22 superellipse ≈ 17.6 circular | 16 |
| Composer | 22 superellipse ≈ 17.6 circular | 20 |

The bubble is 1.6px apart and the composer 2.4px the other way. **Matching the
raw numbers would overshoot both**, making the app rounder than the thing it is
aligning to. Left alone, with the arithmetic recorded so the next reader does
not redo it.

### Aligned

| | Before | After |
| --- | --- | --- |
| User message max-width | `77%` | **`70%`** — the reference's own `--user-chat-width` for a standard bubble |

Its `min(456px, 100%)` belongs to `_compactMessageBubble_`, a variant this app
has no counterpart for, so 70% is the whole of the alignment. At the shared
768px measure the bubble goes from 591px to **538px**.

Fifty-one goldens regenerated, and the assertion that pinned `77%` now pins
`70%` with the reason beside it.

### Already aligned, verified rather than assumed

| | Reference | Flame |
| --- | --- | --- |
| Reading measure | `--thread-content-max-width: 48rem` | `--content-max: 768px` |
| Corner curve | `superellipse(1.5)` | `superellipse(1.5)` |
| UI face strategy | bundled + native fallback | bundled + native fallback |
| Body size | `--text-base: 14px` | `--fs-ui-md: 14px` |

### Needs a decision

**The code voice.** The reference bundles no mono — `ui-monospace,
SFMono-Regular, SF Mono, Menlo, Consolas, monospace`. Flame bundles JetBrains
Mono. Strict alignment means dropping the bundled mono, which changes every code
block, every diff, every path, every timestamp and every golden that contains
one. That is the largest visual change left, and unlike the bubble width it has
no arithmetic that settles it.

### Verification

- `typecheck`, `lint`, `format:check`, `knip` clean.
- `npm run test` excluding the live-runtime e2e — 2295 passed, 2 failed, both
  `runtime/contract`'s own sample.
- `npm run visual:test` — 388 passed.

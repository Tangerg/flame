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

---

## Round 14 — the font setting only worked on half the app

Status: **complete**

### How it was found

Round 13 ended on a question: the reference bundles no mono, Flame bundles
JetBrains Mono — should the bundled face go? Rather than answer it by taste,
the two stacks were rendered side by side. **They came out identical**, which
was the tell: the override had not taken effect at all.

### The finding

`globals.css` declares `--font-sans` and `--font-mono` inside `@theme inline`.
The `inline` keyword compiles a token's **value** into every utility that uses
it, so `font-mono` emits the literal `"JetBrains Mono", ui-monospace, …` rather
than `font-family: var(--font-mono)`.

Settings → Font → UI / Code is a shipped preference. `documentAppearance` sets
those two tokens on the root when the user picks a face. Measured through that
same path:

| | Token | Before | After |
| --- | --- | --- | --- |
| `body`, inheriting from `html { font-family: var(--font-sans) }` | Times New Roman | Geist | **Times New Roman** ✓ |
| any `button`, carrying the `font-sans` utility | Times New Roman | Geist | **Geist** ✗ |
| any `.font-mono` element | Courier New | JetBrains Mono | **JetBrains Mono** ✗ |

**74 files** carry the `font-mono` utility. Four CSS rules use
`var(--font-mono)`, and one uses `var(--font-sans)`. So the user's choice reached
prose and inline markdown code, and nothing else: every button, chip, badge,
code block, file path, timestamp and diff kept the bundled face while the text
around them changed.

| | Before | After |
| --- | --- | --- |
| The two font tokens | `@theme inline` | a plain `@theme` block, so utilities emit `var(…)` |

The rest of the theme stays `inline` — for a colour that resolves to another
token that is what makes theme switching work. Fonts are the case where it does
the opposite.

Nothing moved: **389 goldens pass with none regenerated**, which is the evidence
that the defaults render identically and only the override path changed.

### On round 13's open question

It is less pressing now, and arguably answered: the app offers a code-font
preference and, as of this round, honours it everywhere. Hard-coding the
reference's system stack would take that choice away rather than align with it —
the reference has no such setting to align to.

### Verification

- `typecheck`, `lint`, `format:check`, `knip`, `check:tokens`, `check:styles`,
  `check:style-invalidation`, `check:design-system`, `check:chrome` clean.
- `npm run test` excluding the live-runtime e2e — 2295 passed, 2 failed, both
  `runtime/contract`'s own sample.
- `npm run visual:test` — **389 passed, no golden regenerated.**
- The new assertion fails if the tokens go back inside `@theme inline` —
  verified by putting them back.

---

## Round 15 — the interaction half of the alignment, and a shortcut spelling that could not fire

Status: **complete**

### Audit scope

"UI **与交互**" — the previous alignment rounds compared static measurements.
This one compares the command surface. The reference ships a native menu whose
locale files carry its whole command vocabulary: **36 commands**, against
Flame's nine.

Most of the 36 are concepts Flame does not have — a browser sidebar, a pet
overlay, dictation, temporary chats. Mapping the rest onto Flame's model:

| Reference | Flame |
| --- | --- |
| `navigateBack` / `navigateForward`, `CmdOrCtrl+[` / `]` | `history.back` / `history.forward`, `Mod+[` / `Mod+]` — **already aligned** |
| `newThread`, `searchChats`, `toggleSidebar` | `chat.new`, `chat.search`, `view.toggle-sidebar` |
| **`previousThread` / `nextThread`, `CmdOrCtrl+Shift+[` / `]`** | **nothing** |
| `thread1`…`thread9` | nothing |

The accelerators were read from the bundle rather than guessed:
`menuTitle: "Previous Chat" … defaultKeybindings: [{ key: "CmdOrCtrl+Shift+[" }]`.

### Added

Switching sessions had no keyboard path at all — the Work Index is a mouse
surface, which rule 8 ("所有核心交互必须支持键盘操作") does not allow for something
this central.

| | Combo |
| --- | --- |
| `session.previous` | `Mod+Shift+[` |
| `session.next` | `Mod+Shift+]` |

One step out from history's own pair, which is where the reference puts them
too. `stepAgentSession` is pure and wraps at both ends; a selection the list no
longer carries — what a deletion leaves behind — enters from whichever end the
step came from.

### The finding this uncovered

The first version did not work, and the reason is a latent bug older than it.

`KeyboardEvent.key` for ⌘⇧] is **`}`**, not `]`. Measured in a real browser:
`{ key: "}", code: "BracketRight" }`. tinykeys matches a binding against
`event.key` **or** `event.code`, so `Mod+Shift+]` matches neither and could never
fire.

`lib/combo.ts` already had the fix for the general case — `dispatchKey` maps
letters and digits to physical codes, because "⌘K under Cyrillic reports `к` and
matches no registration". **It did not cover punctuation.** So:

| | Before | After |
| --- | --- | --- |
| `Mod+Shift+]` | `$mod+Shift+]` — matches nothing | `$mod+Shift+BracketRight` |
| `Mod+[` (shipped since before this round) | `$mod+[` — works on a US layout **only** | `$mod+BracketLeft` |

The existing history shortcuts were layout-dependent and nobody had noticed,
because the layout they were written on is the one they were tested on. Eleven
punctuation keys now dispatch by physical code.

Verified end to end in a real browser: the emitted `$mod+Shift+BracketRight`
fires on ⌘⇧], and a binding spelled `$mod+Shift+]` registered beside it does not.

### Verification

- `typecheck`, `lint`, `format:check`, `knip`, `check:locales` (1068 keys × 8
  locales) and fourteen gates clean.
- `npm run test` excluding the live-runtime e2e — 2300 passed, 2 failed, both
  `runtime/contract`'s own sample.
- `npm run visual:test` — 389 passed, no golden regenerated.
- New tests: four on `stepAgentSession`'s order and wrap-around, one on
  punctuation dispatch, and the pinned command set updated to ten.

### Open

1. `thread1`…`thread9` — jumping to the Nth session by number. The reference has
   it; whether a nine-slot numeric index suits a Work Index grouped by project
   rather than a flat tab strip is a design question, not a transcription.
2. `delegated` has no raster coverage.
3. Nine settings panes have no fixture coverage.

---

## Round 16 — two facts you could see and not take, and a ring nobody asked for

Status: **complete**

### Audit scope

The rest of the reference's menu vocabulary — 192 keys, of which round 15 read
only the commands. Two of the remaining ones name a surface Flame has:
`threadHeader.copySessionId` and `threadHeader.copyWorkingDirectory`.

### Finding 1 — the header showed two facts and let you take neither

The content header renders the workspace path **as its basename**, inside
`max-w-[160px] truncate`, and only at `lg` and above. The session title is
truncated at 420px. Neither carried a `title`, and neither could be copied.

So the full working directory was unreachable: not on hover, not by copy, and
not at all below `lg`. That is round 1's finding — a lossy label with no way back
to the value — in a second place.

| | Before | After |
| --- | --- | --- |
| Workspace path | `basename(path)`, no title | the same, with the **full path** as its title |
| Session title | truncated, no title | the same, with the full title |
| Either, copied | nothing | a context menu on the header: **Copy working directory**, **Copy session ID** — the reference's own two items |

### Finding 2 — every context menu drew a keyboard ring when a mouse opened it

Building that menu surfaced it. `globals.css` guards the one global focus rule
with `html:not([data-pointer])`, and **`data-pointer` is written by nobody** — it
appears twice in the stylesheet and nowhere else in the tree. The gate always
matched, so the rule it qualified was never gated at all.

That matters because a menu popup takes focus so the keyboard can drive it, which
makes it match `:focus-visible` even when a right-click opened it. Measured on
the **existing** message context menu, before any change of mine:

| | Outline |
| --- | --- |
| Message context menu, opened by right-click | `oklab(… / 0.5) solid 1px` — the accent ring |
| Approval dropdown, opened by clicking its trigger | none — the trigger is a mouse-clicked button, so nothing matches |

| | Before | After |
| --- | --- | --- |
| Menu popup | accent ring around the whole menu on every right-click | `data-chrome-focus` — the design system's own opt-out for "a row state stands in for the ring" — plus `focus-visible:outline-none` so the UA default does not take its place |
| The dead gate | `html:not([data-pointer])`, always true | removed; the rule reads as what it always did |

Keyboard navigation is unchanged and verified: `ArrowDown` highlights the first
item with a visible wash, which is the indicator the container's ring was
duplicating.

### Verification

- `typecheck`, `lint`, `format:check`, `knip` and seven gates clean, including
  `check:chrome`, which owns this vocabulary.
- `npm run test` excluding the live-runtime e2e — 2300 passed, 2 failed, both
  `runtime/contract`'s own sample.
- `npm run visual:test` — **390 passed, no golden regenerated.**
- Both behaviours pinned by new assertions.

### Open

1. `thread1`…`thread9`, waiting on a decision about what nine numeric slots mean
   in a Work Index grouped by project.
2. `delegated` has no raster coverage.
3. Nine settings panes have no fixture coverage.

## Round 17 — a shortcut only fired if a second list happened to name it

The reference's command menu binds `CmdOrCtrl+/` to `showKeyboardShortcuts`.
Flame has the pane; reaching it meant opening Settings and finding the row. The
surface that documents every key was the one surface with no key.

Adding it is a four-line contribution. It did not work, and why it did not work
is this round.

### Correction to round 16

Round 16 stated that `data-pointer` "is written by nobody" and removed the gate
`html:not([data-pointer])` from the one global focus rule. That reading was
wrong: it is written by `index.html` — three times, outside `src/`, which is
where the grep stopped. The gate was live and load-bearing, and its removal put
the accent ring back on plain mouse clicks in the WebView, which is the exact
problem that boot script exists to solve. **The gate is restored.**

It stayed broken for three rounds because `npm run check` runs the tests before
the guards, and this repository has two contract tests that fail for reasons
outside this scope — so `check:bootstrap`, which owns exactly this pairing and
was red the whole time, never got to run. Every guard is now run individually
each round, not through the pipeline that stops at the first red.

| | Before | After |
| --- | --- | --- |
| Focus ring, mouse click | drawn — the WebView reports `:focus-visible` for a click | gated on the last input device again |
| `check:bootstrap` | red since round 14, unseen | green |
| Goldens moved | — | **none**: the gate changes behaviour, not a static frame |

### Finding 1 — a command's combo was a declaration, not a registration

`CommandSpec` documents a combo as: "one carrying a combo is also projected into
the global shortcut registry." It was not. A plugin resolved commands **by a
hand-written list of ids**, once, at its own setup:

```
export const GLOBAL_COMMAND_IDS = [ "chat.new", "chat.search", … ];  // 11 ids
```

Two defects fall out of that, and the second is the one that bites.

**The list carried no information.** It contained exactly the eleven ids that
declare a combo — the set is derivable from the combo itself. It guarded nothing
either: any plugin can contribute a `SHORTCUT` for any key directly, so the list
never had authority over which keys the app answers. What it did have was the
power to silently drop the next command. `shortcuts.show` was written first and
did nothing at all, with no error anywhere.

**It made the binding depend on array order.** The projection read the command
registry once during setup, so a command contributed by a plugin loaded later was
invisible. `builtin/index.ts` opens with the line "this array's order is only a
tie-breaker between independent plugins, not dependency semantics" — and `⌘F`
worked only because `chatSearch` happened to sit above `globalKeymap` in it.
Moving one line would have killed it silently.

| | Before | After |
| --- | --- | --- |
| What registers a key | `combo` **and** an id in `GLOBAL_COMMAND_IDS` | `combo`, alone |
| When it resolves | once, at one plugin's setup | with the registry, in the host that dispatches |
| A command from a later plugin | never bound | bound |
| Same key from a command and a shortcut | whichever plugin ran last | the shortcut, by the rule `ShortcutSpec` already states |
| What the pane lists | the eleven allow-listed | the keymap the listener binds — one entry per key, same source |
| `command/global-keymap/` | 3 files projecting commands, plus Escape | deleted; Escape moves to `workspace/keymap.ts`, whose meaning it is |

The pane and the keydown listener now read one `useKeymap()`, keyed by the same
dispatch string the listener binds, so what the app lists and what it answers
cannot disagree.

Escape's description was `t("shortcut.closeWorkspaceView")` — resolved text where
the contract says catalog key, which froze that one row in the boot locale. It
passes the key now, like everything else in the list.

### Finding 2 — ⌘/ shows the keyboard shortcuts

| | Before | After |
| --- | --- | --- |
| Reaching the shortcuts | Settings → find the row | `⌘/`, the reference's own binding, and a command in the palette |
| Proof it is bound | — | a host test: contribute a command with a combo and nothing else, press the key, assert it ran. It fails against the old projection. |

### Finding 3 — a golden that renders two ways, measured but not solved

`agent golden light/dark empty` fails about one run in four. Diagnosed rather
than budgeted around, because a suite that is randomly red teaches people to
ignore it.

The difference is **1084–1088 pixels in a band 11 rows tall**, containing only
the three composer chip labels. Everything about the layout is identical:

- geometry byte-stable across loads — `Balanced@384.500 w55.859375`, every time
- same text colour (peak 198 in both), both subpixel-antialiased
- four captures of one page: one hash; the difference is between **loads**
- two buckets, **15:1** over 16 loads, with identical rects

Ruled out by measurement, each with a 16-load bucket test: worker contention
(fails at `--workers=1`), capture timing, a stale golden (`--update-snapshots`
rewrote nothing — the committed image is the majority rendering), the label's
`truncate`, the `data-measuring` reflow of `useToolbarLabels` (disabling it
entirely leaves the split), `font-synthesis`, and explicit
`font-variation-settings` (byte-identical to baseline).

What does move it is the origin. The label sits at **x = 384.5** because
`main.agent-content-card` is 845 wide — odd — and the thread column is centred
in it, so the half pixel is born at `mx-auto` and inherited all the way down. At
a viewport one pixel wider the label lands on **385** and 16 of 16 loads agree.
All three harness widths (1120, 1472, 1800) less the 275 rail are odd.

That is not a fix, and it is not the harness's fault either: at
`deviceScaleFactor: 2`, which is what the product actually runs at, the split is
**11:5** — worse. Left as an open item with its reproduction rather than papered
over with a wider budget or a mask; the two candidate fixes both regenerate every
golden, which is a decision, not a cleanup.

### Verification

- `typecheck`, `lint`, `format:check`, `knip`, and **all fifteen guards run
  individually** — green, including `check:bootstrap`.
- 322 plugin and lib test files, 1834 tests, all passing; 2340 across the app.
  The 9 failures are the same out-of-scope pair as every round: `runtime`'s own
  `segment.finished.json` contract sample and the e2e against the live Go
  runtime.
- `visual` — 389 passed, **no golden regenerated**, one known flake above.

### Reclamation

`command/global-keymap/` (3 files) deleted; six probe specs written for the
measurements above and removed.

### Open

1. `thread1`…`thread9`, waiting on a decision about what nine numeric slots mean
   in a Work Index grouped by project.
2. `delegated` has no raster coverage.
3. Nine settings panes have no fixture coverage.
4. The `empty` golden's two renderings — reproduction and eight ruled-out causes
   above.

## Round 18 — the commands existed; nothing showed them

The reference's command menu carries **47 entries**, and lists commands that
already have accelerators — `toggleSidebar`, `navigateBack`, `newThread`,
`searchChats`, `showKeyboardShortcuts` — because a key you have not learned yet
is a key you do not have. Flame registers commands, ships a locale block
literally headed "Default command palette labels", and has no palette. The
labels were written for a surface that was never built.

### Correction to round 17

Round 17's report said `typecheck` was green. It was — at the moment it ran,
which was before that round's last file existed. `ShortcutsProvider.test.tsx`
carried a type error (`setup` returning a `Disposable` where the kernel expects
provided services) that `vitest` does not typecheck and so did not catch. Fixed,
and typecheck is now run after the last edit rather than before it.

### Finding 1 — a controlled dialog handed focus to nowhere

`SearchOverlay` is opened from a store, so Base UI has no trigger node to restore
focus to. Verified by deleting the mechanism and watching the test fail: **Base
UI does not restore it on its own** — focus lands on `<body>` and the next key
press goes nowhere.

The session finder solved this for itself, in an adapter behind a port, with a
module-level `returnFocus` variable. That put a browser fact — who had focus —
three files from the component that took it, and left the port with nothing else
to do.

| | Before | After |
| --- | --- | --- |
| Who remembers the opener | `session-search/adapters/`, in a module variable | `SearchOverlay`, the component that takes focus |
| The port | `SessionSearchLauncherPort` + adapter + `installSessionSearchLauncher` | deleted — with the capture gone it forwarded to the store, which its own comment already called the meeting point |
| Any other overlay | would need its own copy | gets it by using the atom |
| Covered by | nothing | an atom test that opens, moves focus, closes, and asserts the opener has it back |

### Finding 2 — ⌘⇧P opens the command menu

The reference binds `openCommandMenu` to `CmdOrCtrl+K` **and**
`CmdOrCtrl+Shift+P`. Flame's ⌘K already finds chats — which is the reference's
`searchChats`, on the same key — so the menu takes the second binding and ⌘K is
left alone.

| | Before | After |
| --- | --- | --- |
| Seeing what the app can do | nothing lists commands | ⌘⇧P — every registered command, filtered by the label a person can read |
| Learning the key for one | open Settings → Shortcuts | the row that runs it shows it |
| Reaching a command with no combo | impossible | the menu |

It needed no allow-list edit, no ordering change and no plugin bookkeeping —
which is round 17's fix demonstrating itself.

### Finding 3 — ⌘K was a shortcut where everything else is a command

The session finder registered a raw `SHORTCUT`, so it appeared in the shortcuts
pane but could never appear in a command menu. Since round 17 the rule is
one-way: **a user-facing action is a `COMMAND`; `SHORTCUT` is for keys that are
not commands.** Only Escape is one now.

The sidebar's search row spelled the same key a second time — `comboGlyph("Mod+K")`
and `aria-keyshortcuts="Meta+K Control+K"`, both hand-written next to a command
that already carries `Mod+K`. It reads the command now, so a hint cannot outlive
the binding it advertises. `ariaKeyShortcuts` joins `normalizeCombo`, `splitCombo`
and `dispatchBinding` as the fourth projection of one combo, in the module that
owns combo spelling.

| | Before | After |
| --- | --- | --- |
| ⌘K | `SHORTCUT`, invisible to any command surface | `COMMAND` `chat.find`, in the menu and the pane |
| The sidebar's `⌘K` hint | a literal, in a component | read off the command |
| `aria-keyshortcuts` | a hand-spelled literal | derived from the same combo |
| Shortcuts pane rows | 12 | 15 |

### Verification

- `typecheck`, `lint`, `format:check`, `knip` and **all fifteen guards run
  individually** — green.
- 337 test files, 1892 tests in `plugins`/`ui`/`lib`, all passing. The 9 failures
  in the whole-repo run are the same out-of-scope pair as every round.
- `visual` — 389 passed, no golden regenerated; the round-17 `empty` flake fired
  once, as recorded.
- Both new behaviours pinned: the overlay's focus handoff, and the menu's listing,
  filtering and running.

### Reclamation

`session-search/adapters/` and `session-search/application/ports/` deleted
(2 files); the orphaned `shortcut.sessionSearch` string removed from 8 locales.

### Open

1. `thread1`…`thread9`, waiting on a decision about what nine numeric slots mean
   in a Work Index grouped by project.
2. `delegated` has no raster coverage.
3. Nine settings panes have no fixture coverage.
4. The `empty` golden's two renderings — reproduction and eight ruled-out causes
   in round 17.
5. The command menu has no raster coverage; it is a new overlay and the fixture
   harness does not install shortcut dispatch.

## Round 19 — three comments described a palette that was not there

Grepping the repository for what the new command menu should hold turned up
three independent statements, in three files, describing a palette that lists
workspace views:

- the icon pane's own copy: *"Full catalogue: ⌘K → **View: Icon Gallery**"*
- `workspace-views/index.ts`: *"the tab strip, dock destination list **and command
  palette** enumerate views before any is opened"*
- `openWorkspaceViewInDock`: *"the default placement for anything opened from the
  conversation **or the palette**"*

The palette had been removed and its consumers' documentation stayed. Round 18
rebuilt the menu; this round gives it the half those three sentences describe.

### Finding 1 — no view could be reached from the keyboard

23 views, every one of them behind "open the dock, click Browse, click the row".

| | Before | After |
| --- | --- | --- |
| Reaching a named panel | dock → Browse → click | ⌘⇧P, `View: <name>` |
| Where it opens | — | from the dock catalogue: a view it carries opens in the dock, one it does not takes the whole content card — the rule `openWorkspaceView` already documented |
| Second placement list | — | none: placement is read off the same catalogue the dock reads |

The copy that named ⌘K now names the menu's key, read off the command rather
than spelt — round 18's rule, applied to the one literal it had missed.

### Finding 2 — the icon gallery was registered and unreachable

`icon-gallery` is a `WORKSPACE_VIEW`. It is not a dock destination and not the
settings card, so **nothing could open it** — while the settings pane it
complements told the reader to go find it. It renders on the card now, like
settings, because the menu reaches every registered view rather than only the
docked ones.

I first added it to the dock destination list, which was wrong twice over, and
the repository said so: `dockDestinations.test.ts` — a guard I had not found and
had wrongly called absent — asserts that **every destination's view is
`splittable`**, and the gallery is not. It is a full-card catalogue, not a panel.
Reverted; the two comments I had "corrected" on the assumption that no guard
existed are restored to what they said.

### Finding 3 — Settings had no command

The reference lists `settings` in its command menu on the key the platform
reserves for it.

| | Before | After |
| --- | --- | --- |
| Opening settings | sidebar row, or a pane-specific caller | `⌘,`, and a row in the menu |
| The view id `"settings"` | a literal in `openWorkspaceSettingsPane` and another in the contribution | `WORKSPACE_SETTINGS_VIEW`, named once beside `WORKSPACE_DOCK_CATALOG` |

### Verification

- `typecheck`, `lint`, `format:check`, `knip` and all fifteen guards run
  individually — green.
- 337 test files, 1894 tests in `plugins`/`ui`/`lib`, all passing.
- `visual` — **390 passed**, no golden regenerated.
- The menu's two placements are pinned: a docked view opens in the dock, a view
  the catalogue does not carry opens on the card.

### Open

1. `thread1`…`thread9`, waiting on a decision about what nine numeric slots mean
   in a Work Index grouped by project.
2. `delegated` has no raster coverage.
3. Nine settings panes have no fixture coverage.
4. The `empty` golden's two renderings — reproduction and eight ruled-out causes
   in round 17.
5. The command menu has no raster coverage; the fixture harness does not install
   shortcut dispatch.
6. A view's placement is declared apart from the view, so the two can disagree —
   which is how `icon-gallery` came to be unreachable and how `dockDestinations.test.ts`
   misses it (its assembled set does not include the plugins that register views
   outside `workspace-views/`). Making placement part of the view's own spec would
   remove the class, and is a structural change worth agreeing before making.

## Round 20 — a measurement round, including one of my own that was wrong

Six audits against the reference and against the polish rules. Five found
nothing, one found me measuring with the wrong font. The round is recorded in
full because "this dimension is clean" is only worth anything with the numbers
under it.

### The mistake, and how it was caught

A probe span styled `font: 14px Geist` measured ten `1`s at **48.7px** and ten
`8`s at **84.6px** — proportional figures, a 3.5px shift per digit tick. On that
basis I put `tabular-nums` on the activity summary (`1 read · 1 search`, which
counts while a run streams) and on the working line (`Working · 390m 1s`, which
re-reads a clock every second), with comments stating the measurement.

Then the visual suite passed with **no golden moved**, which a 3.5px text shift
cannot do. Measuring the real elements instead of a detached span:

| | `normal` | `tabular-nums` |
| --- | --- | --- |
| `Working · 390m 0s` | 102.000px | 102.000px |
| `1 read · 1 search` | 95.953px | 95.953px |
| ten `1`s vs ten `8`s, inheriting the row's font | 77.047 / 77.047 | 77.047 / 77.047 |

**Geist's digits are tabular already.** The detached span had fallen back to
another family — `1` at 4.87px is no sans. Both changes were inert and both
comments asserted a false number, so both are reverted. The 30 files that do
carry `tabular-nums` are not wasted: the UI font is a user setting, and the next
font need not be Geist.

### Audited, nothing to change

| Dimension | Method | Result |
| --- | --- | --- |
| Live numerals | every element whose own text carries a digit, 16 agent states + 2 workspace states | only the model name `GPT-5.6 Sol`, a question's option numbers, and authored prose — none of them counters |
| Concentric radii | every child that hugs a parent's corner within 8px, inset equally on both sides | **no violation**; the shape ladder is concentric where a reader can see it |
| `transition: all` | source | none |
| `will-change` | source | two, both on `opacity`/`filter`/`transform` |
| Reduced motion | source | a global `prefers-reduced-motion` rule **and** a user-facing `--motion-scale`, which are different questions |
| Horizontal overflow at the 1120×720 minimum | `scrollWidth > clientWidth` with a visible overflow, six states | all deliberate: `sr-only` 1px clamps, the content card's `clip`, `truncate`, the actions row's optical `-6px`, and `md-table-container`'s `-20px` full-bleed |

### Reference comparison

| | Reference | Flame |
| --- | --- | --- |
| Transition durations | `--transition-duration-basic: .15s`, `--transition-duration-relaxed: .3s` | `--dur-fast: 150ms`, `--dur-slow: 300ms` — the same two |
| Enter curve | `--ease-enter-snappy: cubic-bezier(.23, 1, .32, 1)` | `--ease-out: cubic-bezier(.22, 1, .36, 1)` — the same curve under another name |
| Tabular figures | one surface: a chart tooltip value | leaving authored markdown tables proportional matches it |
| Composer attachment radius | `max(0px, composer-radius − attachment-inset)` | the same formula, already |

Two differences are real and neither is a defect to fix unilaterally: the
reference's composer is a **pill at one line** (`--radius-token-composer-single-line`
= 22px against a 44px min height, with the controls inline) and grows into a
rounded rect, where Flame's always carries its footer on a second row; and the
reference's composer **overhangs** the thread column by 24px a side. Both are
design decisions with wide golden churn.

### Changed

Two functions whose whole body forwarded their own arguments unchanged, owning
no name, boundary or policy: `viewIcon` → `knownIconName`, and a file-local
`currentRootAttention` → the `selectCurrentRootAttention` its own import already
named. Eight other forwarders were looked at and kept: each publishes a private
construct under a name its own context uses.

### Verification

- `typecheck`, `lint`, `format:check`, `knip` and all fifteen guards — green.
- 337 test files, 1894 tests, all passing.
- `visual` — **390 passed**, no golden regenerated.

### Open

Unchanged from round 19; the composer's single-line state is added to it as item 7.

## Round 21 — an accessibility sweep, and a guard that had stopped asking the right question

### The sweep

The suite already runs axe across every state in both themes, but only with the
WCAG 2.2 A/AA tags. Everything axe calls *best practice* — heading order,
landmarks, regions — was outside it. Ran it.

| Rule set | Result |
| --- | --- |
| `heading-order` | clean everywhere — round 16's `h1` header over `h2` turns over `h3` bodies holds across six states |
| `region` | one violation, in every agent state: `.agent-seam-rail` |
| everything else | clean |

The seam rail is the sidebar's resize separator, positioned absolutely on the
seam inside `.agent-card-backing`, a sibling of `<main>` and of the `<aside>` it
resizes. Both landmarks are candidates and neither is right: the `<aside>`
carries `contain: paint`, which would clip a handle that deliberately straddles
its edge, and `<main>` does not own the sidebar's width. `region` is a best
practice, not WCAG, and the alternative is restructuring the shell — recorded,
not changed.

**A real gap axe cannot see:** the transcript has no live region. Six exist in
the app; none is on the conversation. A screen reader is told nothing when an
answer arrives. The reference does carry `sr-only` + `aria-live="polite"` status
text (found in the deobfuscated source, which turns out to exist under
`study/chatgpt/deob` and is far better evidence than the minified CSS), but what
it announces at the transcript level was not established, and announcing streamed
tokens is worse than announcing nothing. Left for a decision — see below.

### The guard that stopped asking the right question

`dockDestinations.test.ts` asserts that **every registered view is reachable from
the dock**. Round 19 made that too strong: the command menu now opens a view the
dock catalogue does not carry on the content card, so a non-dock view is no
longer unreachable — it is simply placed differently.

It also assembled the wrong set. It loads the views registry plus `diagnostics`,
and misses the two plugins that register a view elsewhere — which is exactly why
`icon-gallery` could ship registered and unreachable while a file whose whole job
is catching that stayed green.

| | Before | After |
| --- | --- | --- |
| The question | every registered view is a dock destination | every view that **can sit in the dock** is a dock destination |
| Why that is the right question | — | `splittable: true` ⟺ listed holds exactly today: 20 registry views + `diagnostics` are splittable and listed; `settings` and `icon-gallery` are neither |
| The exception | would have been a hand-written list of two ids | derived from the view's own `splittable` |
| The assembled set | views registry + `diagnostics` | plus `icon-gallery` and `kernelSettings` |
| Proof it catches its case | — | marking `icon-gallery` splittable fails it by name; under the old set it stayed green |

A view-contributing plugin still missing from the list fails the first assertion
if it owns any destination. One that owns none is the remaining blind spot, and
it is written down in the file.

### Verification

- `typecheck`, `lint`, `format:check`, `knip` and all fifteen guards — green.
- 1894 tests passing; the new assertion proven against a deliberate violation.

### Decisions this work is now waiting on

1. **A view's placement is declared apart from the view** (round 19, item 6).
   Folding the dock scope into `defineWorkspaceView` would make the two unable to
   disagree and retire both the destination list and two of the three assertions
   above. Structural: touches 23 views, one extension point and the dock catalogue.
2. **The composer's single-line state.** The reference collapses to a 22px-radius
   pill at a 44px min height with the controls inline, and grows into a rounded
   rect; Flame always carries its footer on a second row. It also overhangs the
   thread column by 24px a side. Both move every golden.
3. **What a screen reader should hear when an answer arrives.**

### Open

Items 1–7 from rounds 19 and 20, plus the `region` violation above.

## Round 22 — a view now says where it goes

Agreed in round 21. A view's placement lived in a list beside the registry, so
the two could disagree, and did: `icon-gallery` was registered, absent from the
list, and openable by nothing.

The list turned out to carry **one** fact the view did not: all 21 entries
repeated the view's own `order` verbatim, and `splittable` already meant "can sit
in the dock" — the same predicate as "is listed". So the fold moves a scope and
deletes the rest.

| | Before | After |
| --- | --- | --- |
| Placement | `builtinContextDockDestinations`, 21 entries beside the registry | `dock?: "workspace" \| "session" \| "run"` on the view |
| Catalogue order | the destination's `order`, a verbatim copy of the view's | the view's |
| "Can it be split" | `splittable?: boolean`, a second spelling | `dock !== undefined` |
| The extension point | `CONTEXT_DOCK_DESTINATION` + `useContextDockDestinations` | gone — nothing contributed to it but the list |
| The join | `resolveContextDockItems(destinations, views)`, dropping unresolved ids | a filter over views; an unresolvable id cannot be written |
| The guard | 3 assertions over an assembled set that missed two plugins | deleted: all three now hold by construction |

Not a wrapper removal — the point had one contributor and the wrapper WAS the
disagreement.

### What the fixture could no longer do

The workspace fixture picked eight view ids and contributed destinations for
them, out of a set of nine views it loads. It loaded `toolsView` and withheld its
destination, so the `dock-catalog` golden photographed an add-panel menu that
production does not have — the mirror image of the bug the fixture's own comment
was written about ("the fixture invented the entry that production was missing").
A view now brings its placement with it, so the fixture cannot invent one **or
suppress one**.

Two goldens regenerated: `dock-catalog` light and dark, each gaining the **Tools**
row and shifting the Session group down by one. Measured before regenerating —
2236 pixels, all in rows 286–429, which is exactly that row and the shift.

### A second flaking golden, and a better clue

`foundation dark collapsed` failed once in a full run and passed 8 of 8 in
isolation — the same shape as round 17's `empty`. This one is more informative:
the difference is a **line break**, a paragraph wrapping one word earlier, not
antialiasing at identical geometry. A wrap moves only if the measured text width
did, which points somewhere the `empty` case ruled out. Two instances now, both
only under a full run.

### Verification

- `typecheck`, `lint`, `format:check`, `knip` and all fifteen guards — green.
- 336 test files, 1891 tests passing (one file fewer: the guard that can no longer
  fail).
- `visual` — 390 passed after regenerating the two `dock-catalog` goldens.

### Open

Rounds 19–21's items, less item 6, which this round closed. Item 4 gains
`foundation dark collapsed` and the line-break observation. Next: the screen
reader's status announcements, then the composer's single-line state and overhang.

## Round 23 — the transcript arrived without a sound

Agreed in round 21: announce the **state**, never the text. A polite region fed a
streamed answer re-reads it from the top on every chunk, which is worse than
silence — so the transcript stays a document the reader navigates, and one
`sr-only` region says only that it changed.

| | Before | After |
| --- | --- | --- |
| A turn starts, finishes, fails, is stopped, or asks a question | nothing is announced | `Responding` / `Response complete` / `The turn failed` / `The turn was stopped` / `The turn reached its limit` / `Waiting for your answer` |
| Where the vocabulary comes from | — | `terminalSettlementStatus`, the mapping the OS notifier already settles runs through; it is exported rather than copied |
| The answer's text | — | never enters the region |

### The bug in my own first version

The region rendered its state on mount. Measured in five fixtures: a chat whose
run had finished announced **"Response complete"** the moment it was opened. A
live region whose text arrives in the same commit as the region is announced by
some readers — which is the caveat my own comment had just written down, two
lines above the code that ignored it.

It opens empty now and speaks only about a change that happens while the reader
is there. Landing on a chat that finished yesterday is not an event.

| Fixture | Announced |
| --- | --- |
| `running`, mounted running | nothing — until it settles |
| `idle` (a finished run) | nothing |
| `canceled`, `error`, `waiting`, mounted | nothing |
| any of them, on a transition | the state it moved to |

### Verification

- `typecheck`, `lint`, `format:check`, `knip` and all fifteen guards — green.
- 1897 tests passing, including the mount-silence case.
- `visual` — 390 passed, no golden regenerated; the region is 1×1 and `sr-only`,
  and the WCAG audits over every state in both themes stay clean.

### Open

Rounds 19–22's items. Next: the composer's single-line state and its overhang,
the last of the three decisions.

## Round 24 — the overhang does not exist, and the pill needs a decision

The third of round 21's decisions was "do both": the composer's single-line pill
and a 24px overhang past the thread column. Reading the reference properly before
building either turned one of them into a correction.

### Correction — there is no overhang

Round 20 read `--composer-adjacent-max-width: calc(--thread-content-max-width +
--composer-inline-overhang * 2 - --home-composer-inline-inset * 2)` and concluded
the composer sits 24px wider than the thread column on each side. That was
inferred from a formula without looking at who reads it. Both tokens have exactly
one consumer each:

| Token | Its only consumer |
| --- | --- |
| `--composer-inline-overhang` | the **suggestion strip's** margins: `ms-[calc(--composer-suggestion-inline-inset - --composer-inline-overhang)]` and the matching `me-` |
| `--composer-adjacent-max-width` | one class, `max-w-(--composer-adjacent-max-width)`, on `codex-sonner-toaster` — the **toast container**, so toasts line up with the composer |

Neither touches the composer's own width. Flame's composer matching its thread
column is not a divergence, and there is nothing to align. **Nothing was built for
this half.**

### What the single-line state actually is

Verified from the stylesheet's selectors and the deobfuscated source, which turns
out to exist under `study/chatgpt/deob` and answers questions the minified CSS
cannot.

| | Reference |
| --- | --- |
| How the state is decided | measured — `shouldUseSingleLineComposer`, from the text against the available width |
| Attributes | `data-composer-layout` = `single-line \| multiline` (derived) and `data-composer-radius-variant` (a **prop**, not derived) |
| Single-line footer | `grid-template-columns: auto minmax(0,1fr) auto`, `column-gap` 5–7px, `padding-inline` 8px, `padding-block` 4–8px — the input is the middle track |
| Multiline footer | `minmax(0,auto) auto minmax(0,1fr)`, gap 5px, and the input is **above** it |
| The 44px pill | `h-11` and the 22px radius apply where `isHome` — the empty-state composer. In a thread the radius stays `--radius-3xl` unless a caller asks for the single-line variant |
| Motion | height `duration-basic` (150ms), radius `duration-relaxed` (300ms), both `ease-enter-snappy` = `cubic-bezier(.23, 1, .32, 1)` — round 20 measured that this is Flame's `--dur-fast` / `--dur-slow` / `--ease-out` already |
| Chip labels | `._ComposerLayoutFooterLabel[data-composer-footer-label-responsive] { display: none }` — the reference drops the label whole, which is the mechanism `useToolbarLabels` already implements here |

There is also a `single-line` + `rows=stacked` combination: single-line
treatment with the input keeping its own row, which is Flame's present shape.

### What stops the build

The reference renders its footer's three clusters from a caller I did not chase
through the bundle, so **which controls occupy the left `auto` track is not
established**. Flame has six contributions in ONE slot — `composer.toolbar.start`
carries attach, three chips, context usage and the goal control — and `…end`
carries send.

Mapping Flame's two slots onto three tracks has to answer where those six go when
they share a row with the text. That is a design decision, and splitting the slot
would change an extension point third-party plugins contribute to. A hook that
measured the wrap was written and **deleted rather than shipped without its
consumer**.

### Open

Rounds 19–23's items, plus the arrangement question above.

## Round 25 — two overlays nobody had ever photographed

Not waiting on the composer decision. Open item: the command menu had no raster
coverage. Checking turned up a bigger hole — **the session finder had none
either**. It is loaded by the shell fixture and never opened, so both search
overlays, and the `SearchOverlay` atom under them that round 18 gave focus
handoff to, had never been in a frame or an audit.

| | Before | After |
| --- | --- | --- |
| Frames | none | 2 goldens: the finder light, the command menu dark |
| Accessibility | none | 10 assertions: a WCAG 2.2 AA audit of each in both themes, each at the smallest UI size, and both text-clipping audits |
| How a fixture opens one | it could not | an `overlay` route parameter, driving the store the shortcut drives |

The fixture loads `defaultCommands`, the menu and two views, so the frame carries
both kinds of row — one that runs with a key and one that opens a panel.

### What the frames showed

**A scrim I thought was missing.** The overlay looked undimmed. Measured before
believing it: the same background pixel reads `255,255,255` closed and
`221,221,221` open — 13.3% black, exactly `bg-scrim`. Nothing wrong; I had misread
a PNG.

**A ⌘ said twice on every row.** Every command row carried a constant `command`
glyph, and every command row's key chip starts with ⌘ as well, so each row showed
the same symbol at both ends. The glyph was doing no work: in a list where each
row is either a command or a `View:`, the view's own glyph and the prefix already
say which is which.

| | Before | After |
| --- | --- | --- |
| A command row | `⌘  Close panel or chat        ⌘ W` | `Close panel or chat        ⌘ W` |
| A view row | `⌘  View: Terminal` | the terminal glyph, as before |
| Label alignment | — | the icon slot is held empty, so both kinds start on one edge |

### Verification

- `typecheck`, `lint`, `format:check`, `knip` and all fifteen guards — green.
- 1897 tests passing; the menu's icon rule is pinned in its component test.
- `visual` — **402 passed**, up from 390: two goldens and ten closure assertions
  added. One run showed the round-17 `empty` flake; the next was clean.

### Open

Rounds 19–24's items, less the command menu's missing coverage.

## Round 26 — eleven settings panes nobody had ever audited

The last coverage item. The settings state hard-coded `settings: "appearance"`, so
one pane of twelve was ever rendered — and the fixture loaded three pane plugins,
so the settings goldens photographed a **three-row nav** the product does not
have.

| | Before | After |
| --- | --- | --- |
| Panes a fixture can open | one | twelve, by a `pane` route parameter |
| Pane plugins loaded | 3 | 12 — the nav in every settings frame is now production's |
| WCAG 2.2 AA audits | 1 pane × 2 themes | **12 panes × 2 themes** |
| Frames | the appearance pane | plus the densest list and the most form-heavy: `plugins` and `providers` |

### Two harness assumptions the panes exposed

**"Wait for the Appearance heading."** Both `closure` and `workspace` waited for a
control that only the hard-coded pane owns, in two hand-written copies of the same
assumption. Every other pane timed out. The ready boundary every pane shares is
the Suspense fallback's own `aria-busy`, scoped to the pane's section — the dock
keeps skeletons that never settle in a fixture seeding no data for them, and they
say nothing about the pane.

**A gateway seeded too early.** The usage pane reads a port, not a data provider,
and installs the Runtime gateway in its own `setup`. Seeding before
`loadPluginsForTest` is overwritten, so the pane rendered a connection failure and
the audit was about the fixture rather than the pane. Seeded after the plugins
load, it passes.

### Goldens regenerated

Four, all for one reason and measured before accepting it: the settings nav is
now the product's. The diff is confined to `x=16..204` — the rail — in every one.

| Golden | Why |
| --- | --- |
| `workspace-light-settings`, `workspace-dark-settings` | the nav lists twelve panes |
| `closure-light-settings-font18`, `closure-dark-settings-font18` | the same nav at the largest UI text |

### Verification

- `typecheck`, `lint`, `format:check`, `knip` and all fifteen guards — green.
- 1897 tests passing.
- `visual` — **428 passed**, up from 402: 24 pane audits and two frames added. One
  run showed the round-17 `empty` flake; the next was clean.

### Open

Rounds 19–24's items, less the settings panes' missing coverage.

## Round 27 — five audits, five null results

No decisions were pending on any of these, and none of them found anything. That
is worth writing down: it is what stops the next round re-treading the same
ground, and after twenty-six rounds a null is information.

| Audit | Method | Result |
| --- | --- | --- |
| Paste-to-attachment threshold vs the reference | the reference's composer paste handler, in the deobfuscated source | it handles **files only** — long text becoming an attachment is Flame's own feature, so `LARGE_PASTE_LINES = 12` / `CHARS = 1600` are not a divergence from anything |
| Dead locale keys | all 1084 keys against every source file, allowing i18next plural suffixes and dynamically composed prefixes | **0 unreachable**. A naive literal match reports 141 — every one of them a plural form or a template tail |
| Port configured then overwritten | every `configure*` call site in `src` | all live in an `adapters/` module called from their own plugin's setup; `router.tsx` configures the navigator at the composition root. No ordering hazard — the trap round 26 hit exists only in the fixture, where a seed ran before `loadPluginsForTest` |
| The settings view's three never-settling skeletons | ancestor chain of every `main [aria-busy]` | all inside `aside.agent-context-dock` at 0×0 — the dock stays mounted behind a full-card view on purpose, the same way `Activity mode="hidden"` keeps the chat. Not a leak |
| Moving the seam rail into a landmark | the computed styles of both candidates | `main.agent-content-card` is `position: relative` **and `overflow: clip`**, and the rail is `translateX(-5px)` at `left: 0` — deliberately half outside the card. Moving it in clips half its hit area. Round 21's judgement, now measured rather than reasoned |

### The golden flake, narrowed

`empty` and `foundation dark collapsed` again, with one new fact and one ruled out.

- **Ruled out: fonts.** `font-display: swap`, not `optional`, so both renderings
  end up in Geist. Bucketed 16 loads of `foundation` in isolation: one hash,
  paragraph 46px tall and 578.08px wide, `document.fonts.check('14px "Geist"')`
  true every time.
- **Ruled out: leaked page state.** Playwright gives each test its own context, so
  neither the appearance store's `localStorage` nor an inline `--ui-*` style
  survives into the next test.
- **What is left** is what the config's own comment already said about
  `delegated`: it "renders a pixel apart depending on what its worker drew
  first". All three are the same family — deterministic alone, nondeterministic
  after a process has drawn other pages, unaffected by the worker count. One
  differs only in antialiasing at identical geometry; another moves a line break.

### Verification

Nothing changed, so nothing to verify beyond the audits above, each of which is
reproducible from its method.

### Open

Unchanged. The two that would take real work — the composer's single-line
arrangement and what `⌘1`–`⌘9` should mean — are both waiting on a decision.

## Round 28 — the frame `delegated` could have had all along, and what it caught

The page frame for `delegated` was given up because its transcript "lands a pixel
apart". A ninth cause was never tried: **an element frame does not have to
escape that shift** — a clip taken relative to the card's own box carries
identical content at an identical raster phase when the whole block moves a whole
pixel. Bucketed 12 loads to one hash before writing it down, and it survived two
full-suite runs, where the page frame never did.

The card is the whole of what the state is named for: a sub-agent's run with its
status and step count, a nested delegation inside it, a terminal call and the
approval pair.

### What it caught in its first frame

**"1 steps".** `agent.steps` was `"{{count}} steps"` with no plural form. Auditing
the catalogue for the class found **40 keys interpolating a count with no plural
form**, of which 22 are wrong at one: `1 files`, `1 matches`, `1 lines`,
`1 commands`, `1 calls`, `1 headers`, `1 runs`, `1 sessions`, `Couldn't read 1
images`, `Pasted · 1 lines`. The other eighteen are correct — `{{count}} of`,
`{{count}} available`, `{{count}} more` — no noun follows the count.

All 22 now carry plural forms in all eight locales, following the catalogue's own
conventions: `_one`/`_other` where the language distinguishes, `_other` alone for
ja/ko/zh/zh-TW whose text is already count-neutral, and the `_many` category that
es and fr require — with French's `de`/`d'` before the noun, as its seven existing
`_many` entries already do. **No code changed**: every one of the 22 call sites
already passes `count`, so i18next selects the form.

One test had frozen the bug: `terminalSubtext(…{ commandCount: 1 })` asserted
`"1 commands"`. It asserts both forms now.

### A budget wide enough to hide a word

The golden written before the plural fix kept passing after it. An `s` at 13px is
about thirty pixels, and `maxDiffPixels` is 40 — so the frame went on documenting
a bug that no longer existed, and `--update-snapshots` does not rewrite a passing
test. Deleted and regenerated. Worth knowing about the budget: it is small enough
for an icon swap and large enough for one letter.

### Verification

- `typecheck`, `lint`, `format:check`, `knip` and all fifteen guards — green;
  `check:locales` now counts 1106 keys complete across 8 locales.
- 1897 tests passing.
- `visual` — 429 of 430, the one failure being the round-17 `empty` flake. Both
  delegated card frames passed inside the full run.

## Round 36 — globals.css was holding three facts that already had owners

Opened after the `agent-*` ownership refactor (`25de9d4b`) as a deliberate sweep
of the stylesheet itself. All three findings are the same shape: a fact whose
owner exists somewhere else, kept as literals here instead.

### A — the layer ladder does not reach the shell (已完成)

`--layer-*` has exactly two rungs, `floating: 50` and `modal: 100`, and
`check-design-tokens` refuses a raw `z-` at any CALL SITE. The stylesheet that
owns the ladder carries six raw multi-digit values for the whole shell stacking
order — 40, 30, 10, 25, 15, 25 — and **25 appears twice**, on the drawer seam
rail and the dock resizer, sharing a rung by coincidence rather than by
declaration. Nothing states why the drawer is 10 and the card backing 15.

This is the defect `check-design-tokens`' own header describes for type sizes,
in a property the guard never reads because it only reads `src/**/*.tsx`.

Acceptance: every shell rung named in `--layer-*`; no multi-digit raw `z-index`
left in `src/styles/`; the guard reads the stylesheets so the ladder's owner is
held to the ladder.

### B — the drag region is spelled two ways (已完成 · null result)

`-webkit-app-region` + `--wails-draggable` always travel together — they are one
fact, the Wails/WebKit pair. Written as raw CSS four times in `globals.css` and
once as arbitrary Tailwind in `ChatSearchOverlay.tsx:89`. `globals.css` already
uses `@utility` for `media-edge`, so the mechanism to give this one owner is
present and unused.

Acceptance: one `@utility` per intent; no call site spells the pair.

### C — pure layout written as global CSS classes (已完成 · 3 of 4)

`CLAUDE.md` §4 is "Tailwind first … 不写新 .css 文件". Four `agent-*` classes
carry nothing but layout Tailwind already expresses — `.agent-view-split` is
`display:flex; min-height:0; flex:1`. Each is a global name that can collide,
that the layer guard cannot see, and that now needs the round-35 guard to police.
Deleting them removes the collision instead of guarding it. The classes that stay
are the ones Tailwind genuinely cannot reach: ancestor-state descendants, mask
ladders, `::-webkit-scrollbar`, and the `box-shadow` seams that
`check-design-tokens` requires to go through `--shadow-*`.

Acceptance: the four are gone from `globals.css` and expressed at their single
owner; the visual suite is unchanged, since none of this is a visual change.

### What actually changed

| | 修改前 | 修改后 |
| --- | --- | --- |
| A `--layer-*` | 2 rungs (`floating` 50, `modal` 100) | 6 rungs; `drawer` 10, `card` 15, `resizer` 25, `chrome-control` 30 added |
| A shell stacking | 6 raw values in `globals.css`; `25` on two elements by coincidence | every rung named; the two resizers share `--layer-resizer` **by declaration** |
| A window vs dock control | 40 and 30 — two rungs for elements that sit at opposite ends of one strip and cannot overlap | one `--layer-chrome-control` |
| A guard | `check-design-tokens` read `src/**/*.tsx` only; `globals.css` exempt from every stylesheet rule | new `EVERY_STYLESHEET_RULES` applies to `globals.css` too; multi-digit `z-index` refused, single digits still legal |
| C `.agent-card-backing` | global class | `relative flex h-screen min-h-0 min-w-0 flex-1 z-[var(--layer-card)]` at `app-shell.tsx` |
| C `.agent-view-split` | global class | `flex min-h-0 flex-1` at `workspace-view.tsx` |
| C `.agent-view-body` | global class | `flex min-w-0 min-h-0 flex-1 flex-col` at `workspace-view.tsx` |
| dead export | `ResizeHandle` re-exported from `ui/atoms/index.ts` | removed — orphaned by round 35, both real consumers import the module directly |

`globals.css` 1331 → 1314 lines. Guarded `agent-*` set 28 → 25.

### B was withdrawn, on the evidence

The plan called the `-webkit-app-region` / `--wails-draggable` pair a duplicated
fact. Reading it again: it is a **vendor pair**, written where it applies, exactly
like the `-webkit-mask-image` / `mask-image` pair this same file repeats eight
times without anyone calling it duplication. `@apply` appears nowhere in the
repo, and a `@utility` consumed by one call site is the abstraction this prompt
forbids manufacturing. One call site does spell it as arbitrary Tailwind
(`ChatSearchOverlay.tsx:89`); that is the documented escape, not a defect.

### C stopped at three, and the fourth taught the criterion

`.agent-drawer-header` was migrated to `ps-[var(--window-controls-gutter)] pe-3`
and then **reverted**. It is not standalone layout — it overrides
`.agent-surface-header`'s `padding-inline`, and that base class is UNLAYERED
while Tailwind utilities live in `@layer utilities`. Unlayered wins, so the
migrated version would have silently lost the traffic-light gutter.

Confirmed in the built stylesheet rather than argued:

```
.agent-surface-header{   UNLAYERED
.agent-drawer-header{    UNLAYERED
.h-screen{               @layer utilities
```

**The criterion, for the next pass:** a class that overrides another class's
property cannot move to Tailwind. Only classes nothing else targets can.

### Verification

- `prettier --check`, `typecheck`, `lint` — green.
- All fifteen guards — green. `check-design-tokens` now reports the layer ladder;
  negative-tested by restoring `z-index: 10` on `.agent-drawer`, which it refused
  (`styles/globals.css:851`).
- `knip` — green after removing the dead `ResizeHandle` re-export it caught.
- `check:bundle` — 981 emitted utilities, every class renders; entry CSS 112.3 KB.
- Unit: 2360 passing. 9 failures, all in `src/rpc/`
  (`segment.finished.json` vs `RunEvent`, plus the Go-runtime e2e) — confirmed
  pre-existing by stashing this branch's desktop changes and re-running.
- **Visual: 430/430, no golden regenerated** — the correct result, since nothing
  here is a visual change.

### Resources reclaimed

Port 4174 freed, no stray `visual:dev` or Playwright processes, `test-results`
empty, temp backups removed, `playwright.visual.config.ts` unmodified.

### Next round

The Codex composer geometry audit this round was opened with and set aside:
Codex switches the composer's radius by content height —
`[data-composer-layout=single-line]` → `--radius-full`,
`[data-composer-layout=multiline]` → `--radius-3xl` — where ours is a fixed
`--shape-composer: 20px` at every height. Whether 20px on a single-line box reads
as an unresolved almost-pill needs measuring before it is called a defect.

## Round 37 — three declared intents that never reached the screen

Opened as a pixel-alignment audit against `study/chatgpt`, which produced two
null results and then a probe that found something the golden suite could not
see. Everything below was measured in a browser, not argued.

### Null results, recorded so they are not re-opened

**Composer radius.** Codex switches it by content height —
`[data-composer-layout=single-line]` → `--radius-full`, `multiline` →
`--radius-3xl`. Ours is fixed. Measured: our composer is **96.69px tall when
empty**, because the chip row always sits under the input, so 25px of radius is
**52% of a pill** and never reads as an unresolved almost-pill. Codex's rule
presumes a slim single-line bar we do not have.

**Composer type.** The textarea renders at 16px against 14px chrome — it is the
`prose` ladder step (`<TextArea size="prose">`), the same step the transcript
uses. Deliberate, not drift. The 2px left/right asymmetry is also declared
(`--density-composer-footer-end` exists to make the end differ).

### The probe, and what it found

Walked all 31 fixture states at 1120x720 (the narrowest window the shell allows)
with the UI font at 18px (the largest a person can pick), looking for boxes
shorter than their own text. **7 findings at 18px, 0 at 14px.**

Horizontal overflow was checked first and came back clean once the probe learned
to skip `visibility: hidden` subtrees (the collapsed dock parks itself off-screen
by design) and horizontal scrollers (the dock tab strip).

### A — an element default outranked every call site (已完成)

`h1..h6 { font-weight: 600; text-wrap: balance }` sat UNLAYERED. Utilities live
in `@layer utilities`, and unlayered beats layered, so the block silently
defeated three declared intents:

| call site | wrote | got |
| --- | --- | --- |
| `SessionIdentity.tsx:45` | `truncate` | **wrapped** — `white-space` and `text-wrap` are both shorthands for `text-wrap-mode`, so `balance` reset the `nowrap` |
| `QuestionCard.tsx:235` | `text-pretty` | `balance` |
| `QuestionCard.tsx:235` | `font-medium` | **600** |
| `ChatStream.tsx:119` | `font-medium` | **600** |

The title one is the sharp end: at 18px the `<h1>` grew to **53px inside a 46px
header** (header `scrollHeight` 49 vs `clientHeight` 46). Invisible at the
default size, because one line fits either way — **and no golden had ever
photographed it**, since the `font18` goldens are other states.

Moved into `@layer base`, which is where element defaults belong and why Tailwind
puts its own preflight there. After: `<h1>` 53px → **26px**, header back to 46.

### B — `cn()` believed something untrue about our type steps (已完成)

`Button`'s cva base declares `leading-tight`. It never reached the DOM:

```
cn("leading-tight", "text-ui-sm")  ->  "text-ui-sm"
cn("text-ui-sm", "leading-tight")  ->  "text-ui-sm leading-tight"
```

Tailwind Merge models a font-size utility as also setting line height, because
Tailwind's own steps do. **Ours do not** — `@theme inline` gives `--text-ui-*` a
size and a tracking and no leading. So every `leading-*` written before a size in
the same expression was dropped, silently and order-dependently, and the element
fell back to the body's PROSE rhythm.

Two live victims: every `Button` (its box was then shorter than its own line at
18px), and `MessageBlock.tsx:93`, where the transcript lost the `leading-prose`
it declares. Reordering the classes would have been a patch that the next edit
undoes, so the fix is to stop `cn()` believing it:
`override: { conflictingClassGroups: { "font-size": [] } }`.

Measured after: Button 26.35px → **19.55px** line (17px x 1.15, the
`leading-tight` it asked for) and its 20px box no longer overflows;
`.msg-content` 24.8px → **24px**, exactly `calc(1em + 8px)` as
`--leading-prose` declares.

### C — two more ladders Tailwind Merge could not see (已完成)

The same question asked of every ladder found two more:

```
cn("leading-body", "leading-prose")   ->  both kept
cn("rounded-sm", "rounded-composer")  ->  both kept
```

`classNames.ts` declared the `text` ladder and not `leading` or `radius`, so our
own steps did not conflict with anything and stylesheet order picked the winner.
Both now declared. **No golden changed**, so this removed a latent hazard rather
than a live symptom — worth saying plainly.

`check-design-tokens` was holding only the type half, and reading the whole file.
It now reads `@theme inline` alone — which is exactly what Tailwind turns into
utilities, so `--radius-scale` (a multiplier) and `--leading-markdown-*`
(stylesheet values) correctly stay out — and holds all three ladders.
Negative-tested by removing a step from each.

### What was NOT fixed, and why

Six boxes remain 2-3px short of their text at 18px: three `<span>`s whose glyph
box exceeds a `leading-none` line box (normal font metrics, `overflow: visible`,
nothing clipped) and controls whose pinned height is a deliberate choice —
`typography.ts` keeps geometry in absolute px on purpose. Growing them would be a
redesign, not a fix.

Giving `--text-ui-*` its own line height was considered and rejected on
measurement: **90 multi-line `ui-md` elements** currently rely on the inherited
1.55 body rhythm, so a tighter step value would restyle the app rather than fix
a defect.

### Verification

- `typecheck`, `lint`, `format:check`, `knip`, all fifteen guards — green.
- `check-design-tokens` now reports the layer, leading and radius ladders;
  negative-tested on all three.
- `check:bundle` — 981 emitted utilities, every class renders; entry CSS 112.3 KB.
- Unit: **2362 passing**, +2 from the new `cn()` contract tests. The 9 failures
  are all `src/rpc/` (`segment.finished.json` vs `RunEvent`, plus the Go-runtime
  e2e), confirmed pre-existing by stashing this branch and re-running.
- Visual: **430/430**. 17 goldens regenerated for A and B, each verified by
  pixel bounding box to be confined to the text whose leading or weight changed;
  C changed none.
- Evidence: `/tmp/round37/{before,after}` (3x crops of the header before and
  after truncation) and the bounding-box report per golden.

### Resources reclaimed

All six probe scripts deleted, port 4174 freed, no stray `visual:dev` or
Playwright processes, `playwright.visual.config.ts` unmodified.

### Next round

Pick up the current Runtime protocol — the nine `src/rpc/` failures are the
contract having moved (`segment.finished.json` no longer satisfies `RunEvent`),
and the generated wire files are newer than the samples.

## Round 38 — the authoritative carrier of the context footprint was being dropped

Runtime moved `contextTokens` onto `segment.finished` as a REQUIRED field. Tracing
where the frontend would put it found that it had nowhere to go, because the
footprint was modelled as a detail of the ephemeral channel.

### The defect (已完成)

`contextTokens` is a Run-level fact. Runtime states it on all three Run frames —
the started frame's `RunFact`, `segment.progress`, and now `segment.finished` —
and `RUN_EVENT_RELIABILITY` marks the finishing one **authoritative** while
`segment.progress` is **ephemeral** and listed in `SuppressibleRunEventType`.

The frontend kept it inside `AgentRunView.progress`, the bag that expires at the
segment boundary, and dropped two of the three carriers:

| carrier | before |
| --- | --- |
| `segment.finished.contextTokens` | **dropped** at `runtimeAgentFacts.ts:326`, never reached the SDK type |
| `RunFact.contextTokens` (snapshot / cold read) | stuffed into a fake `progress` object by `projectRunRef` |
| `segment.progress.contextTokens` | kept — the ephemeral, suppressible one |

So a Run whose progress stream was suppressed — a reconnect, a cold read —
delivered no context-window reading at all, even though Runtime had stated the
exact number in the frame that cannot be dropped.

The tell was already in the tree. `settledContextProgress` existed only to rescue
one field from the progress bag at the segment boundary, and its own comment says
why: *"Activity, step and provisional usage expire at the segment boundary. The
latest prompt footprint does not."* That comment is the design; the representation
contradicted it.

### The fix

`AgentRunView.contextTokens: number | null` is now a Run-level fact written by all
three carriers, and `progress` is typed `Omit<AgentRunProgress, "contextTokens">`
so the old home is unconstructable rather than merely unused. `progress` becomes
plain `null` when a segment ends, and `settledContextProgress` is deleted.

One rule governs all three carriers: **zero is not a footprint**. The finishing
frame carries the field unconditionally, so without it a Run that never reported
one would erase the value a live frame did state. It is the same reading
`contextUsageReadout` already refuses — *"a gauge reading zero claims 'empty',
which here would be false"*.

Two tests pin the behaviour this round exists for: a finished Run with no progress
frame at all lands its footprint, and a finishing frame beats a stale live one.

### Also fixed: two stale e2e expectations (已完成)

`workspace.files.head` and `workspace.files.read` no longer echo the caller's
path — runtime commits `834261d3` and `4fdd4697`, and the contract agrees
(`FileHead { lines }`, `FileContent { content, totalLines, … }`). The e2e still
asserted the echo.

### 阻塞 — six e2e failures that are runtime behaviour, not frontend shape

| failing test | symptom |
| --- | --- |
| MCP + managed-Skill moves after lost responses | `RpcError: internal_error` |
| skill archive/restore through `skills.changed` | `data: []` |
| home/project-root/workspace knowledge cascade | timed out waiting for `knowledge.changed` |
| workspace files, recipes, agent docs, hook trust | `recipes.list()` omits the **global**-scope recipe |
| durable compaction winner on provider failure | timed out at the cutpoint |
| durable compaction winner after SIGKILL | timed out at the cutpoint |

None has a matching shape change in `wire.generated.ts`, and no committed runtime
change explains them. `runtime/internal/application/agent/sessions/` —
`query_coordinator.go`, `session_crud.go`, `coordinator.go`, `plan_boundary.go` —
is uncommitted right now, which is where list queries, knowledge and compaction
live. Editing the frontend to accept the current answers would freeze a
half-finished backend, so these stay untouched and reported.

`samples.test.ts` / `schema.test.ts` are the same story in miniature:
`runtime/contract/typescript/samples/segment.finished.json` predates the field and
needs `"contextTokens": 0`. One line, in `runtime/`, which this scope does not
modify.

### Verification

- `typecheck`, `lint`, `format:check`, `knip`, all fifteen guards — green.
- `check:bundle` — 981 emitted utilities, every class renders.
- Unit: **2365 passing**, +4. Failures 9 → **8**, one e2e recovered; the rest are
  the blocked set above.
- Visual: **430/430**, no golden regenerated — this is a data-path change and the
  gauge renders the same numbers from a different owner.

### Resources reclaimed

Port 4174 freed, no stray processes, no probe scripts,
`playwright.visual.config.ts` unmodified.

### Next round

The blocked six, once `runtime/internal/application/agent/sessions/` settles.

## Round 39 — every class the cascade throws away, not just the three I found

Round 37 found three call sites whose declared utility was silently overruled by
an unlayered rule, and I left the rest of the element defaults alone on the
argument that "no utility competes with them". That was reasoning, not
measurement. This round measured it.

### The detector

An unlayered rule beats `@layer utilities` whatever its specificity —
`:where(...)` at zero specificity included. So: walk every stylesheet rule with
the layer it sits in, and for every element, find a property that both an
unlayered rule and a matching utility declare **with different values**. That is
a call site being overruled.

Two refinements were needed before the output meant anything:

- **Compare values, not properties.** 22 pairs dropped to 13 once identical
  values stopped counting as conflicts.
- **Evaluate conditions.** 13 dropped to 4 once `@media`/`@supports` blocks that
  do not apply were skipped — the touch hit-area floor
  (`:where(button, …) { min-width: 44px }`) lives under `pointer: coarse` and
  never meets a desktop pointer. Without that, it looked like the largest defect
  in the tree.

### What survived, and what it was (已完成)

Four, all the same shape: a stylesheet rule had already decided the property, and
the call site's class decided nothing.

| call site wrote | already decided by | inert call sites |
| --- | --- | --- |
| `tabular-nums` | `.font-mono` — sets `font-variant-numeric` and `"tnum" 1` | **24** |
| `gap-1` | `.agent-context-dock > .agent-surface-header:first-child { gap: 4px }` | 1 |
| `flex-1` | `.panel-scroll { flex: 1 1 0 }` | 2 |

None of the three changed a pixel. That is what makes them worth removing: each
reads as an instruction, and editing it does nothing.

`.font-mono`'s rule now says who owns the decision, and `tabular-nums` on a
PROPORTIONAL face — a real instruction — stays at its 14 remaining call sites.

### The `panel-scroll` split (已完成)

The `flex-1` conflict had a cause worth fixing rather than deleting around.
`.panel-scroll` bundled the LAYOUT of a scroller (`flex: 1 1 0`, `min-height: 0`,
`overflow-y`, `overscroll-behavior`) with its scrollbar APPEARANCE. Because of
that, `ScrollArea`'s hidden-scrollbar branch had to restate all four properties
as utilities — the same contract written twice — and an unlayered rule was in
charge of geometry that call sites also spell.

`.panel-scroll` is now appearance only. `ScrollArea` states the layout once for
both branches, and the three direct users of the class carry it explicitly.
Pixel-identical, because every property was preserved.

### Two things the change exposed

`msg-scroll` had no rule anywhere — a class used purely as a Playwright locator,
where every other hook in this tree is a `data-*` attribute. `.msg-scroll >
.panel-scroll` selects exactly the element that already carries the styled
`.msg-scroll-viewport`, so the class is gone and both locators name the real one.
`check-dead-utilities` had never objected; the enlarged class list is what made
it look.

I also broke two assertions with the bulk edit: `container.querySelector(
".font-mono.tabular-nums")` is a CSS selector, not a class list, and the stripper
took the token out of it. Caught by the suite, repaired to `.font-mono`, which is
what the assertion means now.

### The detector is now a test

`visual/cascade.visual.spec.ts` runs the same walk over six fixture states and
fails with the offending property, both values, and a sample element.
Negative-tested by putting one inert `tabular-nums` back.

### Verification

- `typecheck`, `lint`, `format:check`, `knip`, all fifteen guards — green.
- `check:bundle` — 981 emitted utilities, every class renders.
- Unit: **2365 passing**; the 8 failures are the `src/rpc/` set blocked in round 38.
- Visual: **431/431** — 430 goldens with none regenerated, plus the new cascade
  check. A refactor that changes no pixels is the correct outcome here.

### Resources reclaimed

Probe deleted after promotion to a spec, port 4174 freed, no stray processes,
`playwright.visual.config.ts` unmodified.

### Next round

The six e2e failures, once `runtime/internal/application/agent/sessions/`
settles. Failing that, the detector generalises: it currently compares utilities
against unlayered rules only, and the same question can be asked of inline
`style` against utilities, which nothing checks today.

## Round 40 — six invisible things that took clicks

`ui_rules 6` asks for hit areas of at least 40x40 and forbids adjacent controls
from overlapping. Round 39 had just shown that the only rule enforcing a floor
lives under `pointer: coarse` and never meets a desktop pointer, so nothing was
checking either half here. Measured both.

### Three refinements before the measurement meant anything

The first pass reported 15 overlapping pairs. Each of the following removed a
class of phantom, and the count is the honest record of how wrong an unrefined
probe is:

1. **Element identity has to travel with its box.** `contains()` was indexing a
   second `querySelectorAll` against boxes built from the first, which had
   skipped hidden and zero-size elements — so the nesting test compared unrelated
   elements. 15 -> 10.
2. **Clip to scrolling ancestors.** Inside `overflow: auto` a rect keeps its full
   width, so every scrolled dock tab invented an overlap with the chrome beside
   it. 10 -> 3.
3. Of the three, two were read and dismissed: `Allow once` / `Approval options`
   overlap by **1px**, which is the `-ml-px` seam of a split button and the
   correct way to draw one; the third was the real finding below.

**The 40x40 floor is not met and should not be**: 42 distinct controls are under
it, nearly all wide-but-short (`104x22`, `90x26`) because `--control-height-*` is
22/26/30/34 — a visual-style token a theme owns. `DESKTOP_UI_POLISH.md` says
desktop is not web, and inflating every control to 40px tall is a redensification
of the whole app, not a defect fix. Recorded as a considered non-conformance.

### The real finding (已完成)

The third overlap was a Work Index row action sitting on the row's right end. It
led to the mechanism: an element at `opacity: 0` is invisible **and still takes
clicks**. Measured across six states: **six of seven hover-reveals were invisible
and clickable.** The largest was a 774x26 strip of message actions; the others
included two 44x210 columns over markdown tables and the action on every row.

Nine reveal sites, eight spellings, one correct — `context-dock` used `invisible`
(visibility, which does gate hit-testing). `globals.css` already said so in its
own words: *"The reveal itself is eleven different class lists — which group,
which pseudo-class, opacity or visibility"*. And `MessageBlock` had the guard on
its `hidden` variant and not on its `hover` one, **two adjacent lines apart**,
which is what says nobody decided they should differ.

Each resting state now carries `pointer-events-none` beside the `opacity-0` it
guards, and each reveal restores it in the same variant that restores opacity.
The disclosure chevron is `aria-hidden` decoration inside the header button, so
it is pointer-transparent at every state rather than switching.

### The regression I wrote, and what it proved

I first put the guard in one place — `[data-reveal="hover"] { pointer-events:
none }` in `globals.css` — reasoning that a single owner beats nine call sites.
It broke closing a dock tab, and the cause was **the exact defect rounds 37 and
39 were about**: an unlayered rule beats `@layer utilities`, so it defeated every
`group-hover:pointer-events-auto`. Moving it to `@layer base` did not fix it
either; Playwright's actionability check showed the reveal working
(`opacity:0 visibility:visible pointer-events:auto`) and the click still failing
inside `scrollIntoViewIfNeeded`, which scrolls the tab out from under the pointer
and drops the hover.

The lesson is the placement, not the ownership: a guard that has to be overridden
by a variant belongs **in the same layer as the variant**. As utilities, the
resting state and the reveal resolve by ordinary variant order and no layer
fights anything.

Worth noting that `cascade.visual.spec.ts` could not have caught this: it
compares selectors that match at rest, and `group-hover:` matches only while
hovered.

### The detector is now a test

`visual/reveal.visual.spec.ts` walks six states and fails with the box size and
class list of anything invisible that still takes clicks. Negative-tested by
removing the guard, which reports the 774x26 strip.

### Verification

- `typecheck`, `lint`, `format:check`, `knip`, all fifteen guards — green.
- Unit: **2365 passing**; the 8 failures are the `src/rpc/` set blocked in round 38.
- Visual: **432/432** — 430 goldens with none regenerated, plus the cascade and
  reveal checks. `pointer-events` changes no pixels, which is why the four
  interaction failures the first attempt caused were real and worth listening to.

### Resources reclaimed

Both probes deleted after promotion, port 4174 freed, no stray processes,
`playwright.visual.config.ts` unmodified.

### Next round

Still the six blocked e2e. Failing that: the hit-area probe also measured 42
controls under 40x40 and a `1x1` hidden file input — the latter is worth one look
to confirm it is a label-driven picker and not a stray target.

## Round 41 — a compensation that existed, was commented, and had no users

`ui_rules 8` asks for a focus state that is clear, continuous and **not
occluded**. One rule draws every ring: `1.5px` at `outline-offset: 1px`, so it
reaches 2.5px past the border box. `globals.css` also carries
`[data-focus-inset]`, which redraws it inward at `-2px` — the compensation for a
control flush against something that clips.

`data-focus-inset` had **zero users in `src/`**. `check-dead-styles` never
objected because it reads classes, not attribute selectors.

### What was measured

For every focusable element that takes the default ring, expand its box by 2.5px
and ask whether a clipping ancestor cuts it. Three refinements before the answer
meant anything:

1. **Only the RING may poke out.** An element scrolled out of its own container
   reported a 1096px "cut", which is not a defect — the container scrolls to it.
   7 findings -> 4.
2. **A scrollable ancestor pins nothing.** The compaction banner sat 1px under
   the content card's clip edge, but it lives in the transcript scroller, and
   focusing it scrolls it clear — `.msg-scroll-viewport` even carries
   `scroll-padding-top`. So the walk stops at the first scroller instead of
   blaming the clip beyond it.
3. Viewport matters: the suite runs at 1120x720 and the first probe at 1280x800,
   which is why the spec found one the probe had not.

### The finding (已完成)

Three controls, all flush against a box that cannot scroll:

| control | clipped by | effect |
| --- | --- | --- |
| the tool-summary disclosure trigger | `min-w-0 overflow-clip rounded-*` | **no ring at all** |
| `.agent-seam-rail` (drawer resize) | `.agent-shell { overflow: hidden }` | ring caps cut |
| `.agent-pane-resizer` (dock resize) | `.agent-content-card { overflow: clip }` | ring caps cut |
| shiki preview body | `.shiki-block { overflow: hidden }` | 1.5px cut |

**The disclosure is the one that matters, and it is photographed**:
`/tmp/round41/disclosure-{before,after}.png`, 3x, the trigger focused by keyboard
in both. Before there is **no ring whatever**; after there is a clear one. That
control repeats down the whole transcript, so a keyboard user had no focus
indicator on the most frequent thing in the app.

The two resize handles are honestly smaller: they run the full height or width of
their pane, so the caps that were cut fall outside the viewport anyway. They are
fixed because a handle is by definition flush against what it drags — and arrow
keys resize it, so it is a keyboard control with a keyboard-invisible focus. The
marker is on `ResizeHandle` itself rather than remembered at two call sites.

### The detector is now a test

`visual/focusRing.visual.spec.ts` walks eight states and fails with the cut in
pixels, the control, and the box that clips it. Negative-tested by removing the
disclosure's marker.

### Verification

- `typecheck`, `lint`, `format:check`, `knip`, all fifteen guards — green.
- Unit: **2365 passing**; the 8 failures are the `src/rpc/` set blocked in round 38
  (`runtime/internal/application/agent/sessions/` is still uncommitted).
- Visual: **433/433** — 430 goldens with none regenerated, plus the cascade,
  reveal and focus-ring checks. The ring paints only under
  `html:not([data-pointer])`, so no golden could have caught any of this.

### Resources reclaimed

Probe deleted after promotion, port 4174 freed, no stray processes,
`playwright.visual.config.ts` unmodified.

### Next round

Still the six blocked e2e. Otherwise: the same question asked of `:focus-within`
rings and of the `[data-chrome-focus]` controls, which opt out of the ring
entirely on the promise that a row state stands in for it — nothing checks that
the row state is actually there.

## Round 42 — the promise `data-chrome-focus` makes, and the three that broke it

`data-chrome-focus` turns the focus ring off on a stated promise. `menu.tsx`
spells it out: a popup takes focus so the keyboard can drive it, *"the highlighted
ITEM is the indicator here, so the popup opts out the way the design system says a
row state may"*. Fourteen call sites opt out. **Nothing checked the promise.**

### The measurement, and the two probes that lied first

Press Tab until a control that opted out has focus, photograph it, blur,
photograph again. Identical bytes mean a keyboard user sees nothing.

Two earlier versions produced findings I nearly reported and did not:

1. **`element.focus()` is not keyboard focus.** Programmatic focus skips the
   roving-tabindex activation a dock tab uses, so it reported the theme toggle and
   five dock tabs as silent when a real Tab shows them plainly. **7 findings, 5 of
   them artifacts.**
2. **The `Tab` press that arms the ring lands on the first tabbable.** Whatever
   that was got measured already focused and reported "no change" — that was
   "Back to app" in settings.
3. **Reading computed style straight after `.focus()` catches a transition
   starting.** The row carries `transition-[background-color,color]`, so the
   colour had not moved yet.

With real Tab, no pre-focus and a settled transition: **2 findings**, and fixing
those exposed a third.

### The three (已完成)

| control | the promised stand-in | what was there |
| --- | --- | --- |
| `QuestionCard`'s surface | — | a `tabIndex={0}` stop showing nothing |
| `HeaderDiffStat` | — | **no row at all** — it sits in the surface header |
| the **active** dock tab | `focus-within:text-fg` | the active tab already has `data-[active]:text-fg` |

- The question card is focused **programmatically** (`requestRef.current?.focus()`)
  when a question arrives, so the prompt is read and the next Tab reaches the
  first option — the same reason a menu popup takes focus. It is now
  `tabIndex={-1}`: still a landing target, no longer an invisible tab stop.
- `HeaderDiffStat` opted out with nothing to opt into. The attribute is gone and
  it takes the ordinary ring.
- The dock tab's stand-in works for an inactive tab and is invisible on the active
  one, which is exactly where a keyboard lands first. It now takes the ring,
  `data-focus-inset` because the strip scrolls and clips. **Photographed**:
  `/tmp/round42/active-tab-focused.png`, 3x — a clear ring where there was
  nothing.

### The detector is now a test

`visual/chromeFocus.visual.spec.ts` walks the real tab order in four states and
fails with the route, tag and label of anything that opted out and shows nothing.
Negative-tested by putting the attribute back on `HeaderDiffStat`.

### Verification

- `typecheck`, `lint`, `format:check`, `knip`, all fifteen guards — green.
- Unit: **2365 passing**; the 8 failures are the `src/rpc/` set blocked in round 38
  (`runtime/.../sessions/` still uncommitted at 7 files).
- Visual: **434/434** — 430 goldens with none regenerated, plus cascade, reveal,
  focus-ring and chrome-focus. None of this is visible to a golden: the ring
  paints only under `html:not([data-pointer])` and goldens do not focus anything.

### Resources reclaimed

Both probes deleted after promotion, port 4174 freed, no stray processes,
`playwright.visual.config.ts` unmodified.

### Next round

The blocked six, if `runtime/.../sessions/` has settled. Otherwise the fourth
question in this family: `[data-reveal="rest"]`, the other end of the reveal pair,
which is supposed to give way at the same moment — nothing checks the two are
actually synchronised.

## Round 43 — the other end of the reveal, and a spec that could not fail

`globals.css` describes the pair: *"`rest` is the other end of it: what the reveal
displaces, which has to give way at the same moment or the two overlap."* Nothing
checked the two ends were driven by the same condition.

### The finding (已完成)

They were not, and the two lines say so plainly when read together:

```
RESTING_GLYPH  retires on:  group-hover/row  +  group-focus-visible/row-trigger
HOVER_ACTION   appears on:  group-hover/row  +  group-focus-within/row
```

Different pseudo-class, different group. So a state exists where the action
appears and the resting glyph does not retire — **focus landing on the action
itself**, which is where a keyboard puts it. Measured: `restOpacity: 1` and
`actionOpacity: 1` together.

Honestly sized: the two do **not** collide geometrically — 3px apart, so this is
not the overlap the comment warns about. What it is, is a row that looks
different under keyboard than under a pointer, showing its detail and its action
at once in a combination hover never produces.

Both ends now watch `group-hover/row` and `group-focus-within/row`. The
`group/row-trigger` marker existed for that one reference and is gone with it.
After: at rest `1/0`, action focused `0/1`, hovered `0/1`.

The transitions were checked too and are symmetric — same property, duration,
delay and easing on both ends. No finding there.

### The spec that could not fail, twice

Written, passing, and worthless until a negative test said so:

1. **It focused the first control in the row** — the trigger — where BOTH
   conditions hold, so the disagreement never appeared. It has to focus the
   revealed end, which is where the two disagree.
2. **It never moved the pointer away before focusing.** `:hover` had already
   retired the resting end, masking exactly what the step was there to find.

Only after both did removing the fix produce
`rest=1 shown=1 in .group/row`. The lesson is the one this log keeps
re-learning: a check is worth nothing until it has been seen to fail for the
reason it exists.

The pair check covers both mechanisms that carry two ends here, `[data-reveal]`
and `.t-icon-swap`'s `[data-glyph]`, and lives beside the click-trap check in
`visual/reveal.visual.spec.ts`.

### Verification

- `typecheck`, `lint`, `format:check`, `knip`, all fifteen guards — green.
- Unit: **2365 passing**; the 8 failures are the `src/rpc/` set blocked in round 38
  (`runtime/.../sessions/` still 7 files uncommitted).
- Visual: **435/435** — 430 goldens with none regenerated, plus cascade, reveal
  (now two checks), focus-ring and chrome-focus.

### Resources reclaimed

Probes deleted, port 4174 freed, no stray processes,
`playwright.visual.config.ts` unmodified.

### Next round

The blocked six once `runtime/.../sessions/` settles. Otherwise the family is
running out of unexamined members: what remains is `[data-reveal="hover"]` under
`@media (hover: none)`, where the marker forces everything visible — nothing
checks that the row still fits once every action is permanently shown.

## Round 44 — the reservation that did not know the control had grown

`@media (pointer: coarse)` puts a 44px floor under every control, and
`@media (hover: none)` forces every hover-reveal permanently visible. Together
they are a layout **nothing in the suite had ever rendered**: 436 tests, all with
a fine pointer.

### The probe that measured nothing while appearing to work

`Emulation.setEmulatedMedia` accepts a `features` array and silently ignores
`hover` and `pointer`. The first probe ran, produced a tidy report of row heights
and spills, and `matchMedia("(hover: none)").matches` was **false in both modes** —
the whole thing was noise. A touch CONTEXT (`hasTouch`, `isMobile`) is what
Chromium derives those features from.

Once it worked the difference was plain: `min-height` 0 -> 44px, row 34 -> 44px,
the row action's opacity 0 -> 1, the resting glyph 1 -> 0.

### What turned out NOT to be a defect

Fifteen controls measured 44px inside a 28px `--dock-tab-height` row — an eye-
catching "8px of overflow on every dock tab". Photographed at 3x before calling
it anything, and it is **the floor working as designed**: a 44px transparent
target centred on a 28px visible pill, invisible in the render. The screenshots
also show the touch layout doing the right thing — every tab's × permanently
shown, because a touch user has no other way to close one.

### The defect (已完成)

```
14x44px  button "Browse panels"  <->  button "Collapse right workspace"
```

Two controls that belong to different subtrees, sharing 14px of tap target on a
touch screen: a tap aimed at the dock's browse button can collapse the whole
workspace. Zero such pairs with a fine pointer.

The arithmetic is exact. `.agent-dock-control` sits at `inset-inline-end: 6px`
and the tabstrip reserves `--dock-control-span: 36px` for it. The control is
26px, so it occupies 32px and the reservation covers it. Under a coarse pointer
the control is 44px and occupies 50px — **50 − 36 = 14**.

One fact, how wide a chrome control is, written in two places. Both spans now
derive from it: `calc(var(--control-height-sm) + 10px)` normally, and
`calc(var(--touch-target) + 10px)` under a coarse pointer, with `--touch-target`
now naming the floor the rule applies. A visual style that resizes controls moves
the reservations with them.

**And I wrote the override in the wrong place first** — beside the floor rule
that motivates it, which is *earlier* in the file than the `:root` block it had
to beat. Same specificity, so the base declaration won and the fix silently did
nothing, which is the defect over again. The probe caught it; the comment now
says why the override sits where it does.

The two remaining overlaps are a row and the action stacked on it. Round 40 read
that as composition rather than ambiguity and this round keeps that reading: the
action is on top and wins inside its own box.

### The detector is now a test

`visual/touchTargets.visual.spec.ts` runs with `test.use({ hasTouch, isMobile })`
and asserts the context really reports a coarse pointer before measuring
anything — the failure mode above is too quiet otherwise. Composition is told
from adjacency by **containment**, not by a list of known pairs. Negative-tested
by pinning the span back to 36px, which reports the 14x44px pair in two states.

### Verification

- `typecheck`, `lint`, `format:check`, `knip`, all fifteen guards — green.
- Unit: **2365 passing**; the 8 failures are the `src/rpc/` set blocked in round 38.
- Visual: **436/436** — 430 goldens with none regenerated (the desktop values of
  both spans are unchanged at 36px), plus cascade, reveal x2, focus-ring,
  chrome-focus and touch targets.

### Resources reclaimed

Probes deleted, port 4174 freed, no stray processes,
`playwright.visual.config.ts` unmodified.

### Next round

The blocked six if `runtime/.../sessions/` has settled. Otherwise the coarse
layout has more that has never been rendered: `[data-reveal="rest"]` is
permanently hidden there, so every row's detail — counts, timestamps — is gone on
a touch screen, and nothing says whether that was intended or is just what the
pair rule does when it cannot fade.

## Round 45 — the one animation that ignored the motion preference

`ui_rules 9` requires motion to respect the user's preference. This app has two:
the OS `prefers-reduced-motion`, and its own Settings slider published as
`--motion-scale`. Both halves looked handled — `MotionConfig reducedMotion="user"`
in `App.tsx` for the first, and `lib/motion.ts` for the second, whose header
states the contract:

> Every preset's duration multiplies by the published motion scale AT READ TIME,
> so the user's preference reaches every animation **without a hook at each call
> site**.

### The finding (已完成)

One call site did not use a preset. `sidebar/footer.tsx` — the theme toggle's
sun/moon swap — carried `transition={{ type: "spring", duration: 0.3, bounce: 0 }}`.

Measured by watching inline-style mutations during the swap with the preference
at zero: **25 style frames**, scale interpolating 1 -> 0.86 -> 0.74 -> 0.62 ->
0.51, while every other animation in the app was already still. After:

| | `--motion-scale` | style mutations during the swap |
| --- | --- | --- |
| motion off | 0 | **25 -> 3** |
| motion on | 1 | **25**, unchanged |

`0.3s` was never arbitrary: it is this ladder's `slowMs`. The rung existed and
only the preset was missing, which is the exact shape `check-design-tokens`'
header describes — *"a value the ladder cannot express is a signal the ladder
needs a step, not that this callsite needs an exception"*. `lib/motion.ts` gained
`glyphSwapTransition`, a scaled spring at `slowMs`, so the feel at full motion is
byte-identical and the preference now reaches it.

The spring shape is kept rather than folded into the existing tween presets: the
two glyphs travel through one 16px square, and an eased cross-fade there reads as
a dissolve rather than a swap.

### The guard

`check-design-tokens` gained a rule for a literal `duration` inside a
`transition`, exempting `lib/motion.ts` where the ladder is authored. Negative-
tested by restoring the literal, which it reports at `footer.tsx:31`.

### Two things looked at and left alone

- **`[data-reveal="rest"]` under `@media (hover: none)`** is permanently hidden,
  so on a touch screen every row loses its detail — counts, timestamps — while
  its action is permanently shown. That is what the pair rule *must* do when it
  cannot fade, and whether the trade is right is a product decision, not a defect.
  Recorded, not changed.
- Every other `motion` call site already takes a preset from `lib/motion.ts`; the
  scan found exactly one literal.

### Verification

- `typecheck`, `lint`, `format:check`, `knip`, all fifteen guards — green.
- Unit: **2365 passing**; the 8 failures are the `src/rpc/` set blocked in round 38
  (`runtime/.../sessions/` still 7 files uncommitted).
- Visual: **436/436**, none regenerated. The suite runs with motion at zero, where
  the fixed animation is now instant — which is why no golden moved.

### Resources reclaimed

Probes deleted, port 4174 freed, no stray processes,
`playwright.visual.config.ts` unmodified.

### Next round

The blocked six if `runtime/.../sessions/` has settled. Otherwise: `App.tsx` sets
`MotionConfig reducedMotion="user"`, which answers the OS preference, and
`lib/motion.ts` answers the app's — but nothing checks the two agree when they
disagree, e.g. OS reduced-motion ON with the app slider at full.

## Round 46 — seven ways to say "waiting", one of them shipped

### First, a null result on the round's stated direction

Round 45 ended asking what happens when the two motion preferences disagree.
Measured across the matrix rather than reasoned about:

| | `--motion-scale` | OS reduce | JS frames | CSS transition | keyframes |
| --- | --- | --- | --- | --- | --- |
| os normal + app full | 1 | false | 25 | 0.1s | 1.6s |
| os normal + app off | 0 | false | **3** | 0.001s | 0.001s |
| os reduce + app full | 1 | true | **7** | 0.001s | 0.001s |
| os reduce + app off | 0 | true | **3** | 0.001s | 0.001s |

All four cells correct. The 7 rather than 3 is not a leak:
`MotionConfig reducedMotion="user"` suppresses transform and layout by design and
keeps opacity, because the OS preference is about vestibular motion while the
app's slider at zero means no motion at all. **Two preferences, two meanings, two
behaviours — recorded and left alone.**

### The finding (已完成)

`Loader` offered seven variants — `dots`, `typing`, `pulse-dot`, `wave`, `bars`,
`terminal`, `text-shimmer`. Every `<Loader>` in the tree, both of them, asks for
`text-shimmer`. Six implementations and five sets of `@keyframes` animated
nothing.

`ui_rules 2` forbids one function having several treatments, and nothing here
could see it: a `variant` the product never passes is still reachable through the
union, so `knip` reads `Loader` as used and `check-dead-utilities` reads every
`animate-[flame-loader-*]` class as emitted — each is named in the source that
renders it, which is exactly the check's question.

`animate-pulse-dot` was checked separately before deleting `PulseDotLoader`: it
has **eight** users across the tree, so `flame-pulse` and its theme entry stay.
The five `flame-loader-*` keyframes each had exactly one, inside `loader.tsx`.

| | before | after |
| --- | --- | --- |
| `loader.tsx` | 177 lines | **49** |
| `globals.css` | 1349 lines | **1292** |
| emitted utilities | 981 | **971** |
| entry CSS | 112.3 KB | **111.8 KB** |
| entry JS | 2476.7 KB | **2474.8 KB** |

The component says one thing now, and the header says why a variant that comes
back is one rung to add rather than six kept warm.

### Verification

- `typecheck`, `lint`, `format:check`, `knip`, all fifteen guards — green.
- Unit: **2365 passing**; the 8 failures are the `src/rpc/` set blocked in round 38.
- Visual: **436/436**, none regenerated — the one variant that shipped is
  untouched, so nothing moved.

### Resources reclaimed

Probe deleted, port 4174 freed, no stray processes,
`playwright.visual.config.ts` unmodified.

### Next round

The blocked six if `runtime/.../sessions/` has settled. Otherwise the same
question asked of the rest of `ui/atoms`: a `variant` union the product never
exercises is invisible to every guard here, and `Loader` is unlikely to have been
the only one.

## Round 47 — six null results, one of them a rule I rebuilt without noticing

Round 46 ended on a hypothesis: `Loader` carried six variants nothing asked for,
so *"`Loader` is unlikely to have been the only one."* **That was wrong**, and the
rest of the round went the same way. Recording it so none of these is re-opened.

### 1. `cva` variant values nothing passes — inconclusive

A sweep of every `cva` variants block in `ui/` reported ten unused values. Every
one was a false positive: `Well`'s `wrap` arrives through
`wrap={body.isJson ? "pre" : "anywhere"}`, `StatusDot`'s and `Badge`'s tones
through lookup tables (`STATUS_TONE[phase]`, `SCOPE_TONE[scope]`), `Button`'s
`icon-*` sizes through `IconButton`'s own record.

Broadening "asked for" to *the literal appears anywhere outside the defining
file* took it to zero — and that is not evidence of health, it is the test
becoming useless, since `"md"` and `"error"` appear everywhere for unrelated
reasons. Deciding this properly needs type analysis, not grep. **Left undecided
rather than reported either way.**

### 2. Other hand-rolled variant switches — none

`Loader` was found because its union was a hand-written `switch`, which makes the
call sites enumerable. There is no other `switch` on a variant prop anywhere in
`ui/`. The hypothesis is closed.

### 3. Dead catalog keys — 0, and **already guarded**

Measured directly: of 1099 `en` keys, 185 literals never appear in source, which
falls to **zero** once i18next plural suffixes and seventeen template prefixes
(`` t(`rpcError.${type}`) ``) are discounted.

I then built that measurement into `check-locales` as a new rule — and only after
it reported five orphans did I find **rule 9 already there**, documented in the
header and implemented at line 618, doing exactly this. The five were my own bug:
`NAMED_STRING` used `"([^"\n]+)"`, and an adjacent EMPTY literal —
`lines.push("", t("a.b"))` — leaves the engine unable to pair `""`, so it pairs
the second quote with the key's opening one and swallows the key. Rule 9 uses
`blob.includes()` and has no such seam. **Reverted in full.**

### 4. Plural forms — already guarded

Round 28 fixed 22 keys interpolating a count with no plural form by hand. A guard
does exist: `check-locales` compares plural FAMILIES rather than keys and holds
each locale to `Intl.PluralRules`, and rule 15 refuses `{{count}} task(s)`.

### 5. Overlays outside the viewport — 0

`ui_rules 12`. Right-clicked every trigger within 220px of an edge across five
states and measured every open menu, dialog, listbox and tooltip. Nothing escapes.

### 6. The `1x1` input from round 40 — not a target

`HiddenFileInput` is `className="hidden"`, so `display: none`: no box on a fine
pointer and none under the 44px coarse floor either, which does apply to it.
Loose end closed.

### What this round produced

No code change. Six directions closed, one existing rule found that I had been
about to duplicate, and one wrong hypothesis retracted. The log is the artifact.

### Verification

`check-locales` clean at 1106 keys across 8 locales; the working tree carries no
change from this round.

### Resources reclaimed

All four probes deleted, `check-locales.mjs` restored to HEAD, port 4174 freed,
no stray processes.

### Next round

The blocked six if `runtime/.../sessions/` has settled — it is still 7 files
uncommitted. Otherwise the honest note from this round is that grep-shaped audits
are running out: what is left needs the type checker, which is how
`check-design-system-boundaries` already reads the tree.

## Round 48 — the question round 47 could not answer, asked with the AST

Round 47 abandoned "which component API does nothing exercise?" because grep
cannot tell which component an attribute belongs to. The AST can, and
`check-design-system-boundaries` already reads the tree that way.

### Two false starts, both from the probe

- `project.program.getTypeChecker` does not exist on the sync API, and the
  `project.checker` that does exposes `getTypeAtLocation` and reference lookups
  but no `getPropertiesOfType`. So props are read syntactically instead, from the
  `interface *Props` or inline type literal each component annotates — **own
  members only**, since a component extending `ComponentPropsWithoutRef<"div">`
  inherits hundreds of DOM props nothing passes and nothing should.
- The first run reported twelve components, **nine of them declaring `text`**.
  `interface Props` is the commonest local name in this tree, and keying the
  props map by name alone let the last file parsed answer for every component
  annotated with it. Keyed by file and name: **twelve → five**.

### The five, each read before touching anything (已完成)

Every one is declared, wired internally, and passed by no call site.

| prop | what it does | verdict |
| --- | --- | --- |
| `CatalogSearch.onEscape` | wired to `onKeyDown` | **deleted** — the name promises Escape and the wiring takes any key; the one call site passes neither |
| `ShikiCodeBlock.file` | renders a filename header | **deleted**, header and all — a branch that never ran |
| `SkeletonList.style` | forwarded to the element | **deleted** — CLAUDE.md §4 allows inline style only for a value computed at runtime, and offering the prop invites what the rule discourages |
| `ScrollArea.style` | forwarded to the element | **deleted**, same |
| `Checkbox.disabled` | styles the label and disables the primitive | **kept** |

`Checkbox.disabled` was checked for the more interesting failure first — a prop
that styles a control as disabled without disabling it — and it does pass
`disabled` to `CheckboxPrimitive.Root`, so there is no latent bug. It stays
because `disabled` is a state every control in the library has (`Button` inherits
it from the DOM props it extends), and a closed interface omitting it would make
the checkbox the one control that cannot be turned off. The reason is now written
at the declaration so the next audit does not re-open it.

### Why this did not become a guard

The audit is worth running and wrong as a gate. A prop legitimately precedes its
first call site inside a single change, and the one survivor —
`Checkbox.disabled` — would need an exception. A hand-maintained exception list is
the thing this log keeps removing, and it would be worse than re-running the
audit when someone wants it.

### Verification

- `typecheck`, `lint`, `format:check`, `knip`, all fifteen guards — green.
- Unit: **2365 passing**; the 8 failures are the `src/rpc/` set blocked in round 38
  (`runtime/.../sessions/` still 7 files uncommitted).
- Visual: **436/436**, none regenerated — four of the five props were never
  passed, so nothing they controlled was ever on screen.

### Resources reclaimed

Probe deleted, port 4174 freed, no stray processes,
`playwright.visual.config.ts` unmodified.

### Next round

The blocked six if `runtime/.../sessions/` has settled. Otherwise the same AST
lens has one more question in it: a prop that IS passed but always with the same
literal is a variant point that never varied, which is the `Loader` shape one
level down.

## Round 49 — the six "blocked" e2e were never blocked

Round 38 set six e2e failures aside on the reasoning that
`runtime/internal/application/agent/sessions/` was uncommitted and editing the
frontend to accept the current answers would freeze a half-finished backend.
**That reasoning was wrong, and this round proves it.**

### The experiment

`git worktree add --detach /tmp/flame-head HEAD` — an isolated checkout that
touches the working tree not at all — with `node_modules` symlinked in. The e2e
builds the runtime from `../../runtime`, so a clean worktree builds a clean
runtime.

All six reproduce there. They are not the in-flight work; they are red at HEAD.
Which also means **`npm run check`, the gate this repo relies on, has been red**,
since the e2e is in `src/**/*.test.ts` with no skip and no env gate.

One methodological trap on the way: checking out an old commit reverts the
FRONTEND too, so the first bisect step measured an old pairing and reported
round 38's already-fixed path-echo failure. The isolation that answers the
question is tip frontend with `git checkout <old> -- runtime`.

### Four of the six, one cause (已完成)

`FLAME_HOME` is not the data directory. `runtime_bootstrap.go` roots it one level
deeper:

```go
dataDirectory, err := localruntime.DataDirectoryAt(filepath.Join(flameHomePath, "runtime"))
```

Every user-scoped store hangs off THAT — `SkillsUserDir`, `RecipesGlobalDir`,
`knowledgefile.New(config.DataDirectory, …)`. The suite wrote its fixtures a
level up, at `$FLAME_HOME/skills`, `$FLAME_HOME/recipes`, `$FLAME_HOME/FLAME.md`,
where nothing reads them. That single mistake produced four unrelated-looking
failures:

| symptom | cause |
| --- | --- |
| `RpcError: internal_error` moving a managed Skill | the skill was never there |
| skill archive/restore returns `data: []` | same |
| `knowledge.changed` never arrives | the user `FLAME.md` was never there |
| `recipes.list()` omits the global recipe | the global recipe was never there |

A `runtimeStore` derived once, with the layout written down beside it, and the
one assertion that spelled the old path out. **6 → 2.**

### The two that remain, and why I stopped

Both compaction cutpoints time out: the run never reaches the provider call that
should carry the summary. Two hypotheses tested and discarded — the trigger
estimates tokens locally from the request payload, so a fake provider not
reporting usage cannot be it; and tripling `compactionSteerCount` from 21 to 60
does not help, so it is not a stale tuning constant. What is left is inside the
runtime's compaction path, which this scope does not modify and which would cost
more to diagnose than the finding is worth from here.

The other two failures are the round-38 pair: `segment.finished.json` predates
its own required `contextTokens` field. One line, in `runtime/`.

### Verification

- `typecheck`, `lint`, `format:check`, `knip`, guards — green.
- Unit: **2369 passing, 4 failing** — up from 2365/8.
- Worktree removed; `git worktree list` shows only the main tree.

### Next round

The four remaining failures all need `runtime/`. Everything this scope can reach
in that area is done, and the log should say so plainly rather than keep
re-listing them as "blocked".

---

## Round 50 — every state "jumped", and the ruler was wrong

`ui_rules 12` asks that loading and content replacement not visibly jump, and
nothing in the suite had ever read the browser's own record of it. A probe over
all 31 fixture states, collecting `layout-shift` entries with their sources,
reported **31 of 31 shifting after first paint** — a universal `3,3 → 0,0` on
`#root` plus reading-column blocks moving ~122px.

Neither was real, and finding that out was the round:

- The `#root` shift is the dev server delivering CSS through the module graph,
  so the first paint is an unstyled root. `dist/index.html` links the stylesheet
  in `<head>`, where it blocks paint — production cannot do this. CLAUDE.md §5
  names exactly this trap.
- The 122px move lands at t=448ms against a first-contentful-paint at t=472ms:
  before content exists, so nothing the user can see moved.

Re-measured from first contentful paint: **0 of 31**. The load path was already
correct.

### Changed

`visual/layoutShift.visual.spec.ts` — the measurement kept as a standing
instrument, three tests over the fixture-exported state lists plus a negative
test that injects a post-paint jump, so the observer is known to fire rather
than passing because it never installed.

### Verification

`npm run check` guards green, unit 2369/4 (pre-existing), visual 436 → 440.

### Next

The instruments themselves — the round's lesson was that a probe reports
whatever its baseline lets it.

---

## Round 51 — a model's own words could not break the line

A model writes hashes, base64 and paths with no space in them, and no fixture
contains one: snapshots are prose, so a renderer that forbids the break paints
correctly in every golden. Injecting a 217-character unbreakable run into every
text-holding element found the case.

First pass over `p, li, code, pre, …` reported nothing. Widening to *any*
element that directly holds text — which is where the tool previews live, in
`div` and `span` — found two:

| site | what it renders |
| --- | --- |
| `QuestionCard.tsx` `<h3>` | the model's own question |
| `ApprovalCard.tsx` title | the tool's approval title, often a command |

Both paint ~1200px outside their card. Both had hand-copied the same class
string, which is how the property went missing in both at once.

### Changed

`wrap-anywhere` at both — the spelling that also shrinks min-content, so the fix
survives either card later becoming a flex track. `visual/unbreakableContent.visual.spec.ts`
holds it, auditing the panel only: the agent fixture writes its own drawer
caption, and scaffolding failing a product rule teaches the wrong lesson.

### Verification

Guards green, unit 2369/4, visual 442 — no golden moved, because no fixture
contains the input.

---

## Round 52 — the sweeps were walking past most of the controls

Three audits ask the same question — a control cut in half, a control too small
to hit, a control drawing its own focus ring — and each had written its own
answer:

| audit | names |
| --- | --- |
| clipping | button, a[href], input, textarea, select, tab, option |
| touch targets | …plus role=button, menuitem, switch |
| focus ring | …plus tabindex, minus input/textarea/select |

The clipping list was the narrowest and never named `[role="button"]`,
`[role="menuitem"]`, `[role="checkbox"]` or `[role="radio"]`. Worse, **none of
the three ever opened an overlay**: the finder and the command palette — 23
options across the two most keyboard-driven surfaces in the app — were audited
by nobody.

The focus sweep also waited a flat 200ms instead of for the fixture, and Shiki
highlights asynchronously: it had **never once seen a highlighted code block**.

### Changed

`visual/controls.ts` names the two sets once — a control, and focusable, which
is wider because a scroll region carries `tabindex` without being a target.
Overlay routes added to all three. Both remaining sweeps assert their own reach,
so a selector that stops matching fails instead of passing against an empty tree.

Given a code block, the focus sweep reports a real defect: Shiki gives its
`<pre>` a tabindex, so the scrollable code takes keyboard focus, and the rounded
block wrapping it clips the ring by 2.5px. The `<pre>` now declares
`data-focus-inset` through a transformer — what the preview body three lines
below already says in JSX.

### Verification

Guards green, unit 2369/4, visual 444 — the `data-focus-inset` attribute changes
no pixel until something is focused.

---

## Round 53 — the workspace surface had two owners

Same defect class as round 52, one file over: `cascade.visual.spec.ts` waited a
flat 200ms and had been walking a partial tree. Given the whole one it reports
two call sites writing a class the cascade discards.

`AgentWorkspaceView` asked for `bg-canvas` while `.agent-context-dock
.agent-workspace-view` read the same fact off DOM ancestry and rendered it
transparent inside the dock. The placement is already owned — `ViewPlacement`
says "full" or "dock" — so the ancestry rule was a second answer to a question
that had one.

The view header restated `gap-2` over a header that had already fixed the same
8px: same pixels either way, one of them a decision and the other a copy.

### Changed

The surface sits with the rule that already owns the class, stating the canvas
and its one exception together. `gap-2` removed from both `ViewHeader` sites.

### Verification

Guards green, visual 444 — pixel-identical, which is the point: moving a value to
its owner should change nothing on screen.

---

## Round 54 — a diagram's hidden actions took clicks

Third audit in a row navigating without waiting for the fixture. `reveal.visual.spec.ts`
has two tests; the second waits properly, the first did not, and had never seen
a rendered mermaid diagram.

Given one, it reports what the contract exists to prevent: the diagram's zoom
and copy actions sit at `opacity-0` while still taking the pointer, so a click
in that corner lands on a control nobody can see.

### Changed

`MermaidBlock.tsx` pairs both halves — invisible means `pointer-events-none`,
and hover and focus restore them together — the way `shiki-code-block.tsx`
already does three files over.

### Verification

Guards green, unit 2369/4, visual 444.

---

## Round 55 — twelve of thirteen ladder rules fired

Mutation testing on `check-design-tokens`: one injected violation per rule, in a
temporary file under `src/`. Twelve fired. The silent one is the rule that exists
because a literal duration escaped once already and animated for 25 style frames
with the user's motion preference at zero.

It reads `transition` and `duration:` from a **single line**, and prettier breaks
that object as soon as it carries a second key.

### Changed

That rule now runs against whole file text and reports the line the match starts
on. Nothing in the tree violates it today; the shape that would have is no longer
invisible. The walk also states its own reach — 1246 files — so a broken glob
fails instead of printing the same OK as a full pass.

### Verification

Re-mutated in both shapes: single-line and wrapped, both caught.

---

## Round 56 — the §5 rule nothing was watching

CLAUDE.md §5 requires `disposeOnHmr` for any module-scope subscription. The
helper exists and all three call sites use it — and nothing kept it that way.

The failure mode is why it needs a guard rather than care: correct in
production, correct on a fresh dev boot, slower the longer someone works. No
error, no failing test, nothing visible in a diff, and by the time it is felt the
cause is thirty reloads back.

### Changed

`scripts/check-hmr-disposal.mjs`, wired into `npm run check`. It runs its own
detector against four fixtures before walking anything — a bare subscribe, one
with disposal, one nested in a function, a module-scope listener — because a
pattern that quietly stops matching reads exactly like a clean tree.

### Verification

878 files, 3 registrations, all disposed. Mutation-verified: dropping the
disposal fails the guard.

---

## Round 57 — content-visibility works; the estimate of its margin did not

`transcriptTurnContentVisibility` gives every non-tail turn `content-visibility:
auto`. Measured with `checkVisibility({contentVisibilityAuto: true})`, nothing
was ever skipped, including turns 706px above the viewport.

Three probe corrections before the answer:

1. The element carrying `content-visibility` is itself rendered — only its
   contents are skipped. The question has to be asked of a descendant.
2. `contain-intrinsic-size: auto 220px` REMEMBERS the last rendered size, so
   height is not a skip signal either.
3. Calibrated against a synthetic block 5000px below the fold, the descendant
   test flips true. The real turns needed the same distance: pushed to −4825px,
   a turn skips.

Chrome's relevance margin is far wider than the 50%-of-viewport I assumed, and
the fixtures are too short to reach past it. **No defect, no change** — and no
false report, which was the round's actual product.

---

## Round 58 — the large-refactor sweep says nothing needs doing

`REFACTORING.md` calls for a whole-tree pass every fifteen to twenty rounds.

- Largest non-locale module: 670 lines. No oversized file.
- A 7-line clone detector over `src/` finds 8 duplicated blocks: theme palettes
  (data), a fact and its projection sharing identity fields (idiomatic here), and
  two consumers calling one owner (not duplication).
- The session-command prologue repeats twice — below the 3+ threshold.
- knip, circular, layer and context guards clean.

**No structural change.**

---

## Round 59 — thirteen locale rules, all still firing

Mutation testing on `check-locales`, the densest rule set left. Seven code-facing
rules exercised from a temporary file in each ring; six catalog-facing rules from
a locale perturbation reverted with `git checkout` immediately after.

All thirteen fire. **No dead rules, no change.**

---

## Round 60 — every dark machine opened on a white frame

First audit of the Go shell. The window carries a colour until the WebView
paints, and it was the light canvas unconditionally. The reasoning recorded with
it — Go cannot read a preference the WebView owns — is true and was the right
call. But that preference DEFAULTS to "system", and system resolves to the OS
appearance, which Go can read without holding a second copy of anything.

### Changed

`system_appearance_darwin.go` reads `AppleInterfaceStyle` through NSUserDefaults
(the window's colour is decided before there is an NSApplication to ask).
`desktopWindowBackground()` states both canvases and picks. macOS only: other
desktops answer the scheme question through GTK, Qt and the portal separately
and disagree.

`check-bootstrap` pins the pair rather than the single value; a Go test holds the
branch, which no regex over the file can see.

### Verification

Mutation-verified on both literals. The appearance read itself only ran light
here — this machine reports no `AppleInterfaceStyle`, and flipping a real user's
system appearance to test it is not mine to do.

---

## Round 61 — the endpoint stated twice, compared by string

`desktop_host.go` and `runtimeEndpoint.ts` each state where the local Runtime
listens. `container.ts` attaches the local gate token **only** to a client whose
endpoint equals the one `Bootstrap()` handed over.

A drift between them does not fail to connect. It connects with no token, and
neither side says why.

### Changed

`check-bootstrap` pins the two. The Runtime owns the port and publishes no
constant for it, so the shell cannot read the value rather than restate it —
what it can stop doing is restating it unwatched. Mutation-verified.

---

## Round 62 — the IPC surface, named once

`binding_names_test.go` pins the exported method set of `DesktopHost`, because
in v3 that set IS the application's entire IPC surface. Mutation-verified: an
ordinary-looking setter fails it.

The list was written twice in that file — once as the set, once as the names
checked against the frontend's spelling. Only the first fails when the surface
widens, so a deliberate addition would update it and leave the new name unchecked
on the boundary it actually crosses. One declaration now, read by both; the test
name loses its count with it.

---

## Round 63 — no nested scroller anywhere

A scroller inside a scroller sends the wheel to the wrong box. Eight states
audited for a scrollable ancestor containing a scrollable descendant: **zero**.
Self-tested by injecting a nested pair, which reports 3. No change.

---

## Round 64 — placeholder parity holds

A translation that drops `{{count}}` renders a sentence missing its number, in
one language only. `check-locales` rule 7 compares placeholder sets against `en`.
Mutation-verified by removing one from `zh`: caught. No change.

---

## Round 65 — the bundle is already tuned

Entry payload 2474.8 KB against a 2719.7 KB budget; syntax highlighting (737 KB),
diagrams (1542 KB), settings panes, workspace views and the math stylesheet all
lazy. Seven of eight locale catalogs are lazy plugins; only `en` is static.

The budget's own comment records that it once sat at 3.5× the actual payload —
a gate measuring nothing — and was corrected. No measurement here shows startup
parse to be a bottleneck, and AGENTS.md forbids optimising without one.
**No change.**

---

## Round 66 — a control painted under the caption beside it, in 57 goldens

Found by looking at a golden rather than at code. A grey square sat behind the
"e" of "Agent states" in every one: the sidebar collapse control, drawn under the
caption.

Both fixtures had asked for the clearance — `pl-[78px]` — and neither got it.
`.agent-surface-header` is unlayered and spells that edge logically,
`padding-inline`, and an unlayered rule beats `@layer utilities` whatever its
specificity.

**The cascade audit exists to catch precisely that and could not see it**: it
compares the two rules by property NAME, and `padding-left` is not
`padding-inline-start`.

### Changed

The audit folds logical properties onto their physical edges before comparing.
With that it finds this and nothing else — no product call site writes a utility
that loses this way.

The caption moves rather than growing a bigger padding: the product's own drawer
header is EMPTY in that band, because the control is placed at the same gutter
the header's content box starts at. Scaffolding that mirrors the product cannot
teach the wrong thing.

### Verification

57 goldens regenerated. Verified confined: above the antialiasing floor, no
golden changes a pixel outside the drawer. (The raw difference box reached the
whole frame; thresholding at 24/255 to ignore antialiasing is what made the claim
checkable.)

---

## Round 67 — one row out of alignment in a column of seven

Every control in the appearance pane ends at the card's inner edge —
`SettingRow` puts them there. The font size segments sat in a nested grid that
`SettingRow` does not reach, content-width and left-aligned, so they stopped 28px
short while the two fields directly above and the three segmented controls around
them all landed on the line.

| control | right edge |
| --- | --- |
| Accent tint, Density, Corners, both font fields | 1007 |
| Font size | 979 |

### Changed

`justify-self-end` on the one control the pane's own idiom had missed.

### Verification

All four segmented controls now share 1007. Checked at the largest UI text the
app allows, where the segments are widest: 23px still between them and their
label, page overflow 0. Four goldens regenerated, each differing only across that
row.

---

## Round 68 — a sparkline and a tab strip, both correct

Two candidates from reading the dock goldens, both refuted by measurement:

- The zigzag crossing the `apply_patch` bar is a 48×16 sparkline — `polygon` +
  `polyline`, drawn only when a tool has more than one call — and it does not
  overflow its own box.
- Four dock tabs sit past the panel's right edge at the minimum window. The
  strip's parent `.agent-dock-tabs` is `overflow-x: auto` with 414px of scroll,
  and the half-visible tab at the edge is the affordance. Reachable.

**No change.** The round's value is the two reports not filed.

---

## Round 69 — 390 minutes is not a reading on any clock

`Working · 390m 0s`. Both duration formatters — `fmtDuration` and the translated
`durationText` — stopped at minutes, and neither's tests had ever gone past
twelve. The repository names long execution as a thing to hold.

### Changed

Both roll up the way a clock does, dropping the finest unit as the coarsest
grows: 59m 59s, then 1h 00m. Eight catalogs carry the hour string; the
untranslated one pads minutes the way it already pads seconds, so the column
still aligns.

### The instrument round 50 built, re-anchored

Regenerating the goldens exposed it. Its baseline was first CONTENTFUL paint — a
browser milestone racing the app's own staged loading. `dock-tools` fills in
waves, and whether one landed before or after that milestone decided the run:
three failures in four, then none in the next.

It now cuts at the fixture's ready signal, the app's own claim that it has
finished, which is the only line a jump can be judged against.

The first attempt at that armed a `MutationObserver` on a document element the
init script runs before, so the stamp never landed and every shift was filtered
— **an instrument reporting a perfectly still page because it was not looking.**
The negative test caught it. Arming now waits for the element, the cut is made
from timestamps after the fact, and a missing stamp throws instead of passing.

### Verification

Ten goldens regenerated, each differing only across a 48×9px box. Spec run three
times consecutively: 4/4 each. Disarming it deliberately fails all four.

---

## Round 70 — the protocol references pointed at three files that do not exist

Four documents sent the reader to `runtime/doc/API.md`, `runtime/doc/AUX_API.md`
and `runtime/doc/TRANSPORT.md` for the authoritative protocol. None of the three
exists. The Runtime moved that material:

| was | is |
| --- | --- |
| `doc/API.md` | `contract/API_REFERENCE.md` (generated) over `manifest.json` / `openrpc.json` / `schema.json` |
| `doc/AUX_API.md` | the same file's HTTP endpoints section — where the sidecars are |
| `doc/TRANSPORT.md` | `doc/ARCHITECTURE.md`, which holds transport and projection boundaries |

Ten references repointed at what each was actually reaching for. Every markdown
link in the desktop documentation set now resolves.

---

## Round 71 — the rest of the documentation holds

Same lens, the remaining surfaces:

- 44 design tokens named across four documents, two apparently missing. Both
  false: `--font-ui` is named to say it does NOT exist, and `--sidebar-width` is
  written from `app-shell.tsx` in a `useLayoutEffect`, so it is set before paint
  rather than declared in the sheet.
- Every directory the architecture documents name exists, except `src/domain/`,
  which `FRONTEND_PLUGIN_CONTEXTS.md` names to argue it should not.

**No change.**

### Resources reclaimed, rounds 50–71

Vite fixture servers on 4174 stopped after each probe; all `visual/probe-*.mjs`
scratch files deleted; the temporary mutation files in `src/` and the locale
perturbations removed and verified with `git status`; the round-49 worktree
already gone.

### Next

This log itself was the gap: it stopped at round 49 while twenty-two rounds ran.
Written now, and the loop's own bookkeeping is part of the loop.

---

## Round 72 — the log itself

Status: **complete**

`refactor-prompt.md` §2 and §6 make this file part of the loop. It stopped at
round 49 while twenty-two rounds ran. Written up from the evidence each round
produced; six of the twenty-two changed nothing, and those entries are the ones
worth having, because a round that files no report costs the next reader the same
question.

---

## Round 73 — re-reading the instruction I was executing

Status: **complete**

Finding round 72 in the standing prompt was reason to read the whole of it again
rather than the part I remembered. Two more things it asks for that I was not
doing, and one honest correction:

### `study/chatgpt` is the pixel reference (priority 5) and I had never opened it

It is at `~/Desktop/study/chatgpt` — 787MB, `extracted/webview/assets/*.css`.
The comparison, on the two measurements the transcript actually hangs off:

| measurement | reference | Flame | |
| --- | --- | --- | --- |
| reading column | `--thread-content-max-width: 48rem` | `--content-max: 768px` | same |
| transcript top inset | `--thread-content-top-inset: calc(var(--spacing) * 8)` = 32px | measured 32px | same |

Both already match exactly. The alignment predates this loop; not having consulted
the reference left no gap in the values that carry the layout.

Its radius and spacing ladders do not compare usefully — they are minified into
utilities, while Flame publishes a `--shape-*` ladder with a user-adjustable
`--radius-scale`, which is the richer system. And the prompt's own priority order
puts the repository's design system (4) ABOVE pixel alignment (5), so a documented
Flame decision wins where the two differ. Mining further has low yield and would
risk overwriting deliberate choices.

### `<round_report>` asks for a before/after table

Reports had been prose. Adopted from this round.

### Scope, stated plainly

`<scope>` forbids proactively changing "infrastructure, performance work or
architecture refactoring not directly related to UI". Four rounds were outside
it: 56 (HMR disposal guard), 61 (endpoint pin), 62 (IPC surface), 70 (protocol
doc references). Each was a real defect and each is recorded above, but the
instruction did not ask for them, and saying so is worth more than the four
findings.

### Next

Back inside the scope the prompt draws: visible state, interaction, and the
components that render them.

---

## Round 74 — two states, one channel, and the preference that removes it

Status: **complete**

`ui_rules 5`: a state change may not be carried by colour alone. The Work Index
marks a session's attention with a 6px dot:

```tsx
session.attention === "running" ? "bg-accent animate-pulse-dot" : "bg-warning"
```

Two channels, colour and motion — and `globals.css` answers
`prefers-reduced-motion` by setting `animation-iteration-count: 1` and
`animation-duration: 1ms` on everything. **The pulse is the only non-colour
difference, and the preference takes it away.**

The accent is also the user's to pick. Measured against `--color-warning` in
CIE Lab:

| accent | ΔE light | ΔE dark | hue apart |
| --- | --- | --- | --- |
| Blue (default) | 111.9 | 121.3 | 179° |
| Purple | 140.1 | 141.5 | 214° |
| Green | 56.4 | 65.5 | 120° |
| **Orange** | **37.3** | **49.0** | **22°** |

Orange is in the swatch row. Two 6px marks 22° apart in the same warm family is
hard for anyone, before considering colour vision at all.

### Before / after

| | before | after |
| --- | --- | --- |
| running | filled accent dot, pulsing | accent **ring**, pulsing |
| needs input | filled warning dot | unchanged |
| under reduced motion | two dots, hue only | ring vs solid |
| in greyscale | indistinguishable | distinguishable |

Hollow reads as "under way" and solid as "your turn", which is also the right
weight: the state that wants an answer is the heavier mark.

### Verification

- Captured at 1×, 2×, light and dark, then converted to greyscale — the proof
  the rule actually asks for. `/tmp/dots-{1x,2x,dark}-gray.png` at capture time;
  the reproducible form is the fixture route plus the three-line probe in this
  entry's commit message.
- `typecheck`, `lint`, `format:check`, guards green. Unit 2370/4 (pre-existing).
- Visual 444/444, **including every WCAG audit** — the accent ring on the sidebar
  surface holds contrast in both themes.

### A note on what the goldens did not catch

No golden moved. A 6px mark changing from filled to ring differs by fewer pixels
than `maxDiffPixels: 40`, so the pixel suite tolerates it. That tolerance exists
for antialiasing and is correct, but it means small marks are not protected by
the goldens — they are protected by looking, which is how this one was found.

### Next

The same question of the other status marks: the breadcrumb chip names its state
in words, which is fine; the tool-stats badges and the dock tab dots have not
been checked.

---

## Round 75 — the view no fixture had ever opened

Status: **complete**

Round 74's fix raised the general question, so the sweep: every painted mark
under 18px with no text of its own, across twelve states. Ten distinct marks,
eight without an accessible name — and all but one of those sit beside their own
word ("Running", "Needs input"), which is what the rule asks for.

Reading the last one led somewhere else. `timeline.tsx` maps four statuses onto
three colours and nothing else:

```
ok → bg-success    err → bg-negative    approved → bg-success    declined → bg-warning
```

`tool-start` and `tool-end` carry the same kind icon and the same label, so **a
succeeded tool call and a failed one differed by one 6px circle being green
instead of red** — the pair colour vision fails on, with nothing else in the row
to read instead.

### Why it survived

`timeline` is one of **13 of the 20 registered workspace views that no fixture
state ever opens.** It is listed as a tab — which is why "Timeline" appears in
the strip — but never made active, so nothing in the suite had rendered it. Not
the goldens, not the WCAG audits, not the contrast or clipping sweeps.

| | rendered by a fixture | never rendered |
| --- | --- | --- |
| views | plan, inbox, tool-stats, tools, file, diff, catalogue | search, explorer, files, terminal, skills, skill-proposals, skill-library, recipes, knowledge, agent-memory, agent-docs, run-summary, timeline, notifications |

### Before / after

| | before | after |
| --- | --- | --- |
| ok / approved | green dot | `check` glyph, success |
| err | red dot | `alert` glyph, negative |
| declined | amber dot | `x` glyph, warning |
| in greyscale | one circle, four meanings | three distinguishable marks |

The row's left-hand kind mark already speaks in glyphs; this answers in the same
vocabulary, and `ok` beside `approved` stays legible because their kind marks
differ.

### The regression on the way, and the guard for it

First attempt moved `aria-label={entry.status}` onto the `Icon`. It compiles —
**TypeScript does not check hyphenated JSX attributes against a component's
props** — and `Icon` renders `aria-hidden` and destructures four props, so the
name was silently dropped. Types passed, tests passed, and only a screen reader
would have noticed.

The label now lives on a wrapper that claims `role="img"`. `check-chrome` grew
one rule for the trap, mutation-verified: `<Icon … aria-label=` fails the build.

### Coverage added

`dock-timeline`, which brings the view into every sweep the state list drives.
`workspace.visual.spec.ts` refuses a state with no declared ready boundary — a
good refusal, because this view resolves a query and takes ~3s; ready is the
failed patch's own mark, the thing the state exists to photograph.

### Verification

- Captured at 2× and converted to greyscale, before and after.
- Accessible names read back from the DOM: `ok ×5, err, declined`.
- 20 goldens moved by the added sidebar row; verified confined — 0 change a pixel
  outside the fixture sidebar. Regenerated.
- Visual **452/452** (444 before; the new state adds eight). Guards green, unit
  2370/4 pre-existing.

### Next

The other twelve unopened views. Each is a surface the suite has never seen, and
the first one opened had a `ui_rules 5` defect in it.

---

## Round 76 — the terminal printed its colours instead of wearing them

Status: **complete**

Round 75's finding said the unopened views were where to look. Two of the nine
dock destinations had never been opened — Explorer and Terminal — so both got a
state. Explorer renders its tree and is clean. Terminal was not.

`go test` sends its colours as SGR escapes when it thinks it is on a TTY, and the
fixture seeds exactly that, because it is what a Runtime capturing a command
actually receives. `CommandLog.tsx` put the string in a bare `<pre>`:

```
[1m=== RUN   TestCommitAtomicity[0m
[32m--- PASS: TestCommitAtomicity (0.01s)[0m
[31m--- FAIL: TestRollbackOnFlushFailure (0.02s)[0m
```

Printed verbatim, the codes are the loudest thing in the pane and the failure
they were marking is the hardest thing to find.

### The app already knew how

`ToolOutputPanel` has read these correctly all along, through a hand-written
parser that emits TONES rather than literal colours so they follow the scheme.
Its own comment even sends the reader to the terminal view for the full output —
to the pane that could not read them.

The parser sat in `chat/tools/domain/`, out of the workspace plugin's reach. It
is a technical mechanism neither context owns, so it now lives in `lib/ansi.ts`,
and the tone→token map — the answer to "what colour is a failure" — is one atom,
`ui/atoms/ansi-text.tsx`, that both surfaces render through.

### Before / after

| | before | after |
| --- | --- | --- |
| terminal view | escape codes as text, 10 lines of them | tone: green PASS, red FAIL, amber warning |
| ESC bytes in the DOM | present | 0 |
| tone→token map | one copy in the chat plugin | one atom, two consumers |
| ANSI parser | `chat/tools/domain/` | `lib/` |

### Verification

- Captured before and after at 2×. `esc bytes in DOM: 0, visible colour codes: 0`.
- Both new states declare their own ready boundary; `dock-terminal`'s is the
  failing line *in its tone*, which is the thing the state exists to hold.
- Visual **468/468** (452 before). Guards green including `check:layers` and
  `check:published-boundaries` — the move is legal in both directions. Unit
  2370/4 pre-existing.

### Next

Eleven views still unopened, all `dock: "workspace"` scope: search, files,
skills, skill-proposals, skill-library, recipes, knowledge, agent-memory,
agent-docs, plus `notifications` (session) and `run-summary` (run). Two rounds,
two views opened, two defects — the ratio argues for continuing.

---

## Round 77 — the catalogue could not reach its own last four rows

Status: **complete**

The fixture registered nine of the twenty workspace views. The reasoning for the
settings panes sits four lines below it and is the right one — *"every remaining
pane, so the pane list itself is the one production renders rather than a
three-row stub"* — and had never been applied here. Every view is registered now,
and the dock catalogue shows what production shows: twenty destinations across
Workspace, Run and Session.

**And a 720px-tall dock immediately failed to hold them.** `closure`'s vertical
clipping sweep caught it the moment the content was production-sized:
`ASIDE.agent-context-dock 720<878`.

### The chain

| element | box | content | overflow |
| --- | --- | --- | --- |
| `.agent-context-dock` | 720 | 860 | hidden |
| `relative min-h-0 flex-1` | 674 | 814 | visible |
| the catalogue | **814** | 814 | auto |

The catalogue carries `flex min-h-0 flex-1 overflow-y-auto` and none of it
applied: its parent is a `position: relative` BLOCK, so `flex-1` had no flex
container to size against and the element grew to its content. Every sibling in
that container is wrapped in `absolute inset-0 flex flex-col`, which is what
bounds them to 674px. The catalogue was the one child left plain.

So at the minimum window, with an empty dock, **Plan, Timeline, Notifications and
Tool stats rendered below the fold and nothing scrolled** — four of twenty
destinations unreachable.

### Before / after

| | before | after |
| --- | --- | --- |
| dock content height | 860 in a 720 box | 720 |
| catalogue scroll range | 0 | 140px |
| last destination | bottom 844, clipped, unreachable | bottom 704 after scrolling, clickable |

### Also this round

Two more states, `dock-search` and `dock-files`, both rendering real content. And
the tab rule that had been restated once per addition — three near-identical
branches deciding whether a view is "a tab you opened" — is one set now, with the
paragraph that explains it written once instead of twice.

### Verification

Measured before and after, including scrolling the catalogue to prove the last
row lands inside the viewport (`bottom 704, insideViewport: true`). Goldens
regenerated. Visual **484/484** (468 before). Guards green, unit 2370/4.

### Next

Five views still render "Couldn't load" because the fixture seeds no data for
them: skills, skill-proposals, skill-library, recipes, agent-docs. And the
fixture sidebar is three rows from the bottom of a 720px window — the state list
has outgrown the scaffolding that lists it.

---

## Round 78 — the scaffolding had outgrown the window it renders in

Status: **complete**

Round 77 stopped three rows short of the bottom of a 720px window. Every round
that opens a view adds a row to both fixture sidebars, and a state below the fold
is a state whose golden cannot be taken — so the list becomes the scrollport it
should always have been, and the active row is scrolled into view.

### The mistake in the middle of it

`min-h-0 flex-1 overflow-y-auto` on the list, and 65 goldens moved. The sibling
below it was `min-h-4 flex-1` — a spring, from when the list was content-sized —
so the two split the free height and the list fell from 610px to 326px. The
spacer is a gap above the caption, not a spring; saying so puts every row back
where it was.

### Before / after

| | before | after |
| --- | --- | --- |
| list height (720px window) | 610, content-sized | 627, fills the free space |
| capacity | overflows silently past the fold | scrolls |
| active row when it overflows | wherever it landed | scrolled into view |
| goldens | — | **unchanged** |

### Verification

- 484/484 with **no golden regenerated**: the change is invisible at the size the
  suite photographs, which is the point.
- Proven at 480px, where the list does overflow: scroll range 223px, `empty`
  holds at the top, `waves` scrolls to 222 and its row lands inside the viewport.
- Guards green, unit 2370/4.

### Next

The five views that render "Couldn't load" — skills, skill-proposals,
skill-library, recipes, agent-docs — need query data seeded before their surfaces
mean anything. The sidebar can hold them now.

---

## Round 79 — five catalogues with no provider, and the contrast they were hiding

Status: **complete**

Five views rendered "Couldn't load" the first time a fixture opened them, which
is what a missing DATA_PROVIDER looks like from the inside. Seeded, each sample
spanning what its view sorts on rather than repeating one row: both scopes, both
lifecycles, both proposal origins, all three knowledge scopes.

Four now render — skill proposals, skill library, recipes, agent docs — and each
got a state. Skills still shows "Skills are off": that is a capability gate, not
missing data, and the fixture advertises only `git` and `plan`.

### And the WCAG audit failed on two of them immediately

Both `serious`, both dark-only, in views no audit had ever run against:

| element | measured | needs |
| --- | --- | --- |
| `Tag ink="faint"` on `bg-surface-2` | 3.99:1 | 4.5:1 |
| the recipe name in `text-accent` on a card | 3.40:1 | 4.5:1 |

The Tag one is a design-system defect, not a call site's: measured against the
same surface, `fg-faint` is 3.99, `fg-muted` 5.13, `fg` 9.05. The atom's own
comment says a Tag "carries a VALUE the reader must be able to copy back" — an
ink that renders it below AA contradicts the reason the atom exists. **The
`faint` ink is gone**; the surface already does the quieting and `muted` is quiet
AND legible, which is why it was already the default. Three call sites.

The recipe name takes `text-fg`, the ink the command menu gives a command. Accent
there was an emphasis that cost the reader the thing being emphasised.

`text-accent` as small text on a card is 3.4:1 in dark generally, and two other
views use it — `agentMemory` and `run-summary`, both still unaudited. Noted, not
guessed at: it will be measured when their states exist.

### Also

`layoutShift`'s workspace test began timing out — 19 routes in one 30s budget.
The budget is derived from the route count now, because a fixed one fails as a
timeout, which reads like a shift that was never measured.

### Verification

60/60 workspace WCAG audits green. Visual **516/516** (484 before). Guards green,
unit 2370/4.

### Next

The capability gate: `skills`, `knowledge` and `agent-memory` render their "off"
ramp because the fixture advertises two features. Both sides are real states and
only one of them has ever been photographed.

---

## Round 80 — a fixture that said the Runtime could not serve its own features

Status: **complete**

The fixture advertised two capabilities, `git` and `plan`. Three views therefore
rendered their off-ramp in every state and their actual surface in none — not
because anything was wrong with them, but because the fixture told the app the
Runtime could not serve them.

It advertises what a Runtime has now, and **one state, `dock-feature-off`, is the
Runtime that does not** — so the off-ramp stays photographed deliberately rather
than as a side effect of an under-declared fixture.

With that and two more providers seeded, every one of the **twenty registered
workspace views now renders in a fixture state**. It was seven when round 75
started.

### The third instance of one mistake

`dock-agent-memory`'s WCAG audit failed on the same thing recipes did:

```
.text-accent — 3.4:1 (#3574f0 on #27292e, 13px)
```

Three call sites now: the recipe name, this pinned label, and `run-summary`'s
tone map. The Accent setting states its own scope — *"Functional highlight color
— play / active / CTA"* — which does not include prose, and the command menu
renders a command's name in `text-fg`. So these are call-site drift, not a
missing token: adding an accessible accent ink would sanction a use the design
system deliberately does not have.

The pinned label keeps the accent on its **mark**, where 3:1 is the bar a graphic
answers to, and gives the **word** an ink that can be read.

`run-summary` opened clean — measured, not assumed, which is why it got a state
rather than a guess.

### Before / after

| | before | after |
| --- | --- | --- |
| workspace views rendered by a fixture | 7 of 20 | **20 of 20** |
| capabilities advertised | git, plan | git, plan, skills, knowledge, agentMemory |
| the "feature is off" ramp | every state, by accident | one state, on purpose |
| accent as prose ink | three call sites, 3.4:1 in dark | none |

### Verification

78/78 workspace WCAG audits green across every state and both themes. Visual
**564/564** (516 before, 444 when round 75 started). Guards green, unit 2370/4.

### Next

The agent fixture's own coverage: sixteen states there, all of the transcript.
Whether the same "registered but never rendered" gap exists on that side has not
been asked.

---

## Round 81 — twenty-four tool previews nothing had ever rendered

Status: **complete**

The same question as round 75, asked on the other side. Thirty tool names carry a
preview; the agent fixture calls six. **Twenty-four of those panels — their
placeholders, their overflow rules, their inks — had never been rendered.**

A first four in a new `tool-search` state: `glob`, `search_memory`,
`search_conversations`, `search_tools`, with the result strings each projection
actually parses rather than prose about them. All four render, and the third one
was wrong.

### The date that truncated to nothing

`search_conversations` puts speaker and day in one 7.5rem cell and truncates the
pair as one string, so what it cut was the **day**:

```
user · 2026-0…        assistant · 2…
```

Two rows that do not even agree on where they stopped, and the one fact you would
scan the column for — when — destroyed in both.

Split, the date is `shrink-0` and the speaker gives way instead. The column is
9.5rem, not the 11.13 that would fit "assistant" whole: measured, that is 26% of
the row spent on metadata, and the speaker is a two-value enum whose first four
characters already tell them apart.

### A false positive it also exposed

`closure`'s horizontal-clipping check then failed on the new state, reporting the
whole content card as cutting text. It was not: the snippet's box sits wholly
inside the card, clipped with an ellipsis the reader can see. The check's
text-node branch measures a RANGE — the laid-out text, which runs past a
`truncate` box by design — and did not grant that branch the ellipsis exemption
its element branch already grants.

Fixed where the text lives, and **mutation-verified in both directions**: with
the exemption the false positive is gone; replacing `truncate` with clipping that
has no ellipsis brings the failure straight back.

### And a flake I had introduced in round 78

`agent golden light waves` failed right after being regenerated. The sidebar
scrolls now, and `scrollIntoView` answers in fractions: the browser clamps to a
fractional maximum — this list can scroll 18.6px — so the readback rounded to 18
on one run and 19 on the next, moving every row a pixel.

Integer offsets decide the target, flooring **after** the clamp decides the
landing, and a `ResizeObserver` re-lands it when fonts finish. Measured six runs
per state across five states: one value each.

### Verification

Visual **572/572**, twice consecutively, after regeneration. Guards green, unit
2370/4.

### Next

Twenty more previews with no fixture call: the schedule family, the goal family,
`http_request`, `web_search`, `web_fetch`, `read_shell_output`, `stop_shell`, the
skill family, `lsp`, `ask_user`, `enter_plan_mode`/`exit_plan_mode`,
`read_tool_result`.

---

## Round 82 — a wire timestamp printed at a reader

Status: **complete**

Second batch of previews no fixture had called, the ones answering in JSON:
`web_search`, `web_fetch`, `http_request`, `list_schedules`, `create_goal`. Each
result is the exact shape its projection reads, so a preview that stops parsing
fails here rather than degrading to a blank panel in front of someone.

Four render correctly. The fifth printed its wire value:

```
next 2026-08-01T03:00:00Z
```

The settings pane renders the **same field** as `formatDateTime(schedule.nextRunAt)`
— "next Aug 1, 11:00 AM", in the reader's locale. The preview handed the string
straight through, so one fact had two renderings and the tool row got the machine
one.

### Before / after

| | before | after |
| --- | --- | --- |
| schedule preview | `next 2026-08-01T03:00:00Z` | `next Aug 1, 11:00 AM` |
| formatter | none | the one the settings pane already uses |
| type | `font-mono` | prose, because a formatted date is not a literal |

### Verification

Read back from the DOM after the change. Visual **580/580** (572 before), twice.
Guards green including `check:locales` rule 13, which is about exactly this class
of mistake on the other axis. Unit 2370/4.

### Next

Fifteen previews still uncalled: the skill family, `lsp`, `ask_user`, the plan
transitions, `read_shell_output`/`stop_shell`, `read_tool_result`,
`delete_schedule`, `get_goal`, `report_goal_outcome`.

---

## Round 83 — three tool families that should never have had a row

Status: **complete**

A third batch of previews no fixture had called turned into a design correction the
user stated directly: **Plan, Goal and Schedule tools do not belong in the
transcript at all — each family has a surface of its own.**

`BlockRenderer` already implements that: a tool registered against a
`TOOL_STANDING_SURFACE` has its row dropped. Only four of the nine were
registered.

| family | surface | registered before | now |
| --- | --- | --- | --- |
| Plan | the plan bar | `set_plan` | all three |
| Goal | the goal bar | `create_goal`, `get_goal` | all three |
| Schedule | the Schedules pane | none | all three |

`report_goal_outcome` is named in CONTENT_RENDERING §7.6 as one of the three whose
row is dropped and had simply been left out of the loop.

### What that made dead

A preview for a row that cannot exist is a component nobody can reach and a claim,
to the next reader, that the transcript shows one. Removed with everything that
only fed them: two preview plugins (`goal`, `plan`, `schedule` — three), five
preview components, `projectGoalToolPreview`, `projectSchedulePreviews`,
`projectDeletedScheduleId`, `planStepsFromToolArgs`, `ToolResultProse`, their
tests, and three orphaned locale keys.

### Two corrections I made on the way

- **I over-read the spec.** "无展开体" also applies to `delete_schedule` and
  `read_tool_result`, and I removed their previews too — which makes them *worse*,
  because `ToolPreview` falls back to a generic inspector. Every row in this
  component model expands; a tailored preview is the smaller body, not the larger.
  Reverted.
- **The schedules registration only worked in production.** It lives in the
  Schedules plugin, which the agent fixture did not load — so the fixture drew a
  `list_schedules` row the app never draws. The fixture loads it now, for the rule
  it declares rather than for its pane, and the workspace fixture stops loading it
  twice.

### The test the change had to teach

`toolRendering.test.ts` asserted "a preview for every known tool". It now asserts
both halves — every tool the transcript draws has one, every tool a standing
surface answers for has none — with the names read from the three surface owners
rather than copied, because which tools a surface answers for is theirs to say.

### Verification

Rendered rows, measured per state: `tool-agentic` draws `list_skills`,
`load_skill`, `read_shell_output`, `lsp`; `tool-remote` draws `web_search`,
`web_fetch`, `http_request`; `running` draws `read`, `grep`. No plan, goal or
schedule row anywhere. Visual **588/588** twice; guards green; unit 2312/2 outside
the runtime-contract e2e.

---

## Round 84 — the last five previews, and the golden that could not be taken twice

Status: **complete**

Five previews still had no fixture call: `propose_skill`, `read_skill_resource`,
`read_tool_result`, `stop_shell`, `ask_user`. All five render correctly — the one
suspicion, `ask_user` showing "answer · Race detector", is a translated label
(`回答 · ` in Chinese), not a leaked JSON key. Checked before reporting.

**21 of 21 registered previews are now rendered by a fixture.** It was six when
round 81 started, and the fifteen between held a raw XML dump, a wire timestamp,
a date truncated to "2026-0…" and a row of escape codes.

`agentSessionSnapshots.test.ts` keeps it closed: every tool with a preview must
be called by some state. Mutation-verified — removing one call names it.

### The flake it uncovered

`foundation dark collapsed` failed in the full suite, passed 3/3 alone, and
failed again immediately after being regenerated. The caption re-wrapped by one
word.

`globals.css` gives every paragraph `text-wrap: pretty`, and Chromium falls back
to greedy wrapping under load. A caption sitting on the break boundary therefore
wraps one way when the suite runs alone and another when it runs with everything
else — the golden could not be photographed twice the same.

| | before | after |
| --- | --- | --- |
| caption | two lines, 539px + 148px in a 578px box | **one line, 432px** |
| wrapping decisions | one, on a knife edge | none |

One line is the only width at which no algorithm gets a vote. The product's
typography is untouched; what changed is a fixture caption that should not have
been standing on the boundary.

### Verification

Visual **596/596**, twice consecutively after regeneration. Guards green; unit
2313/2 outside the runtime-contract e2e.

### Next

Every registered preview and every registered workspace view now renders in a
fixture, both pinned by a guard. The transcript's own surfaces — approval cards,
question cards, delegated runs, banners — have states, but whether every VARIANT
of them does has not been asked.

---

## Round 85 — three blank circles asking for several answers

Status: **complete**

Same question as the last four rounds, asked of the transcript's own surfaces
rather than its tool rows. The question card draws four shapes; the one questioned
state carries two of them — a single `choice` and a `text` field. **`multiple` and
`allowCustom` had never been rendered.**

Given a state, the multi-select is wrong in the way that matters before anything
is clicked:

| | single choice | multi-select |
| --- | --- | --- |
| unchecked mark | circle with the option's NUMBER | **empty circle** |
| checked mark | circle with a dot | circle with a check |
| what it says | pick one, press 1–3 | *pick one* |

`ChoiceOption` gave both modes `rounded-full`. A multi-select's unchecked mark
carries no number and no check, so three of them read as three blank radios — and
the app's own `Checkbox` atom is square, so the same question was being asked two
different ways in two places.

### Before / after

| | before | after |
| --- | --- | --- |
| many-of mark | `rounded-full` | `rounded-2xs`, the radius `Checkbox` uses |
| one-of mark | `rounded-full` | unchanged |

Round for one-of, square for many-of — the distinction every platform makes.

### Verification

Captured both modes at 2×: squares with room to check, circles with their
ordinals. Visual **604/604** (596 before); guards green; unit 2313/2 outside the
runtime-contract e2e.

### Next

The other transcript surfaces and their variants: the approval card across the
safety classes it can be asked about, the delegated run at depths beyond one, and
the banner family — recovery, cwd-missing, run-error — of which one state each is
photographed.

---

## Round 86 — three verbs, three left edges

Status: **complete**

`ToolFileChange.status` has four values. The fixtures had two: every patch receipt
in every golden carried exactly one row, `added` or `modified`. So `moved` — the
only row that draws two paths and an arrow — and `deleted` had never been rendered
once.

The first attempt gave them their own `apply_patch` call. That was wrong twice
over: it moved `6 calls` to `7` and broke three hard-coded stats assertions and
eight goldens for nothing, and it put the new verbs in a *different list* from the
old ones. Folding all three into one receipt is both smaller and the better test —
a refactor commit really does edit, move and delete at once, and reading three
verbs down one column is the only way their alignment is visible at all.

It was not aligned. The verb was `shrink-0` with no width, so each path began
wherever its own verb ended:

| row | path starts at |
| --- | --- |
| `Edited` | 393.5 + 6 |
| `Moved` | 393.5 + 6 |
| `Deleted` | **393.5 + 24** |

One row per receipt — which is all any fixture had ever had — hides this
completely.

### Before / after

| | before | after |
| --- | --- | --- |
| row | `flex`, verb `shrink-0` | `grid-cols-subgrid`, `col-span-2` |
| list | (none) | `grid-cols-[auto_minmax(0,1fr)] gap-x-1.5` |
| verb / path edge | 442.92 / 448.92, 442.92 / 448.92, 460.6 / 466.6 | identical on all three |

`auto` rather than a chosen width: a single-row receipt stays exactly as wide as
its own verb — which the goldens confirm, none of the single-row states moved —
and no locale needs a number picked for it.

### Verification

Guard added: a multi-file receipt must put every path on one left edge, and the
three rows must carry three *different* verbs so identical labels cannot satisfy
it. Mutated back to `flex` + `shrink-0`, it fails; restored, it passes. Visual
**605/605**; the golden diff is confined to `tool-shells` light and dark; the
20-script gate is green; unit 2358/4 outside the runtime-contract e2e.

### Next

The transcript surfaces still at one photographed state each: the approval card
across the safety classes it can be asked about — `network` appears in no fixture
at all — the delegated run at depths beyond one, and the banner family, recovery,
cwd-missing and run-error.

---

## Round 87 — asking permission to run `web_fetch`

Status: **complete**

Started at the approval card, expecting it to vary by safety class. It does not
read safety class at all — so that suspicion cost one file read and no work. What
it reads is `toolCategory`, which answers a *different* question: the shape of a
tool's arguments and result. It names two of its seven cases, and everything else
falls through to `{ icon: "tool", label: toolName }`.

So the card asked permission to run **`web_fetch`** — the wire identifier, in
snake_case — while the transcript row beneath it said "Fetched" over a download
glyph. One call, two identities, and the card had the worse one.

The families already own that word, and the eyebrow's register — `Shell`,
`Files`, `Network` — is the one the catalogue uses.

### Before / after

| | before | after |
| --- | --- | --- |
| eyebrow source | `toolCategory`, 2 of 7 named | `toolFamilyId` + `toolIconFor` |
| `shell` | Terminal | Shell |
| `apply_patch` | Edit files, `edit` glyph | Files, `replace` glyph — the row's own |
| `web_fetch` | **`web_fetch`**, generic glyph | Network, `download` glyph |
| an MCP tool | its own name | unchanged: nobody has a better word |
| `approval.identity.terminal` / `.fileEdits` | 8 locales | deleted |

### The fixture was lying

`fetch` is one of six activity families and had never rendered. `tool-remote`
calls all three network tools — but two fixture helpers assigned safety class by
two different rules, one hard-coding `safe`, the other falling back to `exec` for
any name it had not been told about. Three network calls had spent every golden
counted as searches.

One table now, transcribed from the Runtime's own `descriptors.go`, and an
unlisted name **throws** instead of defaulting. It threw immediately:

> no Runtime safety class recorded for the tool `edit`

The narrative fixture had been photographing a tool that does not exist — the
Runtime has a test asserting it never exposes `edit` — so the one write in that
state was drawn with the fallback glyph and the generic verb. It is `apply_patch`
now, with the real argument and receipt shapes.

| family | before | after |
| --- | --- | --- |
| `fetch` | **never rendered** | tool-remote |
| `run` | tool-shells | tool-shells, tool-tail |
| `search` | 8 states incl. tool-remote | 7 — tool-remote's were never searches |

### Verification

Guard: every name in `TOOL_FAMILIES` must approve under a family word, not its
own. Mutated back to `{ label: toolName }`, 31 of 33 fail. Visual **605/605**;
12 goldens moved, all of them states whose fixture had been wrong; the 20-script
gate is green; unit 2388/4 outside the runtime-contract e2e.

### Next

`grep` in the narrative fixture still returns the string `"7 matches"` where the
Runtime returns `{hits: […]}` — the same class of lie, in a state that folds the
row so nothing has shown it yet. Then the delegated run past one level, and the
banner family.

---

## Round 88 — a settled call that answered nothing

Status: **complete**

Two suspicions, both refuted by reading rather than by changing anything:

- the shell preview passes `tool.result` raw while the shape-keyed one extracts
  `.output` — **correct**, because the fold already extracts for `toolCategory
  === "command"`, which only `shell` is;
- `ToolOutputPanel` receiving an envelope — it does not; it takes a string and
  gets one.

The real one was the lead left over from round 87. `grep` in the narrative
fixture answered the string `"7 matches"` where the Runtime sends `{hits: […]}`,
so the fold read no hits and the row was photographed without its count.

### The guard, not the fix

The Runtime declares a result shape per tool name, and the generated contract can
validate all four. So rather than correcting one fixture, assert the rule:

> a settled call to a tool whose result shape the Runtime declares must carry a
> result of that shape

It found a second one on its first run, which no golden could have — `waves`
folds its rows, so a `grep` there had gone settled-with-no-result unseen.

| | before | after |
| --- | --- | --- |
| narrative `grep` | `"7 matches"` | `{hits: […3]}` |
| narrative `read` ×3 | no result | `{content, total_lines}` |
| narrative `web_search` | no result | `{results: […2]}` |
| `waves` `grep` | **no result** | `{hits: […2]}` |

### And what an honest result then showed

`web_search` had never carried one, so its row had never rendered a count. Given
one, it read **2 matches**. `hits` is one field because a count is one fact, but
a web search returns *results* — the Runtime's own word — and matches claims a
precision the search never offered.

| | before | after |
| --- | --- | --- |
| `web_search` | 2 matches | 2 results |
| `grep` / `glob` | 2 matches | unchanged |
| `tool.meta.results` | — | added to 8 locales |

### Verification

Both guards mutation-checked: the shape guard fails on a restored `"7 matches"`,
the wording guard on a shared label. Visual **605/605**, only the narrative
goldens moved; the 20-script gate is green; unit 2390/4 outside the
runtime-contract e2e.

### Next

The delegated run past one level, and the banner family — recovery, cwd-missing,
run-error — of which one state each is photographed.

---

## Round 89 — the error banner's other half

Status: **complete**

The run-error banner offers a Retry action, and **it had never rendered**. The
one photographed error is `provider_rejected`, which is on the banner's own
unretryable list, so every golden showed the banner with its recovery action
missing and nothing said so.

Three suspicions on the way in, all closed by reading rather than by changing:

- the shell preview passing `tool.result` where the shape-keyed one extracts
  `.output` — each right for its own case, because the fold extracts for
  `toolCategory === "command"` and only `shell` is;
- `useActiveSessionProblem` returning a fresh object each render, which would
  make the countdown's `countdown.problem === error` check never hold — it
  returns a reference into the stored view;
- the primary action wearing the banner's negative tone rather than the accent
  the approval card uses — both banners do it, identically, and the tint groups
  the action with the problem it resolves. Consistent, so left alone.

And one contradiction that WAS real, in the fixture: its invented detail read
"Verify the selected model and retry" on the one code the banner refuses to
retry. Replaced with the Runtime's own wording for that failure.

### Before / after

| | before | after |
| --- | --- | --- |
| retryable error | **no state** | `error-retryable`, `provider_error` |
| Retry action | never rendered | rendered, enabled |
| `error` detail | "…and retry", invented | the Runtime's own string |
| countdown | never rendered | unit-tested under fake timers |

### Where each half is tested

The countdown label changes once a second, so a golden photographs whichever
number it landed on. The *shape* goes in a golden — a retryable error with no
retry-after, deterministic — and the *ticking* goes to jsdom where time is
controllable: 3 → 2 → enabled, and a click during the wait sends nothing.
Mutated by removing `disabled={retryIn > 0}` and the guard in `onRetry`, two of
the four fail.

One casualty worth recording: `getByRole`'s `name` matches as a SUBSTRING, so
the sidebar row "Error, retryable" answered a test looking for "Retry". The test
now scopes to the transcript, which no future state name can reach into.

### Verification

Visual **613/613**; 49 goldens moved, 47 of them because the fixture sidebar
gained a row and every agent frame includes it; the 20-script gate is green;
unit 2394/4 outside the runtime-contract e2e.

### Next

The delegated run past one level — `delegated` photographs a single child — and
the cwd-missing banner, which has an editing mode no state opens.

---

## Round 90 — a fixture that switched the product off

Status: **complete**

The cwd-missing banner had never rendered — no fixture ever set a workspace to
`missing`. Setting one showed the banner but **not its relocate action**, which
is gated on a Runtime capability the agent fixture did not advertise. It
advertised none at all.

That is the failure mode the workspace fixture had already written down for
`skills`, `knowledge` and `agentMemory` — and when that lesson was paid for,
`schedules` and `relocate` were still missing. The whole schedules settings pane
read **"Schedules unavailable — the connected runtime doesn't expose scheduled
runs yet"**, in every golden, and its list, form and rows had never been drawn.

A fixture that omits a capability does not photograph *nothing*. It photographs
the app refusing to work, which looks exactly like the app.

### Before / after

| | before | after |
| --- | --- | --- |
| agent fixture features | **none** | the shared list |
| workspace fixture features | 5 | 7 — `schedules`, `relocate` added |
| capability list | inline in one fixture | `VISUAL_RUNTIME_FEATURES`, shared |
| cwd banner | never rendered | `cwd-missing` state; relocate editor drawn |
| schedules pane | "unavailable" | two seeded rows, one on, one off |

The guard reads the required list off the app's own `useRuntimeCapability` call
sites, so a new gate arrives as a failure rather than as a surface that quietly
never renders. Mutated by dropping `schedules`, it fails.

### What rendering them then found

**The apply button's label was `"…"`.** Untranslated in eight locales, and it is
the button's accessible name, so a reader heard "ellipsis" where the app meant
"working". It also collapsed the button's width mid-click and shoved Cancel
left. The app's own convention — the approval card's — is to keep the label and
shut the control.

**A switched-off schedule dimmed itself out of legibility.** `opacity-60` on the
whole row put its run, edit and delete buttons *below* the `opacity-64` the app
draws a genuinely disabled control at, while they stayed perfectly clickable.
Scoping the dim to the text was not enough either — the contrast audit, running
on this pane for the first time, measured it:

| | ratio | needs |
| --- | --- | --- |
| title | 4.36 | 4.5 |
| cron chip | 2.43 | 4.5 |
| instructions | 2.68 | 4.5 |

A row marked inactive by making itself unreadable. The state is a step down the
token ladder now — contrast the design system owns, rather than a multiplier
landing wherever two colours leave it — and the Switch beside it says the rest.

### Verification

Visual **621/621**; 51 goldens moved, most because the fixture sidebar gained a
row that every agent frame includes; the 20-script gate is green; unit 2394/4
outside the runtime-contract e2e (a fifth, a cursor e2e, passes in isolation and
is runtime-side).

### Next

The delegated run past one level — `delegated` photographs a single child — and
the schedules pane's own remaining shapes: its form, and the empty list.

---

## Round 91 — four suspicions, one deletion

Status: **complete**

The delegated state was the last "one photographed shape" on the list. It turned
out to be right about almost everything, and the round is mostly a record of
things that survived being checked.

**A stuck `Loading` in a static fixture.** It was an `sr-only <output>` inside
the shimmer. Not a hung query — but worth reading, and reading it found the one
real defect below.

**A grandchild's reply missing from the DOM.** `item_nested_response` is not
rendered. Traced through the projection — `selectDelegatedRunNarratives` buckets
it correctly, `readTurnFacts` walks into it, `BlockRenderer` has its narrative —
to `autoExpanded: status === "waiting"`. A *running* delegated run collapses,
and a closed disclosure does not mount its children. Deliberate, commented, and
correct.

**A nested run's content reading as its parent's.** Measured instead: root
content sits at x 393.5 w 768, the child at 405.5 w 744, the grandchild at 417.5
w 720. One 12px step per level, from both sides, exactly.

**An approval attributed to the wrong agent.** Its anchor is
`turn:item_child_response:i:item_child_approval` inside the depth-1 disclosure,
at the depth-1 indent. Correct; the eye was wrong because a running grandchild's
block has no visible end.

### The one real find

`Loader` carried its own live region saying "Loading".

| | |
| --- | --- |
| what it announced | less than the visible text beside it |
| who already owns that | `RunAnnouncer`, the app's one `aria-live` for run state |
| when it spoke | never — it mounts WITH its content, and by the announcer's own test a region a reader first meets already carrying a message says nothing |
| could the visible label take its place | no: it counts elapsed time and would announce a new one every second |

Deleted. `common.loading` keeps five other readers.

### What was locked instead

Depth-2 nesting had never been asserted or photographed, because the only
grandchild in the fixtures is `running` and therefore closed. The new test opens
it and pins the step: each level indents by the same amount, from both sides, or
a deeper reply is drawn wider than the one it belongs to. Mutated by removing
the disclosure's horizontal padding, it fails.

### Verification

Visual **622/622**, no golden moved; the 20-script gate is green; unit 2395/4
outside the runtime-contract e2e.

### Next

The schedules pane's remaining shapes — its form, and the empty list — now that
the pane renders at all.

---

## Round 92 — a group that forgot its own answer under the pointer

Status: **complete**

The schedules form is a second surface behind the pane that only started
rendering last round, and reaching it takes a click — so no audit had ever
opened it. Audited now, and clean. What axe cannot see is what was wrong.

Its cron presets are a one-of group, and **both halves of "which one" were
missing**:

| | before | after |
| --- | --- | --- |
| assistive tech | nothing — a plain `<button>` group | `aria-pressed`, as nine other groups do |
| chosen fill | `bg-selected`, a 4% black wash | unchanged |
| hover fill | `bg-hover`, a **3%** black wash | unchanged |
| what says "chosen" under the pointer | nothing | an accent edge |

One percent of alpha apart. Moving the pointer across the group made every
option it touched look like the one that had been chosen.

### The fix that was wrong first

Accent text separates them by hue, and the approvals list — the app's other
one-of group — uses exactly that. So did this, until the new audit measured it:

> `.bg-accent-wash`: contrast **2.85** (foreground `#3574f0`, background
> `#2a3647`), expected 4.5:1

Accent on accent, in dark, at 13px. The audit I had just written caught my own
change one run after writing it, which is the whole reason to write it.

An **edge** instead: hover only ever deepens a fill, so a border is the one
channel it cannot forge, and an outlined pill is what the form's own Cancel
button already is. The label stays `text-fg`, which needs no exemption in either
theme.

### Verification

The guard asserts the edge, not the fill: whatever the pointer is over, exactly
one option is outlined and it is the one that answered. Mutated to
`border-transparent`, it fails. Visual **624/624** with **no golden moved** — the
form is not in any frame, which is exactly why it took a click to find. The
20-script gate is green; unit 2395/4 outside the runtime-contract e2e.

### Next

The schedules pane's empty list — the only shape of it left — and the row's edit
mode, which reuses this form with a schedule already in it.

---

## Round 93 — one click, and the schedule was gone

Status: **complete**

A saved schedule is a prompt somebody wrote, a cron they chose and a working
directory they set. Deleting one took **a single click on a quiet trash icon**
wedged between Run and Edit in a dense row — no menu in front of it, no
confirmation, no undo behind it.

It was the least-protected destructive action in the app. The comparison is in
the app already:

| | in front of it | asks first |
| --- | --- | --- |
| delete a session | a context menu | yes, naming the session |
| delete an MCP server | opening it, then a labelled red button | no |
| **delete a schedule** | **nothing** | **no** |

It asks now, named, in the words the session dialog uses: *"Nightly dependency
audit" and its instructions go away. This cannot be undone.*

### And the dialog it asks in

Writing the guard, `getByRole("alertdialog")` found nothing. `ConfirmDialog`
announces as a plain `dialog` — another window — for all three of its uses, and
every one of them is destructive. A confirmation that interrupts to ask before
something goes for good is the one thing `alertdialog` names.

| | before | after |
| --- | --- | --- |
| role | `dialog` | `alertdialog` when `destructive` |
| non-destructive confirm | — | would stay `dialog`; none exists yet |

Two tests moved with it, and the move is the assertion getting more specific.

### Verification

The guard holds that the click ASKS — the row survives until the dialog is
answered — and names the button it answers with, so the dialog cannot be settled
by whichever control happens to be first. Mutated back to a direct delete, it
fails. Visual **625/625** with no golden moved; the 20-script gate is green;
unit 2395/4 outside the runtime-contract e2e.

### Next

The schedules pane's empty list, the last shape of it nothing has drawn.

---

## Round 94 — seven languages nobody had ever looked at

Status: **complete**

Started by sweeping every fixture state for off-ramp and loading text, the
question that found the schedules pane. It found nothing new: the three
persistent `Loading` regions behind the settings page are inside `display: none`
`Activity` trees, hidden from the accessibility tree as well as the eye.

So a different question: **the app ships eight languages and every check runs in
one.** The fixture pins `setLocale("en")` and no spec had ever asked for
another — while the chrome is full of fixed columns, pills and `truncate` whose
widths were all chosen, and checked, against the shortest language the product
has.

### Getting there

| | |
| --- | --- |
| fixture | a `locale` parameter, English by default so no golden moves |
| order | select FIRST, then load the plugins — a language plugin fetches its dictionary during setup only when it is already the selected one, so the other way round leaves every string English with `html lang` claiming otherwise |
| settling | wait for the fetch's EFFECT; a frame photographed mid-fetch is the English one, which is the bug the parameter exists to look for |
| shell readiness | the gate named an English string, so the one surface most likely to overflow in German could not be reached; it counts a numeral now |

Not goldens: eight times the frames to review, and one translation edit would
move them all. The two clipping detectors already answer the only question that
matters — can a reader see the whole word — and they answer it in any language.

### What it found

Seven locales across seven routes: **the layout holds**. What did not was the
detector.

> `agent/waiting →: MAIN.agent-content-card 845<1338`, Japanese only

The straddling text was `あなた` — "You" — in an `sr-only` label. A range
measures laid-out text, and a screen-reader-only node is a 1px clipped box whose
glyphs still lay out at full width. Three narrow Latin characters landed inside
the card's edge; the same word in Japanese did not. A fact about font metrics,
reported as text a reader cannot see being cut off.

The outer loop already had the rule — "a 1-2px box holds no readable text by
construction" — and the text scan did not apply it. It does now, to the owner
chain, which cannot hide a real clip: a clipped-but-visible label has a
normal-sized box.

### Verification

**632/632**, no golden moved; 7 locales × 7 routes now run on every suite, in
about thirteen seconds; the 20-script gate is green; unit 2395/4 outside the
runtime-contract e2e.

### Next

The locale sweep on the surfaces it does not reach yet — the composer's model and
mode pickers, the dock tab strip — where a fixed pill row meets German.

---

## Round 95 — the worst case, and two statuses nobody drew

Status: **complete**

First, pushed last round's locale sweep to the combination the product actually
ships and nothing had ever checked together: **the smallest window, the largest
type, in the longest language.** Each had been checked alone.

| | |
| --- | --- |
| routes | 7 → 10, adding the preview-heavy states where the literal widths live |
| axes | + font size 18 |
| result | **holds** — no clipping in any of the seven languages |

Then a targeted probe for what the clipping detectors deliberately exempt: a
label that FITS in English and truncates elsewhere, which is what a column
measured against "assistant" invites. Ten routes, four languages, nothing. The
recall grid's 9.5rem survives because its content is result data, not chrome —
so the locale axis cannot stress it at all, which is worth knowing.

### Goal, and its four statuses

`GoalStatus` is `active | paused | blocked | completing`. **Two had ever been
rendered.** What each one offers is different, and the code's own comment worries
about exactly this: *"Naming them here rather than in a bare set is what stops the
row from removing the resume control in silence."*

| status | says | offers |
| --- | --- | --- |
| active | Pursuing goal | clear, pause, edit |
| blocked, resumable | **Goal stalled** | clear, resume, edit |
| paused by a spent cap | Cost budget reached | clear, edit — the Runtime refuses to resume, so the row does not offer it |
| completing | **Finishing goal** | clear only |

`completing` is the Runtime's word for the settlement window after the model has
declared success and before the drive has charged the final Run. Withholding
pause and edit there is right: there is nothing left to pause and the objective
is already being cleared. All four carry proper accessible names.

Nothing was wrong. The two that had never been drawn are drawn now, and what
each status offers is pinned — mutated by dropping `blocked` from the resumable
set, the guard fails.

### Verification

**636/636**; four goldens moved, the two states that gained a Goal row; the
20-script gate is green; unit 2395/4 outside the runtime-contract e2e.

### Next

The Goal budget readout — the collapsed row reports the axis that will stop the
loop first, and only one arrangement of that has been photographed.

---

## Round 96 — a second delegation, and the sub-agents disappeared

Status: **complete**

Chased the Goal budget readout first — `used` and `budget` ride the read model
end to end and nothing renders them. Then found the test that says so on purpose:
*the Goal surface stays quiet and omits Runtime constraints*, asserting the row
shows no `$4.50/$5.00`, no `7/20`, no progressbar. A decision, guarded. Dropped.

Then the closed set: `AgentRunPresentationState` has **six** values and the
fixtures drew **two**. Adding a fan-out — one delegation spawning four children
that finished, failed, were canceled and hit a limit — did not render four new
rows. It rendered **none**.

### What one extra delegation did

| | |
| --- | --- |
| before | one `delegate_task`, one sub-agent below it |
| after | two `delegate_task` calls, **"2 calls"**, and every sub-agent gone |
| including | the approval a person had to answer |

`delegate_task` is `safe`, and the planner folds read-only calls into a group of
glances. A delegation is read-only and is **not** a glance: it owns a whole
sub-agent. One delegation never reached the two a group needs, so no fixture had
ever shown it — and the renderer's grouped branch never consults
`facts.delegatedRuns` at all.

Excluded by DATA, not by name: the planner now takes the set of calls that
spawned a Run, which is `Object.keys(facts.delegatedRuns)`. A plugin's own
delegating tool is covered the same way `delegate_task` is.

### And the column that was not one

With four siblings finally on screen, their statuses sat at x **967, 989, 932 and
548**. The last is the row with no detail: `flex-1` lived on the detail element,
so a row without one stopped pushing its trailing right and put its status
wherever its label ended — 440px from the column a reader scans to find the
sub-agent that failed.

The slot is always present now, empty or not. Same element either way, so rows
that do have a detail are untouched; rows that do not gain the alignment the
component already gave everything else. Across the goldens it turns `3 steps`,
`2 steps`, `exit 1 8.4s`, `3 files` and `denied` into one right-hand column.

### Verification

Both halves mutation-checked: restore the grouping and six sub-agents vanish;
remove the empty spring and the four ends stop agreeing. Visual **637/637**; 28
goldens moved, every one of them a row whose trailing found its column; the
20-script gate is green; unit 2395/4 outside the runtime-contract e2e.

### Next

`Canceled`, `Error` and `Limit reached` now exist as delegated rows. The run
TREE view in the workspace dock draws the same six states from the same model —
and its fixture has the same two.

---

## Round 97 — the run tree that had drawn one node

Status: **complete**

The timeline dock asks two questions with one view: what happened, in order, and
which Runs it happened in. Its fixture answered the first — `tool-shells` has
five tool outcomes to sort — and the second read **"8 events · 1 run"**, a single
finished root. One of the six states a Run can be in, and no lineage at all.

`dock-runs` points the same view at `delegated`, which after last round is seven
runs across all six states and two levels deep. It draws:

| | |
| --- | --- |
| header | 15 events · 7 runs |
| lineage | `parent run_root`, `parent run_child` — nesting the old fixture had none of |
| statuses | Running, Needs input, Canceled, Error, Limit reached, Finished |
| events | run started, tool finished, **approval requested**, run finished, run error |

### One suspicion, refuted by measurement

A canceled run's terminal row reads "✓ Run finished — Stopped once the answer was
already known", and a check beside a stopped run looked like two channels
disagreeing. Measured instead: that row carries **one** icon, at x 931 — the
event-kind glyph — where a tool row and the error row carry **two**, the second
at 1373. The status column abstains, exactly as `terminalTimelinePatch` says it
should: `completed` is `ok`, a failure is `err`, and canceled or limit-reached
gets a summary and no verdict at all.

### Verification

The suite refused the new state until it declared its own ready boundary, which
is the deepest node — a run whose parent is itself delegated, the last thing the
tree paints — plus all four terminal statuses by name. Visual **645/645**; 52
goldens moved, 50 of them because the fixture's state rail gained a row that
every workspace frame includes; the 20-script gate is green; unit 2395/4 outside
the runtime-contract e2e.

### Next

`TimelineEntry.status` has four values — `ok`, `err`, `approved`, `declined` —
and the timeline has now photographed two.

---

## Round 98 — a denial filed as an approval, in four languages

Status: **complete**

`TimelineEntry.status` has four values and the timeline had drawn two. `approved`
looked at first like dead vocabulary — nothing in the app sets it outside tests —
until the emitter turned up: an `approval-result` entry is appended **locally,
the moment somebody answers**, which no static snapshot can hold.

`dock-runs` puts a pending approval and the timeline on screen together, so the
path is reachable at last. Answering it wrote the entry, and the row read:

> 🛡 **Approval**   ✓   14:30:01

Every sibling is a statement — "Run started", "Run finished", "Run error",
"Approval requested". This one is a bare noun, and the verdict lives only in the
mark beside it.

### Which is worse than thin

| locale | said | means |
| --- | --- | --- |
| zh-TW | 核准 | **granted** |
| ja | 承認 | **approved** |
| ko | 승인 | **approved** |
| es | Aprobación | **approval/granted** |

A *declined* approval was filed under approved in four of the eight languages
that ship. The English noun is ambiguous enough to hide it; those four are not
ambiguous at all.

### After

Settled, not granted — the pattern `tool-end` already uses, where the label says
the call finished and the mark says whether it worked. The verdict stays where
the file's own comment puts it: two different glyphs, `check` and `x`, so colour
carries nothing on its own.

| | before | after |
| --- | --- | --- |
| en | Approval | Approval settled |
| zh / zh-TW | 审批 / 核准 | 审批已答复 / 審批已回覆 |
| ja / ko | 承認 / 승인 | 承認判断 / 승인 처리됨 |
| fr / de / es | Approbation / Freigabe / Aprobación | traitée / entschieden / resuelta |

### Verification

The guard answers an approval and holds both halves: the mark appears, and the
label states only that the request was settled. Mutated by dropping the
optimistic entry, it fails. Visual **646/646**, no golden moved — the row only
exists after a click; the 20-script gate is green; unit 2395/4 outside the
runtime-contract e2e.

### Next

`declined` is still undrawn — the same click, answered the other way.

---

## Round 99 — settled, but which one?

Status: **complete**

Answered the same approval the other way. `declined` draws correctly — an amber
`✗` against the `✓` of an allow, two glyphs rather than two colours — and the
label now says the same neutral thing either way, which was last round's fix.

Drawing it showed what the row still would not say. Every neighbour names its
subject:

| | |
| --- | --- |
| Approval requested | `go list -deps ./...` |
| Tool finished | `Verify package dependencies` |
| **Approval settled** | **—** |

A run that asks twice settles into two rows reading "Approval settled" with
nothing to tell them apart, and the mark says only *how* it was answered.

The subject comes from the REQUEST rather than being restated: same approval,
same `refId`, one owner for the fact. Restating it would have been a second
answer to "what was this about" that could drift from the first.

### Verification

The guard now runs both decisions and holds three things each time: the mark
appears, the label states only that the request was settled, and the row names
the command it settled. Mutated by dropping the summary, both fail. Visual
**647/647**, no golden moved — these rows exist only after a click; the
20-script gate is green; unit 2395/4 outside the runtime-contract e2e.

### Next

Both approval decisions are drawn. The question interrupt settles through the
same local-entry path and has no timeline kind of its own.

---

## Round 100 — `apply_patch`, in the audit trail

Status: **complete**

Two findings, one fixed and one reported.

### Fixed: the timeline printed the fallback the fold hands it

`labelSource` falls back to the tool's **wire name** when a call carries no
identifying argument, and says why: *"spelling '3 files' here freezes a language
into view state."* The fold is right to. The transcript resolves that sentinel
through `toolIntent`; the timeline printed it.

| | before | after |
| --- | --- | --- |
| a patch with a multi-file receipt | `apply_patch` | Applied patch |
| a settled question | `ask_user` | Asked you |
| a stored result | `read_tool_result` | Read stored result |
| everything with an argument | unchanged | unchanged |

Same defect as round 87's approval card, in the surface a person opens to
reconstruct what a run did. Resolved through the same owner rather than a second
table: the row takes the ToolCall its `refId` names and asks `toolIntent`.

The guard checks against the whole built-in vocabulary rather than the two names
that happened to appear, so a tool added later is covered without being listed.

### Reported, not changed: questions leave no trace

The timeline records an approval twice — asked, then settled with a verdict and
a subject. A **question** records nothing:

| state | its whole timeline |
| --- | --- |
| `question` | `run-start` |
| `question-multi` | `run-start` |

A run parked on a question — one of the two ways a run waits for a person —
shows only "Run started" on the surface whose job is to say where the time went.
`TimelineEntryKind` simply has no question member, with nothing stating that as
a decision. Mirroring the approval pair is a small change; it is also new
product behaviour on an audit surface, so it is written down here rather than
taken.

### Verification

Mutated by restoring the raw summary, the guard fails on four rows. Visual
**648/648**; two goldens moved, the rows whose subject changed. Two others and a
0.0007 layout shift failed once under load and pass in isolation — the same
magnitude-stable, identity-unstable flake this file already records for
`delegated`. The 20-script gate is green; unit 2395/4 outside the
runtime-contract e2e — one of which was mine: a new hook the timeline's own unit
test had to be told about.

---

## Round 101 — the seventh event kind

Status: **complete**

Six of the timeline's seven event kinds had been drawn by the end of last round.
The seventh is `tool-start`, which exists only while a call is **in flight** —
and every fixture item is settled, so no state's timeline had ever held one.

Its label was the odd one out in all eight languages:

| kind | label |
| --- | --- |
| run-start | Run started |
| run-end | Run finished |
| tool-end | Tool finished |
| approval-request | Approval requested |
| **tool-start** | **Tool** |

A bare noun where every sibling is a statement — the same shape as the
"Approval" row two rounds ago, without that one's mistranslation. "Tool started",
and the same in the other seven.

### Making it render without paying for a state

`waiting` is the only fixture whose timeline holds a `tool-start`, and no
workspace state binds it — and adding one costs ~50 goldens, because the fixture's
state rail is in every workspace frame. `dock-runs` already has a delegated run
that is *running* with nothing in it, so the call goes there: the grandchild is
now searching while its parent waits on an approval, which is what a two-level
delegation looks like mid-flight anyway.

| | before | after |
| --- | --- | --- |
| dock-runs timeline | 15 events | 16 — the last one still open |
| its ready boundary | four terminal statuses | + "Tool started", the kind nothing settles past |

### Verification

Visual **648/648**; two goldens moved, both `dock-runs`; the 20-script gate is
green; unit 2395/4 outside the runtime-contract e2e.

### Next

Every timeline kind is drawn and every entry status but one — questions still
record nothing, which round 100 wrote down rather than took.

---

## Round 102 — a question, after it is answered

Status: **complete**

Both fixtures that carry a question park it at `requires-action`, so the card had
only ever been photographed *waiting*. It has two settled shapes, and neither had
been drawn:

| | |
| --- | --- |
| answered | a closed disclosure — "Asked 2 questions" — holding each prompt beside what was said |
| never answered | one line: "Closed without an answer" |

The second is reachable exactly as the model's own comment says: *"the Run may
have been canceled without accepting any answer"* — so it belongs in the state
named for that, and the prompt goes with the Run that owned it. Nothing invites
an answer that can no longer be given.

### One thing measured, not changed

In the answered disclosure the prompt is `text-fg-muted` and the person's answer
is `text-fg-faint` — the human's words are the fainter of the two, which inverts
how this app weights them everywhere else. Measured on the rendered pixels:
**6.40:1** and **5.58:1**, both clear of AA, one step apart on a ladder the
design system owns. A deliberate hierarchy in a summary, not a defect.

### Verification

Both guards hold what matters: the exchange stays closed until asked for and then
carries both halves, and the abandoned one says so in a single line with no
request surface left. Visual **650/650**; four goldens moved, the two states that
gained a question; the 20-script gate is green; unit 2395/4 outside the
runtime-contract e2e.

---

## Round 103 — ten numbers for one decision

Status: **已完成**

### 本轮待办

| 状态 | 事项 |
| --- | --- |
| 进行中 | 弹出面板最小宽度收敛到一个令牌 |
| 待处理 | 宽于该下限的站点逐个测量，留下的必须有理由 |

### 问题证据

`ui_rules` 4 禁止"用大量相近数值模拟设计系统"。弹出面板的 `min-w` 有 **17 处、10 个值**：

| 值 | 出现 | 站点 |
| --- | --- | --- |
| 160px | 2 | 会话行右键菜单、复制菜单 |
| 168px | 1 | dock 标签右键菜单 |
| 170px | 1 | 消息右键菜单 |
| 176px | 1 | 输入区工具栏 |
| 180px | 3 | 语言选择、消息右键菜单 |
| 196px | 1 | 审批"记住"下拉 |
| 200px | 1 | 会话标识右键菜单 |
| 220px | 5 | 主题、字体、模型角色选择 |
| 240px | 1 | 主题 |
| 248px | 1 | 输入区工具栏 |

同一类菜单会因为点开的是哪一个而宽度不同，相邻值相差 4–10px。

### 影响页面与组件

`ContextMenu.Content`、`DropdownMenu.Content`、`Select` 的全部调用点：会话侧栏、消息操作、dock 标签、输入区工具栏、审批卡、外观/模型设置。

### 根因

弹出面板的下限没有所有者。每个调用点各自挑一个数，没有任何一处说明依据。仓库已有 `--menu-row-height`，却没有配套的宽度令牌。

### 依据

参照物 `study/chatgpt` 对同类表面只用两个值：`12rem`(192px) 与 `180px`。取 **12rem** 作为唯一下限——它落在现有 160–248 区间中部，抬高的菜单不会截断内容（内容宽于下限自然撑开），压低的两处仅 4–8px。

### 验收标准

1. `src` 中不再出现弹出面板的裸数值 `min-w-[...]`，除非有实测理由并写明。
2. 每个受影响菜单在浏览器中打开，内容不被截断。
3. 类型检查、lint、format、20 项脚本、单测、视觉套件全绿。

### 修改前 / 修改后

| | 修改前 | 修改后 |
| --- | --- | --- |
| 弹出面板下限 | 14 处、9 个裸值（160–248） | 全部 `min-w-[var(--menu-min-width)]`，12rem |
| 设置选择框宽度 | 3 处、2 个裸值（180 / 220） | 全部 `min-w-[var(--select-min-width)]`，220px |
| `src` 中弹出层裸数值 `min-w-[…]` | 17 | **0** |

### 验证中发现并修回的错误

第一次收敛把两类决策并成了一个令牌：`ThemeSection:97`、`FontSection:39`、`LanguageSection:22`
是 **SelectTrigger**（表单字段），不是弹层。压到 12rem 后 settings 的四张 golden 全部改变
——套件当场报出 4 条失败。**菜单下限**与**字段宽度**是两个决策，拆成两个令牌后重跑，
650 全过且**零 golden 位移**，证明数值本身没错、错的是我把它们并了。

### 测量证据

| 弹层 | 下限 | 实际宽 | 内容自然宽 |
| --- | --- | --- | --- |
| 主题选择 | 240 → 192 | 192 | 127 |
| 模型角色 | 220 → 192 | 192 | 171 |
| 模型角色（长） | 220 → 192 | **274** | 274 |
| 审批"记住" | 196 → 192 | 192 | — |
| 会话行右键 | 160 → 192 | 192 | — |

没有一个较宽的值是内容需要的；内容超出下限的自行撑开，不受影响。全部菜单 `clipped: 0`。

### 验证

`npx tsc --noEmit`、`npm run lint`、`format:check`、20 项 `check:*` 全绿；
`npm run visual:test` **650/650**，无 golden 位移；单测 2395 通过、4 条失败是既有的
runtime 合约 e2e（`segment.finished.json` 缺 `contextTokens`、两条 compaction 超时），与本轮无关。
浏览器验证：1400×900 与 1280×900，light/dark，逐个打开设置面板四个选择框、审批"记住"下拉、
会话行右键菜单，测量 `scrollWidth > clientWidth` 均为 0。

### 资源回收

关闭 4174 端口的 vite 预览服务；删除本轮临时探针 `probe-*.mjs`；临时截图留在 `/tmp` 未占用仓库。

### 剩余问题与下一轮方向

用户已决定从 Tailwind 迁移到 StyleX。**该方向与 `desktop/CLAUDE.md` §6.1 的强反向不变量直接冲突**
（"❌ 引入 CSS-in-JS"），按 refactor-prompt `<instruction_priority>` 第 1 条，已记录冲突并等待用户
对该不变量作出明确修订，未擅自绕过。

---

## Round 104 — 只留两套主题

Status: **已完成**

### 本轮待办

| 状态 | 事项 |
| --- | --- |
| 已完成 | 删除 8 个第三方主题预设，只留 flame-light / flame-dark |
| 已取消 | ~~删除自定义主题~~ —— 用户中途明确「customTheme 可以留着」，已从 git 恢复 |
| 已完成 | 清理注册表、明暗解析注释、测试引用 |
| 已完成 | 修复焦点守卫的超时预算（验证中暴露） |

### 依据

用户明确要求：「我们的主题，全部都删掉，只保留两套 1 light 2 dark」。
按 `<instruction_priority>` 第 2 条执行。

### 现状证据

`theme/index.ts` 注册 10 个内置主题 + custom + 视觉风格：

| 组 | 成员 |
| --- | --- |
| 保留 | flame-light、flame-dark |
| 删除 | atom-one-light/dark、catppuccin-latte/mocha、solarized-light/dark、tokyo-night-light/storm |
| 删除 | custom（三色派生 + 实时预览，带 `CustomThemeColors.tsx` 设置分区） |
| 不动 | 视觉风格本来就只有 `flameStyle` 一套 |

爆炸半径（`grep` 全仓）：8 个预设仅被 `theme/index.ts`、`documentAppearance.test.ts`、
`themeScheme.ts`（注释举例）引用；custom 另外被设置面板、偏好、store、theme kit
与两个视觉 fixture 引用。

### 根因

主题是**可选装的贡献**，不是核心能力；十套预设各自维护一整份色板，
而产品只在 flame-light / flame-dark 上做过打磨与视觉回归。多余的九套是熵。

### 验收标准

1. `COLOR_THEME` 扩展点只注册两个 id。
2. 明暗解析、主题切换、外观设置面板在删除后仍成立（注册表驱动，不得留硬编码 id）。
3. 类型检查、lint、format、20 项脚本、单测、视觉套件全绿。

### 修改前 / 修改后

| | 修改前 | 修改后 |
| --- | --- | --- |
| `COLOR_THEME` 注册项 | 11（10 预设 + custom） | **3**（flame-light、flame-dark、custom） |
| `themes/` 文件 | 12 | 3 |
| 删除的预设 | — | atom-one ×2、catppuccin ×2、solarized ×2、tokyo-night ×2 |
| `themeScheme.ts` 注释举例 | `"solarized-light"`（已不存在） | `"custom"`（仍成立） |
| `documentAppearance.test.ts` 里的假主题 | 命名为 solarized | 改名 palette —— 测试自建的 spec，名字不该指向不存在的文件 |

**中途修正**：先按字面「其他都删掉」把 custom 一并删了（含 `CustomThemeColors.tsx`），
用户随即说明保留，已 `git checkout` 完整恢复三个文件并复原注册。

### 验证中发现并修复的问题

`chromeFocus` 那条守卫（4 条路由 × 40 次 Tab × 110ms 落定）单独跑要 **30.2s**，
而 Playwright 默认上限正是 30s —— 空载勉强过、有负载必挂。按它自身工作量给预算
（`ROUTES × TAB_STEPS × 300ms + 20s`），复跑 28.1s 通过。这不是本轮引入的问题，
但一条只在机器空闲时才成立的守卫不算守卫。

### 验证

`npx tsc --noEmit`、`lint`、17 项 `check:*` 全绿；主题单测 **68/68**；
整体单测 **2395 通过**（与删除前一致），4 条失败是既有 runtime 合约 e2e；
`npm run visual:test` **649 通过 + 1 超时**，超时项修复后单独复跑通过，
**无 golden 位移** —— 主题选择器在 golden 里是收起态，列表长度不进画面。

`format:check` 与 `knip` 的失败**不属于本轮**：来自同一工作树里尚未完成的 StyleX
脚手架（`postcss.config.mjs`、`tokens.stylex.ts` 与两个依赖暂无人使用），
已与本轮拆成两次提交。

### 资源回收

关闭 4174 端口预览服务。

---

## Round 105 — StyleX，第一块地基

Status: **已完成（第一批）**

用户决定从 Tailwind 迁移到 StyleX，并同意修订 `CLAUDE.md` §6.1。本轮只做**地基 + 一个组件**，
用它把整条管线的真实代价和坑全部量出来，再谈规模。

### 管线（四个配置，一个来源）

| 文件 | 职责 |
| --- | --- |
| `stylex.babel.mjs` | 唯一一份 Babel 配置 |
| `stylex.vite.mjs` | 唯一一个 Vite 插件封装 |
| `postcss.config.mjs` | 收集并产出 CSS |
| `src/styles/stylex.css` | StyleX 自己的表，`@stylex;` 独占 |

三个 Vite 配置（app / visual / vitest）都取同一个插件。**这不是洁癖**：每次写第二份都立刻出事。

### 踩到并修掉的五个坑（都有证据）

1. **Babel 8 装不上** —— `vite-plugin-babel` 与 StyleX 都按 Babel 7 写；降到 `^7`。
2. **StyleX 不认 `@/` 别名** —— 它自己解析主题文件（变量名由定义它的文件哈希而来），必须给
   `rootDir` + `aliases`，否则 `Could not resolve the path to the imported file`。
3. **两份 Babel 配置会静默分叉** —— Vite 那份转 JS、PostCSS 那份产 CSS；不一致时
   **构建全绿、JS 里有类名、CSS 里没有规则**，组件裸奔。共享同一个对象。
4. **Tailwind 与 StyleX 抢管线** —— 先把 Tailwind 挪到 PostCSS，结果
   `globals.css` 的 `@import` 不再内联，`markdown.css` 整份丢失，**38 条视觉失败**，
   构建还慢到 2×。正解是**各用各的表**：Tailwind 留在 Vite 插件上，StyleX 独占 `stylex.css`。
5. **排版步级是捆绑，不是字号** —— `text-ui-xs` 同时带 `letter-spacing`。只搬 `fontSize` 的结果：

   | | 原 | 只搬字号 | 修正后 |
   | --- | --- | --- | --- |
   | letterSpacing | -0.132px | **normal** | -0.132px |
   | 宽度 | 95.4 | **97.5** | 95.4 |

   —— 12 张 golden 因此位移。令牌里现在把步级做成 `stylex.create` 的捆绑，调用点只能说步名。

### 代价（实测，非估计）

| | 基线 | StyleX 管线 |
| --- | --- | --- |
| `npm run build`（real，×4） | 1.71 / 1.72s | **2.89 / 3.00 / 2.90 / 2.89s** |
| 倍数 | — | **≈1.7×** |
| CSS 产物 | 104K | 112K |
| dist 总体积 | 16M | 16M |

代价来自 Babel 必须过每个 `.tsx`（`include` 只能按路径匹配，无法问"这个文件用没用 StyleX"），
所以**这个倍数在迁移全程基本恒定**，不会随进度继续恶化。

### 迁移期的两条硬事实

- **`className` 逃生口仍然保留**：`cn(props.className, className)`。所有调用方仍是 Tailwind，
  要求两端同时改就不是迁移而是重写。
- **StyleX 不分层**（`useCSSLayers: false`）。本仓库已记过一课：未分层规则胜过任何
  `@layer`。分层会让 StyleX 被遗留的全局规则压住。

### 修改前 / 修改后

| | 修改前 | 修改后 |
| --- | --- | --- |
| `ui/atoms/loader.tsx` | Tailwind utility + `cn()` | `stylex.create` + 令牌 |
| 渲染 | — | **逐像素一致**（95.4 / 12px / -0.132px / 18.6px） |
| 组件识别 | `.animate-shimmer`（类名） | `data-slot="loader"`（本仓库既有约定） |
| 令牌 | — | `color` / `motion` 包住现有 CSS 变量；`type` 是捆绑步级 |

令牌**包住** `globals.css` 的变量而不是内联数值 —— 那些变量是计算出来的
（`--style-shape-md × --radius-scale × --corner-scale`）且按主题重定义，内联等于把一套主题冻死。
未被使用的令牌已删（knip 会盯），随迁移进度再加。

### 验证

`typecheck` / `lint` / `format` / **20 项 `check:*` 全绿**；单测 **2395 通过**
（4 条既有 runtime 合约 e2e 失败）；`visual:test` **650/650，零 golden 位移**。
浏览器实测：1280×900 light，比对迁移前后 Loader 的 4 项计算样式完全相同。

### 资源回收

关闭 4174 预览服务；删除全部临时探针；卸载误装的 `@tailwindcss/postcss`。

### 下一轮方向

按 Polar 的做法逐文件推进。下一批建议从 `ui/atoms` 里**无变体的展示型原子**开始
（skeleton、divider、tag），再碰 `button` —— 后者是 cva 变体 + 650 张 golden 的交汇点，
值得单独一轮。

---

## Round 106 — className 逃生口是个漏洞，不是设计

Status: **已完成**

### 本轮待办

| 状态 | 事项 |
| --- | --- |
| 已完成 | 补齐本批令牌（surface / radius / space / type 步级） |
| 已完成 | 迁移 `kbd`、`divider` |
| 已放弃 | `chip` —— 依赖 `group-hover:`，**StyleX 没有祖先状态选择器**，不属于机械迁移 |
| 已完成 | 由守卫暴露并修掉一处真实设计缺陷（见下） |

### 刻意不做

间距在 `ui/atoms` 里用了 **13 个档位**（0.5 / 1 / 1.5 / 2 / 2.5 / 3 / 3.5 / 4 / 5 / 6 / 8 / 12），
是 Tailwind 的 4px 基准。文章主张按角色命名（`xs`/`s`/`m`/`l`），但**本轮不做这件事**：

样式引擎迁移与令牌语义重命名如果同时进行，每一张 golden 的差异就无法归因 —— 分不清是
StyleX 换了渲染，还是间距换了值。所以本轮只做**机械等价**迁移，令牌镜像现有刻度、
逐像素守恒；语义化作为独立一轮，届时 golden 的每一次位移都只有一个原因。

`refactor-prompt` 第 4 条（优先复用现有设计语言、不得随意引入另一套视觉体系）也指向同一结论。

### 验收标准

1. 三个原子的计算样式与迁移前逐项相同。
2. 视觉套件零 golden 位移。
3. 全部门禁绿。

### 本轮最重要的发现：`className` 逃生口在 StyleX 下不成立

上一轮我保留了 `cn(props.className, className)`，理由是"调用方仍是 Tailwind，要求两端同时改
就是重写"。`cascade.visual.spec.ts` 立刻证明这条推理是错的：

```
height:      `.h-auto`        asks auto,        `.x8161z7:not(#\#):not(#\#):not(#\#)` renders var(…)
min-width:   `.min-w-0`       asks 0px,         `.x1ebxd6j:not(#\#):not(#\#):not(#\#)` renders var(…)
background:  `.bg-transparent`asks transparent, `.xzzci7k:not(#\#):not(#\#)`          renders var(…)
font-family: `.font-mono`     asks mono,        `.xhtk421:not(#\#):not(#\#)`          renders sans
… 九项，全部被丢弃
```

StyleX 生成的选择器带 `:not(#\#)` 特异性提升，**任何单个工具类都压不过它**。
所以"组件迁移、调用方不动"这条增量迁移的接缝，对**任何被调用方覆盖过样式的组件都不成立**。

### 而漏洞底下是一处真实的设计缺陷

那个调用方（`sidebar/actions.tsx`）取消了键帽的**九项属性**：高度、最小宽、底色、内距、
字体、字号、字重、颜色。它要的根本不是键帽，是**行内的快捷键字形**——只借 `<kbd>` 的语义。

这正是 CLAUDE.md §4 说的「缺档就往库里加一档，别在 callsite 手搓」。按第二法则治本：

| | 修改前 | 修改后 |
| --- | --- | --- |
| 调用点 | `<Kbd className="h-auto min-w-0 bg-transparent px-0 font-mono text-ui-2xs font-normal text-fg-faint">` | `<Kbd variant="inline">` |
| 组件 | 一种形态 + 开放的 className | `cap` / `inline` 两种命名形态 |
| 渲染 | — | **完全一致**，650 张 golden 零位移 |

Tailwind 时代这个漏洞"能用"，所以没人发现调用点在取消一个设计。StyleX 把它变成编译期
可见的事实——这恰是采用它的理由本身。

### 刻意不做（保留）

间距仍镜像 Tailwind 的 4px 刻度、不做语义重命名：引擎迁移与令牌重命名同时做，
golden 的每次位移就无法归因。语义化作为独立一轮。

### 验证

`typecheck` / `lint` / `format` / **20 项 `check:*` 全绿**；单测 **2395 通过**（4 条既有
runtime e2e）；视觉 **650/650，零 golden 位移**；`cascade` 守卫由红转绿。

### 资源回收

关闭 4174 预览服务，删除临时探针。

### 下一轮方向

`chip` 那类依赖 `group-hover:` 的组件需要单独设计（StyleX 无祖先选择器）——先盘清
全仓有多少个 `group-*` 依赖，这个数字决定迁移的真实规模。

---

## Round 107 — 迁移的真实规模，和第二批原子

Status: **已完成**

### 先量规模

StyleX 没有祖先/兄弟状态选择器。全仓统计带 `className` 的 **217** 个 tsx 中，
依赖这类选择器的有 **15 个（7%）**：

| 变体 | 文件数 |
| --- | --- |
| `group-hover:` | 6 |
| `group-focus*` | 3 |
| `group-data-[…]` | 1 |
| `[&_…]` / `[&>…]` | 5 |
| `peer-` / `has-[` / `*:` | 0 |

**93% 可以机械迁移**；其余 15 个要把祖先状态改成 props 或 state，是设计工作而非搬运。
这个数字之前不存在，迁移规模一直是估的。

### 本批迁移

| 组件 | 结果 |
| --- | --- |
| `diff-stat` | 迁移完成；`+N` / `−N` / `—` 三种形态 |
| `color-picker-input` | 迁移完成 |
| `external-link` | **无需迁移** —— 它一行样式都没有 |
| `glyph-swap` | **不可机械迁移** —— 用的是全局类 `t-icon-swap` + 祖先 hover |
| `hidden-file-input` | 保留 —— 只有一个 `hidden`，换成 StyleX 无收益 |

### 修改前 / 修改后

| | 修改前 | 修改后 |
| --- | --- | --- |
| `diff-stat` 的 `gap-1.5` / 颜色 | utility 字符串 | `space.s1_5` / `color.success` / `color.negative` |
| 令牌 | 5 组 | 6 组（新增 `space.s1_5`、`color.success`） |
| 渲染 | — | **650 张 golden 零位移** |

### 验证

`typecheck` / `lint` / `format` / **20 项 `check:*` 全绿**；单测 **2395 通过**
（4 条既有 runtime e2e）；视觉 **650/650，零 golden 位移**。

### 资源回收

关闭 4174 预览服务。

### 下一轮方向

`ui/atoms` 里剩下的机械候选（tag、skeleton、chip 之外的展示型），以及那 15 个
`group-*` 依赖者的第一个——后者需要先决定"祖先状态"在 StyleX 下的统一表达方式，
是一次设计决策，不是搬运。

---

## Round 108 — 一个原子替九个调用点做了它不该做的决定

Status: **已完成**

### 迁移

`file-path`、`section-label`。`progress-bar` 暂缓（`indicatorClassName` 是第二个逃生口，
需要先决定它的替代形状）。

### 守卫又一次抓到设计缺陷

`cascade` 守卫报出 `SectionLabel` 的调用方在改它的内距（`pt-0` / `px-0` / `pt-4` / `pb-1`），
被 StyleX 丢弃。查全部调用点：

| 内距写法 | 调用点数 |
| --- | --- |
| 用组件自带的 `px-2 py-2` | **1 / 13** |
| 自己改写 | **12 / 13** |

十三个里只有一个接受这个默认值。**内距根本不该由这个原子拥有** —— 它拥有排版、颜色、
字重、布局与截断；"嵌进容器多深"是容器的事。Tailwind 时代覆盖能生效，所以这个分歧
一直不可见。

### 修改前 / 修改后

| | 修改前 | 修改后 |
| --- | --- | --- |
| `SectionLabel` | 自带 `px-2 py-2` | 不拥有内距 |
| 13 个调用点 | 8 个部分覆盖、1 个全用默认、4 个完整改写 | **13 个各自声明完整内距** |
| 渲染 | — | 650 张 golden 零位移 |

### 过程中我犯的错，以及是什么抓住它

删掉默认值后，那些**只覆盖了部分内距**的调用点连带丢了其余部分。后果是可测的：

> `target-size`: Target has insufficient size (**22px by 22px**, should be at least 24px)

`projects.tsx` 写的是 `pt-0`，期待 `px-2 pb-2` 仍在；默认消失后行内控件缩到 22px，
跌破触控下限 —— **WCAG 审计**当场报出。补完五处部分覆盖、两处零覆盖后归零。

### 迁移的一项固定成本

**断言 utility 类名的测试会在迁移时失效**。本轮第二次遇到（上一次是 `ReasoningBlock`
断言 `.animate-shimmer`）：`file-path.test.tsx` 断言 `.truncate`。StyleX 的类名是生成的，
所以断言改成它真正想表达的**结构**——三个元素、分隔符是独立兄弟、文件名在最后。

### 验证

`typecheck` / `lint` / `format` / **20 项 `check:*` 全绿**；单测 **2395 通过**
（4 条既有 runtime e2e）；视觉 **650/650，零 golden 位移**；`cascade` 与 `target-size`
两条守卫由红转绿。

### 资源回收

关闭 4174 预览服务。

### 下一轮方向

`progress-bar` 的 `indicatorClassName`、以及其余带第二逃生口的原子 —— 每一个都要先问
"调用方到底在改什么"，答案多半又是一个没被命名的形态。

---

## Round 109 — 第二个逃生口，第三个没被命名的形态

Status: **已完成**

### 迁移

`progress-bar`。它带两个逃生口：`className` 和 `indicatorClassName`。

### 三个调用点要的是三种形态

| 调用点 | 传的类 | 它其实要的 |
| --- | --- | --- |
| `activity-disclosure` | `h-0.5 rounded-none` + `indicatorClassName="rounded-none"` | 披露块边缘的**发丝接缝**（方角，因为它与那条边连续，不是躺在上面的物体） |
| `toolStats` | `h-1 flex-1` | 统计行内的细条 |
| `TasksPill` | `mt-1.5 ml-[18px]` | **只调位置**，外观不动 |

高度与形状是这个组件的决定；**位置不是**。

### 修改前 / 修改后

| | 修改前 | 修改后 |
| --- | --- | --- |
| API | `className` + `indicatorClassName` | `weight: "bar" \| "row" \| "seam"`，`className` 只留给布局 |
| 第二个逃生口 | 存在 | **删除** |
| 调用点 | 传高度与圆角 | 说出形态名；只有位置仍用 `className` |
| 渲染 | — | 650 张 golden 零位移 |

`indicatorClassName` 是个尤其坏的形状：它让调用方伸手进组件**内部的第二个元素**。
命名形态之后它没有存在理由。

### 验证

`typecheck` / `lint` / `format` / **20 项 `check:*` 全绿**；单测 **2395 通过**；
视觉 **650/650，零 golden 位移**。

### 资源回收

关闭 4174 预览服务。

### 累计

已迁移 7 个原子（loader、kbd、divider、diff-stat、color-picker-input、file-path、
section-label、progress-bar）；暴露并修掉 **3 处真实设计缺陷**；**零 golden 位移**；
构建代价恒定在 1.7×。

---

## Round 110 — 迁移不该削弱类型

Status: **已完成**

### 迁移

`gauge`、`ansi-text`。

`Gauge` 的 `className="text-accent"` **不是逃生口**：它用 `currentColor` 描边，墨色本来
就该由所在行决定。保留。

### 第三次：规格断言 utility 类名

`dock-terminal` 的就绪判据是 `.text-negative` —— 一个生成后不存在的类名。这次它跑在
真实浏览器里，所以断言可以改得比原来**更强**：不是"有没有这个类"，而是

> 那一行渲染出的 `color`，是否等于 `--color-negative` 解析后的值

这条状态存在的理由本就是"终端面板读转义码而不是把码打印出来"，颜色才是它要证明的东西。

### 自己造成的一处退化，以及是什么抓住它

`ansi-text` 原本是 `Record<AnsiTone, string>` —— **对 tone 联合类型穷尽**。我换成
`stylex.create` 后，键不再受联合类型约束：新增一个 tone 会静默渲染成周围的墨色。
`knip` 报出 `AnsiTone` 未被使用，暴露了这件事。

已补回 `Record<AnsiTone, …>` 映射：**少答一个 tone 现在是编译错误**。
迁移的目的是让决策进类型系统，途中把类型削弱掉是本末倒置。

### 修改前 / 修改后

| | 修改前 | 修改后 |
| --- | --- | --- |
| `ansi-text` tone→色 | `Record<AnsiTone, string>` 类名映射 | `stylex.create` + `Record<AnsiTone, …>` 穷尽映射 |
| `dock-terminal` 就绪判据 | 类名 `.text-negative` | **渲染出的颜色**等于 `--color-negative` |
| `gauge` | utility 字符串 | StyleX；`currentColor` 与 `className` 墨色保留 |

### 验证

`typecheck` / `lint` / `format` / **20 项 `check:*` 全绿**；单测 **2395 通过**；
视觉 **650/650，零 golden 位移**；补回穷尽映射后重跑 `dock-terminal` 与
`WCAG audit workspace dock` 共 83 条全过。

### 资源回收

关闭 4174 预览服务。

### 累计

已迁移 **9 个原子**；暴露并修掉 **3 处设计缺陷 + 1 处我自己造成的类型退化**；
**零 golden 位移**；构建 1.7×。

---

## Round 111 — cva → StyleX 的样板

Status: **已完成**

### 迁移

`tag`。选它是因为它是**第一个 `cva` 组件** —— 迁移它等于确立后面所有变体组件
（含 `button`）要用的样板。

### 样板

| cva | StyleX |
| --- | --- |
| `cva(base, { variants: { size, ink } })` | 一个 `stylex.create`，base 与每个变体各一个键 |
| `defaultVariants` | 函数参数默认值 |
| `VariantProps<typeof styles>` | 显式联合类型 + `Record<Union, …>` 映射 |
| `cn(styles({size, ink}), className)` | `stylex.props(base, styles[size], styles[ink])` 后再拼 `className` |

`Record<Union, …>` 那一步是**刻意的**：上一轮 `ansi-text` 用裸对象丢了穷尽检查，
这里从一开始就写成映射，少答一个变体是编译错误。

### 调用点审查

三处传 `className`，全部良性：`ml-auto`（布局）、`tabular-nums`（数字变体）、
以及一处看似覆盖颜色的 `not-italic text-fg` —— 查证在 `<em>` 上，不在 Tag 上；
那处 Tag 本就正确地用了 `ink="strong"`。**本轮没有暴露新的设计缺陷**，
这也是有意义的结果：`Tag` 的变体本来就设计对了。

### 验证

`typecheck` / `lint` / `format` / **20 项 `check:*` 全绿**；单测 **2395 通过**；
视觉 **650/650，零 golden 位移**。

### 资源回收

关闭 4174 预览服务。

### 累计

已迁移 **10 个原子**（含首个 cva 组件）；暴露并修掉 3 处设计缺陷 + 1 处类型退化；
**零 golden 位移**；构建 1.7×。

---

## Round 112 — 一个扮成链接的按钮

Status: **已完成**

### 为迁移 `button` 先清场

`button` 是 34 个调用点、6×2×7 变体 + 复合变体的交汇点，还带一条 StyleX 无法表达的
后代选择器。直接迁会让"渲染变化"与"设计变化"混在一起。所以先在 **Tailwind 仍生效时**
把调用点的覆盖清掉——每一步都能用 golden 证明等价。

### 调用点覆盖分类（证据）

| 站点 | 覆盖 | 真正缺的 |
| --- | --- | --- |
| `ProjectSelector` | **14 项** | **`link` 变体** ← 本轮 |
| `ApprovalCard` | `rounded-l-none` + `before:` 分隔线 | 分段按钮 |
| `ConnectionPane` ×2 | `h-9`（36px） | 控件高度阶梯没有这一档 |
| `ModelPicker` | `text-negative` + `px-2` | `danger` 已有；内距差 1px 属漂移 |
| `chip` | 隐藏到 group-hover 才出现 | 揭示型控件（另属 group 阻塞） |

### 本轮：`link`

那个调用点取消了按钮的高度、圆角、边框、底色、内距、字重与不换行，再加上下划线、
点线装饰与一个扩大命中区的伪元素——**它不是按钮的一种外观，是一个用按钮元素扮的链接**。
按 CLAUDE.md §4「缺档就往库里加一档」，它进库。

| | 修改前 | 修改后 |
| --- | --- | --- |
| 调用点 | 14 个 utility | `variant="link"` + `className="max-w-full"`（只剩布局） |
| 库 | 6 个变体 | 7 个，`link` 带命中区伪元素 |
| 渲染 | — | **650 张 golden 零位移** |

### 一处必须写下来的实现约束

`cn` 是 tailwind-merge —— 冲突由**类序靠后者**取胜。cva 按 `variants` 对象的键序输出，
原来 `size` 排在 `variant` 之后，会压过 `link` 的中和类。已把 `variant` 移到最后；
其余变体只设墨色与填充，`size` 从不触碰，所以这次重排对它们不可见。

### 验证

`typecheck` / `lint` / `format` / **20 项 `check:*` 全绿**；单测 **2395 通过**；
视觉 **650/650，零 golden 位移**。

### 资源回收

关闭 4174 预览服务。

### 下一轮方向

清场继续：分段按钮形态、36px 高度档、`ModelPicker` 的 1px 内距漂移。全部清完再迁 `button`
本身，届时没有覆盖可与它冲突。

---

## Round 113 — 一行控件差 4px，因为按钮没有那一档

Status: **已完成**

### 实测的缺陷

连接设置里 URL 字段与它旁边两个按钮同处一行：

| | 高度 |
| --- | --- |
| `TextField size="lg"` | **32px** |
| Reset / Apply 按钮 | **36px** |

差 4px。原因是调用点写了 `h-9` —— 而它当时**无档可选**：Button 的尺寸只有
xs 22 / sm 26 / md 30，`--control-height-lg: 34px` 存在却只被 `icon-lg` 用着。
没有一档能承起"站在字段旁边的文字按钮"。

### 修改前 / 修改后

| | 修改前 | 修改后 |
| --- | --- | --- |
| Button 尺寸 | xs / sm / md + icon-* | 加 **`lg`**（走 `--control-height-lg`） |
| 调用点 | `size="sm" className="h-9 shrink-0"` ×2 | `size="lg" className="shrink-0"` ×2 |
| 实测行高 | 32 / **36** / **36** | 32 / **34** / **34** |
| `ModelPicker` 错误按钮 | `className="gap-1.5 px-2 text-ui-sm text-negative"` | `variant="danger" size="xs"`（内距 8→7px，属漂移） |

### 上报：两条高度阶梯本身不对齐

剩下的 2px 不是调用点的问题：

| 档 | `--control-height-*` | `--field-height-*` |
| --- | --- | --- |
| sm | 26 | 26 ✅ |
| md | **30** | **28** |
| lg | **34** | **32** |

`sm` 一致，其上各差 2px。所以"同名尺寸的按钮与字段等高"这件事**在令牌层就不成立**。
改任一条阶梯会波及全部控件，属于设计系统决策 —— 记录，不擅自改。

### 验证

`typecheck` / `lint` / `format` / **20 项 `check:*` 全绿**；单测 **2395 通过**；
视觉 **650/650，零 golden 位移** —— 连接面板只进 WCAG 审计不进 golden 集，
所以这处 4px 修正是真实的但未被拍照。

### 资源回收

关闭 4174 预览服务。

### 下一轮方向

清场只剩分段按钮（`ApprovalCard` 的 `rounded-l-none` + `before:` 分隔线）。
清完即可迁 `button` 本身。

---

## Round 114 — 分段按钮：把接缝还给库

### 计划（写在改代码之前）

`ApprovalCard` 的"Allow once ▾"是一个分段按钮：主动作 + 限定它的菜单。
两半的形状不是由 `Button` 给的，而是调用点用两串 className 手搓出来的 ——
这是第 113 轮清场后 `Button` 上剩的最后一处形状级 override。
按 CLAUDE.md §4「业务层不自己拼交互件：缺档就往库里加一档」，这一档叫 `join`。

### 证据

| 位置 | 手搓的 className |
| --- | --- |
| `ApprovalCard.tsx` 主按钮 | `rounded-r-none` |
| `ApprovalCard.tsx` 菜单触发 | `-ml-px rounded-l-none before:absolute before:inset-y-1.5 before:left-0 before:w-px before:bg-cta-text/20` |

调用点在描述**两个按钮如何拼成一个控件** —— 这是控件自己的知识，不是这张卡的。

### 根因

`Button` 只有"独立按钮"一种形态。产品需要第二种：*一对按钮读作一个控件*。
库里没有这一档，调用点就只能自己画接缝 —— 于是接缝的宽度、颜色、内缩、
以及"用 `-1px` 收掉两条边留下的缝"这四个决定，全散在业务文件里。

### 改动

`buttonStyles` 新增 `join: "start" | "end"`：

| | start | end |
| --- | --- | --- |
| 圆角 | 右侧收平 | 左侧收平 |
| 接缝 | — | `before:` 画 1px 竖线（`bg-cta-text/20`，上下各内缩 6px） |
| 贴合 | — | `-ml-px` 收掉两条边之间的缝 |

接缝用伪元素而非 `border`：border 落在填充之外，会读成整对按钮的描边。

### 过程中的真 bug

`join` 变体加进了 `buttonStyles`，但 `Button` 组件**没解构它**，
于是它顺着 `...props` 落到了 DOM 上，`buttonStyles()` 从没收到过。
表现是"全绿的 typecheck + 成功的 build + 完全没生效的样式"。

抓住它的是视觉套件：6 张 golden 位移（审批卡在 light/dark × waiting/narrative，
外加两张 Retina closure）。实测确认两个按钮圆角都还是 6px、`margin-left: 0`。
修好后重测 —— `tr: 0` / `tl: 0` / `ml: -1px`，650/650 零位移。

**教训**：cva 变体与组件签名是**两处**要同步的地方，类型系统不覆盖第二处
（多余的 prop 合法地流进 `...props`）。golden 是这里唯一的守卫。

### 验证

`typecheck` / `lint` / `format` / **17 项 `check:*` 全绿**；单测 **2395 通过**
（4 项失败均为既有 runtime 契约项，属用户并行修改区域）；
视觉 **650/650，零 golden 位移**。

### 资源回收

关闭 4174 预览服务与探针脚本。

### 下一轮方向

`Button` 上的形状级 override 已清空，但它自己的迁移**还不能开始** ——
`chip.tsx` 依赖 `group-hover:`，而 StyleX 没有祖先/兄弟状态选择器。
"祖先态如何在 StyleX 下表达"是一个波及 15 个文件的设计决定，需要先定型。
在此之前，转向不依赖祖先态的下一批 atom。

---

## Round 115 — 四个呈现件迁 StyleX，兼两处调用方在纠正的设计

### 计划（写在改代码之前）

`Button` 的迁移被 `chip.tsx` 的 `group-hover:` 挡住（StyleX 无祖先态选择器），
所以本轮转向不依赖祖先态的一批：`surface` / `badge` / `status-dot` / `skeleton`。
按前几轮的惯例，先审计调用点在 override 什么 —— override 是设计缺档的指纹。

### 证据

| atom | 调用点 | 带 `className` 的 | 说明 |
| --- | --- | --- | --- |
| `Surface` | 12 | 11 | 8 处是 `flex flex-col gap-*`（容器内的排布，合理）；`SettingsGroup` 是另一回事 |
| `Badge` | 22 | 8 | 8 处全是 `font-mono` |
| `StatusDot` | 6 | 1 | 唯一一处是 `mt-1.5`（对齐首行，合理） |
| `SkeletonList` | 3 | 0 | 干净 |

### 根因一：`SettingsGroup` 不是"加了点东西的 Surface"，是第二种面

```
className="overflow-hidden border-[length:var(--control-edge-width)] border-field bg-transparent"
```

`bg-transparent` **取消**了 `Surface` 的填充，`border-*` **换掉**了它的边界机制。
两者都不是装饰，是在把"填充的卡片"改写成"描边的透明组"—— 这是第二种面，
不是同一种面的调整。Tailwind 下它靠 `cn()` 的后来者优先侥幸成立；
StyleX 下生成选择器带 `:not(#\#)`，这两条会被静默丢弃。

### 根因二：8 个 badge 在用 UI 字距渲染等宽字形

`--text-ui-xs--letter-spacing: var(--tracking-ui)` = `-0.011em`，
而 `--text-code--letter-spacing: 0`。调用点写 `font-mono` 只换了字族，
换不掉字距 —— 于是 HTTP 状态码、工具名、provider id、错误码这 8 处，
全都是等宽字形套着为比例字体调的负字距。这正是 `well.tsx` 注释里已经
点名过的同一个错误（"UI tracking that never belonged on mono"）。
调用点表达不了它，因为字距压根不在 `font-mono` 这个工具类里。

### 改动

| | 新增 |
| --- | --- |
| `Surface` | `fill: "card"（默认，填充+主题阴影钩子）\| "outlined"（透明+`--control-edge-width` 描边）` |
| `Badge` | `face: "text"（默认）\| "mono"`—— 同时换字族**和**归零字距 |

### 验收标准

- `SettingsGroup` 与 8 处 mono badge 的调用点不再出现形状/字体 className；
- mono badge 的宽度变化可测且被 golden 记录（字距修正是真实的位移，不是回归）；
- 其余 golden 零位移；17 项 `check:*` 全绿。

### 不做：`Well`

`well.tsx` 导出的 `WELL_SURFACE` 是一串 Tailwind class，被仍是 Tailwind 的
`text-field.tsx`（`variant="well"`）消费 —— 那行注释的原意就是"well 和它的
编辑态不能漂开"。单迁 `well` 会把这条共享断掉，所以它和 `text-field` 必须同批。
本轮不碰，记在下一轮。

### 计划的修订：`fill` 两值 → `variant` 四值

写计划时只看到 `SettingsGroup` 一处在取消填充，于是设计了 `fill: card | outlined`。
第一次跑视觉，**15 张 golden 位移**，其中 10 张与 badge 无关 —— StyleX 把另外
两处对抗也顶了出来，而它们在 Tailwind 下一直是静默成立的：

| 调用点 | 覆盖了什么 | StyleX 下的后果 |
| --- | --- | --- |
| `ApprovalCard` / `QuestionCard` | `rounded-bubble` | 圆角被丢弃，退回卡片角 |
| `QuestionCard` | `shadow-[var(--shadow-popover)]` | 浮层投射被丢弃 |
| `ProvidersPane` | `inset="sm"` 之后又写 `p-2` | 内距 8 → 12px |

于是这个面一共有四种，每一种都是产品里一个有名字的东西：

```
variant: "card" | "group" | "request" | "prompt"
```

**没有拆成三个正交属性**（fill / corner / raise）—— 那会拼出 8 种，其中一半无意义
（没有填充却带浮层投射）。一个面的身份是一个决定，不是三个恰好一致的决定。
`inset` 同时补上缺的 `xs`（8px）档 —— `ProvidersPane` 要的就是它。

### 守卫抓到的第二件事：`DotTone` 有三份

迁 `status-dot` 时我给联合起了个名，`check:published-boundaries` 立刻报重名同义。
把它挪进 `lib/tone.ts`（`Tone` 已经住在那里，理由相同：application 环不得导入 ui 环，
所以词汇要放在两边都够得着的地方）之后，守卫又看见了**第三份** ——
`ServerRow.tsx` 里内联重写的同一个联合，此前因为这个词汇没有"已发布"位置而看不见。
一并收敛。

### 验证

| | 结果 |
| --- | --- |
| 视觉 | **650 / 650** |
| 位移的 golden | 5 张，全部 diff 框在 badge 自身位置（`34×18` 单个 badge、工具列的安全等级列、provider 列），无版式移动 —— 确认后更新 |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | 2395 通过；4 项失败均为既有 runtime 契约项（用户并行修改区域） |

字距修正的量：`--tracking-ui` = `-0.011em`，12px 步进下每字符 `-0.132px`，
13px 步进下 `-0.143px` —— 每个 badge 累计半个到一个多像素，方向是变宽（负字距被移除）。

### 过程中差点漏掉的

迁 `skeleton` 时我把它原有的 `motion-reduce:animate-none` 丢了。
这是我引入的行为变化，不是迁移，已补回。
（顺带发现：`globals.css` 有一条 `!important` 的全局 reduced-motion 规则，
所以逐组件的 guard 在今天是冗余的 —— 但"一个事实两个所有者"该怎么收敛
是独立的一件事，不混进这轮机械迁移。记在后面。）

### 资源回收

关闭 4174 预览服务与全部探针脚本。

### 下一轮方向

`well` + `text-field` 必须同批（`WELL_SURFACE` 是它们共享的一串 Tailwind class）。
再往后是 reduced-motion 的所有者收敛，以及仍然挡在 `button` 前面的
`chip.tsx` `group-hover:` 设计决定。

---

## Round 116 — `well` + `text-field` 同批迁移

### 计划（写在改代码之前）

这两个必须同批：`well.tsx` 导出的 `WELL_SURFACE` 是一串 Tailwind class，
被 `text-field.tsx` 的 `TextArea variant="well"` 消费 —— 那是"well 和它的
编辑态不能漂开"的实现方式。单迁任一个都会把这条共享断掉。

### 证据：31 个调用点，13 处带 `className`

| 覆盖内容 | 处数 | 判定 |
| --- | --- | --- |
| 外边距 / 宽度 / `flex-1` | 8 | 调用方的事（兄弟之间的间距），保留 |
| `text-fg-soft` | **3** | 缺档 —— 见根因一 |
| `bg-surface-3 rounded-xs px-1` | 1 | 第三种边 —— 见根因二 |
| `block`（`as="code"`） | 1 | 原子自己该推导 —— 见根因三 |
| `tabular-nums`（`type="number"`） | 1 | 设计系统的硬规则，不该由调用点记得 |

### 根因一：`TextArea` 没有 ink 档，而 `Well` 有

`agentMemory` ×2 与 `knowledge` 都写 `text-fg-soft` —— 三处，够一档。
而 `Well` 早就有 `ink: soft | strong`，且 `TextArea variant="well"` 就是
同一块面的编辑态。所以不是新造一个维度，是把已有的词汇借过去：
**一套词汇，两个组件**。Tailwind 下这三处能成立，StyleX 下 `BASE` 的
`color` 会以 `:not(#\#)` 特异性把它们全部丢弃。

### 根因二：行内就地编辑是第三种边

`SessionRow` 的标题编辑器写 `variant="bare"` 之后又加回
`rounded-xs bg-surface-3 px-1` —— `bg-surface-3` 与 `bare` 的
`bg-transparent` 直接冲突。它要的不是"无边框"，是"在一行里就地编辑：
不占 chrome，但要显示自己可编辑"。这是 `EDGE` 的第三个值 `inline`。

### 根因三：`<code>` 是行内元素，`Well` 却总是块

`ApprovalCard` 写 `className="block"` 只因为它传了 `as="code"`。
`<pre>` 和 `<div>` 本来就是块，只有 `<code>` 不是 —— 这是原子从 `as`
就能推导的事，不该让调用点记得。同时 `TracesPanel` 的 `grid gap-2`
必须继续有效，所以 `Well` 不能无条件设 `display`。

### 改动

| | 新增 |
| --- | --- |
| `TextField` / `TextArea` | `ink: "default" \| "soft"`（借 `Well` 的词汇） |
| `TextField` 的 `variant` | 第三值 `inline` |
| `TextField` | `type="number"` 时自动 `tabular-nums`（CLAUDE.md §3 的硬规则） |
| `Well` | `as="code"` 时自动 `display: block` |

### 验收标准

- 上述 6 处形状/字体 className 从调用点消失，8 处布局类保留；
- `WELL_SURFACE` 从"导出的 class 串"变成两个组件共享的 StyleX 样式对象；
- golden 零位移（这轮全是让既有渲染在 StyleX 下继续成立，无设计修正）；
- 17 项 `check:*` 全绿。

### `WELL_SURFACE` 从 class 串变成共享样式

```
- export const WELL_SURFACE = "rounded-sm bg-sunken px-3 py-2.5 font-mono text-code leading-relaxed";
+ export const WELL_SURFACE = stylex.create({ face: { … } });
```

共享的东西没变（还是那一张脸），变的是它现在是**编译期产物**而不是一串希望
调用顺序正确的字符串。`TextArea variant="well"` 把它排在 size 步进之后，
理由和从前一样：well 自己的内距要压过尺寸档的。

### 顺带清掉的一条不变量

`TextField` 的 `bare` 和 `inline` 都不再声明高度 —— 从前 `size` 的
compoundVariants 只对 `boxed` 生效，等价语义靠"某几条 compound 恰好没写"
表达。现在 `variant === "boxed" && INPUT_SIZE[size]` 把它写成了一句话：
**只有 boxed 自己定高，另外两种由容纳它的东西定。**

### 验证

| | 结果 |
| --- | --- |
| 视觉 | **650 / 650，零 golden 位移** |
| 守卫 | 17 项 `check:*` 全绿（含 `check:styles` 与 `cascade`） |
| 单测 | 2395 通过；4 项失败均为既有 runtime 契约项 |

`cascade.visual.spec.ts` 在整套并行跑时超时过一次（等不到 `data-visual-ready`），
单跑 9.2s 通过，六条路由逐一手工加载也都正常 —— 是并行下的启动竞争，不是回归。

### 资源回收

关闭 4174 预览服务与探针脚本。

### 下一轮方向

atoms 里仍是 Tailwind 的还有 30 个上下，其中 `button` 仍被 `chip.tsx` 的
`group-hover:` 挡着。下一批挑不依赖祖先态的中型件：`option-row` / `step-row` /
`empty-state` / `segmented` / `vertical-tabs`。

---

## Round 117 — 自身属性态解锁，迁 `empty-state` / `step-row` / `segmented`

### 先解决的一个未知：StyleX 能不能表达 `data-[active]:`

`option-row` 用 `aria-selected:` 和 `data-[highlighted]:`，`segmented` 用
`data-[active]:` —— StyleX 的条件键必须以 `:` 或 `@` 开头，属性选择器不行。
实测 `:is([data-highlighted])` **可以**，而且编译产物把 `:is()` 拆掉了：

```
.x1ahn7o9[data-highlighted]:not(#\#):not(#\#):not(#\#){background-color:#040506}
```

这解锁了所有**自身属性态**的组件。解锁不了**祖先态**（`group-hover:`），
所以 `button` / `chip` 仍然挡着 —— `:is()` 作用在元素自己身上。

### 证据：这批调用点几乎没有对抗

| atom | 调用点 | 带 `className` 的 | 内容 |
| --- | --- | --- | --- |
| `EmptyState` | 18 | **0** | — |
| `StepRow` / `StepMark` | 1 | 0 | — |
| `Segmented` | 10 | 1 | `justify-self-end`（布局，调用方的事） |

18 个调用点零覆盖，说明 `EmptyState` 的两档尺寸把需求覆盖完了 —— 这次迁移
是纯机械的，不带设计修正。

### 不做：`option-row`

`menu.tsx` 把 `floatingRowStyles({ size: "sm" })` 当**字符串**拼进
`MENU_ITEM_CLASSES` —— 和上一轮 `WELL_SURFACE` 同一种耦合。两者必须同批，
留到下一轮。

### 验收标准

golden 零位移；17 项 `check:*` 全绿。

### 抓到一个跨越前几轮的静默回归：圆角只迁了一半

`empty-state` 的图标圆迁完后，6 张 golden 位移。逐属性比对新旧 computed style，
唯一的差异是 **`corner-shape`**：

| | OLD | NEW |
| --- | --- | --- |
| `border-radius` | `3.35544e+07px` | `9999px` |
| `corner-shape` | `round` | **`superellipse(1.5)`** |

半径本身不是原因 —— 实测 `9999px` 与 `33554432px` 在 1× 和 2× 下渲染**逐字节相同**。
真正的原因在 `globals.css`：

```css
:where(*, *::before, *::after) { corner-shape: superellipse(1.5); }
.rounded-full, .rounded-pill, .type-caret { corner-shape: round; }
```

**退出机制挂在 Tailwind 的类名上。** StyleX 组件永远不带那些类名，所以
只设半径 = 把设计系统里的每一个圆变成圆角方块。

这不是这一轮的问题 —— `status-dot`（6px）、`divider`（18px）、`progress-bar`、
`badge` 在前几轮就已经这样了。没被发现是因为 golden 的容差是
`maxDiffPixels: 40`，而这些尺寸上的圆角差异**都在 40 像素以下**。
只有 `empty-state` 的 40px 图标圆大到足以越过阈值。

### 治本

这是 `type` 步进的同一课，在另一条阶梯上重演：**一个圆角步进携带的是两个决定**。
所以 pill 变成 bundle，且 `radius` 不再暴露它 —— 两半分不开，因为它们从来
就不是两个决定：

```ts
export const corner = stylex.create({
  pill: { borderRadius: "var(--radius-pill)", "corner-shape": "round" },
});
```

（StyleX 不认识驼峰的 `cornerShape`，会静默丢弃；kebab 的 `"corner-shape"` 才发得出。
这本身也值得记：**StyleX 对不认识的属性是静默丢弃，不是报错。**）

六个已迁组件全部改用 `corner.pill`。`progress-bar` 顺带少了一层重复 ——
它的 `barFill` / `rowFill` / `seamFill` 只是把轨道的圆角又写了一遍，
一个形状两处声明就是两个可以互相矛盾的地方，现在是一张 `CORNER` 表。

### 被这个回归污染过的 golden

第 115 轮我重录的那 5 张（`dock-run-summary` ×2 / `dock-tools` ×2 /
`settings-providers`）在容差内固化了超椭圆的 badge。它们已按修复后的渲染重录。
其余 golden 从未重录，仍持有迁移前的正确图像 —— 修好后它们只会更接近。

### 守卫抓到的第三件事

`EmptyStateSize` 具名之后，`check:published-boundaries` 看见
`data-view.tsx` 里内联重写了同一个联合。收敛（和上一轮的 `DotTone` 同一类）。

### 验证

| | 结果 |
| --- | --- |
| 视觉 | **650 / 650，零位移** |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | 2395 通过；4 项失败均为既有 runtime 契约项 |

### 下一轮方向

浮层簇：`floating-surface` 被 `menu` / `popover` / `tooltip` /
`lightbox-dialog` / `confirm-dialog` / `search-overlay` / `text-editor-dialog`
以 class 串消费，`option-row` 又被 `menu` 以同样方式消费 —— 九个文件一个单元。
先迁所有者 `floating-surface`，未迁的消费方用 `stylex.props()` 取类名，
再逐个迁消费方。

---

## Round 118 — 浮层簇：先迁所有者

### 计划（写在改代码之前）

`floating-surface.tsx` 导出五串 Tailwind class（`FLOATING_PANEL` / `FLOATING_TIP` /
`FLOATING_MOTION` / `MODAL_SCRIM` / `FLOATING_LAYER`），被七个文件消费：
`menu` / `popover` / `tooltip` / `lightbox-dialog` / `confirm-dialog` /
`search-overlay` / `text-editor-dialog`。这是本仓库最大的一处 class 串共享。

**不一次迁完九个文件。** StyleX 样式对象可以被未迁文件用 `stylex.props()`
取出类名来消费 —— 所以所有者先迁，消费方按自己的节奏跟上。这一轮迁
`floating-surface` 本身，加上两个薄到可以整体迁的消费方：`popover` / `tooltip`。

### 消费方冲突检查（迁之前必须做）

StyleX 生成的选择器带 `:not(#\#)` 特异性，未迁消费方叠加的 Tailwind class
只要与所有者声明同一属性就会被丢弃。逐个查过：

| 消费方 | 叠加的 class | 与所有者是否同属性 |
| --- | --- | --- |
| `menu` | `p-1 focus-visible:outline-none` | 否 |
| `lightbox-dialog` | `cursor-zoom-out` + 位置/尺寸/圆角/底色/阴影 | 否 |
| `confirm-dialog` / `search-overlay` / `text-editor-dialog` | 同上 | 否 |

`FLOATING_MOTION` 只声明 transition / scale / translate / opacity，
`MODAL_SCRIM` 只声明 position / inset / z-index / background / transition —
没有一个消费方碰这些。安全。

### 验收标准

golden 零位移；17 项 `check:*` 全绿；未迁的四个 dialog 与 `menu` 继续工作。

### 结果：五串 class 变成五个样式对象

| 导出 | Before | After |
| --- | --- | --- |
| `FLOATING_PANEL` | 一串 class，靠拼接顺序 | `[face, motion, panel]` |
| `FLOATING_TIP` | 同上，只有圆角不同 | `[face, motion, tip]` |
| `FLOATING_MOTION` | 一串 data-variant | `motion` |
| `MODAL_SCRIM` | 一串 class | `scrim` |
| `FLOATING_LAYER` | `"z-[var(--layer-floating)]"` | `layer` |

未迁的五个消费方（`menu` 与四个 dialog）改用 `stylex.props(...)` 取类名 ——
这条路走得通，正是它让"九个文件一个 commit"变成"所有者先走，消费方跟上"。

`tooltip` 的 `TIP_PADDING` 也一并收进样式对象，`Tooltip` 的 `max-w-[280px]`
成为 `tip.label` —— 它是"标签不是段落"这个决定，不是一个魔法数字。

### 顺带治了一个守卫的根因

`check:locales` 报 `transitionProperty: "opacity, scale, translate"` 是
"查找表里的文案"。它的逃生口是：

```js
// 每个 utility 都带连字符或变体冒号，而目录里的句子不会
const CLASS_LIST = /[-:]/;
```

这是**按 Tailwind 值恰好长什么样**判的。StyleX 的值没有那个形状 ——
`"opacity, scale, translate"` 三个"词"，既无连字符也无冒号。

没有把 CSS 写歪去迁就它，而是让守卫认结构：`stylex.create({…})` 块内
的一切都不是文案，无论长什么样。加了一个括号配对的 `styleBlocks()`，
把落在块内的 `TABLE_ENTRY` 跳过。

反向验证过：在同一文件的 style block **之外**放 `a: "Recent tasks"`，
守卫照抓不误。

### 验证

| | 结果 |
| --- | --- |
| 视觉 | **650 / 650，零位移** |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | 2395 通过；4 项失败均为既有 runtime 契约项 |

### 资源回收

关闭 4174 预览服务。

### 下一轮方向

浮层簇剩下的消费方：`menu` + `option-row`（后者被前者以 class 串消费，
且 `ContextIconItem` 与两个 `OptionRow` 调用点都在指定网格列 —— 三处，够一档），
以及四个 dialog。

---

## Round 119 — 浮层簇之二：`option-row` + `menu`

### 计划（写在改代码之前）

`menu.tsx` 把 `floatingRowStyles({ size: "sm" })` 当**字符串**拼进
`MENU_ITEM_CLASSES` —— 这是浮层簇里最后一处 class 串共享，两者必须同批。

### 证据：三处调用点在替 `layout="grid"` 补它缺的那一半

| 调用点 | 写的网格列 |
| --- | --- |
| `FileMentionPopup` | `grid-cols-[auto_1fr]` |
| `SlashSuggestions` | `grid-cols-[auto_1fr]` |
| `menu` 的 `ContextIconItem` | `grid-cols-[14px_minmax(0,1fr)]` |

`layout: "grid"` 只给了 `display: grid`。一个没有模板的 grid 是单列，
所以每一个真正要两列的调用点都得自己补 —— 三处，而且补法还不一致。
这不是"调用点想定制"，是这一档本身只做了一半。

查过溢出：两处的第二个子元素都带 `truncate`（含 `overflow-hidden`），
所以 `1fr` 的自动最小尺寸已被压成 0，`1fr` 与 `minmax(0,1fr)` 在此等价 ——
不是 CLAUDE.md §4 要防的那种撑爆。统一用 `minmax(0,1fr)` 是因为它自己说清楚了。

### 改动

`layout` 从 `grid | flex` 变成 `grid | flex | glyph`，
`glyph` 携带 `grid-template-columns: auto minmax(0, 1fr)` —— 一个字形，然后是其余。

`menu` 的 `destructive` 也进 `OptionRow`：它是"这一行会毁掉东西"这个状态，
不是调用点挑的三个 class。

### 验收标准

三处网格列 className 消失；golden 零位移；17 项 `check:*` 全绿。

### 结果

| | Before | After |
| --- | --- | --- |
| `floatingRowStyles` | cva，被 `menu` 当字符串拼接 | `stylex.create`，加一个 `floatingRow()` 组合器 |
| `layout` | `grid \| flex` | `grid \| flex \| glyph` |
| 三处网格列 className | `grid-cols-[auto_1fr]` ×2、`grid-cols-[14px_minmax(0,1fr)]` ×1 | `layout="glyph"` |
| `destructive` | 调用点写三个 class | `floatingRowStyles.destructive` |
| `MENU_ITEM_CLASSES` | 字符串 | `menuItem(layout)` |

`ContextIconItem` 的 `layout` 是穿过 `ContextItem` 传下去的，不是自己再叠一层
StyleX —— 两次独立的 `stylex.props()` 调用无法互相去重，同一个 `display`
会退回按源序决胜。一个属性一次组合。

### 又一处守卫的根因

`check:published-boundaries` 报 `LoaderSize` 与新具名的 `RowSize` 是
"同一个联合的两个名字"（都是 `sm|md|lg`）。

但档位阶梯不是词汇 —— 它是每个控件按构造共享的**命名约定**，各自支持不同子集。
Loader 的 `sm` 是字号步进，行的 `sm` 是行高；把它们收敛等于断言这两个是
同一个决定，并且让每个控件拿到所有控件档位的并集。守卫自己的注释写得很清楚，
它防的是主题值、审批立场、文档范围那类**词汇**。

所以排除"成员全部来自档位名"的联合，narrow 且有理由。
反向验证过：真的词汇重名（`comfortable|compact` 换个名字）照抓不误。

### 验证

| | 结果 |
| --- | --- |
| 视觉 | **650 / 650，零位移** |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | 2395 通过；4 项失败均为既有 runtime 契约项 |

### 下一轮方向

浮层簇只剩四个 dialog（`lightbox` / `confirm` / `search-overlay` /
`text-editor`），它们已经在用 StyleX 的 `MODAL_SCRIM` / `FLOATING_MOTION`，
剩下的是各自的版式。

---

## Round 120 — 浮层簇之三：四个 dialog，与它们各写一遍的同一个面板

### 计划（写在改代码之前）

`MODAL_SCRIM` 被抽出来了，**它遮住的那个东西没有**。四个 dialog 各自
手写了一遍"模态面板"，而且互相不一致：

| | 定位 | 圆角 | 填充 | 宽度 |
| --- | --- | --- | --- | --- |
| `confirm-dialog` | `inset-0 m-auto` | `--floating-panel-radius` | `bg-canvas` | `min(400px,…)` |
| `text-editor-dialog` | `inset-0 m-auto` | **`--shape-composer`** | `bg-card` | `min(420px,…)` |
| `lightbox-dialog` | `inset-0 m-auto` | `--floating-panel-radius` | `bg-card` | `w-fit max-w-…` |
| `search-overlay` | **`inset-x-0 top-24 mx-auto`** | `--floating-panel-radius` | `bg-canvas` | `min(520px,…)` |

四者一致的部分：`position: fixed`、`z-index: var(--layer-modal)`、
`box-shadow: var(--shadow-modal)`、`outline: none`。这些抽成 `MODAL_PANEL`，
定位分 `centred` / `top` 两档（3 : 1）。

### 上报，不擅自统一：填充 2 : 2 分裂

`bg-canvas`（`--color-bg`，不透明）两处，`bg-card`（`--color-elevated`）两处。
两者都不是 `--app-floating-surface`（那是 `--app-content-surface` 的 90% 半透明，
配 `::before` 的背景模糊，而这四个都没有那层模糊）。

**这是一个没有所有者的分歧**，不是某一处写错。四个模态里两个说自己是画布材质、
两个说自己是卡片材质 —— 挑哪一个是设计决定，不是迁移能顺手做掉的。
本轮把它们迁成令牌引用（`surface.canvas` / `surface.card`），
让分歧从四串 class 变成两个可数的名字，记录在此等定夺。

圆角的 `--shape-composer` 同理：`text-editor-dialog` 是唯一一个用它的，
理由可能是"它里面装着一个 composer"，也可能只是漂移。一并上报。

### 验收标准

四个 dialog 的 `position` / `z-index` / `shadow` / `outline` 只写一次；
golden 零位移；17 项 `check:*` 全绿。

### golden 抓到这轮最大的一处对抗：lightbox 的三种用法是三种东西

迁完第一版，两张 golden 报 **"Expected 1120×720, received 272×208"** ——
对话框塌成了它内容的大小。原因是 `ImagePreviewGallery` 在写：

```
className="h-[100dvh] w-screen max-h-none max-w-none overflow-hidden
           rounded-none bg-media-preview p-0 shadow-none"
```

**一口气取消面板的七条属性。** Tailwind 下靠 `cn()` 后来者优先成立，
StyleX 下七条全被 `:not(#\#)` 丢弃。

再看另外两个调用点，三者是三种东西，不是一种的三次调整：

| 调用点 | 装的是 | 收敛为 |
| --- | --- | --- |
| `MermaidBlock` | 一张图，贴合内容，视口封顶 | `kind="figure"`（默认） |
| `MarkdownTable` | 一份文档，可读宽度，可滚动 | `kind="document"` |
| `ImagePreviewGallery` | 整屏的媒体，近黑场 | `kind="media"` |

`media` 没有圆角可倒、没有平面可投影 —— 这正是它此前要逐条取消的原因。
内距也随之收进各自的档（0 / 6 / 8+12），因为它和"装的是什么"完全同步。

### 第二次位移：`document` 漏了基座的 `w-fit`

第二轮跑，表格预览从 **408px 被撑到 896px**。原因是我给 `document` 写了
`min-width` / `max-width` 却没写 `width: fit-content` —— 而
`inset: 0` + `margin: auto` + `width: auto` 会把固定定位的元素拉满可用宽。
基座类里那个不起眼的 `w-fit` 是有活儿干的。补回后通过。

### 上报，不擅自改

1. **模态填充 2 : 2 分裂** —— `confirm` / `search-overlay` 用 `bg-canvas`，
   `lightbox` / `text-editor` 用 `bg-card`。两者都不是 `--app-floating-surface`
   （那是半透明配背景模糊，这四个都没有那层模糊）。没有所有者的分歧。
2. **`text-editor-dialog` 是唯一用 `--radius-composer` 的模态** ——
   可能因为它里面装着 composer，也可能只是漂移。
3. **`document` 档同时有 `border` 和 `--shadow-modal`**（后者含 ring）——
   这是 DESIGN.md §5 明禁的双边。**原样保留**：去掉哪一条是设计决定。

### 验证

| | 结果 |
| --- | --- |
| 视觉 | **650 / 650，零位移** |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | 2395 通过；4 项失败均为既有 runtime 契约项 |

### 资源回收

关闭 4174 预览服务。

### 下一轮方向

浮层簇迁完。`ui/atoms` 里仍是 Tailwind 的还有二十余个，
`button` 依旧被 `chip.tsx` 的 `group-hover:` 挡着 —— 那是一个需要定型的设计决定，
不是一次迁移。下一批挑独立件：`icon-button` / `pill-button` / `text-button` /
`checkbox` / `switch` / `slider` / `collapsible`。

---

## Round 121 — 四个独立控件：`checkbox` / `switch` / `slider` / `collapsible`

### 计划（写在改代码之前）

浮层簇迁完，转向不依赖祖先态、也不被 class 串共享绑住的独立件。
先审计过全部调用点：

| atom | 调用点 | 带 `className` 的 | 内容 |
| --- | --- | --- | --- |
| `Checkbox` | 3 | 1 | `mt-1`（外边距，调用方的事） |
| `Switch` | 8 | **0** | — |
| `Slider` | 1 | 0 | — |
| `Collapsible` | 5 | 0 | — |

零设计缺档 —— 这一轮是纯机械迁移。

### 不做：`IconButton` 与 `TextButton`

| atom | 调用点 | 带 `className` 的 |
| --- | --- | --- |
| `IconButton` | 59 | **25** |
| `TextButton` | 11 | **8** |

两者都够单独成轮。`IconButton` 另有多处依赖 `group-hover:` /
`group-hover/output:` —— 和 `button` / `chip` 挡在同一件事上：
**祖先态在 StyleX 下如何表达**，那是一个需要定型的设计决定，不是一次迁移。

### 一处要小心的地方

`Slider` 现在写的是 `className ?? "w-36"` —— 调用方的 `className` **替换**
默认宽度而不是叠加。迁成样式后这个"替换"语义会消失（StyleX 会赢）。
唯一的调用点没有传 `className`，所以把 `w-36` 变成 atom 自己的宽度是安全的，
而且比"默认值藏在一个 `??` 里"更诚实。

### 验收标准

golden 零位移；17 项 `check:*` 全绿。

### 结果

四个文件迁完，**golden 一次通过、零位移**。这一轮没有设计发现 ——
调用点本来就没在跟这四个控件对抗，所以它就该是纯机械的。

三处 `rounded-full` / `rounded-pill`（switch 的轨道与拇指、slider 的
轨道 / 填充 / 拇指）走上一轮建的 `corner.pill`，没有再变成超椭圆。

`Slider` 的 `className ?? "w-36"` 改成 atom 自己的宽度 —— 唯一的调用点
没有传 `className`，而"默认值藏在一个 `??` 里、并且会被调用方整个替换掉"
本来就不是一个可读的契约。

### 验证

| | 结果 |
| --- | --- |
| 视觉 | **650 / 650，零位移**（一次通过） |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | 2395 通过；4 项失败均为既有 runtime 契约项 |

### 下一轮方向

`TextButton`：11 个调用点里 8 个带 `className`，其中
`SessionList.tsx` 的注释已经自己说出了根因 ——
"TextButton has no height of its own, so at the smallest UI size its box is the
text line"。那是一个真实的缺档，够单独一轮。

---

## Round 122 — `TextButton`：调用点已经把根因写在注释里了

### 计划（写在改代码之前）

11 个调用点，**8 个带 `className`**。而其中一处的注释自己说出了根因：

```
// TextButton has no height of its own, so at the smallest UI size its box is the
// text line — under the 24px target minimum for a row control.
```

这不是调用点想定制，是 atom 造成了一个可达性缺陷，每个调用点各自打补丁。

### 证据：8 处 override 分成三类

| 类 | 调用点 | 写的是什么 |
| --- | --- | --- |
| **行**（3） | `SessionList` | `min-h-6 rounded-[var(--row-radius)] px-2 py-1 text-ui-xs text-fg-faint hover:bg-hover hover:text-fg` |
| | `ToolOutputPanel` | `w-full justify-center py-1.5 hover:bg-hover` |
| | `tools.tsx` | `px-[var(--density-column-gutter-wide)] pt-3.5 pb-4.5 leading-body` |
| **链接**（2） | `FileRefLink` | `font-mono text-accent underline decoration-transparent hover:decoration-current` |
| | `ApprovalArgsEditor` | `font-mono text-ui-xs font-semibold text-accent hover:underline` |
| **外边距**（3） | `PluginsPane` / `skillProposals` / `CompactionBlock` | `mt-1.5` / `mt-1.5` / `max-w-full self-start py-1.5` |

前两类是两种没有名字的形状，第三类是调用方自己的事（保留）。

### 改动

`shape: "inline" | "row" | "link"`：

- `inline`（默认）—— 今天的形状：句子里的一段会响应的文字。
- `row` —— 列表里的一条控件：**自己有最小高度**（满足 24px 目标尺寸）、
  行圆角、`bg-hover` 行状态、占满宽度。内距**不进** atom ——
  三处各不相同（`py-1.5` / `px-2 py-1` / `pt-3.5 pb-4.5`），
  而 StyleX 下 atom 不声明的属性调用点才能补。
- `link` —— 等宽 + accent + hover 出现下划线。用 `FileRefLink` 的做法
  （下划线常在但透明，hover 上色），因为它能跟着 `transition-colors` 过渡；
  两者在未 hover 时渲染一致，所以不会动 golden。

另补两档缺的：`tone: "faint"`（`SessionList` 要的）与 `size: "xs"`。

### 验收标准

8 处 override 减到 3 处（全是外边距/布局）；
`row` 形状实测最小高度 ≥ 24px；golden 零位移；17 项 `check:*` 全绿。

### 中途 11 张位移，根因不在 `TextButton`

第一版跑完 11 张 golden 位移，`cascade` 守卫把话说得很准：

```
padding-top: `.py-1.5` asks (shorthand), `.x1717udv:not(#\#)` renders 0px
padding-left: `.px-2`  asks (shorthand), `.x1717udv:not(#\#)` renders 0px
```

`TextButton` 的 base 里有 `padding: 0` —— 那是**浏览器按钮内距的重置**。
Tailwind 下调用方能盖掉它，StyleX 下盖不掉，四个调用点的内距全部消失。

看 `ButtonPrimitive`：

```
"border-0 bg-transparent font-sans text-left focus-visible:outline-none
 disabled:cursor-not-allowed disabled:opacity-45"
```

它已经关掉了 border / background / font / text-align / focus / disabled ——
**唯独漏了 padding**，于是每个 atom 各自补一遍那半个重置。

治本在这一层：`p-0` 进 `ButtonPrimitive`。它在 primitive 里是普通 utility
特异性，所以 StyleX atom 和 Tailwind 调用方都能自由覆盖；
而 `TextButton` 的 base 顺带删掉了四条本就属于 primitive 的声明
（`background` / `border` / `padding` / `text-align`）。改完 **650 / 650**。

这也是设计系统三环该有的样子：**primitive 关掉浏览器 chrome，atom 穿令牌。**
重置漏一半，漏掉的那一半就会散到每个 atom 里去。

### 结果

| | Before | After |
| --- | --- | --- |
| 带 `className` 的调用点 | 8 / 11 | 6 / 11，且**全是内距 / 外边距 / 字重** |
| 形状 | 无 | `inline` / `row` / `link` |
| `row` 的最小高度 | 无（调用点补 `min-h-6`） | `--control-height-sm`，实测 **26px** |
| 色调 | `muted` / `accent` / `negative` | 加 `faint` |
| 字号 | `sm` / `md` | 加 `xs` |

### 上报，不擅自改

`TextButton` 的 base 与 `ButtonPrimitive` 都声明 disabled 态，
但透明度一个 `0.5` 一个 `0.45`。Tailwind 与 StyleX 下都是 `0.5` 赢，
所以今天没有差异 —— 但这是**一个事实两个所有者**，且两个值不一样。
收敛到 primitive 会让禁用态从 0.50 变到 0.45，是个视觉决定，记录待定。

### 验证

| | 结果 |
| --- | --- |
| 视觉 | **650 / 650，零位移** |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | 2395 通过；4 项失败均为既有 runtime 契约项 |

### 下一轮方向

`icon-button`（59 个调用点、25 处 override），但其中多处依赖
`group-hover/output:` 一类祖先态 —— 先把那批数出来，能迁的先迁。

---

## Round 123 — 「显形」：一个事实的十一种拼法

### 计划（写在改代码之前）

`button` / `chip` / `icon-button` 三个最大的剩余 atom 全部卡在祖先态上 ——
StyleX 没有祖先选择器。而 `globals.css` 自己早就写下了问题：

> The reveal itself is **eleven different class lists** — which group, which
> pseudo-class, opacity or visibility — so what is marked here is the one part
> that is the same decision everywhere.

`data-reveal` 只认领了其中"触屏上一律显形"那一小块，**机制本身仍是十一份**。

### 机制：自定义属性通道（已实测）

StyleX 能设自定义属性、也能读 `var()`，而自定义属性天然继承 —— 于是宿主
发布自己的指针/焦点状态，任意深度的后代读它，全程不需要祖先选择器：

```
.x1rzenz2:hover{--reveal:1}
.x11qyy8r:focus-within{--reveal:1}
.x15c1u7l:not(#\#):not(#\#):not(#\#){opacity:var(--reveal,1)}
```

回退值是「显形」，所以不在任何宿主里的目标就是可见的 —— 一个安全的默认。

### 证据：十一处的差异只在"哪些属性参与"

| 参与的属性 | 处数 |
| --- | --- |
| opacity + pointer-events | 8 |
| 仅 opacity | 2 |
| opacity + **visibility** | 1（`context-dock`） |
| 另加 scale | 1（`chip`） |
| 反向（`rest`：显形时让位） | 1（`navigation-row`） |

触发条件也不齐：`group-hover` 八处、`group-focus-visible` 两处、
`focus-within` 两处、**`context-dock` 一处都没有**。

### 一个我先判错、读代码后更正的点

初看 `context-dock` 的关闭按钮用 `invisible`（`visibility: hidden`）
且没有任何 focus 触发条件，我判它是"键盘永远关不掉一个 dock tab"的缺陷。
读了那里的注释后更正 —— 它是**有意为之**：

> a focusable sibling inside a `tablist` is an unallowed child
> (axe `aria-required-children`, critical) … Delete/Backspace on the focused
> tab is the ARIA practice for a closable tab and needs no extra stop in the
> tab order.

× 被刻意排除出 tab 顺序，键盘另有 Delete/Backspace 通路。所以机制要容纳
**两种藏法**，而不是把它们抹平：

- 用 `opacity` 藏 —— 仍可聚焦，`:focus-within` 会把它显出来（十处）。
- 用 `visibility` 藏 —— **不可聚焦**，是纯指针可供性，键盘另有通路（一处）。

两者的区别不是风格，是"这个控件该不该占一个 tab 停靠点"。

### 改动

`ui/atoms/reveal.ts`：`host` / `shown` / `displaced` 三个样式。
宿主发布四个值（`--reveal` / `--reveal-events` / `--rest` / `--rest-events`），
十一处调用点各自换成其中一个。未迁的 Tailwind 文件用
`stylex.props(reveal.host).className` 取类名，和浮层簇同一条路。

### 验收标准

十一份 class 列表减到一份；`context-dock` 的关闭按钮键盘可达；
golden 零位移；17 项 `check:*` 全绿。

### 四轮迭代才收敛，每一次都是同一类根因

这一轮的 golden 从 **80 张位移**跌到 0，中间四次全是"同一个属性被两处声明"：

| 轮 | 位移 | 根因 |
| --- | --- | --- |
| 1 | 80 | 通道自带 `transitionProperty`，压掉了元素**自己的过渡列表**（`Button` 的 `background-color,border-color,color,scale`、chip 的 `scale`） |
| 2 | 79 | `MessageBlock` 的动作区拿了 `reveal.shown` 却**没有宿主** —— 回退值是"显形"，于是消息动作变成常驻可见 |
| 3 | 5 | 触屏回退（`[data-reveal]` 全局规则）被生成规则压过；`CompactionBlock` 展开时的 `opacity-100` 同理；dock 的 × 被 `:focus-within` 显了出来，而它**刻意**不该 |
| 4 | 3 | `group/choice` 我判成死标记删了 —— 它其实被 `group-data-[checked]/choice:` 消费，我的 grep 只匹配了 hover/focus |

第 4 条是我的判断错误：**"没有消费方"这个结论必须用足够宽的模式验证**。
最终用 `group-[a-z-]+(-\[[^]]*\])?/[a-z-]+` 复查了全部命名组。

沉淀成三条：

1. **过渡列表属于元素，不属于通道。** 一个 `transition-property` 声明就是全部；
   通道只决定 opacity/pointer-events 取什么值，过渡留在各自原本的位置。
2. **回退值要选安全的一侧，但每个目标都必须真的有宿主。** 回退是"显形"，
   所以漏配宿主的后果是"永远显形"而不是"永远消失" —— 前者 golden 一眼看见，
   后者会静默地藏起一个功能。这个选择是刻意的。
3. **触屏回退随通道一起走。** `[data-reveal]` 那条全局规则出不了 StyleX 的特异性，
   所以每个目标自带 `@media (hover: none)`。全局规则留着，
   因为 `MarkdownTable` 还没迁。

### 结果

| | Before | After |
| --- | --- | --- |
| 显形的写法 | **11 份** class 列表 | 1 个通道，4 个样式 |
| 触发条件 | `group-hover` 8 / `group-focus-visible` 2 / `focus-within` 2 / 无 1 | 统一 `:hover` + `:focus-within`（指针可供性只认 `:hover`） |
| 死的命名组 | `group/code`、`group/code-snippet` | 删除 |
| 挡住 `button` 的东西 | 祖先态无法表达 | **已解开** |

### 留下的一处例外（有记录的理由）

`activity-disclosure` 的 chevron 要的是**兄弟节点**的 `:focus-visible` ——
自定义属性只能向下继承，去不了旁边。它原有的注释已经写明
`:has(:focus-visible)` 在 Chromium 上匹配但不触发子树失效，所以那条路也不通。
这一处保留 Tailwind 拼法，header 同时挂上通道供 `ToolCard` 的动作使用。

### 验证

| | 结果 |
| --- | --- |
| 视觉 | **650 / 650，零位移** |
| 守卫 | 17 项 `check:*` 全绿（含 `cascade` 与那两条触屏 / 键盘闭环测试） |
| 单测 | 2395 通过；4 项失败均为既有 runtime 契约项 |

### 下一轮方向

`button` 终于可以迁了 —— `chip` 已经不再需要 `group-hover:`。
迁完 `button` 才轮得到 `icon-button`（59 个调用点，25 处 override）。
「跟随染色」那一族（4 处 `group-hover:text-*` / `bg-*` / `scale-*`）是另一件事。

---

## Round 124 — `Button` 的档位补齐（引擎不动）

### 计划（写在改代码之前）

`chip` 迁完后 `button` 的阻塞解除了。但它不是一个孤立的文件：
`IconButton` 包着它、`catalog-picker` 直接消费 `buttonStyles`，
两层加起来 **93 个调用点、39 处 `className`**。

**这一轮不换引擎。** 按 `tokens.stylex.ts` 里已立的原则 ——
"Mechanical first, with rendering held still" —— 先在 cva 上把缺的档位补齐、
把调用点清干净，让每一处改动的效果可以逐条比对；下一轮再整体迁 StyleX，
那时每张位移的 golden 就只有一个成因。

### 证据：39 处 override 分成六类

| 类 | 处数 | 写的是什么 |
| --- | --- | --- |
| **媒体上的控件** | 7 | `bg-media-scrim text-on-media hover:bg-media-scrim`（4 处带底色，2 处只在 hover 出现 —— 后者本就坐在一条已经有 scrim 的托盘里） |
| **墨色状态** | 5 | `text-success` ×2、`text-accent`/`text-negative`/`text-fg` ×3 |
| **圆角** | 4 | `rounded-full` ×3、`rounded-md` ×1 |
| **浮起的圆钮** | 1 | `JumpToBottomButton`：`bg-canvas border-0 shadow-[var(--shadow-raised)] hover:bg-surface-2` |
| **布局 / 定位 / 动效** | 19 | `absolute …`、`shrink-0`、`animate-spin` —— 调用方的事，保留 |
| **纯重述** | 3 | `text-fg-faint hover:bg-hover hover:text-fg` = `quiet` + `ghost` 已有的，删掉即可 |

前四类是四个真实缺档；第五类保留；第六类是噪音。

### 尺寸上的一处上报

媒体控件都写 `size-10`（40px）。控件阶梯是
`xs/sm/md/lg = 22/26/30/34`，**没有 40**。

我起初想借 `--touch-target` 解释它 —— 查了一下那是 **44px**，对不上，
理由不成立。40 就是这条阶梯缺的下一档：`--control-height-xl: 40px`，
理由是"这个控件是隔着距离对着一张照片读的，不在密集行里"。
按阶梯补档，而不是继续让调用点写 `size-10`。

### 验收标准

39 处 override 减到 19 处（全是布局 / 定位 / 动效）；
golden 零位移（这一轮每个新档位都逐字复现原有 class）；17 项 `check:*` 全绿。

### 结果：39 → 32，四个最大的簇清空

| 新档位 | 清掉的调用点 |
| --- | --- |
| `variant: "media"` / `"mediaTray"` | 7 |
| `tone: "accent"` / `"success"`（`ghost` 下读作墨色） | 5 |
| `round` | 4 |
| `variant: "raised"` | 1 |
| 纯重述（`text-fg-faint hover:bg-hover hover:text-fg` = `quiet` + `ghost`） | 3 |

`IconButton` 也不再硬编码 `variant="ghost"` —— 它本来就该把这一档传下去，
否则每个想要别的变体的调用点都只能改写 class。

### 一处自我更正

我起初想用 `--touch-target` 解释媒体控件的 `size-10`。查了一下那是 **44px**，
和 40px 对不上，理由不成立。40 就是控件阶梯缺的下一档，
按 `--control-height-xl: 40px` 补，理由写在令牌旁边。

### 守卫又走了一次同样的路

`check:published-boundaries` 报 `IconButton` 的内联 `xs|sm|md|lg|xl`
重述了 `IconSize`。这和第 119 轮是同一件事 —— 档位阶梯是命名约定不是词汇 ——
但那一轮的排除只加在了"两个具名联合"那条规则上，**内联重述那条漏了**。
补上，同样反向验证过。

### 验证

| | 结果 |
| --- | --- |
| 视觉 | **650 / 650，零位移**（一次通过 —— 每个新档位逐字复现原有 class） |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | 2395 通过；4 项失败均为既有 runtime 契约项 |

### 下一轮方向

剩余 32 处里 19 处是布局 / 定位 / 动效（调用方的事，保留），
还有第二批真实缺档，需要单独取证：

- **触发器的展开态** ×2 —— `data-[popup-open]:bg-selected data-[popup-open]:text-fg`
- **行形状的按钮** ×3 —— `w-full justify-start rounded-[var(--row-radius)] font-normal`
- **chip 形状的触发器** ×3 —— `gap-1.5 px-2 text-ui-sm`
- 以及 `send.tsx` 的 `ACTION`/`ACTION_OFF`/`QUIET`、`TasksPill` 的 tone 表、
  `toolbar` 的 `disabled:opacity-25`。

清完才换引擎。

---

## Round 125 — `Button` 的第二批档位

### 证据

| 缺档 | 处数 | 调用点写的是 |
| --- | --- | --- |
| **触发器的展开态** | 5 | `data-[popup-open]:bg-selected data-[popup-open]:text-fg`（4 处）/ `data-[popup-open]:bg-surface-3`（1 处，select 是另一种面） |
| **没有盒子的按钮** | 1 + | `h-auto min-h-6 rounded-none border-0 p-0 font-normal hover:bg-transparent` —— 而 `link` 变体里已经有一模一样的一半 |

`data-popup-open` 是 Base UI 自己在触发器上挂的属性，所以"一个打开了浮层的
按钮要显示自己开着"是 **Button 这一层的事实**，不是四个调用点各自的装饰。
非触发器上这个属性永远不出现，所以进 `ghost` 是无害的。

`bare` 则是把 `link` 里本来就有的那一半提出来命名：`link` = `bare` + 虚线下划线
+ 扩大的命中区。`GoalStatusSurface` 要的正是不带下划线的那一半。

### 上报：尺寸阶梯把字号和高度绑死了

三处触发器（`GoalModeIndicator` / `HeaderDiffStat` / `composer-chip`）都在写
`text-ui-sm` 配默认高度。阶梯是 `xs = h22 + px7 + text-ui-sm`、
`sm = h26 + px9 + text-ui-md`、`md = h30 + px11 + text-ui-md` ——
**没有"md 的高度配 sm 的字号"这一档**，所以三处都自己拆开写。

这不是一个能顺手补的档：把字号从高度里拆出来会让 `size` 从 8 个值
变成 8×3 的矩阵。要么承认"字号是独立的一维"，要么承认这三处该用别的组件。
记录，等定夺。

### 结果

| | Before | After |
| --- | --- | --- |
| 写 `data-[popup-open]:` 的调用点 | 5 | 1（`select-trigger`，它用的是另一种面） |
| `link` 与"无盒按钮"的重复 | `link` 里内联 | 抽成 `BARE`，`link = BARE + 下划线 + 命中区` |
| `GoalStatusSurface` | 取消 6 条属性 | `variant="bare"` |

### 验证

| | 结果 |
| --- | --- |
| 视觉 | **650 / 650，零位移**（一次通过） |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | 2395 通过；4 项失败均为既有 runtime 契约项 |

### 下一轮方向

`Button` 上剩下的冲突性 override 已经很少，主要是上报的那一条
（字号与高度在 `size` 里绑死）以及几处一次性的（`send.tsx` 的三个常量、
`toolbar` 的 `disabled:opacity-25`、`TasksPill` 的 tone 表）。
清完即可整体迁 `button` + `icon-button` 到 StyleX。

---

## Round 126 — `chip` 修饰档，与剩余冲突的盘点

### 决定：`size` 保持绑定，加一个修饰而不是加一档

三处触发器（`GoalModeIndicator` / `HeaderDiffStat` / `composer-chip`）看起来
各要各的，实测是**同一条规则在两个高度上**：

| | `size` 给的 | 调用点改成 |
| --- | --- | --- |
| `sm` | `px-9 text-ui-md` | `px-1.5 text-ui-sm` |
| `md` | `px-11 text-ui-md` | `px-2 text-ui-sm` |

字号降一档、内距紧一档、墨色从 `muted` 提到 `soft` —— 因为 chip 上的字
是**当前值**，不是一个可以点的动作。

所以不是新增尺寸档（那会让 `size` 从 9 个值变成矩阵），
而是 `chip` 修饰已有的档：两条 compoundVariant，高度原样保留，行仍然对齐。

### 中途 98 张位移，成因只有一个：cva 的声明顺序

第一版跑完 98 张位移。逐字段和上一版对比，**只有颜色不同**：

```
OLD  c: rgb(61, 65, 71)   ← text-fg-soft
NEW  c: rgb(90, 93, 99)   ← text-fg-muted
```

`chip` 的 `text-fg-soft` 声明在 `variant` 之前，而 `cn()` 是 tailwind-merge ——
**后来者赢**，所以 ghost 的 `text-fg-muted` 把它盖掉了。

这条规则 `variant` 自己的注释早就写着（"Declared AFTER `size` so its
neutralising classes win"）。我加新档时没读它。把 `chip` 移到 `variant` 之后，
并在那里也写上同一条理由 —— 一个靠声明顺序成立的不变量，
**必须在每一个依赖它的地方都说出来**，否则下一个人还会踩。

移动之后逐字段与旧版一致，全套 650 / 650。

### 结果

| | Before | After |
| --- | --- | --- |
| 三处触发器的 className | `gap-1.5 px-2 text-ui-sm text-fg-soft hover:…` 等 | `chip`（`HeaderDiffStat` 另留 `font-mono`） |
| `size` 的值 | 9 个 | 9 个（没有变成矩阵） |

### 验证

| | 结果 |
| --- | --- |
| 视觉 | **650 / 650，零位移** |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | 2395 通过；4 项失败均为既有 runtime 契约项 |

### 下一轮方向

`Button` 上仍与自身声明冲突的调用点只剩三处一次性的：
`send.tsx` 的 `ACTION` / `ACTION_OFF`（填充与墨色）、
`toolbar` 的 `disabled:opacity-25`、`TasksPill` 的 tone 查找表。
清完即可整体迁 `button` + `icon-button` 到 StyleX。

---

## Round 127 — `tone` 的语义统一，与我上一轮删掉的一个状态

### 先说我的错

第 124 轮我把 shiki 复制按钮的
`copied ? "text-success" : "text-fg-faint hover:bg-hover hover:text-fg"`
整条当成"纯重述"删掉了。**只有后半截是重述**，
`copied ? "text-success"` 是"刚刚复制成功"这个状态。
golden 没有覆盖已复制态，所以没抓到。已修复。

### 第二个错：`tone` 在同一个 prop 下有两个意思

同样是第 124 轮，我加了两条 compound：

```
{ variant: "ghost", tone: "accent",  class: "text-fg hover:text-accent" }   ← hover 色
{ variant: "ghost", tone: "success", class: "text-success" }                ← 静止色
```

一个 prop 两个意思。统一成一条规则：

- **`tone` = 这个控件报告的墨色，静止时就生效**（`text-accent` / `text-success` / …）
- **`quiet` = 退到最淡**；`quiet` + `tone` = 静止时淡、指上去才显出那个色

`quiet` 因此从 `IconButton` 的一个 className 变成 `Button` 的一档 ——
它本来就是按钮的状态，只是此前只有 `IconButton` 用到。
`ScheduleRow`（"运行"与"删除"两个安静按钮）现在只写 `tone={tone}`。

### 另外两处一次性冲突

| 调用点 | Before | After |
| --- | --- | --- |
| `send.tsx` | `ACTION` / `ACTION_OFF` / `QUIET` 三串 class | `variant="primary"` + `round` + 一个 `NUDGE` |
| `toolbar` | `disabled:opacity-25` | `off="faded"` |

`ACTION_OFF` 说的其实是"**填充按钮不能用时该长什么样**" ——
一个实心 CTA 在 64% 不透明度下读起来像坏了，不像关掉了。
所以那是 `primary` 自己的禁用态，不是调用点的装饰。

`off` 两档："现在用不了"（默认）与"这里根本不适用"（给一个读不了图片的模型
挂附件按钮）。

### 上报：`TasksPill` 暂不动

`STATUS_ICON` 的 tone 同时喂给按钮和一个裸 `Icon`，改成语义值会让这张表
有两份表示（语义名 + class）。而 `running` 用的是 `text-fg` ——
比 ghost 的静止墨色更强，那是"强调"而不是"色调"，Button 没有这一维。
维持原状，记录为迁 StyleX 前最后一处待定。

### 一处刻意的视觉改动，3 张 golden

`primary` 的禁用态从「64% 的 CTA 填充」变成中性底板。位移的正好是那 3 张
含**禁用 primary 按钮**的 golden（目标编辑器的 Save ×2、providers 面板 ×1），
实测颜色 `rgb(116,150,224)` → `rgb(238,241,245)`。

这不是回归，是把应用里最重要的那个按钮（composer 的发送）
手写了很久的答案变成系统的答案。已重录这 3 张。

### 验证

| | 结果 |
| --- | --- |
| 视觉 | **650 / 650**（3 张按上述理由重录，其余零位移） |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | 2395 通过；4 项失败均为既有 runtime 契约项 |

### 下一轮方向

`Button` / `IconButton` 上还与自身声明冲突的只剩 `TasksPill` 一处（已上报）。
下一轮整体迁 `button` + `icon-button` 到 StyleX。

---

## Round 128 — 清完 `Button` 的最后四处冲突

### `TasksPill`：`quiet` 在那里什么也没决定

```
quiet                                   → text-fg-faint
className={cn(tone, …)}  tone="text-fg" → text-fg
```

`cn()` 是 tailwind-merge，**后来者赢** —— 所以 `quiet` 永远被 tone 的 class 盖掉，
它在这个调用点上是死的。删掉它，两个真实色调走 `tone`，
`running` 不带色调（它就是普通状态，chrome 自己的墨色就是普通的样子）。

行内那个裸 `Icon` 没有 `tone` 这个 prop，所以同一套词汇在那里投影成一个 class ——
表还是只有一张，投影只是替读不懂它的消费方把答案拼出来。

### `navigation-row`：又是「行」这个形状

`w-full justify-start rounded-[var(--row-radius)] font-normal` ——
和第 122 轮 `TextButton` 命名的是同一件事，只是在另一个环上。
所以 `Button` 也拿到 `shape: "control" | "row"`。

### 两处在几乎重写整个按钮

| 调用点 | 写了什么 | 判定 |
| --- | --- | --- |
| `SettingsPage` 返回按钮 | `h-8 rounded-sm border-0 bg-transparent px-2 font-medium text-fg-muted hover:bg-hover hover:text-fg` | 除 `h-8`/`rounded-sm`/`px-2` 外全是 `ghost` + `size="md"` 已有的 |
| `ChatErrorBoundary` 重试 | `rounded-md bg-canvas px-3 py-1 text-ui-md text-fg font-sans hover:bg-surface-2` | 除填充外全是 `soft` + `size="sm"` 已有的 |

`h-8` 是 **32px**，而控件阶梯是 22/26/30/34/40 —— 它根本不在阶梯上。
两处都改用系统的答案。

### 6 张 golden 位移：settings 侧栏整体上移 2px

diff 是 240×640 的一整条侧栏，不是按钮本身 —— 返回按钮 32px → 30px，
把它下面的一切上移了 2px。30px 是它该在的位置（阶梯上没有 32），
且仍满足 24px 目标尺寸。已重录。

### 一次并行抖动

`code blocks stay readable and expose the wrap control` 报
hover 后 opacity 仍为 0；单跑 4.5s 通过。是并行下的 hover 时序抖动，不是回归。

### 验证

| | 结果 |
| --- | --- |
| 视觉 | **650 / 650**（6 张按上述理由重录） |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | 2395 通过；4 项失败均为既有 runtime 契约项 |

### 下一轮

`Button` / `IconButton` 上已无与自身声明冲突的 override ——
剩下的全是布局 / 定位 / 动效。可以迁 StyleX 了。

---

## Round 129 — `button` + `icon-button` 迁 StyleX

最大的一次，93 个调用点。`cascade` 守卫从头到尾是主要工具 —— 它一条条报出
"某个调用点写了、而层叠丢掉了"的属性，每一条都是一个要么补档、要么删掉的决定。

### 迁移前先把两条规则搬出组件

| 规则 | 为什么不能留在组件里 | 去了哪 |
| --- | --- | --- |
| `[&_svg:not([class*='opacity-'])]:opacity-80` | 后代选择器，原子类表达不了 | `globals.css`，键在 `[data-slot="button"]` |
| `ButtonPrimitive` 的浏览器 chrome 重置 | 它是**下一环**，不是调用点；作为 utility 它和自己的消费方同权重，上一环要"压过"它才能声明一个边或一个填充 | `globals.css` 的 `@layer base`，键在 `[data-control="button"]` |

第二条是这轮最有价值的一处结构修正：`@layer base` 输给 `@layer utilities`
（未迁移消费方照旧赢）、也输给 StyleX 的无层级规则（已迁移的原子直接声明）——
**层，正是"下面"的意思**。搬完之后 `cascade` 报的 20 多条重置冲突一次清零。

### 迁移中抓到的自身错误

| 错误 | 症状 | 教训 |
| --- | --- | --- |
| 字重写成字面量 `400` / `500` | 119 张 golden 位移 | 本项目 `--fw-regular` 是 **430**，不是 400。**一个看起来正确的字面量比一个缺失的令牌更危险** |
| `color.cta` 指向填充色而非文字色 | primary 按钮文字与底色同色 | 令牌命名要说清它是墨还是面：填充属于 `surface`，墨属于 `color` |
| `bare` 用 `padding: 0` 抵消 size 的 `paddingInline` | 无盒按钮仍带 7px 内距 | **物理简写与逻辑属性展开成不同 longhand，不会互相去重** |
| 临时备份用了 basename | `atoms/button.tsx` 被 `primitives/button.tsx` 覆盖 | 两个同名文件的 `/tmp/$(basename)` 会互相覆盖；已按记录重写恢复 |

### 迁移中暴露的既有问题

- **`.agent-composer-footer[data-measuring] > * { flex-shrink: 0 }` 压不住 StyleX。**
  那条规则自己的注释就写着"已经收缩过的 flex 行会测出不溢出"—— 而它现在正是
  这样失效的：chip 的 `flex-shrink` 带 `:not(#\#)` 特异性。加了 `!important`，
  这是全文件唯一一处，理由是**测量态只持续一次重排、必须压过一切**。
- **`data-chrome-focus` 此前只在调用点被兑现。** 那是设计系统"用行状态代替焦点环"
  的标记，所以按钮该自己认它 —— `":is([data-chrome-focus]):focus-visible"`。
- **`foundation` 展柜 fixture 自己用裸 class 画了个 CTA 按钮。** 展柜更该用系统的词汇。
- **一处单测在断言 `leading-[max(1rem,1.2em)]` 这个 class 名。** 那是 AGENTS.md
  明禁冻结的实现细节：行高搬进了 `bare` 档之后它就失败，而那一行像素完全没动。
  测试名字说的是"顺序"，留下顺序断言，删掉两条被冻结的 utility 断言。

### 一个新的组合入口（不是逃生口）

`ui/agent` 要在原子之上叠自己的形状（agent 行的密度高度与内距）。给 `Button`
加了 `styles`，接 **StyleX 样式**而不是 class：它在同一次 `stylex.props()` 里组合，
所以它声明的属性**替换**组件的、而不是输给它 —— 这正是 `className` 做不到的。
业务调用点要形状仍然是加一档。

### 又一批补齐的档

`wash`（把 `danger` 里编进名字的色调抽出来）/ `fill` / `face` / `active` /
`shape="row"`，以及 `navigation-row` 的 `look: row | quiet | search`。

### 验证

| | 结果 |
| --- | --- |
| 视觉 | **650 / 650**（13 张按上述理由重录；两张确认为并行抖动，已回退） |
| 守卫 | 17 项 `check:*` 全绿（`cascade` 从 20+ 条冲突到 0） |
| 单测 | 2395 通过；4 项失败均为既有 runtime 契约项 |

---

## Round 130 — 五个独立件，与 `SelectTrigger` 缺的那一档

### 证据

`button` 迁完后 `ui/atoms` 剩 14 个未迁。审计全部调用点：

| atom | 调用点 | 带 `className` 的 | 内容 |
| --- | --- | --- | --- |
| `ProviderIcon` | 9 | **0** | — |
| `HiddenFileInput` | 1 | 0 | — |
| `PillButton` | 27 | 1 | `mt-0.5 font-semibold` |
| `Sparkline` | 1 | 1 | `shrink-0 text-fg-faint` |
| `SelectTrigger` | 3 | **3** | 全部是 `min-w-[var(--select-min-width)]` |

`SelectTrigger` 的三处写的是同一件事：**一个 select 触发器有它的最小宽度**。
那个变量本来就叫 `--select-min-width`，却由三个调用点各自去用它 ——
这一档属于原子。

### 本轮不做

`choice-list`（142 行）/ `data-view`（84）/ `vertical-tabs`（77）留下一轮；
`Pressable` 本身不声明任何样式（10 行，只是 `ButtonPrimitive` 的一层），
所以它没有可迁的东西 —— 它 24 个调用点里 23 个带 className 正是因为如此，
那些 class 不与任何东西冲突。

### 验收标准

三处 `min-w-[…]` 从调用点消失；golden 零位移；17 项 `check:*` 全绿。

### 又两条搬出组件的规则

`provider-icon` 用 `[&>svg]:size-full` 让第三方品牌标记填满图标格。
子选择器，原子类表达不了 —— 和上一轮按钮字形那条同类，进 `globals.css`，
键在 `[data-slot="provider-mark"]`。第三方 glyph 的 `size` prop 我们
故意传 `0`，让盒子决定，所以这条规则是必需的而不是装饰。

### 两处我自己引入的偏差

| 错误 | 症状 | 教训 |
| --- | --- | --- |
| `PillButton` 的 `letterSpacing` 写在 base 里 | 被 type 步进的字距盖掉 | Tailwind 的 `tracking-normal` 通过 `--tw-tracking` 间接变量**总是**赢，与顺序无关；StyleX 里顺序是唯一裁判 |
| `SelectTrigger` 多加了一条 `lineHeight: leading.tight` | select 矮了 3px，把整个外观面板往上顶 | 迁移是复刻，不是顺手补齐 —— **原文没有的东西不要加** |

### 验证

| | 结果 |
| --- | --- |
| 视觉 | **650 / 650，零位移** |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | 2395 通过；4 项失败均为既有 runtime 契约项 |

### 下一轮方向

`ui/atoms` 剩 9 个：`choice-list`（142）/ `data-view`（84）/ `vertical-tabs`（77）/
`system-message`（64）/ `resize-handle`（213）/ `scroll-area` / `glyph-swap` /
`external-link` / `pressable`（后者不声明样式，无可迁）。

---

## Round 131 — `system-message` / `scroll-area` / `vertical-tabs`

### 先修正上一轮的清单

"未迁 14 个"里有四个根本**不声明任何样式**，只是组合已迁移的原子或绑定
globals.css 的机制：

| 文件 | 它做什么 |
| --- | --- |
| `data-view` | 按 loading / unsupported / error / empty 分支选一个已迁原子 |
| `external-link` | 给 `AnchorPrimitive` 加 `target` + `rel` |
| `pressable` | `ButtonPrimitive` 的一层 |
| `glyph-swap` | 挂 `t-icon-swap` / `t-icon`，机制是 globals.css 里的兄弟选择器 |

它们没有 stylex import 是**正确的**，不是待办。真正剩下有样式的是五个。

### 一处缺档

`SystemMessage` 两个调用点：`FloatingComposer` 只加 `text-pretty`（文案，保留），
`CwdMissingBanner` 加 `items-start px-3 py-2.5` —— 它装的是一个字段加两个按钮，
不是一句话。横条对齐方式与纵向内距都跟着"装的是什么"变，这是第二种形状。

### 一个会静默吃掉 class 的写法

```tsx
<div className="pane-split" {...stylex.props(styles.rail)}>   // ← pane-split 消失
<div {...rail} className={cn(rail.className, "pane-split")}>  // ← 两者都在
```

JSX 后写的属性覆盖先写的，而 `stylex.props()` 返回的对象里**就有 `className`**。
设置面板的整条导轨因此丢了它投向内容区的接缝（`.pane-split` 是键在
`data-split-side` 上的 globals 机制），4 张 golden 位移。

已用一段脚本扫过全部 `src/**/*.tsx`：同一个 JSX 标签里 `className=` 出现在
`{...stylex.props` 之前的，只有这一处。

### 验证方式的调整

改为**先 `--grep` 只跑受影响的 spec、全量只在提交前跑一次**，
单测也只跑受影响的目录（`src/ui` + 相关插件）。全量视觉一轮 10–15 分钟，
把它当迭代循环用是浪费。

### 验证

| | 结果 |
| --- | --- |
| 视觉 | **650 / 650，零位移**（1 张确认为并行抖动，单跑 20.7s 通过） |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | 受影响范围 82 项全通过 |

### 下一轮方向

`ui/atoms` 只剩 `choice-list`（142 行）与 `resize-handle`（213 行）两个有样式的。

---

## Round 132 — `ui/atoms` 收尾

### `choice-list`：祖先态其实是多余的

它用 `group-data-[checked]/choice:` 让标记跟着行的选中态变色。但**这件事
组件自己就知道** —— Base UI 把 `checked` 作为渲染状态传进 `className` 函数，
而 `selected` 本来就是 `ChoiceOption` 的一个 prop。

所以这里不需要第 123 轮那套自定义属性通道：CSS 里绕一圈表达的东西，
React 侧是现成的。祖先选择器有时只是"没想到还能这么写"的痕迹。

### 三个混写文件

`catalog-picker` / `chip` / `shiki-code-block` 此前是 StyleX + Tailwind 并用 ——
违反 CLAUDE.md §4「同一个组件里不得两者并用」。本轮收掉后两个。

`shiki-code-block` 留下两个 class：`shiki-block` / `shiki-body`。
那是**高亮器自己的钩子** —— Shiki 写 token span，`globals.css` 往下若干层去染它们。
它们不是这个组件的样式，是它与另一个系统的接口。

### 三个不声明样式的行为组件

`resize-handle`（213 行拖拽与键盘）/ `pressable` / `external-link` ——
连同上一轮认出的 `data-view` / `glyph-swap`，共五个。它们没有样式可迁。

### 验证方式

按你的要求改成：**`--grep` 只跑受影响的 spec**（question 25 项 23s，
code/markdown/chip/composer 29 项 45s），全量只在提交前跑一次。

### 验证

| | 结果 |
| --- | --- |
| 视觉 | **650 / 650，零位移**（1 张确认抖动，单跑 7.5s 通过） |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | `src/ui` 50 项通过 |

### 下一轮方向

`ui/atoms` 只剩 `catalog-picker`（345 行、18 处 class 串）一个混写文件。
之后是 `ui/agent`（9 个文件）与业务层。

---

## Round 133 — `catalog-picker`，`ui/atoms` 收官

345 行、18 处 class 串，是最后一个混写文件。两种弹出形态各有一套：
一栏的目录（一个滚动区读完）与两栏的目录（左侧固定导轨 + 右侧列表）。

迁移中把三处"字符串里藏着的决定"写成了注释：

- **搜索框有两种边界**。一栏弹层里的搜索框自己带 field 的边框（弹层没有框），
  两栏弹层里的只在下面画一条线 —— 弹层本身有框，再套一个盒子会读成嵌套面板。
- **行有两个高度档**。带描述的行是两行，需要自己的纵向内距；不带的是一行。
- **导轨宽度是固定的 132px**。分组是稳定集合，让导轨随最长标签伸缩会使
  右侧列表在筛选之间跳动。

### `ui/atoms` 的终态

| | 数量 |
| --- | --- |
| 用 StyleX | **44** |
| 不声明样式（行为 / 组合 / globals 机制） | 5 |
| 仍写 Tailwind | **0** |

保留的 class 只有三类，都不是"这个组件的样式"：
`shiki-block` / `shiki-body`（高亮器的钩子）、`panel-scroll`（滚动条材质，
`::-webkit-scrollbar-*` 若干层深）、`pane-split` / `t-icon-swap` 等
globals.css 机制的绑定键。

### 验证

| | 结果 |
| --- | --- |
| 受影响 spec | model picker / catalog / finder / commands / provider **31 项通过（37s）** |
| 视觉全量 | **650 / 650**（3 张确认为并行抖动，单跑 16 项 18.6s 全过） |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | `src/ui` 50 项通过 |

### 下一轮方向

`ui/agent`（9 个文件）与业务层。

---

## Round 134 — `ui/agent` 的薄壳层

九个文件里六个是薄壳：布局迁 StyleX，globals.css 的机制键保留为 class。
保留的每一处都写了理由 —— `agent-composer-glass`（composer 的材质）、
`agent-composer-footer`（测量键，`globals.css` 靠它把 chip 按住一次重排）、
`agent-overflow-label` / `truncate-fade` / `agent-overflow-track`（遮罩与跑马轨）、
`agent-workspace-view` / `agent-view-navigator` / `pane-split` / `agent-shell`。

### `AgentStatusPill` 自己画了一遍 `StatusDot`

```
running → bg-accent   |  warning → bg-warning
success → bg-success  |  neutral → bg-fg-faint   + animate-pulse-dot
```

这四档就是 `StatusDot` 的音阶，写在另一个文件里。一个说"running"的 pill
和一个说"running"的点必须长得一样 —— 那正是设计系统的意义。
所以 pill 现在**组合** `StatusDot`，色调用共享的 `DotTone`
（`neutral → idle`、`warning → waiting`）。

**一处我先判错、查证后更正**：我 grep 完 `src/` 说"另三档从未被用到"。
漏了 `visual/` —— 两个 fixture 正在用 `neutral` 与 `warning`。
它们不是死档，是只有 fixture 在拍。已按共享词汇改名。

8 张 golden 位移，全是含 running 状态点的画面：点现在带上了
`--shadow-live-glow`（88 / 127 像素的淡蓝辉光）。那是设计系统对
"正在发生"的答案，此前只有 `StatusDot` 用它、pill 没有。已重录。

### 验证

| | 结果 |
| --- | --- |
| 受影响 spec | foundation / shell / dock-catalog / composer **36 项通过（25s）** |
| 视觉全量 | **650 / 650**（8 张按上述理由重录，1 张抖动单跑 5.1s 通过） |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | `src/ui` 50 项通过 |

### 下一轮方向

`ui/agent` 剩三个大文件：`context-dock`（270）/ `activity-disclosure`（196）/
`navigation-row`（161）—— 它们已部分迁移，剩的是混写。

---

## Round 135 — `ui/agent` 的三个大文件

`navigation-row`（161）/ `activity-disclosure`（196）/ `context-dock`（270）——
它们此前是 StyleX 与 Tailwind 混写。迁完后 `ui/agent` 保留的 class 只有
globals.css 的机制键与那一处有记录的 Chromium 阻塞
（`group-focus-visible/activity-trigger`，`:has(:focus-visible)` 不触发失效）。

### 一处我漏掉的分支

`activity-disclosure` 的字形格原本是
`shell === "line" ? "h-4 w-4" : "w-5"`，我只把 card 的宽度搬了过去，
line 的 `w-4` 漏了 —— 格子退回按内容收缩，把整行的文字往左拉。
1000+ 像素、12 张 golden。补上 `markLine` 后 32 项一次通过。

**教训**：一个三元里两个分支各带两个属性时，很容易只看见后一支。
迁移时把原表达式抄在旁边逐项勾掉，比凭记忆重写安全。

### 四处单测在冻结 class 名

`activity-disclosure.test.tsx` 里读 `bg-card` / `bg-surface-2` / `w-4` /
`opacity-0`。其中一条的注释自己写着：

> Asserted on the material rather than on a class name

—— 但它读的正是 class 名。Tailwind 的类名可读，所以"读 class"看起来像在
读材质；StyleX 一来这层错觉就没了。

治本不是把断言改成读生成的类名，而是**让组件把自己的决定说出来**：
字形格加 `data-framed` / `data-tone`，chevron 加 `data-open`。
它们本来就在驱动样式，现在也是可断言的契约 —— 生成的类名不是契约。

### chevron 的 opacity 被三层规则同时争夺

迁完第一版，31 项失败，其中有行为测试 —— 不是颜色偏差。
`cascade` 之外的两条测试直接点出了根因：

- **触屏回退失效**：`[data-reveal="hover"] { opacity: 1 }` 在
  `@media (hover: none)` 里，普通特异性，压不过生成规则。
- **hover / focus-visible 显形失效**：`group-focus-visible/activity-trigger:opacity-100`
  与 `group-hover/activity-header:opacity-100` 同样压不过。

chevron 的 opacity 此前由**四处**决定：base 的 `opacity-0`、两条 group 规则、
一条全局媒体查询。StyleX 把它收进一处之后，另外三处全部失效。

**治本发现的一件事**：chevron 就在 trigger **内部** ——
所以它根本不需要那条有记录的 `:has(:focus-visible)` 变通。两个发布者、一个读者：

```
header  → --chevron: 0，:hover 时 1     （悬停旁边的动作区也要显形）
trigger → 无 default，:focus-visible 时 1（无 default，所以平时继承 header 的）
chevron → opacity: var(--chevron, 1)，@media (hover: none) 下为 1
```

两个 `group/` 标记因此一起消失，label 与 detail 的 hover 提墨也走同一个通道
（`--row-ink`）。**祖先态在 StyleX 下不是限制，是把"谁决定什么"问清楚的机会。**

### 又一次同样的写法错误

`<span className="truncate-fade" {...stylex.props(...)}>` —— 第 131 轮记过的
那个坑，我这轮自己又踩了两次（`navigation-row` 的 label 与 detail、
`shiki-code-block` 的 fallback）。`truncate-fade` 是裁剪遮罩，丢了它文字就把
整行撑宽 —— 只在 **es 语言 + 18px 字号 + 1120px 窗口**三者同时出现时越界，
正是那条闭环测试存在的理由。

已再扫一遍全部 `src/**/*.tsx`，同类 0 处。

### 一处默认值有三分之一的调用点不同意

`activity-disclosure` 的正文纵向内距原本由 atom 给（`pt-1.5 pb-1.5`），
而 6 个调用点里 2 个用 `pt-*` / `pb-*` 覆盖 —— 靠 tailwind-merge 的后来者优先。
StyleX 下覆盖不掉。

按第 108 轮 `SectionLabel` 的先例办：**一个三分之一调用点不同意的默认值不是默认值**。
atom 只留左右内距，纵向由每个调用点自己说 —— 材料需要多少上下空间，
取决于材料是什么（推理引文比问题选项贴得更紧）。

### 验证

| | 结果 |
| --- | --- |
| 视觉全量 | **650 / 650，零位移** |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | `src/ui` 50 项通过 |

### `ui/agent` 终态

15 个文件全部迁完。保留的 class 只有 globals.css 的机制键。

---

## Round 136 —— 菜单不拥有"菜单是什么"

### 计划

进业务层（`src/plugins/**`，263 个文件）。先扫全层重复的整串 class，找**库里缺的档**
而不是逐文件翻译 —— 业务层的 class 大多是布局噪音（`flex flex-col`、`truncate`），
真信号只有两处，而且都指向同一个 atom。

### 证据

```
9  min-w-[var(--menu-min-width)]        ← 13 个业务点 + context-dock 的 styles.menu
4  max-h-[…] overflow-y-auto            ← 3 个不同值：min(60vh,380px) / 320px / 280px
```

`--menu-min-width` 在 `globals.css` 里声明**一次**，被读 14 次，其中 13 次在业务层。
本日志第 5625 行记着它的来历：某轮把 **9 个裸值（160–248）统一成了这一个变量**。

### 根因

那一轮做对了一半 —— **值统一了，归属没有**。一个只为"让调用点共享一个数字"
而存在的变量，恰恰证明这个数字的主人不是调用点。菜单的最小宽度不是调用点的决定，
它就是菜单本身；写 14 遍之后它依然可以被第 15 个调用点写成别的。

滚动那一档更糟：atom 从没提供它，于是 4 个调用点各自猜了个数。
**其中只有一个是视口感知的** —— `max-h-[280px]` 在矮窗口会被屏幕裁掉，
菜单底部的项点不到。这不是不一致，是 bug 类。

而正解设计系统本来就有：`catalog-picker` 这个 atom 已经在用
`min(420px, var(--available-height))` —— Base UI 的 Positioner 会**量出**锚点到
视口边缘的真实空间并发布成变量。`60vh` 只是对它的一次估算，另外三个连估都没估。

### 做法

1. `menuStyles.content` 无条件拥有 `minWidth`，`--menu-min-width` 从 `globals.css` 删除
   —— 它唯一的职责是被 14 处拼写，主人到位后这个职责就不存在了。
2. 滚动上限同样**无条件**：`min(380px, var(--available-height))` + `overflowY: auto`。
   不做成 prop —— 「菜单不该超出为它量好的空间」没有第二种答案，
   而 `max-height` 对本来就矮的菜单无副作用。380px 取四个值里最大的那个。
3. 顺带补两处键盘可达性：`overscrollBehavior: contain`（菜单滚到底不接着滚页面）、
   `scrollPaddingBlock`（高亮项被键盘带进视野时不贴边）—— 后者 `catalog-picker`
   的 list 也有，同一个理由。

### 验收

14 处 `min-w` 与 4 处 `max-h/overflow` 全部消失；其中 6 个调用点的 `className`
整个消失（它只装着这一个值）。10 个此前无上限的菜单获得上限。

### 顺带撞出一条主干上的坏测试 —— 以及我自己验证方法的漏洞

把单测范围从 `src/ui` 扩到消费方之后，`ModelPicker.test.tsx` 立刻红了 ——
**而且在 HEAD 上就是红的**。断言是 `expect(list.parentElement!.className).toContain("h-[240px]")`，
在 `catalog-picker` 迁到 StyleX 那一轮就失效了，只是我当时把单测范围限在了 `src/ui`。

**方法教训**：迁一个 atom，受影响的测试不只在 atom 自己的目录里。
断言住在**消费方**的测试文件里 —— 那正是我一直没跑的那部分。

治本不是把 `h-[240px]` 换成生成的 hash。这条不变量（"目录体保持一个不动的尺度，
否则弹层会随分组变化往上爬"）**完全是几何的，而 jsdom 不加载 CSS** ——
它在那里永远不可能真的失败，只能靠冻结一个类名假装守住。
搬到有 CSS 的地方：fixture 里那个只有一行的分组恰好是最好的证据 ——
没有那个尺度，这个体就只有一行高。

顺带记一条两次踩到的：浮层入场时 `scale(0.97)`，
`getBoundingClientRect()` 会把 240 读成 233、192 读成 186。**量布局要用 computed style。**

### 验证

| | 结果 |
| --- | --- |
| 视觉全量 | **652 / 652**（650 + 两条新闭环测试），零位移 |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | 127 文件 / 656 项通过（`src/ui` + settings + chat） |

| | 之前 | 之后 |
| --- | --- | --- |
| 菜单最小宽度 | 14 处拼写 + 1 个只为共享而存在的变量 | atom 无条件拥有，变量删除 |
| 菜单滚动上限 | 4 处、3 个值，1 个视口感知 | atom 无条件拥有，读 `--available-height` |
| 无上限的菜单 | 10 个 | 0 个 |
| 只装着一个值的 `className` | 6 处 | 0 处 |

### 下一轮的材料（已勘察，未动）

- **`active:` 有两套按下机制**：`send.tsx` 的 `NUDGE = "active:translate-y-[0.5px]"`
  是**有理由的真设计**（"实心圆缩小读起来像 bug，半像素下沉读起来像按下"）——
  但这个理由适用于任何实心圆按钮，它却以字符串常量住在业务文件里。
  正确形状是 `Button` 拥有第二档：`press="nudge"`。
  而 `JumpToBottomButton` 那行 `active:translate-y-0 active:scale-[var(--press-scale)]`
  是在**重述 atom 已经做的事**，外加一个永远不可能触发的 `translate-y-0`
  （按钮不可见时 `pointer-events-none`）。
- **`hover:bg-surface-3`**（`McpRow`）：行状态用了 surface 台阶而不是 ink wash，
  直接违反 DESIGN.md §5。
- **业务层 35 处 `hover:bg-hover`**：token 是对的，问题是"一行有 hover"这件事
  被 35 个地方各自声明。

---

## Round 137 —— 按下有两套机制，其中一套住在业务层

### 证据

```
press?: boolean                          ← 一个布尔，12 个调用点关掉它
const NUDGE = "active:translate-y-[0.5px]"  ← send.tsx，业务层的一个字符串常量
"active:translate-y-0 active:scale-[var(--press-scale)]"  ← JumpToBottomButton
```

把 12 个 `press={false}` 按调用点已经声明的身份归类：

| 身份 | 关掉按下 | 保留 |
| --- | --- | --- |
| `chip` | 3 | 0 |
| `shape="row"` | 1 | 0 |
| `variant="link"` | 1 | 0 |
| `variant="bare"` | 1 | 0 |
| `press="nudge"` 想要的（send） | 2 | — |
| 其余（outline / ghost 宽触发器） | 4 | — |

`shape="row"` 另有 3 处看着像反例 —— 其实是 **`TextButton`，另一个组件**，
它根本没有 `press` prop。所以在 `Button` 的调用点里，四种身份全是 100%。

### 根因

**"按下如何被回应"是控件盒子的属性，不是调用点的开关。** 没有盒子的东西
（`link` / `bare` 是一段文字）没有可缩放的对象；一个整行宽的盒子缩 2% 是
两边各动 5px，读起来像布局在呼吸；一个密排 pill 缩 2% 读不出来。
四种身份各自 100% 关掉它，不是四次巧合。

而 `send.tsx` 的注释写着真正的设计："实心圆缩小读起来像 bug，半像素下沉读起来像按下"
—— 这条理由对**任何实心圆按钮**都成立，却以 `press={false}` + 一个业务层字符串常量的
形式存在。同一个事实（按下的回应）于是有了两套机制，一套在 atom 里、一套在插件里。

`JumpToBottomButton` 那行则是在**重述 atom 已经做的事**（`active:scale-[var(--press-scale)]`
就是 `styles.press`），外加一个 `active:translate-y-0` —— 它永远不可能触发，
因为按钮不可见时是 `pointer-events-none`，可见时 `translate-y` 已经是 0。

（写到这儿我一度以为"两个实心圆按钮意见相反"，还把这句写进了 atom 注释。
查了 variant 才发现 `JumpToBottomButton` 是 **`raised`**（canvas 抬起），不是
`primary`（饱和 CTA 盘）—— 我的正则把两者一起匹配了。它拿到的本来就是默认 `scale`，
所以删掉那行是零变化。**不是分歧，是两种不同的毛病。** 注释已改。）

顺带一处同一主题的分歧：`PillButton` 处理了 `:disabled` 的 cursor 与 opacity，
**却没守住按下缩放** —— 禁用的 pill 被点击时仍然会缩。`Button` 守了
（`":is(:disabled):active": 1`）。同一个事实两个主人，其中一个漏了。

### 做法

1. `press?: boolean` → `press?: "scale" | "nudge" | "none"`（AGENTS.md：用封闭命名值，
   不用原始哨兵）。默认由身份推出：`link` / `bare` / `chip` / `shape="row"` 为 `none`，
   其余 `scale`。6 个调用点因此不再需要说话，调用点仍可显式覆盖。
2. `nudge` 进 atom：`translate` 在 `:active` 时 `0 0.5px`，同样不响应禁用态。
   `send.tsx` 的字符串常量与 `press={false}` 一起删除。
3. `JumpToBottomButton` 删掉重述的那一行。
4. `PillButton` 补上禁用守卫。
5. ~~`McpRow` 的 `group-hover:bg-surface-3`~~ —— **这条我判断错了，撤销**。
   细看之后：那一行的状态本来就是 `hover:bg-hover`（正确的 ink wash），
   `surface-2 → surface-3` 是**行内一块字形底板自己的填充**在随行提亮，
   不是行状态。surface 台阶用在 surface 上没有问题。
   grep 出一个 class 名就断定它违规，跳过了"它是谁的属性"这一步。

### 验收

| | 之前 | 之后 |
| --- | --- | --- |
| 按下机制 | 2 套（atom 一套、插件一个字符串常量一套） | 1 套，两档命名值 |
| `press` 类型 | `boolean` | `"scale" \| "nudge" \| "none"` |
| 说 `press` 的调用点 | 12 | 4 |
| 重述 atom 默认值的手写类 | 1 处（含 1 条永不触发的规则） | 0 |
| 禁用时仍会缩的控件 | `PillButton` | 无 |

| | 结果 |
| --- | --- |
| 视觉全量 | **652 / 652**，零位移（按下只在 `:active`，静态 golden 本就不该动） |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | 136 文件 / 688 项通过 |

### 留给下一轮

"一个控件用不了" 有 **8 个主人、4 个值**，而且根因不只是值不齐 ——
**基础层已经拥有了其中一半，另外七处不知道**：

`@layer base` 里 `[data-control="button"]:disabled { cursor: not-allowed; opacity: 0.45 }`
—— 这个属性只由 `ui/primitives/button.tsx` 发放，所以它覆盖了
`Button` / `TextButton` / `PillButton` / `SelectTrigger` / `Pressable`（全部走
`ButtonPrimitive`）。

| 主人 | opacity | 被基础层覆盖？ | 它的 `cursor: not-allowed` |
| --- | --- | --- | --- |
| `globals.css` `@layer base` | **0.45** | —— 它就是基础层 | 是它在说 |
| `button.tsx` | 0.64 | 是 | **纯重述** |
| `text-button.tsx` | 0.5 | 是 | **纯重述** |
| `pill-button.tsx` | 0.5 | 是 | **纯重述** |
| `select-trigger.tsx` | 0.5 | 是（走 `Pressable`） | **纯重述** |
| `text-field.tsx` | 0.6 | 否（`<input>`） | 需要 |
| `switch.tsx` | 0.5 | 否（Base UI switch） | 需要 |
| `choice-list.tsx` | 0.64 | 否（checkbox / radio） | 需要 |

`button` 的 `0.25` 不在此列 —— `off="faded"` 是一个**命名档**，说的是"退到背景里"，
不是"用不了"。它的存在恰好说明禁用档不必兼任"几乎看不见"。

做法：一个 `--control-disabled-opacity` 住在 `globals.css`（可换肤），
基础层与三个非 button 的 atom 都读它；四处纯重述的 `cursor` 删掉。
值取 **0.5** —— 四个 atom 已经这么说，且它离基础层的 0.45 最近；
两个 0.64 的读数偏轻，容易和 muted 墨色混淆，而"更淡"这一档已经由 `faded` 占着。
（这一档会动到每一个禁用控件的观感，是一个需要用户过目的设计决定。）

---

## Round 138 —— "这个控件用不了" 有八个主人

计划与证据见上一轮末尾那张表（8 个主人 / 4 个值 / 基础层已拥有其中一半）。
实施时又量出一件计划里没有的事。

### 实测：一个调用点的两个意图都被静默丢弃

`GoalStatusSurface` 在一个 `variant="bare"` 的 Button 上写着
`className="disabled:cursor-default disabled:opacity-100"` ——
目标摘要在不可编辑时仍是**内容**，该照常读得清。

把生成的 CSS 按图层列出来：

```
[utilities]  .disabled\:opacity-100:disabled { opacity: 1 }
[base]       [data-control="button"]:disabled { cursor: not-allowed; opacity: 0.45 }
[«unlayered»] .x16jh8n:disabled:not(#\#):not(#\#):not(#\#) { opacity: 0.64 }
```

无图层规则胜过任何 layer，所以 `button.tsx` 的 0.64 同时压过了基础层**和调用点**。
实测禁用后 `opacity: 0.64`、`cursor: not-allowed` —— **两条都没生效**。

（第一次探针读到的是 `1`，我差点当成"调用点赢了"。原因是我在同一个
`evaluate` 里刚设完 `disabled` 就读 —— `button.tsx` 会过渡 `opacity`，
那一瞬间的计算值还是旧的。**量一个带过渡的属性，必须先让浏览器重算。**）

`globals.css` 第 553 行的注释里，前面某轮已经记过同一类事故：
"Unlayered rules outrank `@layer utilities`, so this block silently defeated three
intents"。这次是同一个根因在另一处 —— 而它恰好证明了基础层注释里写的设计意图：
下面那一环声明默认值，上面的调用点才可以覆盖。**atom 不该重述它。**

### 做法

一个 `--control-disabled-opacity` 住在 `globals.css`（与 `--press-scale` 并列）。
不进换肤 spec —— 那张表里 `pressScale` 属于 `VisualStyleMotion`，而"用不了"不是运动，
没有任何视觉风格要求过调这一档。

- 基础层读它；四个走 `ButtonPrimitive` 的 atom（`button` / `text-button` /
  `pill-button` / `select-trigger`）把 `opacity` 与 `cursor` **一起删掉** ——
  基础层已经替它们说了，重述只会把调用点的覆盖挡在外面。
- 三个不是 button 元素的（`text-field` 的 `<input>`、`switch`、`choice-list`）
  自己保留 `cursor`，但读同一个变量。
- 值：先按"八个主人里四个这么说"取了 **0.5**，**做完量了一下，推翻了**。见下。

### 有了量具之后，两个判断都变了

改完跑全量，50 项失败。逐一归因时发现 dock 的每一张 golden 都只差 **47 像素** ——
定位到唯一的禁用控件：composer 的 Send。于是把它的合成对比度算出来
（背景、盘面、墨色都用 canvas 解析真实 RGB，再按 α 合成）：

| α | 浅色 | 深色 |
| --- | --- | --- |
| 1.00 | **4.86** | **3.99** |
| 0.64 | 2.44 | 2.54 |
| 0.50 | 1.94 | 2.07 |

**第一个反转：`primary` 根本不该被淡化。** 它在禁用时已经换成中性盘 + faint 墨
——它自己已经把"不能用"说完了，4.86:1。基础层的淡化是**给没有自己答案的控件的
通用答案**，叠在它上面就是同一句话说两遍，把字形从 4.9:1 压到 1.9:1。
所以 `primary` 显式 `opacity: 1`（无图层，压过基础层），一个控件一种说法。

再量一个普通禁用按钮（settings 的 Test，透明盘 + 正常墨）：

| α | 浅色 | 深色 |
| --- | --- | --- |
| 0.64 | **3.61** | **4.36** |
| 0.60 | 3.27 | 4.02 |
| 0.50 | **2.59** ✗ | 3.25 |
| 0.45 | **2.32** ✗ | **2.91** ✗ |

**第二个反转：通用档取 0.64，不是 0.5。** 0.5 会让浅色下的禁用标签掉到 3:1 以下，
基础层原本的 0.45 两个主题都不合格。我先前那个理由是在**数拼写**（八个主人里
四个说 0.5），不是在数可读性 —— 而禁用控件恰恰需要被读，用户是靠读它才知道
要做什么才能让它可用。"几乎看不见"那一档已经由 `off="faded"`（0.25）占着。

### 验收

| | 之前 | 之后 |
| --- | --- | --- |
| "用不了"的主人 | 8 个 | 1 个（`--control-disabled-opacity`） |
| 值 | `0.45` / `0.5` ×4 / `0.6` / `0.64` ×2 | `0.64` |
| 纯重述的 `cursor: not-allowed` | 4 处 | 0 |
| 被静默丢弃的调用点意图 | 2 条（`disabled:opacity-100` / `disabled:cursor-default`） | 0 |
| 禁用 Send 的字形对比度 | 2.44:1（浅） | **4.86:1** |
| 禁用普通按钮的标签对比度 | 3.61:1（浅，仅 Button）／2.32:1（走基础层的） | **3.61:1（全部）** |

| | 结果 |
| --- | --- |
| 视觉全量 | **652 / 652**（重录 47 张，全部可归因） |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | 1780 项通过 |

### 方法记录

这一轮第一次把**量具**用在了设计判断上，而不是只用来找位移 ——
两个结论都被数据推翻了。此前几轮我判断"哪个值对"靠的是数拼写
（多少个调用点这么写），这轮才发现那是在数**历史**，不是在数**质量**。

（另有一次探针失误：设完 `disabled` 就在同一个 `evaluate` 里读 `opacity`，
读到的是过渡途中的旧值 `1`，差点当成"调用点赢了"。量带过渡的属性要先让浏览器重算。
另外 50 项失败里有 15 次是机器负载导致的超时 flake，安静重跑全过 ——
**归因之前先分离**，否则会去重录一张不该动的 golden。）

### 留给下一轮

业务层 902 处 className 里，35 处是"一行有 hover"，每一处的 grid 模板都不同、
但骨架一致。其中 **`transition-colors` 写了 18 次、漏了 5 次** ——
那 5 行的高亮是瞬变的，其余是淡入。另外"可悬停的行用什么圆角"有 4 个答案
（`rounded-2xs` / `rounded-xs` / `rounded-md` / 无）。
`floating-surface.tsx` 的先例说明这可以不迁文件就修：导出一个 StyleX 样式，
仍在用 Tailwind 的调用点通过 `stylex.props()` 消费它。

---

## Round 139 —— 用户的动效设置只搬动了半个界面

### 起点是行 hover，落点不在那里

按计划去修"35 处各自声明的行 hover"，先把数据取全：
**8 行没有任何过渡**（瞬变，其余 27 行淡入），圆角有 **6 个答案**。
但顺着"这些行的过渡从哪来"往下查，撞到了一个更深的东西。

`option-row.tsx` 与 `text-field.tsx` 这两个 atom 把时长和缓动**写死**成
`0.15s` + `cubic-bezier(0.4, 0, 0.2, 1)` —— 那正是 **Tailwind 的默认过渡**，
是迁移时按数值复现下来的。而项目自己有运动令牌，并且令牌带一个 `--motion-scale` 乘数。

`--motion-scale` 是 Appearance 面板里的用户设置，**四档**：关 (0) / 快 (0.6) /
默认 (1) / 慢 (1.5)。关档有一条 `:root[data-motion="off"]` 的全局 `!important`
兜底，所以"关"是好的 —— 但中间两档没有兜底。实测（`?motion=full` + 手动改乘数）：

| 动效档 | Tailwind `transition-colors` | Button（走令牌） | TextArea（写死） |
| --- | --- | --- | --- |
| 默认 (1) | 0.15s | 0.15s | 0.15s |
| **快 (0.6)** | **0.15s** | 0.09s | **0.15s** |
| **慢 (1.5)** | **0.15s** | 0.225s | **0.15s** |

缓动同样分裂：

```
tailwind transition-colors → cubic-bezier(0.4, 0, 0.2, 1)
--ease-out                 → cubic-bezier(0.22, 1, 0.36, 1)
Button                     → cubic-bezier(0.22, 1, 0.36, 1)
TextArea（写死）            → cubic-bezier(0.4, 0, 0.2, 1)
```

### 根因

**Tailwind 的默认过渡从来没有接到设计系统上。** `@theme` 里配置了字体和颜色，
唯独没配 `--default-transition-duration` / `--default-transition-timing-function`，
所以每一个 `transition-*` 工具类都在用 Tailwind 出厂的 150ms 和出厂的曲线 ——
既听不见用户的动效设置，也不在设计系统那条缓动上。

这不是"业务层没迁完"的症状。**迁移不是修它的手段** —— 就算把 263 个文件全迁到
StyleX，问题也只是被逐个绕过；而不迁，一行配置就能让所有消费者一起对齐。

### 做法

1. `@theme inline` 补两行：`--default-transition-duration: var(--dur-fast)`、
   `--default-transition-timing-function: var(--ease-out)`。
   取 `--dur-fast`（150ms）而不是 `--dur-color`（100ms）—— **它复现今天的时长**，
   这一轮只修缺陷（跟随乘数、走对曲线），不顺手改速度。
2. 两个 atom 的写死值换成令牌。按已有惯例（9 个文件）：
   **纯颜色列表用 `motion.color`，含几何的用 `motion.fast`** ——
   这两个都是纯颜色，归 `motion.color`。
3. 那 8 行瞬变的 hover：补上过渡。圆角的 6 个答案单独报告，不在本轮动
   —— 全出血行没有圆角是对的，它不是分歧。

### 中途改掉了原计划

本来打算导出一个 StyleX 的"行洗色"样式给 35 个调用点消费。数完元素类型之后放弃了：
**35 处里只有 10 处是按钮元素，25 处是 `div`/`tr`/`label`** —— 基础层够不着它们，
而主题默认值一旦修好，`transition-colors` **本身就已经是设计系统的答案**了
（时长与曲线都接在令牌上）。再造一个 atom 只会让同一件事有两种写法。
一个答案、到处同一种写法，好过两套机制恰好一致。

那 8 处"瞬变"里有 2 处是误报 —— 我按**行**扫的，而 `cn()` 是多行的，
`transition-colors` 就在上一行。真实是 6 处，全部补齐。

### 量证

| 动效档 | Tailwind `transition-colors` | 输入框（原写死） |
| --- | --- | --- |
| 默认 (1) | 0.15s | 0.10s |
| 快 (0.6) | **0.09s** | **0.06s** |
| 慢 (1.5) | **0.225s** | **0.15s** |

曲线全部为 `cubic-bezier(0.22, 1, 0.36, 1)`。

| | 之前 | 之后 |
| --- | --- | --- |
| 听不见动效设置的过渡 | 所有 Tailwind 工具类 + 2 个 atom | 0 |
| 缓动曲线 | 2 条 | 1 条 |
| 无过渡的 hover 行 | 6 | 0 |

| | 结果 |
| --- | --- |
| 视觉全量 | **652 / 652**，零位移（套件在动效关档下跑，改的正是过渡本身） |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | 1780 项通过 |

### 留给下一轮

"可悬停的行用什么圆角"有 6 个答案：`md` 13、`2xs` 8、无 11、`sm` 1、`xs` 1、`full` 1。
无圆角的 11 处是全出血行，那不是分歧；真正的分歧是**内缩行**在 `md` 与 `2xs`
之间二选一（13 : 8），以及各一次的 `sm` / `xs`。

---

## Round 140 —— 密度设置只搬动了半个 dock

### 先说一个被量掉的假设

本轮原计划是修"可悬停行的圆角有 6 个答案"。**量完之后放弃了 —— 那不是缺陷。**

我的假设是"30px 的行配 10px 圆角（34%）太圆"。但设计系统自己的行
（`shape="row"` → `--row-radius` = 12.5px）实测是 **h=34 / r=12.5 = 37%**，
更矮的行到 45–47%。34% 正落在这个家族里。圆角的分布也不是随机的：
`2xs`(2px) 全部是 `px-1` 的工具预览行，`md`(10px) 全部是 `px-2`–`px-3.5` 的卡片行，
无圆角的全部是全出血行 —— **它跟着内距走，本来就是同心半径**。

差点写成一条缺陷。量具的价值不只是发现问题，也在于**否掉自己的直觉**。

### 换到一个量得出来的地方

上一轮发现"动效设置只搬动半个界面"，那类缺陷可能有兄弟。逐个 Appearance 设置查：

- `--radius-scale`：**已接通**（`--radius-md → --shape-md → calc(… * --radius-scale)`）。
  当年有人接了圆角却没接过渡 —— 这正是上一轮那个缺口能活下来的原因。
- `--motion-scale`：上一轮修完。
- **`density`：只搬动了一部分。**

量法：切换密度变量，数"有内距/间距、本可以跟随的元素"里有多少真的动了。

| 面 | 跟随密度的比例 |
| --- | --- |
| workspace 各 dock 视图 | 20–25% |
| **Settings 整个界面** | **0 / 67** |

再逐视图量内容左缘（紧凑 vs 宽松）：**12 个 dock 视图里只有 4 个会动。**

### 根因

dock 的内容栏距是一个**有名字的尺度**（`--density-column-gutter` = 12px、
`--density-column-gutter-wide` = 20px），却被写成了三种：

| 写法 | 数量 | 跟随密度 |
| --- | --- | --- |
| `px-[var(--density-column-gutter-wide)]` | 9 个视图 | ✅ |
| `px-4`（16px） | 7 个视图 | ❌ |
| `.agent-surface-header` 里的 `12px` / `20px` 字面量 | shell 表头 | ❌ |

第三行是最说明问题的：那两个字面量**恰好等于**两个密度变量在默认档的值，
连 640px 的断点都和 `columnGutter` / `columnGutterWide` 这一对严丝合缝 ——
变量本来就是为它设计的，表头只是把默认值抄了一遍。

后果不是"不好看"，是**切换 dock 标签时内容会横跳 4px**（16 ↔ 20），
以及用户调密度时半个 dock 不动。

（我一度想说"内容与自己的表头错位"，量了一下：这些视图里 `DockViewBar`
在没有 identity/sub/actions 时返回 null，根本没渲染表头。论据不成立，删掉。）

### 做法

1. `.agent-surface-header` 的两个字面量换成对应的密度变量 ——
   **默认档零位移**，从此跟随设置。
2. 7 个视图的 `px-4` → `px-[var(--density-column-gutter-wide)]`。
   默认档 16 → 20px，是真实位移；方向由多数（9 个视图）和密度词汇表自己的
   `columnGutterWide = 20` 决定。
3. **不动**代码/差异视图的 `px-3` —— 查过了，那是差异行的行号槽、文件头条
   这些**代码面内部**的内距，和面板栏距不是一回事。数值相同不等于事实相同。
4. **不动** Settings（0/67）—— 它的行内距 `px-4 py-3` 由 `SettingRow` 一个组件
   拥有，但密度词汇表里**没有 16px 这一档**。接上去等于新增一个命名尺度，
   那是设计决定，报告给用户。

### 验收

| | 之前 | 之后 |
| --- | --- | --- |
| dock 栏距的写法 | 3 种（变量 9、`px-4` 7、表头字面量） | 1 种 |
| 切换 dock 标签时的横跳 | 4px | 0 |
| 随密度呼吸的 dock 视图 | 4 / 12 | 12 / 12 |
| shell 表头随密度 | ❌ | ✅（默认档零位移） |

| | 结果 |
| --- | --- |
| 视觉全量 | **652 / 652**（closure 350 + 其余 302，分块跑），重录 12 张 |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | `src/ui` 50、`workspace` 299 通过 |

一次差点犯的错：被内存杀掉的那次重录顺手录进了
`workspace-light-dock-timeline` —— 它既不在失败清单里、`timeline.tsx` 我也没碰。
还原后单跑通过，确认是 flake。**中断过的重录必须逐张核对，不能只看最终计数。**

### 覆盖缺口（报告，不改）

`McpRow` 在 `dock-tools` 的 fixture 里渲染 **0 行**（MCP 服务器列表为空），
所以它那 5 处栏距改动**没有任何 golden 覆盖**。

### 留给下一轮

Settings 整个界面 **0/67** 不随密度。它的行内距由 `SettingRow` 一个组件拥有
（`px-4 py-3`），但密度词汇表里没有 16px/12px 这一档 ——
接上去等于新增命名尺度，是需要用户过目的设计决定。

---

## Round 141 —— 最大字号下，标题比正文还小

### 证据

继续上两轮的扫法（逐个 Appearance 设置查它触达了多少界面）。字号这一档：

`text-ui-*` / `text-prose` / `text-code` 都接在 `--fs-*` 上，随设置走。
但**编辑性字号是写死的**：

```
--text-display-sm: 18px   --text-display-md: 20px   --text-display-lg: 24px
--fs-markdown-h1: 24px    --fs-markdown-h2: 20px
--fs-markdown-h3: 17px    --fs-markdown-h5: 15px    --fs-markdown-table: 14px
```

把阶梯算出来对照：

| 基准 | ui-md | prose | display-sm | md-h3 | md-h5 |
| --- | --- | --- | --- | --- | --- |
| 11（最小） | 11 | 13 | 18 | 17 | 15 |
| 14（默认） | 14 | 16 | 18 | 17 | 15 |
| **18（最大）** | **18** | **21** | **18** | **17** | **15** |

最大档下 `display-sm` 与正文同号，而 markdown 的 h3(17) / h5(15) **比它们自己
标题下面的正文（21px）还小** —— 转录区是这个应用里最常读的一面。

### 根因

不是"没接上令牌"，而是**它们和阶梯之间没有任何关系**。
`classNames.ts` 里那份手抄的 `EDITORIAL_STEPS` 把这个决定写明了：
UI 档来自阶梯（一个主人），editorial 档手工维护（两个主人，靠守卫兜）。
"editorial = 固定锚点"在小字号端说得通，**在大字号端会倒挂** ——
层级在字号区间的两端是两种关系，而不是同一种关系被缩放。

顺带量出三处重复：默认档下

```
md-h1  = 1.714×  =  display-lg
md-h2  = 1.429×  =  display-md
md-table = 1.000× =  ui-md
```

五个 markdown 变量里有三个是已有档位的第二次拼写。

### 做法

editorial 档按它们**在默认档已有的比值**进入阶梯：

| 档 | 比值 | base 11 | base 14 | base 18 |
| --- | --- | --- | --- | --- |
| markdown-h5 | 1.071 | 12 | **15** | 19 |
| markdown-h3 | 1.214 | 13 | **17** | 22 |
| display-sm | 1.286 | 14 | **18** | 23 |
| display-md | 1.429 | 16 | **20** | 26 |
| display-lg | 1.714 | 19 | **24** | 31 |

**默认档一个像素都不动**（比值就是从默认档反推的），只有两端被修正。

- `md-h1` / `md-h2` / `md-table` 三个变量删除，改读 `display-lg` / `display-md` / `ui-md`。
- `classNames.ts` 那份手抄的 `EDITORIAL_STEPS` 随之消失 ——
  它旁边的注释早就写着"手抄的副本会在加档时静默失效"。
- `CEILING_PX`（= 最大字号 + 3 = 21px）删除：它挡不住任何今天的档
  （prose 在 base 18 恰好算出 21），却会把新的 display-lg 从 31px 压平到 21px。
  它是为 prose 一个档打的补丁；基准本来就被 clamp 在 [11, 18]，阶梯不会跑飞。

### 一个比例做不到的不变量

`base=11` 时 prose 与 markdown-h3 都舍入到 13px —— 1.14 与 1.214 在那个尺度上
不足一个像素。**标题碰到正文正是这一轮要消灭的东西**，所以它由阶梯保证
（`aboveProse` 档地板取 `prose + 1`），而不是靠"算出来碰巧不撞"。
测试因此验证的是阶梯的承诺，不是一组期望值。

### 验收

| | 之前 | 之后 |
| --- | --- | --- |
| 不随字号走的字号变量 | 8 个 | 0 |
| 重复拼写的档位 | 3 个（h1=display-lg、h2=display-md、table=ui-md） | 0 |
| 手抄的 `EDITORIAL_STEPS` | 1 份 | 0 |
| base 18 的 `display-sm` vs `prose` | **18 < 21**（倒挂） | 23 > 21 |
| base 18 的 markdown h3 vs 正文 | **17 < 21**（倒挂） | 22 > 21 |
| 默认档 14 的全部字号 | —— | **一个像素都没动** |

| | 结果 |
| --- | --- |
| 视觉全量 | **652 / 652**（closure 350 + 其余 302），重录 4 张 font-18 |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | theme + lib 216 通过 |

重录的 4 张全部是 font-18 的截图；同一批测试里的**横向溢出断言本身通过了** ——
标题变大没有撑破版面，变的只是它该有的大小。

---

## Round 142 —— 批量迁移：workspace 视图（第一批 9 个文件）

按用户要求改为**按批推进**：一批吃掉一族文件，只在批末集中验证一次。

### 这一批的真信号

23 个 dock 视图在渲染**同一个形状** —— 一列带栏距的行，每行有标题行、
描述行、说明。那些重复的 class 串就是这个形状被拼写了 23 遍：

```
36  px-[var(--density-column-gutter-wide)]     ← dock 栏距
19  行容器（gutter + py-*）
 4  mt-0.5 text-ui-sm leading-body text-fg-muted   ← 描述行
 3  truncate text-ui-md font-semibold text-fg      ← 标题
```

所以这一批的产物不是"把 class 翻译成 StyleX"，而是
`views/viewStyles.ts` —— **一个 dock 视图由哪些形状组成**，命名一次。
类型步不进这个文件：尺寸是设计系统的词汇，这里只管排布。

### 已迁

`skills` `recipes` `skillLibrary` `skillProposals` `agent-docs`
`notifications` `inbox` `toolStats` `views/PlanList` —— 9 个文件，className 归零。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **652 / 652**（workspace 109 + closure 350 + 其余 302），**零位移、零重录** |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | workspace 299 通过 |

零位移是这一批的验收标准：**迁移应当复现，而不是顺手调整。**
（closure 第一次跑出 1 项失败，复跑 350 全过 —— 又一次负载 flake。）

### 顺带记下的一处

`notifications` 的"已忽略"用 `opacity-0.5`。它不是禁用
（禁用是 `--control-disabled-opacity` = 0.64，"退到背景"是 `off="faded"` = 0.25）——
已处理过的通知仍然要能读。三者是三件事，暂各自保留；
如果之后出现第四个"变淡"的理由，就该收敛成一组命名档。

---

## Round 143 —— 批量迁移：workspace 视图（第二批 6 个文件）

`run-summary` `search` `knowledge` `timeline` `tools` `agentMemory` ——
本批把这一族里 className 密度最高的六个迁完，`workspace-views` 归零。

### 三件不是"翻译"的事

**1. 两张返回 class 名的查找表，改成返回领域值。**
`run-summary` 的 `TONE_INK` 和 `timeline` 的 `STATUS_MARK.tone` 都把
`Tone`（领域词）映射成 `"text-success"` 这样的字符串。而 `runSummaryCommandTone`
返回的本来就是 `Tone` —— 只有视图那一步把它变成了 class。
现在 `inkByTone: Record<Tone, StyleXStyles>` 是**唯一**做这件事的地方。
（`workspace/ui/TasksPill.tsx` 还留着一份两项的副本，下一批收。）

**2. `radius.md` 不存在，而这不是阶梯漏了一档。**
`timeline` 的运行头是 `rounded-md` 的沉降盘，迁移时发现 `tokens.stylex.ts`
的 radius 里没有 `md`。查证：`--surface-card-radius` 与 `--radius-md`
**都是 `var(--shape-md)`** —— 同一个值两个名字。阶梯故意不给 `md`，
因为这个角属于**卡片这个面**，而 `radius.card` 才是视觉风格可以单独移动的那个。
改用 `radius.card`。**同一个数值不等于同一个事实**（第 140 轮同一条教训）。

**3. `leading` 没有 `none`，也不该有。**
状态标记是一个只装字形的盒子，它需要 `line-height: 1` —— 那不是排版档，
是"这里面没有文字"。写成字面量并注明理由，而不是往类型阶梯里加一档
引诱别人拿它当字号用。

### 又一处冻结类名的测试

`the timeline names tools…` 用 `.truncate` 定位时间线的条目。迁移后只剩 1 个。
治本仍是**让组件把自己的决定说出来**：条目主语加 `data-timeline-subject`。
顺带修正了测试的取样 —— `.truncate` 原本还会捞到 run id 和详情，
而这条测试问的只是"主语用的是转录名还是 wire 名"。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **652 / 652**（workspace 109 + closure 350 + 其余 193），**零位移、零重录** |
| 守卫 | 17 项 `check:*` 全绿 |
| 单测 | workspace 299 通过 |

`workspace-views` 全族 className 归零（23 个文件，227 处）。

---

## Round 144 —— 批量迁移：diagnostics 与 TasksPill；顺带量清 StyleX 的真实代价

`DiagnosticsView` `TracesPanel` `primitives` `TasksPill` —— 4 个文件，className 归零。

### 五张表把领域词映射成了 class 名

`STATUS_TONE`（TracesPanel）、`SEVERITY_TONE`（DiagnosticsView）、
`TONE_INK`（TasksPill）、以及上一轮收进 `viewStyles` 的那两张 —— 五处各自写着
`Record<…, "text-negative">`。

`Badge` 的 tone 映射是**盘**（填充 + 墨），这些调用点要的是**裸墨色** ——
两件不同的事，而裸墨色有五个消费者。按"缺档就往库里加一档"，
它归设计系统：`ui/atoms/tone-ink.ts` 的 `toneInk: Record<Tone, StyleXStyles>`。
上一轮那个 `inkByTone` 一并删除。

**领域词从此不再在视图里变成字符串**：`severityTone` 返回 `Tone`，
`STATUS_TONE` 存 `Tone`，只有 `toneInk` 一处把它变成墨色。

### `Cell({ className: string })` —— 列宽由调用方拼字符串

诊断面板的 `Cell` 要求**必须**传一个 class 当宽度。两个面板于是把自己的列模型
写成了 11 个字符串（`w-12` `grow` `w-24` `w-4` `w-16` `w-28`）。
**列宽是表的决定**，所以改成 `styles?: StyleXStyles`，每个面板在自己的表头旁边
声明一次列（`logColumns` / `spanColumns`）。

### 一条后代规则回到它该在的层

`[&_svg]:animate-pulse-dot` —— StyleX 表达不了后代选择器。
（我先写了个无意义的 `::part(x)` 占位，随即改对。）
按第 129 轮的先例：这类规则进 `globals.css`，由 `data-pulse` 触发。

### 门禁抓到了迁移的真实代价

`check:bundle` 失败：入口 CSS 打满预算。**先归因再动预算** ——

| 提交 | 入口 CSS | |
| --- | --- | --- |
| Round 140 | 135,342 | |
| Round 141 | 135,389 | |
| Round 142（迁 9 个文件） | 133,381 | **−2,008** |
| Round 143（迁 6 个） | 134,516 | +1,135 |
| 本批（迁 4 个） | 135,007 | +491，超线 7 字节 |

（第一次量的时候忘了 `git stash -u`，新建的未跟踪文件混进了三次构建 ——
四个数字都被同一个量污染。重量才有了上表。）

拆开这 132.6 KB：

| | 字节 | 占比 |
| --- | --- | --- |
| StyleX 原子（**600 条规则**） | 42,164 | 32% |
| Tailwind + `globals.css` | 90,462 | 68% |

预算 135,000 是 2026-08-11 记的，那时入口 CSS **103 KB、完全没有 StyleX**，
余量 31%。所以：**Tailwind 侧只降了 13 KB，StyleX 加了 42 KB，净 +29 KB。**
这就是迁移期的双份计费 —— 一个工具类要等它**最后一个**消费者迁完才会消失，
而业务层还有 250 个文件在用 `flex` / `gap-2` / `text-fg-muted`。

余量降到 **0.4%** 时，它已经分不清"回归"和"491 字节的日常改动"了。
按门禁自己的要求把 CSS 预算提到 145,000，并把上面这些数字写进它的注释 ——
连同一句：**迁移做完后这条线要降回 135 KB 以下，拿这段注释来对账，不要让空间被填满。**

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **652 / 652**（workspace 109 + closure 350 + 其余 193），零位移零重录 |
| 守卫 | 17 项全绿（CSS 预算已按实测调整并写明理由） |
| 单测 | workspace + `src/ui` 349 通过 |

---

## Round 145 —— 批量迁移：settings 的 kit 与三个目录（13 个文件）

`kit/SettingRow` `kit/SettingsGroup` + `appearance/`（9 个文件全清）
+ `approvals/`（3 个）+ `schedules/`（3 个）。settings 从 243 处降到 176。

### 这一批的产物

`kit/settingStyles.ts` —— **一个设置面板由哪些形状组成**。
29 个文件在渲染同一小把东西，各自拼写：标签 8 次、提示 7 次（两种写法）、
栈 11 次（两种间距）。类型步同样不进这个文件（与 dock 的 `viewStyles` 同一条分工）。

`SettingRow` 顺带把 `first:border-t-0` 变成了条件值 —— 行之间的接缝由**下面那一行**画，
组自己的上边缘保持干净。

### 撞出的两件事

**1. `surface` 的 wash 家族漏了 accent 这一档。**
`ModeRow` 的选中项用 `bg-accent-wash`，迁移时发现 `tokens.stylex.ts` 里
只有 `negativeWash` / `warningWash` / `successWash` / `infoWash` —— 而 `globals.css`
五个都有。这次是真的漏档（不像上一轮的 `radius.md`，那是名字属于另一个面），已补。

**2. `group/accent` 是这一批唯一的祖先态。**
`group-hover/accent:scale-105` —— 强调色色板悬停时内圈放大 1.05。
按第 123/135 轮的通道模式：靶区发布 `--accent-lift`，内圈读它。
这样也把归属说清楚了：**靶区拥有"指针在我身上"，内圈拥有"那看起来是什么样"。**

### 两处自找的麻烦

- 样式对象取名 `f` / `r` / `a`，撞上了 `fonts.map((f) =>` / `rows.map((r) =>` /
  `accents.map((a) =>` 的循环变量。TS 直接报错，但**这类改名要连带改完循环体里
  每一处引用** —— 我漏了一个 `forgetApprovalRule(r.id)`，靠 TS 抓住。
- 批量插 import 时用了"最后一条 `import ` 开头的行"作为锚点，
  遇到 `import {` 换行的多行 import 就插进了它的中间，直接语法错。
  TS 立刻报错所以没有漏出去，但这个启发式对多行 import 是错的。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **652 / 652**（workspace 109 + closure 350 + 其余 193），零位移零重录 |
| 守卫 | 17 项全绿 |
| 单测 | settings + `src/ui` 182 通过 |

---

## Round 146 —— settings 全族清零（15 个文件）

`providers/` `mcp-servers/` `usage/` `connection-settings/` `hooks/`
`plugins-pane/` `icon-gallery/` —— **settings 243 处 className 全部归零**（29 个文件）。

### 做法：先把共享形状抽出来，再批量套

`settingStyles` 扩到 40 档（`hoverRow` / `nameGrid` / `monoName` / `caption` / 各档 stack）
之后，15 个文件里大约七成的 class 是一次机械替换就能落地的 ——
剩下三成是每个面板自己的网格模板与尺度，各自在本文件里 `stylex.create` 一次。

两个 icon-gallery 文件互为近重复（同一个图标卡片网格，120/44px 与 96/34px 两套尺寸），
抽成 `galleryStyles`：**卡片是同一张卡片，只有尺度不同，所以两个档只带尺度。**

### 一次被 golden 抓住的手误

`ProvidersPane` 的 `flex flex-col gap-1` 我换成了 `stackHairline`（`gap-0.5`）——
**4px 变成 2px**，`workspace golden settings pane providers` 差 4668 像素。

我的替换表里有 `gap-0.5` / `gap-1.5` / `gap-2` / `gap-3`，**独缺 `gap-1`**，
于是手工挑了"看起来最近的那一档"。补上 `stackRows`（`gap-1`）后归零。
**批量迁移里，机械替换比手工判断更可靠 —— 手工那一下就是在赌"差不多"。**

### 顺带

- `space` 阶梯缺 `s11`（图标底板 44px），已补。
- `HooksPane` 的"未获信任"用 `opacity-0.55` —— 这是**第四个**"变淡"的理由
  （禁用 0.64、`off="faded"` 0.25、通知已忽略 0.5、hook 未获信任 0.55）。
  四个不同的意思共用一种手段，各自挑了一个数。**这已经到了该收敛成一组命名档的门槛**，
  下一轮单独做，因为它会动到四处不同语义的观感。
- 批量替换把 `className="…"` 换成 `{...stylex.props(…)}` 时，
  对**组件**（大写标签）是错的 —— 组件要 `className={…}`。
  写了个正则把 5 个文件里的这类改回来。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **652 / 652**（workspace 109 + closure 350 + 其余 193），零重录 |
| 守卫 | 17 项全绿 |
| 单测 | settings + `src/ui` 182 通过 |

---

## Round 147 —— "变淡"有七个值，按含义拆开只剩两件事

上一轮记下"四个意思四个数"。真盘完是**七个值**（0.25 / 0.5 / 0.55 / 0.6 / 0.64 / 0.7 / 0.8），
约 18 处。但按**含义**分组之后，问题不是"值不齐"：

| 含义 | 值 | 主人 |
| --- | --- | --- |
| 字形比标签退一步 | 0.8（按钮内）／0.7 ×3、0.6、0.5（按钮外） | 有，但只管按钮 |
| 控件用不了 | `--control-disabled-opacity`；`checkbox` 自己写 0.6 | 有，一处没听 |
| 退到背景里 | `off="faded"` 0.25 | 有，且只有它 |
| 列着但不生效 | 通知已忽略 0.5、hook 未获信任 0.55 | 无 |
| 正在被拖动 | 0.5 | 无（不同家族，未动） |
| ANSI dim | 0.7 | 终端语义（SGR 2），未动 |

### 第一件事：字形那条规则有主人，但它的逃生口被用来改数

`globals.css` 里 `[data-slot="button"] svg:not([class*="opacity-"]) { opacity: 0.8 }`
—— 那个 `:not` 是给"调用点已经决定了"留的口子。实际用法是：
`GoalModeIndicator` 在按钮里，用它把 0.8 改成了 **0.7** ——
**用不同的数说同一句话**。另外四处不在按钮内，规则够不着，各自挑了 0.7 / 0.6 / 0.5。

`--glyph-step: 0.8` 命名一次：基础层规则读它，按钮外的四处也读它。
`GoalModeIndicator` 直接删掉自己的值，交回基础层。

### 第二件事：又一次"同一句话说两遍"

`GoalStatusSurface` 是 `text-fg-faint opacity-70` —— 已经退色的墨再乘 0.7。
第 138 轮量过这类叠加的代价，这里直接删掉乘数：**`fg-faint` 已经把话说完了。**

### 第三件事：手搓的骨架发明了 atom 刻意没有的值

`ModelPickerPlaceholder` 用 `bg-surface-2` 拼了两根条，外加 `opacity-60`。
而 `Skeleton` atom **完全不用 opacity** —— 它用填充 + 扫光说"还没到"。
库里缺的是**行内单控件占位**（只有 `SkeletonList` 列表档），于是调用点手搓。
补 `SkeletonControl`，占位从 9 行变成 1 行，那个 0.6 随之消失。

`ContextUsageGauge` 的 `opacity-60` 加在**文字**上，换成 `text-fg-muted`
—— 浮层里唯一一处这么做的，而设计系统对"次要文字"有命名墨色。
（这一处没有 golden 覆盖，是按第 138 轮的原则改的，不是按测量。）

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **652 / 652**，重录 **37** 张（dock 标签字形出现在每个 dock 视图里，故波及面大） |
| 守卫 | 17 项全绿 |
| 单测 | `src/ui` + chat 524 通过 |

顺带一条观察：同一处 0.7→0.8 的改动在浅色下越过了 golden 阈值、在深色下没有 ——
深色下图标墨色更亮，差异像素更少，落在 `maxDiffPixels: 40` 以内。**阈值又一次决定了"看不看得见"。**

---

## Round 148 —— 三个目录一次清完：workspace 残余 / sidebar / command

`workspace`（9 个文件）+ `sidebar`（7 个）+ `command`（3 个）—— 三族 className 全部归零。
业务层剩下 `chat/`（250）与 `shell/`（74）。

### 代码面的内距不是面板栏距

`viewStyles` 新增 `codeStyles`：命令日志、文件、差异这三个面共享一套词汇
（`sheet` / `gutter` / `wrap` / `hunk` / `split`）。第 140 轮刻意没把它们的 `px-3`
换成密度栏距 —— 那个内距挨着行号槽、属于代码，这一轮把这句话写进了它的注释里。

### 又一次"注释在说代码没做的事"

`McpRow` 的字形底板原本靠 `group-hover:bg-surface-3 group-hover:text-fg` 随行提亮。
我先写了注释说"行会发布 `--reveal`"，**但没有实现那个通道** —— 底板只剩一条
`transitionProperty`，hover 时什么都不会变。发现后按第 123/135 轮的模式补上：
行发布 `--plate-fill` / `--plate-ink`，底板读它们。

**注释先于实现写下来，就会变成一句谎。**

### 又一次顺序陷阱，9889 像素

`CommandLog` 我写成 `stylex.props(cs.sheetInset, cs.sheet, …)` ——
两个档**都声明 `paddingBlock`**，后者赢，`py-3` 就变成了 `py-2`。

治本不是记住顺序，而是**让那一档自己说清它必须在后面**：
`sheetInset` 的注释现在写着"它的 block 内距与 `sheet` 不同，所以必须composed 在其后"。
（第 126 轮 `chip` 声明在 `variant` 前是同一个坑 —— 那次也是 98 张 golden。）

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **652 / 652**，零位移零重录 |
| 守卫 | 17 项全绿 |
| 单测 | workspace + sidebar + command + `src/ui` 373 通过 |

---

## Round 149 —— shell 清零；顺带修掉一个我自己造了六次的错

`shell/` 17 个文件迁完（只余 `panel-scroll` / `@container` 这类机制键）。
业务层只剩 `chat/`（250 处）。

### 菜单行的网格，六种拼写

`ProjectSelector` 的选项行网格和 settings 的四处一样。真正的缺口是
**`DropdownMenu.Item` 没有 `layout` prop** —— 所以每个调用点只能把网格模板当 class 传。

`option-row` 补两档：`pick`（字形 / 名字 / 勾）与 `pickPlain`（名字 / 勾），
外加 `pickWide` 给色板那一档更宽的字形列。mark 列此前有 12px 与 14px 两个答案，收成一个。
五个调用点的网格模板消失。

### `readingColumn.ts` 的注释解释了一个不再存在的约束

它导出三个共享的 Tailwind class 字符串，注释写着"两半必须同文件，因为 Tailwind 读源码文本，
class 必须拼出常量命名的那个属性"。**StyleX 下这个约束消失了** —— 样式本身就是属性。
转成 `readingColumn` 样式后，`ChatStream` / `MessageStream` / `FloatingComposer` 才能迁。

### 一个我自己造了六次的错：半抄一个类型档

`agent empty` 的 golden 差 24388 像素、settings font-18 差 54150。根因不是布局：

`text-display-md` 这个工具类带**三样东西** —— 字号、字距、`line-height: 1.2`。
我在 StyleX 里只写了 `fontSize: "var(--text-display-md)"`，**丢掉另外两半**。
而且这不是这一轮才犯的 —— 前几轮同样的写法在 `UsagePane` / `IconGallery` /
`DiagnosticsView` / `SettingsPage` / `ChatErrorBoundary` / `ChatStream` **六处**。

治本两步：
1. `type` bundle 补 `displayMd` / `displayLg`，各自带齐三样（`displaySm` 本来就没有 leading）。
2. **加一条守卫**：`fontSize: "var(--text-*)"` 出现在 `tokens.stylex.ts` 之外即失败，
   提示"半个类型档 —— 去组合 `type.displayMd`"。这样抓的是**这一类**错，不是这一次。

（`check:tokens` 此前扫的是"有没有用字面值"，看不见"用了变量但只用了三分之一"。）

### 又一次 `className` 写在 spread 前面

修完上面那条，`agent` 还有 11 张 golden 红着。测量发现空态整列高了 10px ——
`<div className="panel-scroll" {...stylex.props(cst.empty)}>`：
**spread 的 className 把 `panel-scroll` 覆盖掉了。**

第 131 / 135 轮记过这个坑，还为它写了扫描器 —— **这一轮我在同一个文件里又造了三处。**
扫描器现在归零。教训不是"记住顺序"，而是：**这个错误没有类型信号，只有扫描器能拦住它，
所以扫描器要在每一批迁移之后跑，不是只在想起来的时候跑。**

### 顺带

`ProjectSelector` 的测试断言 `w-[calc(100%_-_24px)]`（又一处冻结类名）。
组件改成说出决定：`data-tray="attached"`。而那个**几何**主张搬进闭环测试 ——
托盘必须比 composer 窄、且左右内缩相等，因为 jsdom 里它永远只能读回一个类名。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **654 / 654**（含 2 条新闭环测试），零位移零重录 |
| 守卫 | 17 项全绿（新增一条"半个类型档"规则） |
| 单测 | **1781 项**全通过 |

---

## Round 150 —— chat 的六个小目录；以及迁移撞出的一处隐形文字

`goal` `plan-progress` `narrative-rails` `context-usage` `message-actions`
`chat-search` —— 六个目录清零。`chat/` 还剩 `tools/`（88）与 `message/`（83）。

`chat` 这一族没有主导形状（最高重复只有 8 次 `text-fg-faint`），
所以 `chatStyles` 只收词汇（一行、让宽、截断、墨阶），不收版式 —— 这一族是真正的机械翻译。

### 一处对比度 1.00:1 的隐形文字

`ActivePlan` 的计划步骤用 `text-on-fg/60` 与 `/80`。追这个令牌：
`--color-on-fg` → `--color-text-on-fg` → **`--color-bg`** —— 那是**反色墨**，
给"用前景色填充的盘"准备的。而这些步骤画在 `RichTooltip` 上，那不是反色的盘。

量了：

| | 墨 | 浮层填充 | 合成后对比 |
| --- | --- | --- | --- |
| 浅色 | 白 @0.6 | 白 | **1.00** |
| 深色 | 28,32,35 @0.6 | 29,31,35 | **1.00** |

**两个主题下，计划的步骤文字都和它背后的浮层同色。** 改用工具提示自己的墨阶
（已完成 → `fgMuted`，未完成 → `fgSoft`，保持原本的强弱关系）后是 6.60 / 7.41。

这个反色令牌**只有这一个消费者，而它是错的** —— 令牌连同它的别名一起删除。

**没有任何守卫能发现它**：没有 golden 会打开工具提示，而 WCAG 审计的浮层清单里
只有三个点击打开的菜单。所以把它加进那份清单 —— 顺带给这个家族补上"怎么打开"
这一维（悬停 vs 点击）和 `role="tooltip"`。修复前它会因 1.00:1 直接失败。

### 又一处冻结类名，又一次搬进浏览器

`ActivePlan.test.tsx` 断言 `h-8` / `w-full`。它的意图是"这是紧凑药丸、不是被搬过来的计划卡"
—— **计划条不论几步都保持一个高度**，那是几何主张。搬进闭环测试：
高度必须是 32px，且它背后的步骤列表条数不为零（否则断言是空的）。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **656 / 656**（含 3 条新闭环测试），重录 1 张（步骤文字从隐形变可读） |
| 守卫 | 17 项全绿；`className` 顺序扫描器 0 处 |
| 单测 | chat + `src/ui` 全通过 |

---

## Round 151 —— chat/tools 清零；一个"整档漏掉"的错

`chat/tools/` 21 个文件迁完。业务层只剩 `chat/message/`（121 处）。
按 className 计，全库迁移约 **94%**。

### previews 的共享形状

七个预览（file / glob / grep / lsp / recall / skill）渲染同一个行：
最紧的圆角 + 一档内距 + 行洗色，各自写了一遍，其中两个还用了别的圆角。
抽成 `previewStyles`（`row` / `numbered` / `gutter` / `wrap`）。

### 又一张把领域词映射成 class 名的表

`lsp` 的 `SEVERITY_TONE` 存 `"text-negative"`。第 144 轮已经把这类表收敛过一次
（五张），这是第六张 —— 改成存 `Tone`，用 `toneInk`。

### 一个"整档漏掉"的错，golden 抓住了

`TEXT_PREVIEW_CLASS` 是个共享的 class 字符串，20 个消费者：

```
"max-h-60 overflow-y-auto px-0 pt-1 pb-0 font-mono text-ui-md leading-body text-fg-muted"
```

我把它转成 `textPreview.block` 时，**七个工具类里漏掉了 `text-ui-md`**。
后果不在这个块自己身上 —— 它的**子元素**继承了那个字号，于是每个预览的页脚
从 14px 掉回文档的 16px，行高从 21.7 变成 24.8，整列内容位移 3px。

`tool-shells` 的 golden 差 15740 像素。定位过程记一下，因为走了弯路：

1. 同视口做"全元素几何 + 颜色 + 字号"快照对比 → **零差异**。
2. 扩到字体族/行高/字距/背景/连字 → **仍然零差异**。
3. 在 HEAD 上跑同一条 → 通过。所以确实是我的改动，但我量不到。
4. 读 golden 测试才发现：`tool-shells` 会**先展开一个 apply_patch 预览**再取景，
   而我一直在比折叠态。
5. 复现取景步骤后，17 个元素高度有差；最深那个是 `PreviewFoot`：14px → 16px。

**教训不是"小心点"**：一个共享 class 字符串转成样式时，字符串里的每一个工具类
都必须落到某处，而漏掉的那个如果是**被子元素继承的**，这个块自己看不出任何异常。
所以按 `FLOATING_PANEL` 的先例导出**数组** `[previewText.block, type.uiMd]` ——
类型档留在这个 bundle 里，谁组合它就带上它。

上一轮是"抄了三分之一个档"（新守卫能抓），这一轮是"整个档漏掉了"（守卫抓不到，
因为没有可疑的写法可扫）—— **抓住它的是 golden，前提是 golden 拍的是展开态。**

### 顺带

`ToolOutputPanel.test.tsx` 用 `div.whitespace-pre-wrap` 数行数（又一处冻结类名）。
行元素改成自报身份：`data-output-line`。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **657 / 657**，零位移零重录 |
| 守卫 | 17 项全绿；顺序扫描器 0 处 |
| 单测 | chat 全通过 |

---

## Round 152 —— `chat/message` 清零：业务层迁移完成

`chat/message/` 18 个文件迁完。**业务层 902 处 className 归零**，
剩下的 52 处全是 `globals.css` / `markdown.css` 拥有的机制键
（`md` `md-table-*` `md-media-*` `sr-only` `media-edge` `panel-scroll` `[&_svg]:*`）。

### 卡片的内距阶梯

审批卡、问题卡、压缩通知共享一套内距阶梯 —— 头部比正文深、动作区比头部更深 ——
以及一行"正在问什么"：它 `overflow-wrap: anywhere`，因为那可能是一条没有空格可断的路径。
抽成 `messageStyles`。

### 第七种菜单行拼写，以及同一个缺口的第二处

`MessageContextMenu` 的 `ContextMenu.SubmenuTrigger` 写着
`grid-cols-[14px_minmax(0,1fr)_12px]` —— 第七种拼写，mark 列又是 12px。
上一轮给 `DropdownMenu.Item` 补了 `layout`，**submenu trigger 是同一个缺口的另一半**，
一并补上。七种拼写现在是两档（`pick` / `pickPlain`）加一个宽字形变体。

### 我自己造的一处重复

第 149 轮我把 `RunAnnouncer` 的 `sr-only` 手写成了一个本地样式
（`position: absolute; clip-path: inset(50%)` …）。而这一轮发现 `chat/message` 里
还有三处仍在用 `sr-only` —— **同一件事两种拼写，其中一种是我加的。**

撤回那个本地样式：`sr-only` 是 Tailwind 自带的成熟实现，按机制类对待
（和 `panel-scroll` / `md` / `media-edge` 一致）。**迁移时把一个机制类"翻译"成
本地样式，等于给它开了第二个主人。**

### 又一处冻结类名

`QuestionCard.test.tsx` 断言已结算答案带 `whitespace-pre-wrap`。
意图是"多行答案按多行显示" —— 换行进入 DOM 这半 jsdom 能证（文本匹配已经在做），
**按换行绘制这半是渲染主张**。答案加 `data-settled-answer`，
渲染那半加进已有的 `question settlement` 浏览器测试（它此前只拍折叠态）。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **656 / 656**，零位移零重录 |
| 守卫 | 17 项全绿；顺序扫描器 0 处 |
| 单测 | **315 文件全通过** |

### 报告，未改

`MarkdownImage` 的图片用 `shadow-md` —— 那是 **Tailwind 自带的阴影**（`--shadow-md`
由它的主题提供），而设计系统有自己的 `--shadow-popover` / `--shadow-modal` /
`--shadow-floating` 深度模型。一张 markdown 图片该落在哪一档深度上是设计决定。

---

## Round 153 —— 等宽字体上的负字距；以及一句被量翻的预测

迁移做完了，所以这一轮回到设计缺口 —— 而第一个缺口是我自己造的。

### 我在四个模块里各写了一遍 `mono`，每次都只写了一半

`tokens.stylex.ts` 里的 `face.mono` 带**两样**：`fontFamily` 和 `letterSpacing: 0`。
`Badge` 的注释解释了它为什么存在：

> 那个工具类只换字体族，所以八个调用点都在用**为比例字体挑的负字距**渲染等宽字形；
> 调用点自己修不了，因为字距住在类型档里。

我的四个业务样式模块各自定义了 `mono: { fontFamily: "var(--font-mono)" }`
—— **正是 `face.mono` 当初为了修掉而存在的那个形状**。46 个引用点，
其中 **39 个同时组合了 `typeStep`**，而每个 `text-ui-*` 都带 `--tracking-ui: -0.011em`，
且类型档组合在后、字距由它决定。

量出来的代价：

```
40 个等宽字符：带 --tracking-ui = 306.3px，字距 0 = 312.0px
漂移 5.7px（每字符 0.14px，约三分之二个字符宽）
```

**等宽字体加上负字距就不再等宽** —— 而行号槽、路径列、`tabular-nums` 的对齐
全靠字符宽度相等。这不是我这几轮引入的（迁移零位移证明了它一直如此），
是迁移把它**显形**了：以前它藏在 `font-mono` 这个工具类里，现在它是一个有名字的档。

改法：四个模块删掉自己的 `mono`，46 处改用 `face.mono` 并组合在类型档**之后**。
（正则把 `FontSection` 的一处**条件**组合弄坏了 —— 只有等宽字体选择器才该用 mono，
我却追加了一个无条件的。TS 抓住了。）

65 张 golden 重录，每张都是一小段等宽文字的字宽变化。

### 一句被量翻的预测

第 144 轮我在 `check-bundle-size.mjs` 的注释里写下承诺：
双份计费是**暂时的**，迁完后这条线要"降回 135 KB 以下"。
**迁完了，预测是错的。** 分层量清：

| | 字节 | 占比 |
| --- | --- | --- |
| `@layer utilities`（Tailwind） | 24.4 KB | 18% |
| **StyleX（无图层）** | **57.4 KB** | **42%**（821 条规则） |
| `@layer theme` + `base` | 9.5 KB | 7% |
| globals/markdown 的无图层规则 | ~43.8 KB | 33% |

也就是说 **~49 KB 的 Tailwind 工具类变成了 24 KB 工具类 + 57 KB StyleX**。
原因可测且是结构性的：**23.7 KB —— StyleX 输出的 41%，2630 次出现、每条规则 29 字节
—— 是 `:not(#\#)` 特异性填充。**

那些字节不是浪费，它们**就是整个迁移值得做的那个性质** ——
正因为有它，任何工具类都无法静默压过一个设计决定。
gzip 后 135 KB → 27 KB（填充几乎全被压掉），但这是桌面 webview、本地读文件、
**没有传输**，所以 raw 才是每次启动实付的解析成本，也正是这条预算在管的东西。

预算就停在迁移留下的地方，按**真实的**迁后数字留 6% 余量，而不是按一个希望值。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **656 / 656**，重录 65 张（等宽字距） |
| 守卫 | 17 项全绿 |
| 单测 | 1781 项通过 |

### 留给下一轮

我在四个模块里造了**同一套词汇的四份副本**：30 个档名出现在 2–4 个模块里，
其中 **20 个同名同值**（`truncate` `min` `hold` `muted` `faint` `accent` …，共 361 处引用），
**10 个同名不同值**（`line` `fill` `split` `stack` `caption` `pane` …）。

后 10 个正是这整轮重构一直在猎的缺陷 —— **一个名字几个意思** —— 而我自己造了四遍。

## Round 154 — 一个名字一个意思：把我自己造的四份词汇副本收成一份

### 根因

上一轮末尾报的那个缺陷，根因不在我手抖，在**结构**：

`check-builtin-contexts` / `check-layers` 禁止一个内置插件 import 另一个上下文的内部实现 ——
这条守卫是对的（它挡的是插件之间偷偷耦合）。但它的推论是：
**四个插件之间唯一可能的共享住址是 `ui/` 或 `lib/`，别处都不合法。**

我迁移时是一个插件一个插件迁的，于是每到一个插件就地建了一个 `*Styles.ts`。
四次都碰到同一批需求 —— 一行会截断的文字、一个让出宽度的部分、一档墨色 ——
四次都就地写了。守卫从来没报警，因为四份副本各自合法。

### 证据

四个模块共 663 处引用。其中 **363 处（55%）用的是同名同值的 21 个档**：

| 档 | 声明它的模块 | 引用 |
| --- | --- | --- |
| `truncate` | view set chat shell | 77 |
| `muted` | view set chat shell | 56 |
| `hold` | view set chat shell | 38 |
| `faint` | set chat shell | 37 |
| `fill` | view set chat *(shell 不是)* | 26 |
| `min` | view set chat shell | 23 |
| `line` | set chat shell *(view 不是)* | 22 |
| `soft` | view chat shell | 16 |
| `accent` `negative` | view set chat shell | 13 + 13 |
| `ink` | view chat shell | 8 |
| `lineTight` | set chat shell | 6 |
| `figures` | view set chat | 5 |
| `grow` `warning` `strong` `success` `column` | 2–3 个模块 | 4+4+3+3+3 |
| `wrapText` `pretty` `stackHairline` | 2 个模块 | 2+2+2 |

另有 **9 个同名不同值** —— 这一档才是真缺陷，因为**一个名字指了两个东西**：

| 名字 | 一个意思 | 另一个意思 |
| --- | --- | --- |
| `fill` | view/set/chat：`min-width:0; flex:1`（让出宽度） | **shell：`position:absolute; inset:0`（铺满）** |
| `line` | set/chat/shell：flex + center + gap-2 | view：同上**再加** `min-width:0` |
| `split` | set/chat：space-between（gap 3 / 2 两个值） | **view：两列 grid** |
| `caption` | view：`fgFaint` 一档墨色 | **set：一个带 margin 和字重的组标题** |
| `fieldLabel` | view：`fgMuted` + medium 墨色 | **set：一个 flex 竖排容器** |
| `pane` | set：column + gap-6 | **shell：`flex-1 min-h-0` 的窗格** |
| `stack` | view：无 gap 的 column | set：gap-3 |
| `stackTight` | set：gap-2 | chat：gap-1.5 |
| `afterRow` | view：`margin-top: s1_5` | set：`s2_5` |

`fill` 是最坏的一个：同一个名字，一处叫「把宽度让给旁边」，一处叫「盖住整个父元素」。

### 这一轮做什么

1. 新建 `ui/atoms/vocabulary.ts`（`vocab`），21 个同名同值档一份，363 处引用改指过去。
   插件边界是真的，所以这是唯一合法的住址 —— 它跟 `reveal.ts` / `tone-ink.ts` 同层。
2. `vocab` 的收录判据写进文件头：**「每个人都需要、且只有一个说得通的值」进来；
   「某个面自己决定的排布」（一行的 gap、一张卡的内缩）留在那个面，名字要说出它是哪个面。**
   所以 `stackTight` 不进（两个模块两个 gap = 没有共识 = 不是共享事实），
   `medium` / `bodyLeading` 不进（只有 chat 声明，没有重复可消）。
3. 顺手解掉必然冲突的两个：`vocab.fill` 一进来就跟 `shellStyles.fill` 撞名 ——
   shell 那个改叫 `overlay`（它就是个浮层）；view 的 `caption` 就是 `vocab.faint`，删掉。

剩下 7 个同名不同值下一轮改名 —— 改名要一处一处看它在那句 JSX 里到底是什么意思，
跟这一轮的机械替换不是一回事，混在一批里我会把判断当替换做。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **656 / 656，0 unexpected，0 flaky —— 零像素位移** |
| 守卫 | 17 项全绿 |
| 单测 | 1781 项通过 |
| `className` 早于 spread 的扫描 | 0 |

零位移就是这一轮要的那个数：换住址不是设计改动，一张 golden 都不该动。

### 我这轮自己犯的两个错，都值得记下来

**一、跑了三遍视觉套件，三遍都什么也没验。** 我手写 `npx playwright test visual/…`，
漏了 `--config playwright.visual.config.ts` —— 没有它就没有 `baseURL`、没有 webServer，
302 个用例里 301 个死在 `Cannot navigate to invalid URL`。而我读到的是「exit 0」，
因为 `... | tail -18` 之后 `$?` 是 **tail** 的退出码，不是 playwright 的。
两个错误叠在一起，正好合成一个「绿」。**教训**：套件走仓库自己的入口（`npm run visual:test`），
别手搓；要退出码就别放在管道尾巴上。

**二、两次在套件运行中改了源文件。** 第一次改 barrel、第二次改 ViewHeader，
都可能让 dev server 在跑到一半时重编译。这次没造成误判（因为那两次跑的结果本来就是废的），
但它会污染归因 —— 而「先隔离再归因」是我前几轮才立的规矩。

**三、顺带纠正一个我上一轮报错了的数。** 我说业务层 `className` 从 902 降到 0。
那个扫描只匹配 `className="…"` 这一种写法，漏掉了 `className={cond ? "a" : "b"}`、
`cn("…")`、以及作为 prop 传下去的 class 串。按「真正抵达某个 className 且含至少一个
utility 的字符串」重新数：**33 个文件里还有 122 条 class 串、403 个 utility。**
迁移完成的是 `className="…"` 这一种形态，不是整个业务层。

### 留给下一轮：`ui/` 自己也在重写 vocab

刚建好 `vocab` 就能反过来量它：**38 个文件里有 86 处把某个 vocab 档的值又写了一遍**。
但这 86 处要分成两类，只有一类是缺陷：

| | 例子 | 判断 |
| --- | --- | --- |
| **只是换个拼法** | `TracesPanel:hold`、`sidebar/projects:column`、`DiagnosticsView:muted`、`SessionRow:grow`、`FloatingComposer:pretty` | **缺陷** —— 本地名没多说任何东西，就是 `vocab` 那一档 |
| **本地名多说了一层意思** | `text-field:glyph`、`navigation-row:trailing`、`step-row:label`、`button:chip`、`catalog-picker:rowGlyph` | **不是缺陷** —— `trailing` 说的是位置、`glyph` 说的是身份，换成 `vocab.hold` 只会让调用处更难读 |

第三类是真正该治的：`tone-ink.ts`、`button.tsx` 的 tone 表、`ansi-text.tsx` 的 tone 表
**各自重新声明了一遍墨色阶梯**（`color.negative` 写了四遍）。投影该留着，
声明只该有一份 —— `toneInk` 应该指向 `vocab.negative`，而不是自己再写一次那个值。

## Round 155 — 墨色阶梯只声明一次；以及「换个拼法」和「多说一层」的分界

### 根因

`vocab` 建好之后，可以反过来量整个 `src/`：**38 个文件里 86 处把某个 vocab 档的值又写了一遍。**
但这 86 处不是一类东西，混在一起治会把好代码改坏。分界线是：

> **本地名说出了 `vocab` 说不出的东西吗？**

说得出 → 不是重复，是命名。`navigation-row:trailing` 说的是「它坐在行尾」，
`text-field:glyph` 说的是「它是个字形」，换成 `vocab.hold` 只是把「哪里」换成「怎么做」。
说不出 → 就是重复。`TracesPanel:truncate`、`sidebar/projects:column` —— 本地名跟共享名
一个字都不差，那它存在的唯一理由就是「当时手边没有共享的那个」。

### 做了三件

**一、墨色阶梯的四份声明收成一份。** `color.negative` 在四个地方各声明了一遍：

| 位置 | 是什么 | 处理 |
| --- | --- | --- |
| `tone-ink.ts` | `Record<Tone, …>`，自称「一个 tone 变成 token 的唯一地方」 | 改成**投影**：指向 `vocab`，不再自己声明 |
| `button.tsx` | `toneNegative/Warning/Accent/Success` + `TONE` 表 | 删掉，直接 `toneInk[tone]` |
| `activity-disclosure.tsx` | `markNeutral/Warning/Negative` + `MARK_TONE` 表 | 删掉，直接 `toneInk[tone]` |
| `ansi-text.tsx` | 也自称「唯一的地方」（两个文件同时自称唯一） | 六档里五档指向 `toneInk`；`muted` 留下，因为它**不是** `Tone.neutral` |

`ansi-text` 那一档是这轮唯一需要判断的地方：ANSI 的 dim 是「比周围的字更淡」，
比 `muted` 还低一阶，所以它读 `vocab.faint` —— 名字一样、事实不同，不能合。

`toneInk` 的类型也从 `Record<Tone, StyleXStyles>` 改成 `as const satisfies` ——
前者把每一档都放宽成 `StyleXStyles`，调用处就丢了具体类型；后者既保留精确类型、
又照样检查对 `Tone` 的穷尽。

**二、顺出一个死分支。** `activity-disclosure` 里：

```
line && tone === "neutral" ? styles.markNeutral : MARK_TONE[tone]
```

两个分支是**同一个值**。查 `git log -L` 查出了来历：最早是
`shell === "line" && tone === "neutral" ? "text-fg-faint" : TONE_CLASS[tone]` ——
一行里的中性字形比卡片里的更淡，这个区分是真的。后来 `75aea47d` 认为
「最淡的那档把字形的辨识度花在了没有意义的地方」，把 `fg-faint` 提到 `fg-muted`，
于是它跟默认值相等了，分支就此变成空转，而**没人删它**。
决定是对的、注释是对的，只有那个分支是残留。注释搬到 `toneInk.neutral` 旁边 ——
它现在是那个决定真正落地的地方，也是防止有人「好心」把它改回 faint 的地方。

**三、17 个文件里的同名重复改指 `vocab`**（`truncate` ×4、`ink` ×3、`faint` ×2、
`column` ×3、`hold`/`grow`/`pretty`/`line`/`muted`/`wrapText`/`info` 各一），
并且 `viewStyles.info` 消失后，`vocab.info` 补回来了 —— 我上一轮按 YAGNI 把它删了，
这一轮 `toneInk` 和 `ansi-text` 都要它，证据反过来了。

### TypeScript 抓到我一个错

机械替换把 `divider.tsx` / `tag.tsx` / `well.tsx` 里的 `accent` / `muted` / `soft` 也删了 ——
但那三个不是「换个拼法」，它们是 **union 索引表里的键**：`styles[variant]`，
`variant: "accent" | "neutral"`。名字在那里不是对机制的重述，**就是那个 prop 的值**。
删掉一个成员，索引就断了，TS 立刻报 TS7053。已还原。

**这补上了判据的第三种情况**：本地名如果是「一个 union 用来索引的键」，
它既不是重复也不是命名 —— 它是**契约的一半**。

### 顺带发现一个潜在缺口（未改，零实例）

`TextButton` 的 `TONE` 表里，`muted`/`faint`/`negative` 三档都回应 hover
（前两个提亮到 `fg`，第三个降到 opacity 0.8），**`accent` 什么都不回应**。
原因在文档注释里其实写了：`link` 形状「mono、accent、hover 时下划线」——
回应 hover 的是**形状**，不是 tone。三个 `tone="accent"` 调用处全都是 `shape="link"`，
所以缺口是潜在的、零实例。指向 `vocab.accent` 之后这个不对称至少看得见了；
没有为它编一个 hover 值 —— 那会是在猜。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **656 / 656，0 unexpected，0 flaky —— 零像素位移** |
| 守卫 | 17 项全绿 |
| 单测 | 1781 项通过 |
| CSS raw | 135 KB → **134.3 KB**（预算 142.6 KB） |

### 最值得记的一件事：这两轮的重复，字节上几乎不花钱

481 处引用收敛到一个 owner，CSS 只掉了 **~0.7 KB**。

原因是 StyleX 的原子是**按内容寻址**的 —— `{ color: var(--color-negative) }`
无论在几个文件里声明，编译出来都是同一个类、同一条规则。所以那四份墨色阶梯副本、
那 21 个 × 四份的档名，**在产物里本来就只有一份**。

这件事的含义比省下的字节重要得多：**这个缺陷对仓库现有的每一种自动测量都是隐形的** ——
体积预算看不见它（字节一样）、17 个守卫看不见它（四份副本各自合法）、
1781 个单测看不见它（行为一样）、656 张 golden 看不见它（像素一样）。
它只对**读代码的人**可见：下一个人要在四个地方找「失败是什么颜色」，
并且有四次机会给出第四个答案。

所以这类缺陷只能靠**为它专门写的扫描器**找出来，而不是靠已有的门禁跑绿。
两轮各写了一个（同名不同值的碰撞扫描、vocab 档的重拼扫描），都在 `/tmp` 里跑完就扔了 ——
下一轮该判断的是：它们中哪一个值得变成第 18 个 `check:*`。
判据是它会不会误伤 —— 而这两轮已经量出误伤面有多大：
私有 `styles` 块里 112 个「同名不同值」几乎全是合法的局部命名，
所以能进守卫的只可能是**导出的共享词汇模块之间**的碰撞，不是全局同名检查。

## Round 156 — 五个「一个名字两个意思」，五个不同的正确答案

### 根因

这五个是从 round 154 欠下来的。它们看起来是同一种缺陷，但**没有一条通用规则能一起治** ——
每一个的正确答案取决于「那两个意思里，哪个才是这个词该有的意思」，
而这只能一处一处读出来。

| 名字 | 两个意思 | 答案 |
| --- | --- | --- |
| `split` | setting：space-between 一行 · chat：同上但 gap 不同 · **view：两列 grid** | chat 那份**零引用**，删；view 那份改名 `sideBySide`（它是网格，不是行）；setting 保留 `split` —— 「把标签和控件推到两端」就是 split 的本意 |
| `stackTight` | setting：gap-2 · chat：gap-1.5 | 两个都不留 —— 见下面的阶梯 |
| `pane` | setting：column + gap-6 · **shell：`flex-1 min-h-0` 的窗格** | setting 那份拆成 `column` + 一档 gap；`pane` 从此只指 shell 的窗格（它才是真的 pane） |
| `fieldLabel` | setting：flex 竖排容器 · tool：`fgMuted` + medium 的墨色 | setting 那份**零引用**，删；tool 保留 —— 「一个字段的名字」就是它 |
| `afterRow` | setting：`margin-top: s2_5` · view：`s1_5` | 读了调用处才看清：view 的那个 = setting 的 **`afterLine`**（同值同义），所以它是 `vocab.afterLine`；`afterRow`（s2_5，行下面展开面板前的那一步）留在 setting |

**两个是零引用** —— 也就是说 round 154 报的 9 个冲突里，有两个从来没真正存在过，
只是两个死档撞了名。这也是为什么「先数引用再动手」比「先归类再动手」重要。

### `gap` 阶梯：13 个名字各说一个数字

`stackTight` `stackTightest` `stackWide` `stackRows` `stackHairline` `stackGap`
`editorGap` `panelGap` `fieldGap` `lineWide` `lineWrap` `grid2` `pane` ——
这些名字除了大小之外什么都没说，而**比较级会用尽**：
`stackTight` 在设置面板是 gap-2、在对话流是 gap-1.5，
而它想要的那个值（1.5）的名字已经被 `stackTightest` 占了。

行间距**确实**是每个面自己的决定 —— 这正是那个共享名字一直在自相矛盾的原因。
所以让面自己说是哪一档：`vocab.column, gap.s2`。

**一条硬约束写进了 `gap` 的注释**：它只跟 `vocab.column`（不带 gap）组合，
**绝不放在一个自带 gap 的档后面** —— `vocab.line` 默认 gap-2，
两个档都声明 `gap` 就是 round 148 那个「覆盖变隐形」的坑。
改完 grep 验过：没有任何调用处有两个 `gap.*`，也没有任何 `gap.*` 跟在自带 gap 的档后面。

### TypeScript 又抓到我一个错，而且是同一类

我数「某个档有几处引用」时**手写了模块别名列表**（`vs|ss|ct|sh|cs|tool`），
漏了 `toolStyles as os`。于是 `panelGap` `fieldGap` `afterLabel` `fieldLabel` 四个
**活着的**档被我判成零引用删掉了。TS 立刻报 TS2339，已还原。

这跟本轮早先那次 `ct` 写成 `cs` 是**同一个错**：别名是从 import 里读出来的事实，
我却每次凭记忆重写一遍。治本的做法是先 grep 出 `X as y` 再据此计数 —— 
上一轮我这么做过一次（正是那次发现了 `ct`），这一轮又偷懒了。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **656 / 656，0 unexpected，0 flaky** |
| 守卫 | 17 项全绿 |
| 单测 | 1781 项通过 |
| 档名减少 | 13 个「各说一个数字」的名字 → 8 档 `gap` 阶梯 |
| 同名不同值 | 9 → **0** |

## Round 157 — 先把「还剩多少」量准，再动手

### 我报过三个不同的错数字

| 报过 | 怎么数的 | 错在哪 |
| --- | --- | --- |
| 业务层 **902 → 0** | `grep 'className="[a-z]'` | 只认 `className="…"` 这一种字面形态 |
| 还剩 **122** 条 | 加上 `cn(`，但用正则猜表达式的结尾（`\}` 或 `\);`） | 多行 `cn(…)` 在第一个 `}` 就被截断 |
| 还剩 **429** 条 | 改成扫全文的字符串、剔掉 `stylex.create` 区块 | `"data-slot"` `"aria-hidden"` `"agent-state"` 全被当成 class |

第三次错得最有教育意义：**单个带连字符的词，脱离 `className` 上下文就无法判定。**
`font-mono` 是 class，`data-slot` 是属性名，`tool-call` 是测试 id —— 形状完全一样。

所以唯一可靠的做法是**按上下文取，而不是按形状猜**：
找到 `className=` 或 `cn(`，然后用**配平括号**读出整个表达式（跨行、跨嵌套），
只在那段里面找字符串字面量。抽完抽样核对过，11/11 全是真 class 串。

### 真实存量（已核对）

| | 文件 | class 串 | utility |
| --- | --- | --- | --- |
| `plugins/` | 33 | 238 | 707 |
| `ui/` | 15 | 63 | 91 |
| **合计** | **48** | **301** | **798** |

其中 **22 个文件一行 StyleX 都没有** —— 整个 `chat/composer/` 目录（7 个文件）就是这样，
迁移从来没走到过那里。我之前说「业务层迁完了」，对这个子树是完全不成立的。

**教训不是「我数错了」，是「我用形状猜了三次」。** 现在这个抽取器留在
`/tmp/truth3.mjs`，判据是配平括号 + 上下文，不是正则猜边界 ——
它该不该变成守卫，等存量清零那天再说；现在它的用途是让每一轮的「还剩多少」可信。

## Round 158 — 一个承诺「离开文档流」的组件，从来没离开过

### 实测出来的缺陷

审计 composer 时用浏览器量的（不是读代码猜的）：

```
surfaceOverflow: "hidden"       ← AgentComposerSurface 自己的
probeSitsAboveSurface: true     ← 弹层在 surface 上方（bottom-full）
probeVisibleAtItsCentre: false  ← 它自己中心点画出来的不是它
whatIsPaintedThere: "div panel-scroll msg-scroll-viewport …"  ← 是它后面的消息滚动区
```

**`FileMentionPopup`（`@` 文件提示）被完全裁掉，一个像素都看不见。**

### 根因

`FloatingSurface` 的文档注释写着：「everything that **leaves the document flow** is made of」——
但它渲染的是一个**裸 `<div>`**，没有 portal、没有 positioner。所以它从来没离开过文档流。

它只有两个调用处，两处各自坏法不同：

| | 住在哪 | 结果 |
| --- | --- | --- |
| `FileMentionPopup` | `AgentComposerSurface` 里面（`overflow: hidden`） | `absolute bottom-full` → **被裁掉，不可见** |
| `SlashSuggestions` | `Composer` 的兄弟节点 | 在常规流里，**把 composer 往下推**，不是浮在内容上 |

第二处「能看见」不是因为设计对，而是因为它被放到了裁剪盒外面 —— 一个巧合。
而它俩本该是同一个东西：composer 上方的建议列表。

**这两处都没有任何测试、任何 golden。** 这就是裁剪能活到今天的原因。

### 治本

`ui/primitives/popover.ts` 已经导出 Base UI 的 Popover，`ui/atoms/popover.tsx` 已经有
Portal + Positioner + `FLOATING_PANEL` 的完整路径。缺的两块 Base UI 都有：

- `Positioner.anchor` —— 对任意元素定位，**不需要 Trigger**（这两个面板不是点开的，是打字打开的）
- `Popup.initialFocus={false}` —— 不抢焦点（焦点必须留在 textarea 上，
  因为驱动选中的是 textarea 的 `aria-activedescendant`）

所以：**删掉 `FloatingSurface`**，两处改成锚定 + portal 的浮层。
`FLOATING_PANEL` / `FLOATING_LAYER` 这些**材质**导出保留 —— 它们是对的，
menu / popover / tooltip 都在用；错的只是那个自己渲染 div 的组件。

顺带三件一起对：hand-written 的 `absolute bottom-full left-2 right-2` 坐标不再需要
（定位归 positioner）；`SlashSuggestions` 真正浮起来而不再顶开 composer；
两个面板从此住在同一个地方（`Composer` 里），而不是一个在里一个在外。

**Base UI 的豁免不动**：`fileMentions.ts` 顶部已经写明为什么查询逻辑要手写
（Combobox 把输入框的 VALUE 当查询，而这里的查询是自由文本里的一个 `@token`）——
那个理由是对的，这轮只换**外壳**，不碰查询与键盘逻辑。

### 验收

要新增覆盖 —— 这轮的重点之一就是「它没有测试」：
closure 套件里加两条，断言两个建议面板**在自己中心点画的是自己**（不是被裁掉后面的东西）。

### 验证「这条新测试真的抓得住」

一条改前改后都绿的回归测试什么也没证明。所以我把旧的摆法在同一个 fixture 里复现了一遍，
用同一句断言量它：

```
OLD-WAY {"paintsItself":false,"height":100}
```

元素在、有高度、`getBoundingClientRect()` 一切正常 —— 但**它自己那块地方画出来的不是它**。
这就是为什么断言不能写成「元素存在吗」（它一直存在），必须写成
**「元素所在的位置，画出来的是不是它自己」**。

### 一个仍然盖不住的缺口（已记，未修）

`@` 文件提示面板**在 fixture 里打不开**：`active = open && items.length > 0`，
而 agent fixture 没有接工作区文件的 data provider，所以 `items` 永远是空的。
实测：

```
MENTION {"found":false,"textareaValue":"@","ariaControls":null}
```

也就是说 —— **这个 bug 之所以能活下来，正是因为触发它的那个面板在测试里无法到达。**

这一轮之后两个面板走的是**同一个实现**（`Popover.Anchored`），所以 portal 与锚定
对两者都被覆盖了；查询逻辑本身有单测（`fileMentions.test.ts` mock 掉了 query）。
真正还缺的是「文件提示面板 + 真实 DOM」这一格，它要给 agent fixture 接一个
文件列表 provider —— 那是 fixture 的管线活，不是这一轮的题目，留作下一轮。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **657 / 657**（656 + 新增那条），0 unexpected，0 flaky |
| 守卫 | 17 项全绿 |
| 单测 | 1781 项通过 |
| 删掉的组件 | `FloatingSurface`（材质导出保留） |
| Tailwind 存量 | 301 → **287** 条（48 → 45 个文件；无 StyleX 的文件 22 → 19） |

删组件、两个面板改走 portal、composer 编辑区改成命名档 —— **一张 golden 都没动**。
说明这些改动确实只换了实现，没换外观（唯一预期会变的是 slash 面板从「顶开 composer」
变成「浮在上面」，而它本来就没有任何 golden 拍到过）。

## Round 159 — 把「测试到不了」的那格补上

上一轮末尾记的缺口：`@` 面板在 fixture 里打不开，因为 `active = open && items.length > 0`
而 agent fixture 没接工作区文件的 provider。

fixture 里已经有一段注释把同一种病说得很清楚（讲的是 read preview）：

> without one the preview rendered empty here and **the component that draws it appeared in
> no test**.

同样的病、同样的药：给 agent fixture 加一个 `WORKSPACE_LIST_FILES_KEY` 的确定性 provider。
然后那条本来写不出来的测试就写得出来了 —— 它断言的是**真正坏掉的那个面板**：

| 断言 | 为什么 |
| --- | --- |
| `paintsItself` | 面板所在的位置画出来的是它自己（不是被裁掉后面的东西） |
| `escapedTheClippingSurface` | 它不在 `composer-root` 里面 —— 它真的 portal 出去了 |
| `aboveComposer` | 它坐在 composer 上方 |
| `focusStillInInput` | 焦点还在 textarea，因为驱动选中的是它的 `aria-activedescendant` |
| `selects === 1` | 有且只有一行被标为选中 |

**一句话**：一个「表面无法从 fixture 到达」的 fixture，对那个表面什么也验不出来。
这个 bug 活了这么久，根本原因是它所在的地方测试进不去。

### 下一轮已定位的下一个同类缺陷

`ComposerAttachments.tsx` 里，`Chip` 原子只有**一个**调用处（@ 提及那颗），
而它下面 40 行 `PasteChip` **手搓了第二颗近乎一样的 chip**：

| | `Chip` 原子 | 手搓的 `PasteChip` |
| --- | --- | --- |
| 220px 上限 / 等宽 / pill / `uiSm` / 截断 / 图标 / 关闭 / Tooltip | ✓ | ✓ 全都一样 |
| 填充 | `accentBadge` + 真 border | `bg-surface-2`，**没有 border** |
| 墨色 | `fgSoft` | `fgMuted` |

安静一档可能是有意的（提及是「用户引的东西」，粘贴是「附上的内容」），
但它是**靠重造组件**表达的，而不是靠 `Chip` 的一个 tone。仓库硬规则写着：
「业务层不自己拼交互件：缺档就往库里加一档，别在 callsite 手搓」。

**而且两颗 chip 都没有任何 golden** —— 所以它们能在同一个文件里分叉而没人发现。
这一轮的主题一直成立：**composer 是那个没有测试的子树，缺陷就都在那里。**

## Round 160 — composer 迁完，路上捡到三个真缺陷

### 缺陷一：`Chip` 只有一个调用处，而它旁边 40 行有人手搓了第二颗

| | `Chip` 原子 | 手搓的 `PasteChip` |
| --- | --- | --- |
| 220px 上限 / 等宽 / pill / `uiSm` / 截断 / 图标 / 关闭 / Tooltip | ✓ | ✓ 逐项相同 |
| 填充 | `accentBadge` | `bg-surface-2` |
| 边 | 真 border | **没有** |
| 墨色 | `fgSoft` | `fgMuted` |

安静一档是有意的 —— 提及是「读者引来的东西」，粘贴是「跟着来的内容」 ——
但它是靠**重造组件**表达的，于是那颗副本也顺手丢掉了这套设计里每个固定控件都该有的边。
`Chip` 加一个 `kind`（`reference` / `attached`）与 `closeLabel`，`PasteChip` 整个消失。

### 缺陷二：从选择器里选中一个文件，chip 永远不会出现

`draftMentions` 用 `/(^|\s)@(\S+)/g` 读回草稿来渲染 chip 行 ——
它的单测第一条就叫 **“finds a file the draft attached”**，所以 `@path` 就是「附上一个文件」。

而 `accept` 插入的是 `path + " "`，**`@` 连着查询一起被替换掉了**。
后果：从选择器里选文件（附文件的**主要**方式）产生一个裸路径，
chip 行什么都不显示，读者得不到任何「文件已附上」的确认。
`Chip` 原子那唯一一个调用处，走正常流程根本到不了。

实测（同一个 fixture，改前 / 改后）：

```
改前  STEP2 {"value":"runtime/session/store.go ", "chips":0}
改后  STEP2 {"value":"@runtime/session/store.go ","chips":1,"kinds":["reference"]}
```

`accept` 现在保留 `@`。**这会改变发给模型的文本**（多一个 `@`）—— 但没有任何下游解析它，
而 chip 行、`removeMention`、一整个测试文件三样机制都是为 `@path` 而存在的，
它们全都到不了。所以错的是写入方，不是读回方。

`accept` **之前没有任何单测**，这就是它能这样活着的原因。补了两条，
其中一条在把修复 stash 掉之后确实是红的 —— 证明它抓得住。

### 缺陷三：一个「只有 utility class 才够得着」的逃生口

`globals.css` 里有条降透明度的规则：

```css
[data-slot="button"] svg:not([class*="opacity-"]) { opacity: var(--glyph-step); }
```

注释写着「a call site that has decided on an opacity is excluded **by name**」——
逃生口是**匹配类名子串**。而 StyleX 生成的是 `x1abc…`，
**任何迁移到 StyleX 的调用处都会静默地被重新压暗。**

全仓只有一个调用处在用它（审批模式 pill 的图标，它报的就是状态本身，不能退后）。
改成 `Icon` 自己的 `full` prop，发出 `data-glyph="full"`，规则改按属性匹配。
这条必须**先修再迁** —— 否则迁移会静默改变外观，而 golden 不一定拍到那个 pill。

### 顺带

- 两个 `DropdownMenu.Item` 各自手写了 `grid-cols-[minmax(0,1fr)_14px]`
  以及**行自己的 hover / 圆角 / outline** —— 而 `floatingRow` 早就有一档
  `pickPlain`，模板一模一样（3.5 × 4px = 14px）。改成 `layout="pickPlain"`。
- `space` 阶梯补了 `s14`（56px 缩略图）—— 保留实测值，而不是就近取 `s16` 悄悄改设计。
- `ComposerImageDrop` 的拖放蒙层、`ModelPicker` 的 provider 提示，都变成命名档。

### 我的第四个错数字

存量我报过 902→0、122、429、287 —— 现在是 **136**。
这次错的原因跟前三次都不同：**同一个字符串被数了两遍**。
`className={cn("a","b")}` 会同时被 `className=` 和 `cn(` 两个入口匹配到，
而两次配平读取都覆盖了那两个字面量。按字面量的**绝对偏移**去重之后才是真数。

已经用手数对账过两个文件（清干净的 `toolbar.tsx` = 0；未迁的 `DiffView.tsx` = 6，
六条全是 `cn(...)` 里的真 class 串，字面 grep 一条都看不见）。
**在对账之前我不该再报数字。**

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **659 / 659**（+1 条新增），0 unexpected，0 flaky |
| 守卫 | 17 项全绿 |
| 单测 | **1783** 项通过（+2 条 `accept` 的） |
| CSS raw | 134.6 → **133.7 KB**（预算 142.6 KB） |
| `chat/composer/` 的 Tailwind | 42 条 → **0** |
| 全仓存量 | **136** 条 / 331 utility / 39 个文件（14 个一行 StyleX 都没有） |

零像素位移是**应该**的、也是有解释的：新加了边的那颗 chip、改用 `pickPlain` 的那两行菜单，
**都没有任何 golden 拍到**（这正是它们能分叉的原因）；而审批 pill 的图标
从 `opacity-100` 换成 `data-glyph="full"` 之后，算出来的透明度还是 1 —— 同一个值，两种说法。

## Round 161 — 消息家族：一个状态被推导了三遍，一个事实被说了三种语言

### 缺陷一：`model.status` 在一个文件里被推导了三次

`DelegatedRunDisclosure` 里，同一个 `status`：

| 推导 | 在哪 | 说什么 |
| --- | --- | --- |
| `dotTone` | 模型的 `STATUS_VIEW` 表里 | 圆点的 tone |
| 状态词的墨色 | JSX 里一串**四层嵌套三元** | `text-info` / `text-warning` / `text-negative` / `text-fg-muted` |
| 卡片自己的框 | JSX 里**另一串三元** | `negative` / `warning` / `neutral` |

`STATUS_VIEW` 本来就是「一个状态长什么样」的唯一 owner —— 它只是没拿到后两列。
现在三列都在表里，穷尽覆盖那个 union，JSX 一个三元都没有了。

`ink` 和 `shell` **只在一处不同**（`running`：墨色是 info、框是 neutral），
而这一处正是值得保留的区分：**在跑的委派用 info 说「running」，但它不给自己画框 ——
因为一个正在跑的 run 不是一件要你去处理的事。**

### 缺陷二：一个事实，三种语言

```ts
const ACTIONS_VISIBILITY: Record<…, string> = {
  hidden: "invisible opacity-0",                  // Tailwind
  hover: stylex.props(reveal.shown).className ?? "", // StyleX，被读成字符串
  pinned: "opacity-100",                          // Tailwind
};
```

「操作栏有多可见」是一个事实，三档各用一种写法回答。
最坏的后果是**没人能看出 `opacity-100` 是在压过 reveal 通道、还是只是跟它一致**。
现在三档都是 StyleX 档，`hover` 直接指向设计系统的 `reveal.shown`。

`satisfies Record<…, unknown>` 而不是 `StyleXStyles` —— 检查的是「每个状态都有答案」，
而 `StyleXStyles` **表达不了 `reveal.shown`**：它的 `pointer-events` 是个自定义属性，
CSS 那个属性自己的枚举里没有这种值。

### TypeScript 第三次抓到我造同名冲突

我给 `messageStyles` 加 `body` 和 `actions` 时，TS1117 报了重名 ——
那两个名字**已经存在**，指的是**卡片**的三条带（`head` / `body` / `actions`，各带 `px-4`）。

我差点在自己猎了七轮的缺陷上再添两例。修法是让卡片家族说出自己是卡片
（`cardHead` / `cardBody` / `cardActions` / `cardClip` / `cardPrompt`…）——
在一个叫 `messageStyles` 的模块里，光秃秃的 `body` 最自然的意思就是消息正文，
而它现在确实是了。

### 两个「保留原值、只报告不改」的发现

- **`MessageActionButton` 的圆角**：`round={role === "user"}` 与
  `className={cn(role !== "user" && "rounded-md")}` 是同一个决定的两半。
  而 `--button-radius` 是 `--shape-sm`、`rounded-md` 是 `--shape-md` —— **值不同**，
  所以那个 class 是真覆盖：助手的操作按钮是全产品**唯一**一个戴 `--shape-md` 的控件。
  另外 `radius` 令牌是**按角色命名**的（card / field / row / button），
  这个调用处没有角色可命名 —— 所以我原值保留、不借 `radius.card` 给它安个「卡片」的名分，
  注释里写明这是设计问题不是迁移问题。
- **反馈按钮的 tone**：`className={… ? "text-success" : undefined}` 改成 `tone="success"` ——
  按钮本来就收 tone。顺带把 `ButtonTone` 导出了：一个调用方必须会说的词汇不导出，
  每个包装组件就只能重抄那个 union 或者放宽成 `string`。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **659 / 659**，0 unexpected，0 flaky |
| 守卫 | 17 项全绿 |
| 单测 | 1783 项通过 |
| 存量 | 136 → **113** 条（39 → 35 个文件；无 StyleX 的 14 → 10） |
| CSS raw | 133.8 KB（预算 142.6 KB） |

零位移在这一轮有一层额外含义：那张 `STATUS_VIEW` 三列表**逐像素复现**了原来两串三元的输出。
如果我把 `running` 的 ink 或 shell 抄错一格，659 张里必然有几张会动。

## Round 162 — 一个默认值，21 个调用处里 19 个不同意

### `titleStrong`：名字说的是字重，做的是字体

`ViewHeader` 的 `titleStrong?: boolean`：

```
titleStrong ? "font-sans" : "font-mono"
```

**名字说 weight，实际切 face。** 而它真正编码的事实是：
**这个标题是「读的散文」还是「机器文本」**（一个路径、一条命令）。

数了一遍：**21 个 `WorkspaceViewLayout` 调用处里 19 个都传了这个 flag** ——
也就是说 19 个都在退出那个默认值。而剩下 2 个恰好就是标题真的是机器文本的两个：

| 文件 | 标题 | 该不该 mono |
| --- | --- | --- |
| `file.tsx` | `viewer?.path` | 是 —— 它就是一个路径 |
| `terminal.tsx` | `"terminal.title"`（一个词） | **可疑** —— 见下 |

按前几轮立的判据（「三分之一调用处不同意的默认值不是默认值」），这里是 **19/21 不同意**。
改成 `titleFace?: "prose" | "mono"`，默认 `prose`：19 个调用处**直接删掉这个 prop**，
2 个写明 `titleFace="mono"`。名字现在说的就是它做的事。

**预期会动 golden**：`face.mono` 是 round 153 定的**捆绑档**（字体 + `letterSpacing: 0`），
而原来那个裸 `font-mono` 会继承 `--tracking-ui` —— 也就是等宽字承担了给比例字体设计的字距，
正是 round 153 量出来「40 个字符漂 5.7px」的那个缺陷。所以 `dock-file` / `dock-terminal`
这两组 golden 应该动，而且**这是修好了**，不是回归。

**报告不改**：`terminal.tsx` 的标题是一个词（不是路径）却渲染成等宽。
可能是有意的（跟终端一致），也可能是当年顺手写的。它有 golden 拍着，
所以现值就是被接受的现状 —— 这是设计问题，不是迁移问题。

### dock 的代码视图家族：三个「行号槽 + 代码」的网格

| | 模板 | gap |
| --- | --- | --- |
| `FileView` | `44px minmax(0,1fr)` | 2 |
| `DiffView` 统一视图 | `36px 36px minmax(0,1fr)` | 1.5 |
| `DiffView` 分栏视图 | `34px 16px minmax(0,1fr)` | 1.5 |

槽宽不同是**真的**（一个文件一列行号、统一 diff 两列、分栏是行号 + 符号），
所以收成一个共有的 `lineRow`（`display:grid` + `align-items` + `px-3`）
加三档 `gutterOne` / `gutterPair` / `gutterSign`。

**槽宽保持绝对像素**，并在注释里写明为什么：一列数字的宽度由「要塞进几个数字」决定，
那不是间距节奏的一个档位。（顺带记下一处不一致：`codeStyles.matchRow` 把同样的 44px
写成 `calc(var(--spacing) * 11)` —— 那个是异类拼法，但动它会白白挪一张 golden。）

### `ROW_STYLE`：一张表里三种语言

```ts
added:   { tone: "bg-[var(--color-diff-added-tint)]", meta: "text-[var(--color-diff-added-meta)]" }
context: { tone: "",                                  meta: "text-fg-faint" }
```

任意值 Tailwind、令牌 class、**空字符串**各一份。空字符串那个尤其糟 ——
它把「不着色」表达成「一个什么都不做的 class 名」。现在三档都是 StyleX 档，
context 的 tone 是 `null`，注释说明为什么：**未改动的行是另外两种被读出来的底**。

### 我预测会动的 golden 没动 —— 追下去发现了更大的缺口

我预判 `dock-file` / `dock-terminal` 会动（`face.mono` 带 `letterSpacing: 0`，
而原来的裸 `font-mono` 继承 `--tracking-ui`）。**它们一张都没动。**

不接受一个解释不了的绿，追下去，根因是：

```
if (placement?.placement === "dock") return <DockViewBar … />;   // ← 24 个 golden 全走这条
return <FullViewBar … titleFace={titleFace} />;                  // ← 一张 golden 都没有
```

`titleFace` **只在 full placement 用得到**，而 `VISUAL_WORKSPACE_STATES` 里
**24 个状态全是 `dock-*` 加一个 `settings`**。也就是说 `ViewHeader` 有**一半**
（图标、标题、字体切换、间隔点、sub）从来没被拍过。

又是同一个模式：**没有覆盖的那一半，就是缺陷藏身的地方。**

`navigator().go({ view })` 本来就是「主视图」那个参数（`settings` 走的就是它），
所以补一个状态只要六行：加 `"full-view"` 到状态表 + 让 installer 把 `view` 指向一个视图 id。
选 `search`（散文标题 + 有 sub），因为那正是 19/21 个调用处走的默认路径。

spec 里有一道**穷尽门禁**（「an added state must declare its own ready boundary」）
立刻把我拦了下来 —— 这条设计是对的，新状态必须自己说清楚「什么时候算就绪」。

### 三张 unexpected，两种归因

| | 隔离重跑 | 结论 |
| --- | --- | --- |
| `agent light tool-search` | **通过** | flake，不动 |
| `workspace light/dark dock-error` | **两次都红** | 真变化 |

`dock-error` 的 diff 图看得很清楚：动的**只有 fixture 自己的状态选择侧栏** ——
列表多了一行「Full view」，而 `dock-error` 在列表最底部附近，
滚动落点就跟着变了。产品区域（转录、dock、错误面板）一个像素没动。已重录。

**顺带记一处脆弱**：fixture 的脚手架侧栏进了 golden，所以**每加一个状态都会挪到别的 golden**。
fixture 自己的注释早就警告过同一类事。这不是这一轮该改的，但值得记着。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **667 / 667**（659 + 8 条新增），0 unexpected，0 flaky |
| 守卫 | 17 项全绿 |
| 单测 | 1783 项通过 |
| 重录 | 2 张（`dock-error` 两个主题）—— 只因 fixture 侧栏多了一行 |
| 新增 golden | 2 张（`full-view` 两个主题）—— `FullViewBar` 第一次被拍到 |

## Round 163 — 设计系统自己的最后一块，以及我亲手造的一个静默回归

### 先纠正我自己的一个判断

我把 `ui/agent/` 里 4 个文件标成「一行 StyleX 都没有」。**其中 3 个已经是终态** ——
`sidebar.tsx` / `content-card.tsx` / `surface-header.tsx` 里全是 `globals.css` 拥有的
**机制键**（`agent-drawer` / `agent-seam-rail` / `agent-content-card` /
`agent-surface-header` / `agent-surface-divider` / `agent-dock-control`）。
这些是有意留在 CSS 里的：窗口 chrome 的接缝与玻璃是**后代规则**，StyleX 表达不了。

顺手对这些机制键做了一次「有没有对应规则」的核查（一个没有规则的 class 名是静默空操作）：
23 个键里 21 个 grep 到了，`media-edge` / `media-edge-on-scrim` 两个没有 ——
但那是我的 grep 错了，它们是 Tailwind v4 的 `@utility` 声明。**代码没问题，我的检查有问题。**

### 但这次核查顺带抓到一个真的

`ThemeSection.tsx` 的注释写着：

> `media-edge` stays a utility because it is a mechanism `globals.css` owns —
> an inset outline the scheme picks — and **the swatch only asks for it**.

而紧挨着的代码**没有**去要它，它把那个 utility 的内容原样又写了一遍：
`outline: "1px solid var(--color-media-edge)"` + `outlineOffset: "-1px"`。
「图片戴什么边」于是有了两个 owner，而注释还在说只有一个。
（这是 round 148 那条的镜像：注释写在代码之前是假陈述，
**写在代码之后没跟着改也是**。）

### `JumpToBottomButton`：一个 Tailwind 能叠、CSS 不能叠的属性

```
"absolute bottom-[calc(100%+0.5rem)] left-1/2 -translate-x-1/2 z-3"
visible ? "translate-y-0 …" : "translate-y-1 …"
```

`-translate-x-1/2` 与 `translate-y-*` 在 Tailwind 里能共存，**只因为它们各写一个自定义属性**。
而 CSS 里 `translate` 是**一个**属性：两条声明不合并，后一条赢，
按钮会丢掉那半个自身宽度的居中、往右跳。
所以迁移后每个状态都要**同时说两个轴**：`-50% 0` / `-50% 4px`。

### 一个单测冻住了 class 名（同一个文件里已经记过同样的教训）

`GoalStatusSurface.test.tsx` 断言 tray 的 className 含 `rounded-t-composer` / `border-x` /
`border-t`、不含 `mb-2`。而它上一个 `it` 里已经写着：

> Asserting the utility that produced them **froze a spelling** … this failed while the row
> was pixel-identical.

真正的契约是它上一行那句 `data-slot="composer-top-tray-surface"`（用的是**共享的** tray，
不是手搓的）。那四个 class 名想说的是一个**几何事实** ——
「tray 与 composer 是一个面，两者之间没有第二条线」—— 那就该到有 CSS 的地方去量。

量出来的真实设计跟我以为的还不一样：tray 的盒子**越过** composer 顶边 5px、
**塞在它后面**，自己那 27px 的下内边距把内容让开这段重叠。
所以断言不是「两条边相接」，而是「**tray 绝不缩回去、在两个面之间留出一条线**」。

### 我亲手造了一个静默回归，被自己刚写的那条测试抓住

迁移 tray 时我写了 `borderInlineWidth: "1px"`。**实测**：这个元素算出来的
`border-left-width` 是 **0px**；换成 `borderLeftWidth` / `borderRightWidth` 之后是 1px。

我没有查明那个逻辑简写为什么没生效（产物里能 grep 到 `border-inline-width`，
但那份产物早于我这次修改，也可能是 Tailwind 自己的 `border-x` 留下的）——
**查明的是：没有任何东西报错，唯一注意到的是一条把计算值读回来的测试。**

顺手清了一遍全仓的逻辑属性：`paddingInline`(135) / `paddingBlock`(99) /
`marginInline`(9) / `marginBlock`(8) / `insetInline`(5) 都在正常工作
（135 处用了它、656 张 golden 稳定，这就是证据），只有那一处简写是坏的，已修。

### 那个静默回归的真根因，比「一个简写没生效」深得多

视觉套件报了 3 张 unexpected（都在 tray/`empty`，正是我刚迁的那个面），
其中一条是**命名测试**，给出了确切数字：`composer.y - tray.y` 期望 37、实测 55。

把新旧两版在同一个 fixture 里各量一遍：

| | `border-top` | `margin-bottom` | 高 |
| --- | --- | --- | --- |
| **迁移前** | **0px** | **-18px** | 59 |
| 迁移后 | 1px | -1px | 60 |

**迁移前那两个值，component 自己一个都没说。** 它写的是
`border-x border-t border-[var(--composer-tray-edge-color)]` 和 `-mb-px`。
真正在生效的是**调用方**：`ProjectSelector` 的 `pl.traySurface` 里写着
`borderWidth: 0`、`marginBottom: "-18px"`，还换了另一个背景色
（`--app-composer-project-tray-surface`，不是 component 的 `--app-composer-tray-surface`）。

**为什么以前看不见**：调用方是 StyleX、component 是 Tailwind ——
生成类带 `:not(#\#)`，**任何工具类都压不过它**。所以调用方一直在静默地
取消 component 的边、换掉它的填充，而没有任何东西说出这件事。

我把 component 的材质也迁成 StyleX 之后，两边**平级**了，于是边回来了 ——
而决定谁赢的是 `cn()`：**它把两串各自生成的 class 拼在一起，
优先级就交给了样式表顺序**。这个竞争没有正确答案。

### 所以正确的修法不是「让 component 输」

而是：**component 只拥有两个调用方从未分歧的那部分。**

两个调用方长得根本不像 —— Goal tray 有边、有 backdrop 滤镜；项目 tray 没有边、
另一个填充、另一个内缩、另一个宽度（`calc(100% - 24px)`）。
所谓「共享的面」只共享一个名字。真正共享的只有**形状**：
`position: relative`、`overflow: clip`、以及**composer 自己的上圆角**。

- component：留形状，`data-slot` 仍是契约
- Goal tray：材质搬到它自己名下（外观逐像素不变，因为那本来就是它一个人的样子）
- 项目 tray：`borderWidth: 0` 和那两行重述的圆角**删掉** —— 已经没有东西要取消了

那条 closure 测试也跟着改对了目标：原来我照着 component 的边去断言，
而项目 tray **根本没有边**。现在断言的是两者真正都同意的：
composer 的圆角、越过顶边的重叠、缺席的底边、`overflow: clip`。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **669 / 669**（667 + 新增 2），0 unexpected，0 flaky |
| 守卫 | 17 项全绿 |
| 单测 | 1783 项通过 |
| 存量 | 96 → **89** 条 |
| **`plugins/` 里「一行 StyleX 都没有」的文件** | 6 → **0** |
| CSS raw | 133.2 KB（预算 142.6 KB） |

`plugins/` 下已经没有纯 Tailwind 文件了。`ui/` 还剩 3 个，
而其中的 `sidebar.tsx` / `content-card.tsx` / `surface-header.tsx` 是**机制键包装器**，
本来就是终态 —— 换句话说，**"没有 StyleX" 这个指标已经走完了**。
剩下的 89 条是散落在混写文件里的单条，得一处一处看。

## Round 164 — 换个切法：按「同一个事实被写了几遍」排，而不是按文件

「没有 StyleX 的文件」这个指标走完了（`plugins/` 已经是 0）。剩下 89 条散在混写文件里，
按文件推进已经没有杠杆了。改成按**重复度**排 —— 把剩下的 utility 拆成 token 数一遍，
排在最前面的是 `-rotate-90` ×3 配 `transition-transform` ×3。

顺着这个线索查全仓，「**一个 chevron，开着朝下、关着朝右**」这一个事实
写在 **8 个地方、4 种拼法**：

| 拼法 | 在哪 |
| --- | --- |
| `{color: fgFaint, transitionProperty: "rotate"}` + `{rotate: "-90deg"}` | `viewStyles`、`TracesPanel`（**逐字相同的两份**） |
| `{rotate: "-90deg"}` 单档 | `activity-disclosure`、`MessageContextMenu` |
| `{flexShrink: 0, rotate: "-90deg", color: fgFaint}` 永久转 | `select-trigger` |
| `cn("shrink-0 transition-transform", !open && "-rotate-90")` | `FileTree`、`ReviewFileTree`、`diff.tsx` |

**这 8 处之间的差异从来不是决定**，只是当时手边有什么就写了什么。
收成 `ui/atoms/chevron.ts` 两档：`base`（守住宽度 + 只动 `rotate`）和 `shut`（`-90deg`）。

**墨色没有进来** —— 树的 chevron 是 faint、菜单的继承所在行、diff 头部按 `--glyph-step` 退一步。
这才是它们真正有分歧的地方，所以留给调用处说。

注释里记了一条：用 `rotate` 而不是 `transform` —— 它们是**两个属性**，
同一个字形上若还有别的 transform（比如按下时的缩放），写成 `transform` 会互相替换而不是叠加。

**顺带清掉一处死代码**：`MessageContextMenu` 的 `mc.submenu` 零引用，
删掉 chevron 那档之后整个 `stylex.create` 块就空了。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **669 / 669**，0 unexpected，0 flaky |
| 守卫 | 17 项全绿 |
| 单测 | 1783 项通过 |
| `rotate: -90deg` 的 owner | 8 处 4 种拼法 → **1** |

## Round 165 — 两个渐隐遮罩，一个都看不见

按重复度排，`ReasoningBlock` 的两个渐隐层占了剩余 83 条里的 **10 条**，
而且两者近乎镜像（`to_bottom`/`top-0` 对 `to_top`/`bottom-0`）—— 一个事实写了两遍。

### 先发现：同一个意图，仓库里已经有另一套机制

| | 做法 | 依赖 |
| --- | --- | --- |
| `truncate-fade`（`globals.css`，横向） | **mask** —— `mask-image: linear-gradient(...)` | 不依赖背后是什么 |
| `ReasoningBlock`（纵向） | 两个绝对定位的渐变**覆盖层** | **硬编码 `var(--app-content-surface)`** 当不透明端 |

第二种一旦这个块坐在别的面上，渐隐就会画错颜色。

### 然后实测出：那两层一个都看不见

```
OVERLAY {"offsetBeforeScroll":0,"offsetAfterScroll":-200}
```

`position: absolute; top: 0` 在 `overflow-y: auto` 里锚的是**滚动内容的原点**，不是可视上沿。
滚 200px，覆盖层就跑到可视区外 200px。

而 `showTopFade = isOpen && edges.scrolled` —— **它恰好在滚动之后才打开，
而滚动正是把它移出视野的那个动作。** 底部那层同理：`bottom: 0` 落在全部内容的末尾，
而 `showBottomFade` 要求 `!atBottom`（末尾在视口下方）—— 也就是它可见的条件与它被启用的条件互斥。

**两层渐隐、两个 DOM 节点、一个滚动监听喂两个布尔值 —— 没有一处能被看到。**

并排量给出决定性对比：

```
COMPARE {"before":{"overlayOffset":0,"maskStart":"0"},
         "after" :{"overlayOffset":-200,"maskStart":"0"}}
```

覆盖层跑了 200px，**mask 一动不动** —— 它是对着元素自己的盒子画的。

### 治本

改成 scroller 自己身上的一层 mask，两个停点由 `--fade-top` / `--fade-bottom` 驱动。
去掉了：两个 DOM 节点、颜色依赖、`z-1` / `pointer-events-none` / `absolute` 定位，
以及 10 条 class 串。

### 这个缺陷为什么能活下来：fixture 里那段推理只有一句话

窗口是 `max-height: 12rem`，而 fixture 的推理文本渲染出来只有 **21px** ——
**没有任何状态能让那个窗口滚动起来**，所以两层渐隐从来没有被拍到的机会。

把 fixture 的推理文本加长到超过窗口（192px 窗、231px 内容），
这才第一次有了「流式推理窗口装不下」这个状态。新的 closure 测试断言**两条边**：

| | `--fade-top` | `--fade-bottom` |
| --- | --- | --- |
| 刚打开（顶部无遮挡） | `0px` | `24px` |
| 滚动之后（两边都有遮挡） | `24px` | `24px` |

**测试里踩了一次 round 138 的坑**：滚动之后同一个 `evaluate` 里同步读值，
读到的是 React 重渲染之前的旧值 —— 看起来像渐隐坏了，其实是我读早了。
改成 `expect.poll`，并把这条写进注释。

### 又一个单测按 Tailwind class 找元素

`ReasoningBlock.test.tsx` 用 `.overflow-y-auto` 找滚动窗口。
它测的是「窗口能被键盘到达」，而 `overflow-y-auto` 只是当时的拼法。
改成 `data-slot="reasoning-scroller"` —— 顺带这个 slot 也让 closure 套件够得着它。
（同一个文件上一条 `it` 里已经写着「The slot, not the class」。）

### 一个我该自己抓住的错

`0956f4fe` 那次提交**把一个一次性探针文件 `__probe.visual.spec.ts` 一起提交了**。
它匹配 `*.visual.spec.ts`，所以从那之后它**作为套件的一部分在跑** ——
也就是说 round 163/164 报的 669 里有 1 条是这个探针的。数字没有让结论作废，
但它确实不是我以为的那个数。已删除。

根因是我 `git add desktop/frontend/visual`（整个目录），而不是只加我打算提交的文件。
**探针的生命周期必须比它回答的那个问题短。**

### 渐隐第一次真的画出来了

重录 `answer-opening` / `waves` 四张之前，先看了实际截图确认改动就是我预期的那一处：
推理块变成一段被窗口截住的长文本，而**最后那一行明显地淡出去了** ——
这个功能自写下来第一次被画出来。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **669 / 669**，0 unexpected，0 flaky |
| 守卫 | 17 项全绿 |
| 单测 | 1783 项通过 |
| 重录 | 4 张（`answer-opening` / `waves` 两个主题）—— fixture 推理文本加长 |

669 这个数这次是**干净的**：删掉探针（-1）、加上推理渐隐那条（+1），
跟上一轮的 669 正好对上。

### 存量精确分类（不再给毛数）

71 条里：

| | 条数 | 说明 |
| --- | --- | --- |
| **真 Tailwind** | **37** | 15 个文件，还要迁 |
| `globals.css` 机制键 | 28 | 后代规则 / 接缝 / 遮罩 / shiki —— StyleX 表达不了，**设计上的终态** |
| 根本不是 class | 6 | 枚举值、prop 值，被扫描器扫进来的 |

## Round 166 — 目标改了：不是迁完，是把 Tailwind 拿掉

用户把目标说清楚了。移除 Tailwind 不只是 class 串，是**六层**：

| 层 | 现状 | 移除要做什么 |
| --- | --- | --- |
| ① utility class | **本轮清零** | 迁完 |
| ② `@theme inline`（115 行） | 几乎全是 `--color-x: var(--color-x)` 这种**别名**，只为把令牌塞进 Tailwind 命名空间 | 让 `tokens.stylex.ts` 直接指底层名，整层删 |
| ③ `@utility` / `@custom-variant` | 2 个 utility、2 处变体（TSX 里 0 处） | 改普通 CSS |
| ④ **Tailwind 自己的 theme 默认值** | 设计在**静默消费**它们 | 见下 |
| ⑤ `tailwind-merge`（`cn` 里） | 只认 Tailwind 类名，utility 清零后是空转 | `cn` 变纯拼接 |
| ⑥ Vite 插件 + devDep | — | 最后拆 |

### 第 ④ 层：量出来只有 6 个，但其中一个是 89 处

扫「被读到但我们从没定义过」的自定义属性，32 个里大部分是运行时写入的
（`--reveal` / `--fade-top` / `--sidebar-width`）或 Base UI 的（`--available-height` / `--anchor-width`）。
真正由 Tailwind 提供的只有 6 个：

| 属性 | 读取处 | 影响 |
| --- | --- | --- |
| **`--spacing`** | **89** | `space.*` **每一档**都是 `calc(var(--spacing) * N)` |
| `--tracking-normal` | 3 | |
| `--animate-spin` / `--animate-pulse` | 3 | |
| `--shadow-md` | 1 | 之前报过：`MarkdownImage` 用的是 Tailwind 默认阴影而非设计的深度模型 |
| `--leading-normal` | 1 | |

（我一开始怀疑 `--ease-out` 也是 Tailwind 的 —— **查了，它是我们自己定义的**，虚惊。）

### 这次扫描顺带抓到一个静默失效

`ImagePreviewGallery` 的浮动控件条写着 `boxShadow: "var(--shadow-floating)"` ——
而 **`--shadow-floating` 在整个仓库里没有定义**。`var()` 没有 fallback 会让这条声明
在计算值阶段失效，所以那条控件条一直是 `box-shadow: none` ——
**全产品唯一一个从来没解析成功的阴影名**。改成 `--shadow-overlay`（搜索浮标用的那一档，
同样是「浮在内容上的一条」）。

### ① 层清零的路上，最后一处重复

两棵文件树（`FileTree` / `ReviewFileTree`）各自写了一遍同一个行 ——
全宽、hover wash、选中填充 —— 只在高度和内缩上不同。收成 `viewStyles.treeRow`
加两档（`treeRowTall` / `treeRowInset`），字号留给各自。

`catalog-picker` 的 `data-empty:p-0` / `data-empty:hidden` 是 Tailwind 的
**data 属性变体语法**，StyleX 原生就能说：`{ ":is([data-empty])": 0 }` ——
而且说在一个工具类压不过的特异性上。

### 守卫抓到一条我造出来的死规则

`TurnRail` 改用 `corner.pill` 之后，`globals.css` 里那条
`.rounded-full, .rounded-pill, .type-caret { corner-shape: round }` 的前两个选择器
**再也匹配不到任何元素** —— `check:styles` 直接报了出来。

这条规则的来历 `tokens.stylex.ts` 里写着：superellipse 在 pill 半径下是圆角方而不是圆，
所以 pill 要опт出来，而那个 opt-out **当年是挂在 Tailwind 的类名上的**。
现在 `corner.pill` 把它和半径捆在一起了，两个 Tailwind 选择器删掉，
`.type-caret` 留下 —— 它是 markdown 的，由 rehype 加上，没有组件可以捆。

### 「① 层清零」是错的 —— 我的扫描器只读 `src`

守卫报 `.rounded-pill` 死规则，我删掉它和 `.rounded-full` 之后视觉套件红了 12 张，
集中在 shell 与 Retina hairline。看 diff 图，动的是那个 **40px 空态图标** ——
正是 `tokens.stylex.ts` 里那段注释点名的「唯一一个大到 golden 能看出来的圆」。

浏览器里量：

```
ROUNDED {"count":1,"samples":[{"cls":"grid h-10 w-10 place-items-center rounded-full …",
                               "shape":"superellipse(1.5)"}]}
```

**还有一个元素在戴 `rounded-full`**，而它在 `visual/VisualShellFixture.tsx` 里 ——
**我的扫描器从头到尾只读 `src`**。fixture 里还有 **68 条** class 串。

`check-dead-styles` 有**同一个盲区**：一条只被 fixture 用着的规则，它报成死规则。
所以那不是守卫误报，是守卫和我共享同一个错误的作用域假设。

fixture 是脚手架不是产品 —— 但它走同一条 Vite 管线，
**只要它还说 Tailwind，Tailwind 就拆不掉**。所以这一波把 fixture 一起迁了：
`visual/fixtureStyles.ts` 一个模块管四个文件（fixture 的职责是「稳住」，不是长词汇）。

空态图标那一档特意留了注释：它是产品里唯一一个 golden 能看出角形状的元素，
所以它必须用 `corner.pill` 从 `superellipse(1.5)` 里退出来。

**`src` + `visual` 现在都是 0 条。**

### 迁 fixture 时撞出一个存在已久的管线缺陷

fixture 迁完之后视觉套件 **117 张红**。看 diff：动的只有 fixture 自己的状态侧栏，
每行整体偏移 16px；产品区域一个像素没动。

一步步量下去：

| 量到的 | 说明 |
| --- | --- |
| 侧栏所有样式（字号/内缩/gap/行高）**逐项相同** | 样式没变 |
| 滚动落点差 **16px** | 变的是列表高度 |
| 那个 16px 的间隔 `div`：`min-height: auto`、高 0 | `fx.footGap` **没生效** |
| 它有 StyleX 类名 `xyz5y6m` | Babel 那一遍**跑了** |
| 递归搜遍所有样式表：**该类名没有任何规则** | PostCSS 那一遍**没跑** |

根因在 `postcss.config.mjs`：

```
include: ["src/**/*.{ts,tsx}"]        ← PostCSS 只读 src
include: /\.tsx?$/                    ← Babel 读每个文件
```

**两遍的作用域不一致。** 而 `stylex.babel.mjs` 的注释一字不差地警告过这件事：

> Configure them separately and they diverge silently — the build succeeds, the JS carries
> class names, and **no rule ever defines them, so the component renders naked**.

写着这条警告的那一对，自己就是分叉的。

**这不是我引入的**：`visual/` 下任何 StyleX 定义**从来就不生效**。
换句话说，fixture 一直全是 Tailwind **不是选择，是 StyleX 在那里根本不能用**。
把 `visual/**/*.{ts,tsx}` 加进 PostCSS 的 include，两遍就一致了。

### 117 → 12 → 0，四类失败，四种归因

修好管线后还剩 12 张。全部隔离重跑，**12 张都能复现，没有一张是 flake**。逐类查清：

| 失败 | 根因 | 判定 |
| --- | --- | --- |
| foundation ×4 | 大写标签与等宽文字的字距 | **我自己犯的 round 148 那个错**，见下 |
| dock-review ×2 | diff 文件头是等宽文字 | round 153 的规则到达新地方，**是修好了** |
| tool-shells ×2 | `ToolCard` 的 meta 是等宽 | 同上 |
| Markdown lightbox ×2 | 控件条**第一次有了阴影** | `--shadow-floating` 那个修复的证据 |
| Retina closure ×2 | 随上面几处一起 | — |

#### 我在 fixture 映射里又犯了一次「顺序是唯一的裁判」

我把 `typeStep.*` 放在了自己那些档**后面**：

```
fx.specimenLabel, typeStep.uiXs      ← tracking-wide 被 typeStep 的字距覆盖
fx.mono, fx.faint, typeStep.uiSm     ← face.mono 的 letterSpacing: 0 被覆盖
```

**类型档也声明 `letter-spacing`**，composed 在后面就把前面那个换掉了 ——
大写标签失去宽字距、等宽文字背上比例字体的字距。产品代码里的约定正好相反
（`..., typeStep.X, face.mono`），我照抄产品的顺序就对了。已在 `fx.mono` 旁写明为什么。

#### 而「等宽字距」这一类改动是**对的**

`--text-ui-xs--letter-spacing` 就是 `--tracking-ui`。所以
`font-mono text-ui-xs`（diff 文件头、`ToolCard` 的 meta）一直让等宽文字
背着为比例字体设计的字距 —— 正是 round 153 量出「40 字符漂 5.7px」的那个缺陷。
换成 `face.mono` 之后归零。这几张该重录。

### fixture 里那个 78px 内缩，从来没生效过

foundation 的 logo 位移 58px，查出来是：

```
.agent-surface-header { padding-inline: var(--density-column-gutter) }   ← 无图层
pl-[78px]                                                                ← @layer utilities
```

**无图层规则压过任何图层。** 所以 fixture 那句 `pl-[78px]` 一直是死的，
而我把它迁成 StyleX（也是无图层、且带 `:not(#\#)`）会让一条死覆盖**活过来**。

这跟 tray 那次是同一个机制，也是这次「拆 Tailwind」最大的系统性风险：
**每一条曾经静默输给无图层 `globals.css` 规则的 utility，迁成 StyleX 之后都会反过来赢。**
golden 记录的是它输的那个样子，所以正确的处理是**删掉这条覆盖**，不是让它赢。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **669 / 669**，0 unexpected，0 flaky |
| 守卫 | 17 项全绿 |
| 单测 | 1783 项通过 |
| **utility class（`src` + `visual`）** | **0** |
| 重录 | 24 张，四类全部有归因 |

Tailwind 拆除的第 ① 层完成。剩下 ②–⑥ 层：`@theme inline` 的 115 行别名、
`@utility` / `@custom-variant`、Tailwind 自己的 6 个 theme 默认值（`--spacing` 占 89 处）、
`cn` 里的 `tailwind-merge`、Vite 插件与 devDep。

## Round 167 — 第 ②③⑤ 层：89 个声明里 68 个是别名

### `@theme inline` 的 89 行，逐条分类

| | 条数 | 处理 |
| --- | --- | --- |
| **自引用**（`--color-accent: var(--color-accent)`） | 26 | 直接删 —— 主题作用域里已有真值，这行只是把名字注册进 Tailwind |
| **真正的改名**（`--color-fg → --color-text`、`--radius-lg → --shape-lg`） | 42 | 消费方改指底层名，然后删 |
| 有真实值（wash / badge / 动画简写 / 两档零字距） | 21 | 搬进普通 `:root` |
| 谁都没读过 | 12 | 只为生成 utility 而存在 |

**68 个是另一个变量的裸别名** —— 一个已经有名字的事实，第二个名字，
存在的唯一理由是 Tailwind 要求它的 theme 键必须拼成 `--color-*` / `--text-*` / `--radius-*`。

那些别名发明的名字**没有丢** —— 它们在 `tokens.stylex.ts` 里，
那才是组件给决定命名的地方。丢掉的是每个值的**第二份拷贝**。

39 处读取改指了底层名，`@theme` / `@theme inline` 两个块都没了。

### 第 ③ 层顺手

`@utility media-edge` / `media-edge-on-scrim` 是 Tailwind 的注册语法，改成普通类规则。
两条 `@custom-variant dark/light` —— **一个使用者都没有**，删。

### 第 ⑤ 层：`cn` 不再消解冲突，因为没有冲突可消

`cn = twMerge(clsx(...))`，而 `extendTailwindMerge` 配的三条阶梯和一条 override
**全部是关于 utility class 之间谁压过谁**。utility 清零之后，到这里的每个字符串
要么是 StyleX 已经合并过的类列表，要么是 `globals.css` 拥有的机制键 —— 没有可消解的。

那份配置值得记下来而不只是删掉，因为每一行当年都是一个静默 bug：
Tailwind Merge 把 `text-ui-md` 读成**颜色**、丢掉旁边的墨色；
它假设字号 utility 自带行高（我们的不带），于是每个按钮的 `leading-tight` 被丢掉；
没注册的档不跟任何东西冲突，`cn("leading-body", "leading-prose")` 两个都留、
让样式表顺序决定。这些都写进了 `cn` 的注释。

`tailwind-merge` 依赖去掉；`check-design-tokens` 里那段守着这三条阶梯的检查也去掉 ——
它守的机制已经不存在了。

### 我的扫描器第五次漏了一种形态

`check:utilities` 报 `ReasoningBlock.tsx:110` 的 `border-field` 没有规则 ——
它在 **`contentClassName`** 里。我的扫描器只认 `className` 和 `cn(`。

全查一遍：`contentClassName` / `scrollClassName` 底下还有 **20 条**。

其中 **`scrollClassName="py-1"` 出现 14 次**（21 个视图里的 14 个）——
又是「所有调用处都在传同一个值」。改成 `scrollInset?: "rows" | "flush"`，
默认 `rows`；剩下 7 个（文件、终端、树 —— 内容自带内缩）写 `flush`。

`contentClassName="py-1.5"` 3 次，而 15 个调用处里 9 个什么都不传 ——
所以它不能当默认，但 3 次正好是仓库自己那条「3+ 才抽象」的线，
于是成了 `contentInset?: "rows"`，opt-in。

### 第六个盲区：class 串藏在应用层模型里

`dock-files` 的两个变更徽标掉了颜色。查下去：

```ts
// fileChangesViewModel.ts —— 应用层
add: { className: "text-success", letter: "A" },
del: { className: "text-negative", letter: "D" },
mod: { className: "text-warning", letter: "M" },
```

**一个 Tailwind 类名，在应用层的视图模型里决定的。** 我的扫描器只看 `.tsx` 的
`className` / `cn(`，这在 `.ts` 里、还是个对象字段，六个入口一个都没覆盖到。

而 `toneInk` 的文档里就写着这个缺陷：

> five call sites had each written their own `Record<…, string>` of `"text-negative"` and
> friends, **which is how one of them ended up mapping a domain word straight onto a
> utility class**.

这就是第六处，一直还在。**而它现在是死的** —— `--color-warning` 从 theme 块里删掉之后，
`text-warning` 不再生成，徽标就悄悄失色了，没有任何东西报错。

治本：模型发出的是 `Tone`（领域词），视图用 `toneInk` 把它变成墨色。
既修好了颜色，也把「UI 类名出现在应用层」这个层级违规去掉了。

顺手全仓扫了一遍 `.ts` 里的 class 串 —— **只有这一处**。

### 我在这一波里自己造了一个 bug

`contentInset` 我写成了直接塞进 `cn()`：

```jsx
className={cn(stylex.props(…).className, contentInset, contentClassName)}
```

`contentInset` 是字符串 `"rows"`，被当成类名输出了 —— 而 `styles.bodyRows` **从未应用**，
于是那三个调用处**丢掉了它们的 `py-1.5`**。`tool-shells` / `answer-opening` 四张 golden
就是这么红的。TS 不会报（`cn` 收 `ClassValue`），只有 golden 说了话。

## Round 168 — 第 ④⑥ 层：preflight 与最后的依赖

### 先量 preflight 到底给了什么

从产物里把 `@layer base` 整段抠出来读完。承重的是第一条：

```css
*, ::before, ::after, ::backdrop { box-sizing: border-box; border: 0 solid; margin: 0; padding: 0 }
```

**`border: 0 solid` 是为什么一个组件只写 `border-width` 就能得到一条实线边。**
删掉它，产品里每一条边同时消失。

其余是标准的现代 reset：标题的字号字重回到 inherit、`a` 继承颜色、
`ol/ul` 去掉列表符、`img/svg` 变 block、表单控件 `font: inherit` 与透明背景、
`table` 折叠边框、`textarea` 只竖向缩放。

### 我们自己的 reset：转写，不是改良

原样照抄 Preflight 的**值**，因为「换掉 import」必须一个像素都不动。
**没有**照抄的是它为这个应用永远不会跑的浏览器做的归一化 ——
`-moz-*`、`::file-selector-button`、十二条 `::-webkit-datetime-edit-*`、
`optgroup`、`progress`、搜索框装饰。这个应用只在一个 WebKit webview 里跑、
只渲染一套标记（量过：`p` / `code` / `label` / `th` / `td` / `pre` / `button` /
`table` / `h1-h6` / `ul` / `li` / `strong` / `details` / `summary` / `input` / `textarea` / `img`）。

### Tailwind 自己那 6 个 theme 值

`--spacing` 是要命的那个：`space` 每一档都是它的倍数，少了它整个产品同时失去节奏。
其余各被读一两次 —— 两个动画（重连字形、connecting 徽标）、
`--shadow-md`（markdown 图片）、`--tracking-normal`（一个退出 UI 字距的占位符）。
连同 `@keyframes spin` / `pulse` 一起写进 `globals.css`。

### 拆掉

- `@import "tailwindcss"` → 我们自己的 reset
- `vite.config.ts` / `vite.visual.config.ts` 的 `tailwindcss()` 插件
- `package.json` 的 `tailwindcss` 与 `@tailwindcss/vite`

`npm ls tailwindcss` → **empty**。`tailwind-merge` 只剩 `streamdown` 的传递依赖，不是我们的。

### 移除 `@import` 之后 162 张红 —— 而根因只有三个类名

diff 图一眼就说明白了：**`sr-only` 的那些输出全都显示出来了**，
挤走了整个布局。

`sr-only` **是 Tailwind 自己的 utility**，不是我们的。我把它列进「`globals.css` 拥有的机制键」
——**列错了**。同样列错的还有 `@container` 和 `empty:hidden`。

写了个专门的检查（把源码里所有 className token 拿去跟我们三张样式表里定义的类名对账），
结果只有三个真孤儿：

| 类名 | 用了几处 | 归属 |
| --- | --- | --- |
| `sr-only` | 14 | Tailwind 的 |
| `empty:hidden` | 3 | Tailwind 的变体语法 |
| `@container` | 1 | Tailwind 的 |

后两个是**单个属性**，StyleX 原生就能说（`":empty"` 伪类、`containerType`），
所以它们变成档而不是类。

`sr-only` 不一样：它是**一段配方**（七条声明），14 个调用处，
而且是无障碍机械而非设计词汇 —— 所以它留在 `globals.css` 当类，
跟 `panel-scroll` / `truncate-fade` 一样。round 149 试过把它翻译成本地档，
结果造出第二个 owner，当时就退回了；这次的区别是**它现在真的没有别的主人了**。

顺带发现同一个文件里两个 `stylex.props()` 被 `cn` 拼在一起 ——
StyleX 只在**一次调用内**解析优先级，拼接就把优先级交给了样式表顺序。合成了一次调用。

### 第七个盲区，也是最大的一个：转录的整个节奏是一张 Tailwind 类名表

修好 `sr-only` 之后还剩 123 张。这次 diff 很窄：助手那条消息**整体上移 16px**。

走 DOM 祖先链一个个量，第三层就是它：

```
{"cls": "xvqjf64 xxebm3f mt-4", "mt": "0px"}
```

**一个字面的 `mt-4`，算出来的 `margin-top` 是 0** —— Tailwind 不再生成它了。

来源是 `renderUnitRhythm.ts`：

```ts
const SEAM: Record<UnitVoice, Record<UnitVoice, string>> = {
  process: { process: "mt-1.5", prose: "mt-5", panel: "mt-4" },
  prose:   { process: "mt-5",   prose: "mt-3", panel: "mt-4" },
  panel:   { process: "mt-4",   prose: "mt-5", panel: "mt-3" },
};
```

**转录的整个垂直节奏，一张 3×3 的 Tailwind 类名表，写在应用层。**
加上 `MessageStream` 的 `TURN_GAP`（`mt-1` / `mt-4`）。

这个模块的设计其实是对的，注释写得很好：

> Keyed on the PAIR, because a seam is a relationship and **neither side knows the distance
> alone**. This table is the ONLY owner of the distance.

错的只是它**用 Tailwind 的字母表说那个距离**。治本和 `toneInk` 同形：
**应用层拥有「关系」，视图层拥有「距离」。**

`SEAM` 现在发出四个接缝名（`tight` / `close` / `apart` / `wide`），
`messageStyles.seamStep` 把接缝名映射成步长。两张表，各自一个事实 ——
应用层知道两个单元是什么关系，视图层知道那个关系值多少像素。

`unitIndentClass` 顺手删了：三个键全都回答「没有缩进」，那是一个用三行说出来的空。

### 我的 `.ts` 扫描第一次是错的

上一轮我扫 `.ts` 里的 class 串，只找了 `text-|bg-|border-` 开头的，
**没找 `mt-*`**。这次把 utility 前缀补全了重扫，剩下的命中全是
StyleX 的属性**值**（`"flex"` / `"grid"` / `"relative"`）、图标名、或者我自己注释里的引号。

**`.ts` 里再没有真的 class 串了。**

### 第三个根因，而 `postcss.config.mjs` 一字不差地预言过它

节奏修好后还剩 74 张，集中在 markdown 与代码块。看 diff：**代码块丢了内边距。**

`postcss.config.mjs` 的注释里写着：

> with Tailwind on PostCSS the `@import`s in `globals.css` stopped being inlined and
> **every code block lost its padding**.

`globals.css` 第 **1422 / 1423** 行（全文 1423 行）：

```css
@import "./markdown.css";
@import "./overlays.css";
```

**CSS 规范要求 `@import` 必须在所有其他规则之前** —— 跟在后面的会被忽略。
它们能生效，唯一的原因是 **Tailwind 的插件在浏览器看到文件之前就把它们内联了**。

改成从入口加载，紧跟在 `globals.css` 之后 —— 那正是它们需要的顺序：
这两张表是在细化 `globals.css` 立的规矩。不能把 `@import` 移到文件开头，
那会把它们放到 `globals.css` 的规则**之前**，同特异性的冲突就会反过来。

### 验收（上一轮的表，一次 `cd` 把它写进了 `frontend/UI_REFINEMENT_LOG.md`，这里归位）

| | 结果 |
| --- | --- |
| 视觉 | **669 / 669**，0 unexpected，0 flaky，**零重录** |
| 守卫 | 17 项全绿 |
| 单测 | 1783 项通过 |
| `globals.css` 的 Tailwind 指令 | 只剩 `@import "tailwindcss"` |
| 依赖 | `tailwind-merge` 已移除 |

零重录是这一轮该有的答案：删掉 68 个别名、把消费方改指底层名，
是**同一个值换个说法**，一个像素都不该动。

## Round 168 — Tailwind 归零，和三个只有像素才看得见的根因

`npm ls tailwindcss` 空了之后，全套视觉从 162 红降到 49。前四个根因上一轮已解。
最后一个花了整轮，因为**它不在 CSS 里**。

### 排除法：五次测量，每一次都说「没有区别」

| 测什么 | 怎么测 | 结果 |
| --- | --- | --- |
| preflight | 分别构建带/不带 Tailwind 的产物，逐条 diff `@layer base` | 只差 `a { -webkit-text-decoration }` 和 `code,kbd,pre,samp` 里三条 `--theme(…)`（浏览器直接丢弃）|
| 丢失的变量 | 产物里 `var(--x)` 读到的减去定义过的 | 22 个，全是运行时注入（Base UI 锚点、JS 量、markdown alert 的兜底），**没有一个来自 Tailwind** |
| 计算样式 | `:root` / `body` / 那个 `<p>` 的**每一条**属性逐条 diff | 只差 Tailwind 自己的 `--tw-*`、`--radius-*` 这类没人读的变量 |
| 几何与绘制 | 从段落到 `<html>` 十四层的 box、`transform` / `opacity` / `filter` / `contain` / `mask` | **完全一致** |
| 字形 | `Range.getClientRects()` 逐字取 x | **完全一致** |

同样的 DOM、同样的 CSS、同样的盒子、同样的字形位置，像素却不同。
那就只能问像素本身：**墨量相同到 0.004%，质心左移 0.50 px** —— 一次刚性的半像素平移。
而气泡的**底板没有动**：244 从 x544 到 x1081，两态逐像素相同。

### 根因：一个类名里藏着的 `[content-visibility:auto]`

线索在 dump 出来的类名末尾：`DIV.xvqjf64 xxebm3f [content-visib…`。
那是 Tailwind 的 **arbitrary-property** 语法。

```ts
// transcriptTurnContentVisibility.ts —— 改之前
return isLast ? undefined : "[content-visibility:auto] [contain-intrinsic-size:auto_220px]";
```

Tailwind 一走，这两个 class 什么都不生成：off-screen 的 turn 不再跳过渲染，
整条 transcript 的合成方式变了，气泡的文本因此落在不同的子像素相位上。

**丢掉的不是渲染细节，是一个产品决策**：一条无限长的 transcript 要不要渲染它的全部历史。

治本是让它返回 StyleX style，调用方合进同一个 `stylex.props`
（顺带消掉 `cn()` 里两串独立生成的 class 抢优先级那个没有正确答案的竞态）。

### 断言字符串，是它能烂掉一整个版本的原因

`MessageStream.test.ts` 断言的是那两个 class 的**字面量**。
Tailwind 删干净之后，这条断言依然全绿 —— 它测的是「这个函数返回这个字符串」，
不是「这个 turn 会跳过渲染」。

改成断言「解析得出类名」，真正的断言放到渲染出来的文档上：
新增 `historical turns skip off-screen rendering and the tail turn never does`，
读每个 turn 的 computed `content-visibility`。**编译成空的样式在那里无处可藏。**

### 49 → 11：同一个盲区的另外两处

| 位置 | 死掉的东西 | 症状 |
| --- | --- | --- |
| `ChatStream.tsx` 的 `const RAIL` | 12 个 utility 一串，含容器查询 `@min-[1152px]:flex` | rail 从绝对定位的浮层变成流内盒子，压在 transcript 上：narrative golden 少一行标题，WCAG 报三个 34×9 触控目标（窄面板下 rail 本该隐藏）|
| `DiffView.tsx` 的 `const CODE_CELL` + 模板串里的 `text-fg-soft` | 4 个 utility | diff 行不再换行 |

`[&>*]:pointer-events-auto` 是唯一一条 StyleX 表达不了的（后代规则），
它进 `globals.css`，键在 `[data-slot="chat-rail"] > *` —— 因为 rail 是插件 **slot**，
只有包裹层知道「每一个贡献都必须可点」，不能指望每个贡献自己记得。

### 守卫为什么在它最该报警的那一刻沉默

`check-dead-utilities` 判断「这串是不是 class 列表」的办法是
**至少两个 token 能在构建产物里找到规则**。
Tailwind 一走，一个全是 Tailwind class 的字符串**一个 token 都解析不了**，
于是被当成散文跳过 —— 问题越彻底，它越安静。

换成 `check-authored-classes`，问反过来的问题：
**手写的每一个 class，必须是我们自己的样式表定义的。**
这个集合小、封闭、且属于我们；剩下的要么是已经不存在的框架的 utility，要么是拼错。
902 个文件 / 69 个 class。造了个含 4 个死 class 的文件验证过它会失败，四条全报，
常量形式和内联形式都抓到。

### 最后一张红：我上一轮的 import 顺序推反了

`md-table-actions` 在触屏上仍然透明。`globals.css` 最后一段自己写着：

> Last in the file on purpose: a rest state may be declared by any of the sheets imported
> above, and this has to be the one that wins.

我把 `markdown.css` / `overlays.css` 移到入口时**排在了 `globals.css` 之后**，
同特异性下后来者赢，markdown 的 rest 状态盖掉了 `@media (hover: none)` 的兜底 ——
触屏上那条 action strip 没有任何办法出现。

改成 **`globals.css` 最后加载**。上一轮日志里我那句
「不能把 `@import` 移到文件开头，那会把它们放到 `globals.css` 的规则之前」
推错了方向：需要在前面的正是那两张表。此处更正。

### 顺手清掉的 Tailwind 时代残留

`class-variance-authority`（零引用，cva 拼的就是 class 串）、
`classNames.test.ts`（整份文件都在测 tailwind-merge 的冲突消解）、
4 个死 export（`FLOATING_MOTION` 是我删掉 `FloatingSurface` 之后留下的）。

`cn()` 现在就是 `clsx`。按仓库自己的规矩（包装器不拥有策略就删掉）它该没了 ——
但它和 67 个文件里的 `className` 逃生口是**同一件事**：
逃生口存在的理由写在 CLAUDE.md 里，「因为调用方仍是 Tailwind」，这个理由今天不在了。
两个一起删是下一批，不是这一批。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **670 / 670**（669 + 新增 1），0 unexpected，0 flaky，**零重录** |
| 守卫 | 17 项全绿（`check:utilities` → `check:classes`）|
| 单测 | 2393 通过 / 4 失败 —— 全部在 `src/rpc`，本轮 runtime contract 由用户改动，不在范围内 |
| 入口 CSS | 100.0 KB / 142.6 KB 预算 |
| `npm ls tailwindcss` | 空 |

零重录：这一轮没有重录任何一张 golden。
之前那 49 张红全都是**回归**，不是「设计变了」—— 分辨这两者的唯一办法，
就是先把每一张的因果说清楚，再决定要不要动基准。

### 六层，收尾

| 层 | 状态 |
| --- | --- |
| ① utility class（含 arbitrary-property、常量里的整串） | **零** |
| ② `@theme inline` | 已内联为自己的 `:root` |
| ③ `@utility` / `@custom-variant` | 已消 |
| ④ preflight + Tailwind 自带主题默认值 | 已转写并逐条比对 |
| ⑤ `cn()` 里的 `tailwind-merge` | 已删 |
| ⑥ Vite 插件 + devDeps | 已删 |

## Round 169 — 守卫的洞，和洞里坐着的 toast

上一轮的 `check-authored-classes` 只看**位置**：`className=` / `*ClassName` / `cn(` / 大写常量。
写完当轮我就发现，这一轮最糟的那个 bug **恰好不在任何位置里** ——
`transcriptTurnContentVisibility()` 是从函数**返回**一个 class 串。守卫抓不到它。

### 补第二条规则：看形状

| 规则 | 命中 | 误报 |
| --- | --- | --- |
| A：只有 Tailwind 才有的语法（`[...]`、变体前缀） | 68 | **67** —— 日志前缀 `[agent] …:`、CSS 选择器 `[data-turn-id]`、时间 `14:29`。弃用 |
| B：**≥2 个 token，每个都长得像 utility，且都不是我们定义的 class** | 2 | **0** |

规则 B 精确的原因很朴素：图标名（`text-search`）、i18n key、日志前缀都是**单 token**；
多 token 的（`npm run check:api-consumers`）里总有一个 token 过不了 `every`。

验证过它抓得住两个藏身处（函数返回、第三方 prop），也放得过两个像但不是的。

### 规则 B 的两条命中，是同一个文件

```tsx
// toaster/index.tsx —— sonner 的 classNames，键叫 toast / title / description
toast: "rounded-xl bg-canvas text-fg shadow-[var(--shadow-overlay)]",
title: "text-ui-md font-medium",
description: "text-ui-md text-fg-muted",
```

**产品里每一个 toast 现在都是裸的** —— 没有圆角、没有底板、没有阴影、没有字号、没有墨色。
键不叫 `className`，所以任何扫 `className` 的办法都找不到它。

### 它为什么没被 golden 抓到

`plugin notifications use the production toast and dismiss automatically` 这条测试
断言了**文案**、**类型**、**自动消失** —— 三条全绿，而 toast 什么都没穿。

toast 是一个**面**。给面拍照。这条测试现在多一张 `toast-error.png`。

### 材质不从 `FLOATING_PANEL` 拿，理由写在代码里

共享材质是三片：`face`（填充 + 投射 + 伪元素上的模糊）、`motion`、`panel`（圆角）。
这个宿主只能收两片 —— **sonner 自己拥有 toast 的进入、退出、堆叠和滑动**，
`motion` 会往一个别的库正在动画的元素上加 `transition-property`。

`face` 那片也收不了：它的填充 `--app-floating-surface` 是 90% 不透明，
只有配上 `face` 挂在伪元素上的模糊才读得对 —— 而模糊正是收不了的那片。

所以 toast 自己说三件事，用的是设计自己的令牌：`radius.floatingPanel`、
`surface.card`（不透明）、`--shadow-overlay`（另外两个浮在最上层的东西已经在用的深度）。
不是把 `rounded-xl bg-canvas` 这套 Tailwind 时代的自创值原样搬进 StyleX。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **670 / 670**，0 unexpected，0 flaky |
| 新增 golden | 1 张（`toast-error`）—— 覆盖一个此前零覆盖的面 |
| 守卫 | 17 项全绿；`check:classes` 现在有位置 + 形状两条规则 |
| 重录 | 0 |

## Round 170 — 逃生口该不该拆：先量，再决定

`cn()` 现在就是 `clsx`，`className` 逃生口在 **33 个组件**上、**206 个调用点**用着。
按第一法则它该拆。但拆之前先问一个能测的问题：**它现在到底错了几处？**

### 竞态是可测的

StyleX 的每条规则是「一个类名 + 一条声明」，特异性全相同。
所以样式表就是一张 `class → 声明` 的表，
一个元素上同一个属性出现两次 = **只有打包顺序在决定谁赢**。

`stylex.props(...)` 在**一次调用内**能定优先级（后者胜，败者根本不生成）；
跨两次调用不能 —— 组件把自己的类名串接上调用方生成的那串，谁赢看 bundler。

| 覆盖 | 命中 |
| --- | --- |
| 9 个状态（试探） | 1 |
| **全部 24 个 agent 状态 + 5 个 workspace 状态** | **4** |

206 个调用点，真正撞车的 **4 个**。
这就是没有把 33 个组件的契约全改掉的理由 —— 也是把这次测量**变成一条常驻断言**的理由。

### 四处，各自的正确归属

| 位置 | 撞什么 | 治法 |
| --- | --- | --- |
| `JumpToBottomButton` | `transition-property`：`opacity, translate` vs 按钮自己的六项 | 删掉。按钮的声明旁边就写着「a call site cannot add to it — it can only replace it」，这个调用点正在**悄悄减项**，把按下的 scale 和所有颜色过渡一起丢了 |
| `AgentComposerTopTraySurface` | `width`：`100%` vs `calc(100% - 24px)` | 组件不再持有 width。composer 是 `align-items: center`，不 stretch，而两个调用方要的宽度**本来就不同**（Goal 托盘跨满、项目托盘两边内缩）|
| `FilesChanged` → `AgentRow` | `font-family` + `letter-spacing` | `AgentRow` 把调用方的 `styles` **扔在地上**（数组写在 `{...props}` 之后）。修好转发，调用方改用 `styles={[face.mono]}` |
| `run-summary` / `McpRow` → `Badge` | `font-weight` | `Badge` 的 `className` 换成 `styles` —— 它仅有的两个 className 调用方**都在传 StyleX 类名串**，要的本来就是这个接缝 |

`Button` 早就有这个接缝，注释也早就写清楚了：
「composed in the same `stylex.props()` call, so a property it declares **replaces** this
component's rather than losing to it, which is exactly what a `className` cannot do.」
缺的不是设计，是把它用起来。

### 重录 2 张，原因说得出来

`workspace dock-files` 两个主题：**文件行现在是等宽字体了**。
`FilesChanged` 一直在要 `face.mono`，但它走 `className`，输给了按钮的 `font-family` ——
基准里那两行一直是比例字体。这是修复，不是设计变了。

### 一次没复现的红

`agent golden dark tool-tail` 在一次全量里红过一次，单独跑 4/4 绿、之后全量也绿。
不当作修好了记 —— 记成一张**边缘基准**：配置里已经写着 `delegated` 有同类问题
（两个 worker 谁先画决定它差一个像素）。这条待观察。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **672 / 672**（670 + 新增 2 条竞态断言）|
| StyleX 属性竞态 | 4 → **0**，全部状态覆盖 |
| 重录 | 2 张（dock-files 两主题）—— 原因：等宽字体本该生效 |
| 逃生口 | `Badge` 已换成 `styles`；其余 32 个组件**证据不足，不动** |

## Round 171 — `:root` 里最后六个 Tailwind 的名字

上一轮结束时 `globals.css` 顶上还留着一块「Tailwind 主题供过的六个值」。
逐个查消费方之后，**只有一个是这个设计没有名字的东西**。

| 变量 | 消费方 | 判定 |
| --- | --- | --- |
| `--spacing` | `space` 的每一档 | **留** —— 没它整个产品一起丢掉节奏 |
| `--shadow-md` | `MarkdownImage` 一处 | 删。那张图**已经戴着 `.media-edge`** —— 设计自己的图片描边。外框 + 阴影正是 DESIGN.md 禁止的「同一个面两条边」，而同族的 `ImageBlock` / 附件缩略图 / 预览图都只有描边 |
| `--leading-normal` | `system-message` 一处 | 删。1.5 卡在设计自己的 `snug 1.35` 和 `body 1.55` 中间 —— 一个梯子上不存在的档。系统消息是正文嗓音，改用 `leading.body` |
| `--tracking-normal` | 3 处 | 改名 `--tracking-none`，并挪进 `--tracking-ui` 旁边。值没变，说的是「退出 UI 字距」，那是一个决定，该用这个设计的词 |
| `--tracking-wide` | **产品 0 处**，只有 visual fixture 的大写标签 | 从产品 `:root` 拿掉，值落到 fixture 自己身上 |
| `--animate-spin` / `--animate-pulse` | 5 处 | 见下 |

### 两个动画是产品里唯一够不着「减少动态」的

设计自己的四个动画全都把时长包在 `calc(… * var(--motion-scale))` 里，
而 `--motion-scale` 在「减少动态」下是 **0**。

Tailwind 那两个是平的 `1s` / `2s` —— 所以用户要了减少动态之后，
**重连的转圈和连接中的呼吸照转不误**。这是可访问性缺陷，不是命名问题。

改成 `flame-spin` / `flame-breathe`，时长进 `--motion-scale`，
并且从原始 `var()` 提升成 `motion.spin` / `motion.breathe` 令牌。

`--animate-pulse` 也不再和 `--animate-pulse-dot` 撞名 —— 它们本来就不是一回事：
一个是透明度呼吸，一个是圆点的缩放心跳。

`McpRow` 里那条注释自己写着：

> they are named here so the variables are the only thing left to own when Tailwind goes.

这一轮就是来还它的。

### 顺手：一条新的守卫

`--shadow-floating` 曾经是一个**任何样式表都没定义过**的名字，
被图片托盘读着，于是那个托盘根本没有阴影 —— 直到有人拿它跟设计对了一遍才发现。

`check-css-variables`：**没有兜底的 `var()` 必须解析得到定义**。
带兜底的排除在外 —— `var(--composer-overlay, 0px)` 自己写明了没人设置时的答案。

运行时注入的 7 个走一张显式白名单，**值是设置它的文件**，
守卫会检查那个文件还提着这个名字 —— 白名单不会烂成一堆借口。
另一半反着查：列进白名单却没人读的，也报。

两半都验证过会失败（重放 `--shadow-floating`；伪造一个没人读的条目）。

### 重录 2 张，原因说得出来

`agent cwd-missing` 两个主题：横幅的行高从 1.5 走到 1.55，横幅变高，
底部对齐的整列上移 3px。**逐带做纵向相关，残差为 0** —— 纯平移，只有横幅的高度变了。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **672 / 672** |
| `:root` 里的 Tailwind 名字 | 6 → **1**（`--spacing`）|
| 够不着减少动态的动画 | 2 → **0** |
| 守卫 | 18 项（新增 `check:variables`）|
| 重录 | 2 张（cwd-missing 两主题）—— 原因：行高回到设计自己的档 |

## Round 172 — 唯一一条够不着密度设置的导航栏，就在密度设置自己那一页

之前几轮里我报过一条「Settings 不响应密度设置」，当时按「设计问题」挂着没动。
这一轮把它测清楚了 —— 它不是设计问题，是一条**明确的承诺没有兑现**。

设置项自己的文案写着：

> How much air the **chrome** gets. Row heights, gutters, and composer insets.

内容行不在承诺里（Settings 的表单行是内容，不是 chrome），这没问题。
但**选择设置页的那条竖排导航栏是 chrome**，而它是产品里唯一不读密度词汇的一条。

| | 别的导航栏 | Settings 栏（改前）|
| --- | --- | --- |
| 行高 | `--density-row-height` | `--control-height-md` |
| 图标到标签 | `--density-row-gap` | `space.s2_5` |
| 栏内缩 | `--density-navigation-gutter` | `space.s2` |
| 行内缩 | `space.s2` | `space.s2_5` |

顺带把分组标题和它统领的那些行**对齐到同一条左边**：
改前标题 16px、行图标 18px；改后两者都是 20px。

### 为什么这条能一直活着

**整个密度设置在六百多张 golden 里零覆盖。** 没有任何一条测试问过
「换个密度，有东西动吗」。所以一条导航栏用自己的尺子量，永远不会有人发现。

fixture 现在收 `?density=`（走的还是那条真实的 appearance 管线，
不是把变量硬塞进 `:root`），新测试断言：

- 两个密度下 `--density-row-height` 不同；
- 每一条**竖排** tablist 的 tab 和每一条 `.agent-row`，高度**恰好等于**那个令牌值 ——
  不是「接近」，因为「接近」正是一条用自己尺子的栏能一直混过去的方式。

横排 tablist 排除在外：`density.ts` 第一行就写着 chrome bar 的高度**故意不缩放**
（内容头、抽屉头、红绿灯槽共用一个数，跨接缝对齐）。

**两个方向都验证过**：把 `--density-row-height` 换回 `--control-height-md`，
测试立刻报 `29px` vs `30px`。

### 重录 6 张，全部只动了那一列

diff 的 x 范围 **8..247** —— 就是那条栏，内容区一个像素没动。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **673 / 673**（672 + 新增 1）|
| 密度设置覆盖 | 0 → **1 条断言，跨两个密度、两类导航行** |
| 不响应密度的导航栏 | 1 → **0** |
| 重录 | 6 张（settings 相关），diff 全部落在 x 8..247 |

## Round 173 — 两把尺子量同一级台阶

`--control-height-*`（22 / 26 / 30 / 34 / 40）和 `--field-height-*`（26 / 28 / 32）
是两个梯子，`sm` 相同、`md` 和 `lg` 差 2px。

而按钮 `lg` 那一档旁边写着：

> The ladder's top text step: **a button that stands beside a field has to match its height.**

**数字自己否认了这句话。**

### 证据不是读出来的，是量出来的

先扫了一遍「同一个 flex 行里既有输入框又有按钮」的地方 —— 报 0 处。
但那个扫描在 settings 面板上撒谎：pane 是懒加载的，`data-visual-ready` 之后内容还没到，
所以每个 pane 都报 `rows=0`。**一个报零的扫描器和一个没有问题的产品长得一模一样。**

直接量 Settings → Connection 那一行：

```
<INPUT>  x=596 y=124 200x32
<BUTTON> "Reset to default" x=804 y=123 130x34
<BUTTON> "Apply"            x=942 y=123  65x34
```

一行三个控件，输入框 32、按钮 34 —— **正是那句注释描述的场景，正是它说不该发生的事。**

### 为什么两个梯子应该是一个

翻 `git log -S`：`--field-height-*` 是**初始提交**带进来的，
没有注释、没有一次改动、没有任何地方说过输入框为什么该矮 2px。
而唯一写下来的意图（按钮 `lg` 那条）说的是它们应该一样高。

所以合并成一个梯子。方向选「输入框长到控件的高度」而不是反过来：
控件梯子是有注释、有 `xl` 档、有方形图标按钮的那个（它是主梯子），
而且把输入框改高改善的是触控目标，改矮是恶化。

visual style 的令牌表里那三个 `field-height-*` 也一起删了 —— 没有任何内置风格覆盖过它们，
留着就是「一个写了没人读的变量」。

`stylesheetMirror.test.ts` 的下限从 70 降到 67：**正好三个**。
一个只许升不许降的下限，会让「删掉一个令牌」和「漏读一个令牌」变得无法区分。

### 重录 10 张

全部是带输入框的面（dock-search / full-view / settings / 三个 settings pane / 最大字号两张）。
输入框高 2px，偶数（`globals.css` 自己要求每个高度是偶数，
否则居中的 1px 规则会落在半像素上）。

### 两次没复现的红，记下来

- `agent golden dark tool-tail`（Round 170）
- `code blocks stay readable and expose the wrap control`（本轮）

两条都只在**全量并行**里红过一次，单跑各 3-4 次全绿，之后的全量也全绿。
两条都和 hover / 指针位置有关。不记作修好 —— 记成**待观察的边缘用例**。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **673 / 673** |
| 控件高度梯子 | 2 把尺子 → **1 把** |
| 死令牌 | visual style 表里 3 个 `field-height-*` 删除 |
| 重录 | 10 张，全部是带输入框的面 |

## Round 174 — 一个探针，读了一个仓库史上从未定义过的名字

`foundation.visual.spec.ts` 里有个辅助函数，靠「往一次性元素上写这个变量、再读回计算值」
来解析 `--app-content-card-radius`，然后断言内容卡的左上角：

```ts
sidebar === "expanded" ? await declaredCardRadius(page) : "0px"
```

`git log --all -S "--app-content-card-radius:"` —— **空的**。
这个名字在这个仓库的**任何一次提交里都没有被定义过**。

所以 `declaredCardRadius()` 一直返回 `0px`，那条三元一直在拿 `0px` 和 `0px` 比。
四张 golden 从第一天起就在断言一件永真的事，
而它的注释写着「corner 是当前 visual style 该声明的，写死数字会拿一个风格去量所有风格」。

**读了一个没有主人的名字的探针，不会失败。不会失败的测试，没有在覆盖它命名的那件事。**

`.agent-content-card` 自己写着 `border-radius: 0`，从初始提交至今 —— 卡片本来就是方的。
所以断言改成直说这件事，并补上两条真正没人断言过的：
**它的接缝是 shadow 不是 border**（整个边界模型赖以成立的「一条边，不是两条」）。

### 守卫补上这半边

`check-css-variables` 上一轮读的是**构建产物**。组件里的 `var()` 会被 StyleX 编译进去，
所以已经覆盖了；**spec 里的探针不会**。这正是这个 bug 藏了整个仓库历史的地方。

现在它也扫 `visual/**` 的 `var(--x)`（去注释后），对照定义集 ∪ 运行时白名单。
**在未改动的 HEAD 上验证过它会失败**，指着 `foundation.visual.spec.ts:10`。

顺手扫了 `src` + `visual` 全部 `var()` 读取：10 处命中，9 处是已知的运行时注入
（Base UI 的定位器三个、`--dock-measure`），第 10 处是注释里引用 Codex 自己的令牌名。
**真实缺陷只有这一个，已修。**

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **673 / 673** |
| 永真的断言 | 4 张 golden 的三元 → 3 条能失败的断言 |
| 守卫 | `check:variables` 现在覆盖样式表 + spec 探针（351 + 17 个无兜底读取）|
| 重录 | 0 |

## Round 175 — 等宽跟着标题走，不跟着视图走；以及一个终于修到根的 flake

### `titleFace` 是常量的两个调用点

`ViewHeader` 的 `titleFace` 文档写着「prose 是视图的名字，mono 是机器文本 —— 路径、命令」。
两个调用点把它当成**视图的属性**而不是**标题的属性**传了常量：

- `terminal.tsx`：`titleFace="mono"`，而它的 title 是 `t("terminal.title")` = 翻译过的单词 **“Terminal”**。
  它的 `sub` 也是翻译句（“N commands”）。整个头部没有一个字是机器文本。
- `file.tsx`：无条件 `mono`，而 title 在没开文件时回落成翻译句 `t("file.empty.title")`。

**等宽排出来的句子，读起来像是一段让读者照着敲的字面量。**

改完之后 `titleFace` 只剩一个调用点，而且它问的是「有没有打开文件」。

### 为什么没有一张 golden 动

因为**没有一张 golden 拍到过它**：

- dock 里 `DockViewBar` 根本不渲染 title（也完全忽略 `titleFace`）；
- full placement 的 golden 只有一个状态，而 `FULL_VIEW_ID = "search"` ——
  **十几个视图，只有一个的完整头部被拍过。**

这条留给下一批：给 fixture 加 `?full-view=<id>`，才谈得上覆盖。

### 那条 flake，两次之后修到根

`code blocks stay readable and expose the wrap control` 在六次全量里红了两次。
它的注释早就写明了竞态：

> `hover()` reads the box, then moves the pointer — the transcript eases its own scroll,
> so under load the artifact has slid on by the time the pointer arrives.

上一次的处理是先 `expectStableBox` 再 hover —— **不够**，两次红都是在这之后发生的。
原因是：`expect.poll` 只重读 opacity，**永远不会重新瞄准**。
指针一旦落空，轮询到超时也还是落空。

治本是把 **hover 放进重试里**：

```ts
await expect(async () => {
  await target.hover();
  expect(await revealed.evaluate((n) => getComputedStyle(n).opacity)).toBe("1");
}).toPass();
```

不是把超时调长 —— 调长的是等一个已经站错位置的指针。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **673 / 673** |
| `titleFace` 常量调用点 | 2 → **0**（唯一剩下的一个是条件的）|
| flake | hover 进重试；此后全量绿 |
| 重录 | 0 |

### 报告，不擅自决定

- **dock 头部整行强制 mono**（`DockViewBar` 对 identity 和 sub 一视同仁），
  full view 的 `sub` 也是无条件 mono —— 而 sub 常常是翻译句（“N commands”、“3 files changed”）。
  这是「紧凑标识条用等宽表示机器语境」的风格选择，还是和上面同一个错误？**需要你定。**
- **full placement 只拍了 `search` 一个视图**，其余十几个视图的完整头部零覆盖。

## Round 176 — 让 fixture 能打开「不具代表性」的那两个视图

上一轮报了两条：`titleFace` 的两个常量调用点已修，但**没有一张 golden 拍到过它**。
原因写在 fixture 自己的注释里：

> Which view the full-placement state opens. `search` has a prose title and a sub,
> which is the path **nineteen of the twenty-one** `WorkspaceViewLayout` call sites take.

选 `search` 作代表是有道理的 —— 但**恰恰因为它是代表，那两个不一样的视图从来没被看过**，
而 bug 就住在那两个里（标题是路径的那两个）。

### 加一个能力，不是加二十四张 golden

fixture 现在收 `?full-view=<id>`，默认仍是 `search`。
新测试断言的是**规则**而不是像素：

- `search` 在 full placement：标题是视图的名字 → **不是** mono；
- `file` 在 full placement（fixture 本来就带着 `fileViewer`）：标题是路径 → **是** mono。

比较的是字体栈的**第一个 family**：自定义属性保留作者写的换行，
computed 值是规范化过的 —— 两个字符串直接比，是一条永远说「不」的测试。
（我第一版就是这么写的，它立刻假红了一次。）

**验证过它会失败**：把 `file.tsx` 的 `titleFace` 改回无条件 `undefined`，
测试立刻报「a PATH is mono, got Geist, -apple-system, …」。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **674 / 674**（673 + 新增 1）|
| full placement 可拍的视图 | 1 → **任意一个**（`?full-view=`）|
| 标题字面规则 | 0 条断言 → 1 条，覆盖代表用例和例外用例各一 |
| 重录 | 0 |

## Round 177 — 我自己写的那条守卫，漏掉了它被写来找的那个东西

Round 170 的竞态断言里有一行：

```ts
if (!(rule instanceof CSSStyleRule) || rule.style.length !== 1) continue;
```

「StyleX 的每条规则是一个类名 + **一条**声明」—— 这句话对源码成立，
对**读回来的 CSSOM 不成立**：Chromium 会把简写展开成分写，
`border-radius` 的 `style.length` 是 **4**，于是每一条简写规则都被跳过了。

一个「找同一属性两条规则」的检查，跳过了**所有简写** ——
包括消息操作按钮那个：按钮自己的 `radius.button` 和调用方通过 `className` 塞进来的
`--shape-md`，同一个元素上两条 `border-radius`，谁赢看打包顺序。
那正是这条断言被写出来要找的东西。

### 两次读，缺一不可

| 读法 | 抓得到 | 抓不到 |
| --- | --- | --- |
| 展开后的分写 | `padding` vs `padding-top` | 值是 `var()` 的简写（引擎展不开，分写读回来是空） |
| `cssText` 里作者写的那条 | `border-radius: var(--a)` vs `var(--shape-md)` | 简写 vs 它自己的分写 |

两条都上之后，找到 **5 处**：

| 位置 | 冲突 | 治法 |
| --- | --- | --- |
| `ReasoningBlock` | `overflow: hidden` vs `overflowY: auto` | 两个 **key**，`stylex.props` 只能在同一个 key 上定优先级。改成两个分写 |
| `timeline` 的 detail | `text-wrap-mode: nowrap` vs `initial` | `vocab.truncate` 和 `vocab.pretty` 同时用 —— 一行截断的字**没有 rag 可以平衡**，两者本身就矛盾 |
| `SettingsPage.blurb` | `margin: 0` 和 `marginTop` **在同一个对象里** | 删掉简写（reset 已经清零了每个 margin） |
| `MessageActionButton` | `border-radius` 两条 | 走 `styles` 接缝而不是 `className` |

### 顺着这个形状静态扫了一遍：还有三处

`padding` 后面跟 `paddingTop` 读起来像「然后覆盖顶部」——
**CSS 懂这句话，StyleX 不懂**：两个 key，两个类，谁赢看打包顺序。

`ImagePreviewGallery.pan`、`lightbox-dialog.document`、`IconShowcase.intro` 各一处。
四处里有三处把简写写在前面 —— 正是那个读起来最自然的写法。

新守卫 `check-shorthand-longhand`：1283 个文件，任何 style 对象都不许同时出现
一个简写和它自己的分写。**验证过会失败。**

### 值得记的一件事

这七处改完，**674 张 golden 一张没动**。
也就是说打包顺序一直碰巧站在正确的那边 —— 那是运气，不是设计。
歧义消掉之后，它不再需要运气。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **674 / 674**，重录 **0** |
| StyleX 属性竞态（含简写） | 5 → **0** |
| 简写 + 自己的分写 | 4 → **0** |
| 守卫 | 19 项（新增 `check:shorthand`）|

## Round 178 — 把两串生成好的类名并排放，最后四处

上一轮的守卫报 0 竞态，所以我**没有**去动那 82 处 `cn(生成的类名, 调用方的类名)` ——
那是逃生口，改它要有证据。但另有一种形状不需要证据就知道是错的：

**同一个元素上，两次 `stylex.props(...)` 的结果被 `cn` 并排放。**

一次调用内 StyleX 能定优先级（后者胜，败者不生成）；两次调用之间它做不到。
而这四处的两次调用**都在同一个文件、同一个组件里** —— 没有任何理由不合成一次。

| 位置 | 改法 |
| --- | --- |
| `ChatSearchOverlay` | `cn(props(pill), props(undraggable))` → `stylex.props(cs.pill, cs.undraggable)` |
| `CompactionBlock` | 同上，合成一次调用 |
| `navigation-row` ×3 | 常量从**类名串**改成**样式数组** |

`navigation-row` 那三个是模块级常量 `ROW_GROUP` / `RESTING_GLYPH` / `HOVER_ACTION`。
提取到模块级是对的 —— 注释说明了 `RESTING_GLYPH` 和 `HOVER_ACTION` 必须保持同一个决定。
错的是提取出来的**东西**：提 `stylex.props(...).className`（一串已经定型的类名）
只能并排；提 `[reveal.host]`（样式本身）就能在每个元素自己的 props 调用里参与排序。

顺带一个类型上的收获：`action && RESTING_GLYPH` 里 `action` 是 `ReactNode`，
`&&` 可能产出 `""` 或 `0` —— `cn` 会默默吞掉，样式数组不接受。
改成三元。**`cn` 的宽容正是它掩盖问题的方式。**

### 关于 TextButton，一次收回

我一度给 `TextButton` 加了 `styles` 接缝好让 `CompactionBlock` 用。
类型立刻拒绝了：`reveal.host` 只声明自定义属性，StyleX 给它的类型和 `StyleXStyles` 不同。
而且没有第二个调用方需要这个接缝 —— 收回，改成调用点合成一次 props。
**为一个调用方加一个 API，是 YAGNI 的标准形状。**

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **674 / 674**，重录 **0** |
| `cn(两串生成的类名)` | 4 → **0** |
| `className` 逃生口 | 仍是 82 处，**零竞态，不动** |

## Round 179 — 一个视图有两个名字；以及那条 flake 的根因是「记得上一次」

### 从大小写漂移，挖到重复本身

把每个视图在 full placement 打开一遍（上一轮加的 `?full-view=`），扫出来一件小事：
二十个标题里十七个是 sentence case（`Tool stats` / `Run summary` / `Working tree` / `Agent docs`），
三个是 Title Case（`Skill Library` / `Skill Proposals` / `Agent Memory`）。
`Agent docs` 和 `Agent Memory` 在同一个 dock 里隔着两个面板。

改完英文标题之后，**674 张 golden 一张没动** —— 这本身就是线索：
一个用户看得见的字符串改了，没有任何一张照片记录它。

追下去发现每个视图的名字**写了两遍**：

| | 键 | 用在 |
| --- | --- | --- |
| tab | `workspace.view.title.<x>` | dock 标签、catalog |
| header | `<x>.title` | full placement 的头部 |

二十个视图 × 八种语言 = **320 个字符串，承载 160 个事实**。而第二份拷贝的用途就是漂移：

| 视图 | tab | header |
| --- | --- | --- |
| skill-library | Skill Library | Skill library |
| skill-proposals | Skill Proposals | Skill proposals |
| agent-memory | Agent Memory | Agent memory |
| files | **Changed files** | **Working tree** |
| timeline | **Timeline** | **Run timeline** |

前三个是我上一步的**半个修复**：只改了 header，tab 还留在 Title Case ——
改之前两边一致（都是 Title Case），改完反而不一致了。补齐 tab。

守卫 `Rule 10` 加上之后，立刻又抓到**第六处我没看见的**：
西班牙语 `agent-docs` tab `"Documentos del agent"` vs header `"Docs del agent"`。
英文两边一致，只有西语漂了。按英文的短名形式统一到 header 那个。

后两个（`files` / `timeline`）是**名字本身不同**，不是大小写 —— 那是产品决定，
守卫不该替你做，所以它们连同问题一起写进白名单，而不是悄悄分岔。

### 那条 flake：`land()` 记得上一次

`agent golden tool-tail` 第三次出现（Round 170 dark、这轮 light）。
这次单独跑也复现了 —— 6 次里 1 次，**7255 像素**，不是阈值噪声。

裁图看：fixture 自己那条状态栏整体上移 11px，底部多露出一行 "Waves"。

根因在 fixture 的 `StateSidebar.land()`：它**只在 active 行不可见时**才滚。
于是第一次 land 按一套行高选了一个 offset，字体到位后 `ResizeObserver` 再 land，
发现行已经在视野里 —— **什么都不做**，把上一次的 offset 留下了。
两个相差 11px 的结果，取决于第一次跑在什么时候。

治法是让落点**不带记忆**：每次从 0 开始重算。
不是加超时、不是加重试 —— 那些是等一个已经错了的状态。

证据：改前约 1/6 失败；改后 tool-tail 连续 **36 次**观察全绿
（(5/6)^36 ≈ 0.14%），且 44 张 agent golden 两轮全过 —— 落点没有位移。

### 一次我自己造成的污染，记下来

中间有一轮全量红了 14 张，其中 6 张（`layoutShift` ×3、`zh`/`zh-TW`/`de`）根本不该动。
原因是我为了量 bundle **在套件运行期间改了 `MarkdownMessage.tsx`** ——
dev server 热更新，套件后半程拍的是打了桩的 markdown。同一轮还并发跑了两次 `vite build`，
耗时从 9.5 分钟涨到 23.4 分钟。

**运行期间不碰源码**这条我自己写过，又自己犯了。重跑干净之后只剩 8 张预期内的 + tool-tail。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **674 / 674** |
| 重录 | 8 张（三个改名的 tab + catalog × 双主题）—— 原因：文案统一到 sentence case |
| 视图名分岔 | 6 → **0 处未声明**（2 处产品决定已列出并附问题）|
| `tool-tail` flake | ~1/6 → **36 次观察全绿** |
| 守卫 | `check:locales` 新增两条规则（标题大小写、一个视图一个名字），**都验证过会失败** |

## Round 180 — 为一个函数背了 139 KB，和整个依赖树里最后一个 tailwind

上一轮报告「Tailwind 清干净了」之后，用户问：那个依赖可以去掉了吧。
`tailwind-merge` 当时只作为 `streamdown` 的**传递依赖**存在 —— 所以问题变成：`streamdown` 拿来干嘛的。

**一个函数。** `parseMarkdownIntoBlocks`。

### 先量，再决定

| 测量 | 结果 |
| --- | --- |
| streamdown 的组件代码进产物了吗 | 没有，被 tree-shake 掉了 |
| `tailwind-merge` 呢 | **进了** —— `conflictingClassGroups` / `validators` 都在入口 chunk 里 |
| 把那一个 import 换成本地桩再构建 | 入口 chunk **597,138 → 454,501 字节** |
| 即这一个函数的代价 | **139.3 KB**，而入口 JS 预算已用到 **91.4%** |

它 `sideEffects` 没声明，所以打包器必须假定模块图是活的：
`unified` / `remark-rehype` / `rehype-sanitize` / `rehype-harden` 一整套第二份 markdown 管线，
外加 `tailwind-merge` —— **一个专门解决 Tailwind class 冲突的库，躺在零 Tailwind class 的产品的首屏包里。**

### 替换不是照抄，是先把它的行为钉成规格

那个函数有三处非显然的边界。我**先跑它**，把 35 个输入的输出记下来当规格，再写自己的实现：

| 边界 | 规则 |
| --- | --- |
| 脚注 | 引用和定义分开就没有意义 —— 命中任一，整段不切 |
| 块级 HTML | 按标签名压栈，跨多少个 token 都算同一块 |
| `$$` 数学围栏 | 奇数个 = 还没闭合，下一个 token 并进来；紧跟代码块时不判（那里的 `$$` 是字面量）|

前三版都不对，**每一次都是差分测试告诉我的，不是我读出来的**：

1. `<br/>` —— 我把 `<br/` 当成了开标签；
2. `<div />` —— 空格加自闭合；
3. `<img src=x>` —— 语法上不自闭合，但它是 **void 元素**，永远等不到闭合标签。

第三个逼我去读原实现，那里有一张 15 个 void 元素的表和「整标签匹配 + 排除 `/>` 结尾」的计数规则。
照着**语义**重写之后：35 个用例全等，再跑 **4000 个随机文档差分，零差异**。

### 结果

| | Before | After |
| --- | --- | --- |
| 入口 JS | 2485.9 KB（预算 91.4%）| **2379.1 KB（87.5%）** |
| 依赖 | `streamdown`（15 个传递包）| `marked`（MIT，**零依赖**）|
| `package-lock.json` 里 "tailwind" | 3 处 | **0 处** |
| 产物里 `tailwind-merge` | 在 | **不在** |
| 视觉 | 674 / 674 | **674 / 674**，重录 0 |

新测试 13 条：12 条钉死旧函数的输出（其中 3 条正是差分测出来的边界），
外加一条流式不变量 —— 把文档一个字符一个字符地喂进去，
**已经定型的块不许被改写**，那是这个函数存在的全部理由。

## Round 181 — 哪些面从来没被拍过；以及一条三分之二是空的规则

用户说了：本地桌面端，不纠结体积。所以这一轮不碰打包，回到「没有覆盖的地方就是缺陷所在」。

### 先把覆盖画出来，而不是靠运气撞

把 70 条 fixture 路由全走一遍，收集渲染出来的 `data-slot`，和源码里声明的比：
40 个声明，31 个在某个静止状态下出现过。

剩下 9 个大多是**要交互才出现**的（对话框、选择器展开），扫描器没点它们 ——
这是扫描器的局限，不是缺陷。逐个核对之后，真正没有任何测试提到的只有 `mermaid-full`。

### 「可见」不等于「尺寸对」

放大图的对话框**是被打开、被断言可见的** —— 但从来没有被拍过。
而 `globals.css` 里有一条只为它存在的后代规则。
`toBeVisible()` 对尺寸一个字都没说。

补一张 golden 之后，我做了件该做的事：**逐条删掉那条规则的声明，看 golden 会不会红。**

| 声明 | 删掉之后 | 结论 |
| --- | --- | --- |
| `display: block` | golden 全绿 | **死的** —— reset 已经给了每个 `svg` 这一条 |
| `max-width: none` | golden 全绿 | **死的** —— `max-width: 100%` 只加在 `img, video` 上，没有东西要它去解除 |
| `margin-inline: auto` | golden 全绿 | **活的，但没被覆盖** |

第三条为什么是活的：对话框是 `width: fit-content` 加一条 `min-width: min(408px, 80vw)` 的下限。
比这个下限窄的图会在两侧留白，没有这条就会贴着起始边。
而 fixture 里那张图正好填满宽度 —— **所以 golden 也拍不出它**。

三条声明，两条能删，第三条留下并写明它防的是什么、以及为什么现有 fixture 照不到它。
不是因为「看起来像多余的」就删，是**每一条都单独删过一次**。

### 一条没走的路，记下来

顺手量了滚动时的 style / layout / script 开销（5 个状态、每次 60 帧）：
最大一项 27ms/60 帧，DOM 300–500 个节点。**没有病态。**

真正会疼的是几百条消息的长会话，而**最大的 fixture 只有 269 个节点** ——
现有 fixture 到不了那个规模。但仓库自己的规矩写着「不要猜性能在哪，先测，只优化被证明的主导瓶颈」，
现在没有症状，所以不做，只记下来：`content-visibility` 是为这个规模存在的，
而没有任何东西验证它真的把开销压住了。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **674 / 674** |
| 新增 golden | 2 张（放大图，双主题）—— 覆盖一条此前只有 `toBeVisible()` 的后代规则 |
| 删掉的死声明 | 2 条，**各自单独验证过是死的** |
| 重录 | 0 |

## Round 182 — 两条名字比内容大的测试

顺着「哪些面没被渲染过」往下，两处命中，都是同一个形状：
**测试的名字承诺了一件事，断言的是另一件更小的事。**

### 一、打字机模式的测试，从来没走进打字机模式

```tsx
it("uses the typewriter pipeline without layering word fades over it", () => {
  render(<MarkdownMessage text="Hello world" reveal="typewriter" />);
```

选管线的是 **`streaming`，不是 `reveal`**。没有它，`MarkdownBlock` 走的是已定型那条分支
（`[rehypeRaw, rehypeFileRefs, rehypeKatex]`）—— 里面根本没有 `rehypeStreamCaret`。

所以这条测试断言的是「一棵本来就不会有淡入的树里没有淡入」，
对打字机**一个字都没说**。而光标（`.type-caret`）是那个模式**唯一**渲染得不一样的东西，
它在整个仓库里没有任何测试 —— `overlays.css` 里那条 blink 动画和 `globals.css` 里
给它的 `corner-shape` 豁免，都没人验证过。

补上之后还踩到一个真实的性质：**打字机是一个字符一个字符给的**，
所以断言 `toContain("Hello")` 本身就是在断言这个模式没在工作 ——
第一次跑到光标出现时，屏幕上只有 `H`。改成断言**已显示的是源文本的前缀**。

配一条对照：smooth 模式有淡入、没有光标。
**验证过会失败**：把 `rehypeStreamCaret` 从那条分支拿掉，测试立刻红。

### 二、报错面板说了「哪个插件坏了」，没说「坏在哪」

`PluginBoundary` 的兜底 UI 有三条单测：渲染子节点、显示兜底、上报到 error store。
三条全绿 —— 而我把 `<code>{this.state.error.message}</code>` **整行删掉，三条还是全绿**。

那行是唯一告诉用户**发生了什么**的东西。补上断言，并且断言它落在 `<code>` 里 ——
`overlays.css` 里有 6 条声明专门给这个元素上妆。

### 顺带核过、没有问题的

- `overlays.css` 一共 4 个类：`.plugin-boundary-error`（本轮补）、
  `.plugin-boundary-error code`（本轮补）、`.fade-in`（已有测试）、`.type-caret`（本轮补）。
- 130 条后代 / 属性选择器里，其余大多是 `.md ...`，markdown golden 覆盖着。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **674 / 674** |
| 单测 | **2407 通过 / 4 失败** —— 全部在 `src/rpc`，runtime contract 由用户改动中，不在范围内 |
| 新增断言 | 3 条，**每条都验证过能失败** |
| `.type-caret` 覆盖 | 0 → 1 |

## Round 183 — 321 个问号：一个从第一次提交起就没工作过的功能

不再靠猜哪里没覆盖，改用**真实的覆盖率**：用 CDP 的 JS coverage 走完 70 条路由，
dev server 把每个源文件当作独立 URL 提供，所以拿到的是**逐文件**的执行图。

277 个组件，**270 个**在某个 fixture 里渲染过。剩下 7 个里 6 个是 fixture 本来就要替换掉的
生产组合根（`App` / `main` / `router` / `PluginProvider` / devtools）。

第 7 个是 **`IconGallery.tsx`** —— 一个用户可以从 dock catalog 打开的工作区视图。
它不在 `DOCK_VIEW_BY_STATE` 里，所以任何 fixture 都没渲染过它。
用上一轮加的 `?full-view=` 打开它：

**321 张卡片，321 个 `?`。一个图标都没有。**

### 根因：一个匹配不到任何东西的 glob

```ts
// src/plugins/builtin/settings/icon-gallery/ui/iconMap.ts
import.meta.glob("../../../node_modules/@lobehub/icons/es/*/components/Mono.js")
```

从 `…/icon-gallery/ui/` 往上三层是 `src/plugins/builtin/` —— 那底下没有 `node_modules`。
正确的深度是**六层**。

`import.meta.glob` 匹配不到任何东西时返回 `{}` —— **不报错、不警告、构建照过**。
于是 `IconMap` 是空的，每张卡片都走 `?` 兜底。`git log -S` 查过：这个路径从**初始提交**就是错的。

而且它不只坏了图库：**Settings → Brand icons 用的是同一张表**，同样 59 个问号。
那个面板**有 WCAG 审计** —— 审计读名字，看不见一张缺失的图。

### 修完之后，让它不能再悄悄坏

| 补的东西 | 抓的是什么 |
| --- | --- |
| `iconMap.test.ts` | glob 解析出的组件数 > 100；目录里 90% 以上的条目要有组件 |
| `workspace golden settings pane brand-icons` | 那是唯一一个**主题就是图片**的面板 —— 只有照片看得见空白 |

**两个方向都验证过**：把路径改回三层，两条单测立刻红。

图标数：38 → **359**（渲染出的 `svg`/`img`）。入口体积没变 —— 图标本来就在懒加载 chunk 里。

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **675 / 675**（674 + 新增 1）|
| 组件覆盖 | 277 个中 270 个被渲染过；未覆盖的 7 个已逐个解释 |
| 修复 | 一个从初始提交起就没工作过的功能 |
| 重录 | 0 |

## Round 184 — 一轮以否定结果为主的审计

上一轮的覆盖图很有效，所以这轮继续用测量去找，而不是靠读。
**结果大部分是「没问题」** —— 这本身是要记的：它告诉下一个人**不必再去看哪里**。

| 查了什么 | 怎么查 | 结果 |
| --- | --- | --- |
| 其余的 `import.meta.glob` | 找全部 3 处 | 另外 2 处**都已经显式断言过非空**（`items.length > 0`、`WIRE_SAMPLES.length === present.length`）。上一轮那个 bug 的教训，别处早就学过了 |
| 分支级死代码 | Playwright JS coverage 的 range | **测不到** —— 它是函数级的，跑过的函数内部的未执行分支不会体现。要块级得直接写 CDP，收益不明，停 |
| dock 视图的错误态 | 逐个数 query 与 error 处理 | 10 个查询驱动的视图**全部**处理了错误；唯一没有的是 `tabBadges`，而一个失败的徽章不显示是对的 |
| 八种语言是不是真的翻了 | 逐键和英文比对 | 每种语言只有 **1–2 条**多词英文原样保留，且都是协议名（`Streamable HTTP`）。翻译是真的 |
| 整条质量门禁是否自洽 | 23 步逐个单跑 | **22 步全绿**；第 23 步（`test`）只在 `src/rpc` 红，那是你正在改的 runtime contract |

### 一个报告过的问题，现在有数据了

上一轮我报过「dock 头部整行强制 mono」，那时只是观察。这轮把它量出来了：
dock 头部一共 **57 个字符串，32 个走 mono**。其中：

- **该 mono 的**：路径、文件名、`+25` / `−9` / `1/3` 这类需要等宽数字对齐的计数、run id；
- **不该 mono 的（约 20 条）**：`"2 waiting on you"` / `"1 active · 1 archived"` /
  `"0 MCP active · 0 configured"` / `"1 of 3 complete"` —— 全是**翻译过的句子**。

按 `ViewHeader` 自己的文档（"prose 是视图的名字，mono 是机器文本 —— 路径、命令"），
这 20 条是 prose。

**但有一个真实的反方理由**：它们都带计数，而等宽给的是表格数字。
只是设计对这件事已经有答案了 —— `globals.css` 里写着：
「`tabular-nums` 用在**比例字体**上是一条真实的指令，留在调用点。」
也就是说，如果理由是数字对齐，该用 `tabular-nums`，不是换字体家族。

我没有替你决定：这会改动十几个面的排版。**数据在这里，结论是你的。**

（顺带验过一个我怀疑的缺陷：CJK 语言下 mono 栈没有中日韩字形、可能半行回退 ——
实测 dock 标签页用的是 Geist，翻译正常，**没有这个问题**。）

### 验收

| | 结果 |
| --- | --- |
| 视觉 | **675 / 675** |
| 单测（排除 `src/rpc`） | 347 文件 / **1968 全通过** |
| 门禁 | 22 / 22 单跑全绿，`check:bundle` 全项 OK |
| 代码改动 | **无** —— 这轮的产出是一张「哪里不用再看」的图 |

## Round 185 — 「最后注册的赢」，赢的是谁没人在看

`keymap.ts` 的规则写得很清楚：

> the "last registration of a combo wins" rule

这条规则本身是对的 —— 插件要能覆盖默认绑定。但它**分不清两件事**：
一次有意的覆盖，和两个插件碰巧想要同一个和弦。
两种情况下折出来的 keymap 都长得正常，快捷键面板列出的也都是赢家。
输的那条命令**从此无法触达，而且不报任何东西**。

所以检查要放在**折叠之前**，并且要在产品真正装载的那套插件上做。
`builtinManifest.test.ts` 已经在断言「插件身份唯一」，这条是它的同族。

### 写这条测试的过程本身找到了两个坑

1. 我先按 `command.key` 取绑定 —— 错的字段。18 条命令一条都没匹配上，
   于是 `bindings` 只有 1 项。**是我加的下限断言（`> 3`）把它拦下来的** ——
   没有那条，这个测试会以「零冲突」的姿态永远绿着，而它一个绑定都没看。
   真实字段是 `command.combo`，`commandShortcuts()` 从它派生。
2. 112 个内置插件里，只有 18 条命令 + 1 条快捷键注册了按键。绑定数 16。

### 结果

今天 **16 个键，每个恰好一个所有者，零冲突**。
`DELIBERATE_OVERRIDES` 是空的 —— 这是诚实的：现在没有需要声明的覆盖。

**验证过会失败**：把 `chat.search` 的 `Mod+F` 改成 `Mod+B`，
测试立刻报 `$mod+KeyB <- command chat.search, command view.toggle-sidebar` ——
两个所有者都点名。

### 验收

| | 结果 |
| --- | --- |
| 单测（排除 `src/rpc`）| 348 文件 / **1969 全通过**（+1）|
| 快捷键冲突 | 0，且现在有断言守着 |
| 下限断言 | 拦下了我自己写的一个空测试 |

## Round 186 — 被遮蔽的贡献，和我自己给单测开的那个 socket

上一轮的快捷键冲突检查只覆盖了一个点。把同样的问题推广到全部扩展点：

`keying: "single"` 的点上，同一个 key 有两份贡献 = 后者遮蔽前者。
这**是设计**：`keying` 是**读策略**，遮蔽方卸载后被遮的那份会回来 ——
第三方插件覆盖默认值就该是这个形状。

**但两个内置之间不该发生**：它们是同一个产品，不存在互相覆盖的意图。
而 `contributionsTo` 在任何外部代码看到之前就已经把这一对折成一份，
所以输的那个**不留痕迹**。

要看见它，只能读折叠之前的原始 view —— `publishedKernel()` 是导出的，
`host.contributions(token)` 就是解析所依据的那份，**不需要为测试新增任何生产 API**。

25 个 single 点、**160 条贡献、零遮蔽**。
`builtinManifest.test.ts` 本来就在断言「插件身份唯一」，这条是它的同族，放在一起。

**验证过会失败**：把 `BRAND_ICONS_PANE` 改成 `"plugins"`，
立刻报 `SETTINGS_PANE[plugins] <- flame.builtin.plugins-pane, flame.builtin.icon-gallery` —— 两个都点名。

### 我自己引入的问题，同一轮修掉

这两条测试要装载全部 112 个内置插件，于是**跟 Runtime 说话的插件真的去连了 `127.0.0.1:17171`**，
suite 里冒出 8 次 `ECONNREFUSED`。

这不只是噪音：**一条单测的结果不该取决于本机有没有东西在监听那个端口**。
开发机上常常是有的 —— 那时这两条测试跑的就是另一件事了。

修法是把 `fetch` 和 `EventSource` 在这两个文件里存根掉。改完 socket 归零，测试照过。

（上一轮那条快捷键测试是我加的，所以这个洞也是我开的 —— 同一轮堵上。）

### 验收

| | 结果 |
| --- | --- |
| 单测（排除 `src/rpc`）| 348 文件 / **1970 全通过**（+1）|
| 被遮蔽的内置贡献 | 0，且有断言守着 25 个点 |
| 测试里的网络连接 | 8 → **0** |
| 视觉套件 | **未跑** —— 本轮只改了两个测试文件，没有一行生产代码 |

## Round 187 — 我自己那条守卫的洞，就在它修好的东西旁边

Round 179 加了「英文标题用 sentence case」的规则，修了三条 Title Case。
这一轮渲染每一个注册过的工作区视图时，看见了第 22 条：

```
"workspace.view.title.iconGallery": "Icon Gallery"
```

二十二个视图标题，二十一个是 sentence case（`Agent docs` / `File preview` /
`Run summary` / `Changed files` / 上一轮改的 `Skill library`），只有它是 Title Case。

**我的规则看不见它**：判据写的是「key 以 `.title` 结尾」，
而视图标题的 key 结尾是视图自己的 camel id —— `workspace.view.title.iconGallery`。
一条按后缀判断的规则，漏掉了整整一族标题，而且就在它刚修好的三条旁边。

判据改成「以 `.title` 结尾**或**在 `workspace.view.title.` 之下」。
**验证过会失败**：把 `Icon Gallery` 放回去，立刻点名。

（其他七种语言这条都按自己的规范译好了，只有英文漂了 —— 和上一轮一样。）

### 顺带把每一个注册视图都渲染了一遍

用 `?full-view=` 逐个打开 24 个：

- 21 个正常渲染；
- `tool-inspector` / `traces` / `diagnostics` 显示 **"View unavailable"** ——
  它们是诊断插件，fixture 不装载，兜底文案是对的；
- `plan` 在全屏下是空态，而 dock 里有内容 —— 查过：**是 fixture 的两个会话不同**
  （dock 那条的标签上有 `1/3` 徽章，全屏那条没有），不是缺陷。

### 一个报告，不是修复

`IconGallery` 是 20 个视图里**唯一不用 `WorkspaceViewLayout` 的**（19 个用）。
它自己画标题（`@lobehub/icons`），所以在全屏下没有标准头栏，
而它注册的名字（`Icon gallery`）在自己的正文里从不出现。
按硬约定「业务层不自己拼交互件」，这是那个形状；
但给它加头栏是可见的设计改动 —— **留给你定**。

### 关于这几轮的红：是机器，不是代码

最近几次全量出现了**轮换的单张 golden 失败**（`closing tabs` / `dock-timeline` /
`Retina closure`），每一张单独跑都 2–3 次全绿。

对上时长就清楚了：**9.7m → 11.5m → 13.5m → 16.2m**。
查过进程：**没有我漏掉的 dev server**，但机器上有 7 个 `claude` 进程、
其中一个占 29% CPU、几个跑了一天以上。

所以这些不是缺陷，是竞争下的边缘 golden。**不追它们**，如实记下来 ——
`tool-tail` 那条我追到了根（fixture 的滚动落点带记忆），这几条没有对应的证据。

## Round 188 — 「存进去的能不能拿回来」，三个持久化 store 都没人问过

机器被别的会话占着（视觉套件从 9.7 分钟涨到 16.2 分钟），所以这轮挑不需要反复跑视觉的活。

八个 `persist` store 都有 `version` —— 但**没有任何东西检查「形状变了要 bump」**，
而这条规则在桌面端是真会疼的：用户是原地升级的。

### 一个事实，两个所有者

三个 store 用了 `partialize`，每一个都配着一个手工维护的解析器：

```ts
// agentSessionStore.ts
// Mirrors `partialize` below.        ← 注释自己承认了
const sessionPersistSchema = z.object({ ... });
```

**Zod 的 object 默认会剥掉未知键。** 所以往 `partialize` 里加一个字段、忘了加进 schema，
那个字段**会被写进 localStorage，然后在下一次启动时被静默丢掉** ——
设置不生效，重启就没了，哪里都不报错。

而现有的持久化测试**全都是手写 payload 再 rehydrate**，
所以它们会一路绿着穿过这个洞。

### 治法：往返，而且一个字段名都不提

```ts
const written = JSON.parse(localStorage.getItem(key)).state;   // store 自己写的
// …清空、把 payload 放回去、rehydrate…
expect(JSON.parse(JSON.stringify(partialize(store())))).toEqual(written);
```

拿 store 自己的选择跟它自己比。**不点任何字段名**，所以形状改了不用改测试，
也没法用「读回一个子集」糊过去。

写的时候踩到一个真实的坑：**清空 store 会把清空后的状态也持久化掉** ——
所以要先把 payload 存下来、清空之后再放回去，否则读回的是清空写进去的那份，
测试会以「空对空」的方式通过。

**三个都验证过会失败**：往 `partialize` 加一个不成对的字段 / 让解析器拒绝一切 —— 立刻红。

### 第三个 store 一个测试都没有，而它存的是用户没发出去的字

`composerStore` 持久化草稿 —— **整个文件没有测试**。
「打了一半的字，重启还在」是桌面端最该守住的承诺之一。

补了两条：草稿往返，以及**图片不许进 storage**
（`partialize` 的注释说了「Text-only: images are transient」——
一张截图的 data URL 进 localStorage 就是一次配额爆掉，现在这条被守住了）。

### 验收

| | 结果 |
| --- | --- |
| 单测（排除 `src/rpc`）| 349 文件 / **1974 全通过**（+4）|
| 持久化往返覆盖 | 0 / 3 → **3 / 3**，每条都验证过能失败 |
| 视觉套件 | **未跑** —— 本轮只加测试，没改一行生产代码 |

## Round 189 — 把「存进去的能不能拿回来」交给编译器

上一轮给三个用 `partialize` 的 store 补了运行时往返。
剩下五个不用 `partialize`（持久化整个数据状态），它们的配对**可以在编译期查**，那比测试更好。

### 只有一个方向是静默的，而那正是要补的那个

`rehydrateOrDefault` 的签名约束 `State extends Restored`，所以：

| 情况 | 现状 |
| --- | --- |
| schema **比** state **宽**（多一个键）| **已经会报错** —— 那个约束不满足 |
| schema **比** state **窄**（少一个键）| **编译通过** —— state 当然 extends 一个子集 |

而窄的那个方向正是会丢东西的：Zod 的 object 剥掉未名键，
所以少写一个字段 = 写进 localStorage、下次启动被丢掉 = **用户设了、重启就没了，哪里都不报**。

### 一条规则，五个 store

`Paired<State, Persisted>` 放在 `lib/persistedStore.ts` —— 也就是 `rehydrateOrDefault` 所在的地方，
共享的解析策略本来就该在同一个所有者名下。它按「键的值是不是函数」过滤，
所以既能服务把 state 和 actions 分开声明的 store，也能服务写在一起的。

不解析成 `false`，而是解析成**出问题的那些键**：

```
stateHasWhatTheSchemaWillStrip: "cursorStyle"
```

类型错误直接说出是哪一个。

**五个都验证过**：往 state 加一个 schema 没有的字段 —— `streamReveal` / `completionSound` /
`shellLayoutStore` / `appearance` 全部立刻在编译期报 `stateHasWhatTheSchemaWillStrip`。
反方向也验过（往 schema 加一个 state 没有的键），确认是被 `rehydrateOrDefault` 的约束先拦下的 ——
所以注释改成了这个事实，而不是笼统地说「两个方向都是本规则的功劳」。

### 一个操作失误，记下来

验证的时候我用 `git checkout --` 撤销故意制造的破坏 ——
**那会把同一个文件里我刚加的断言一起撤掉**。事后逐个文件核过 `_paired` 还在。
撤销要按改动撤，不能按文件撤。

（另外全量 `vitest run` 这次被 OOM 杀掉了 —— 机器上还有别的会话在跑。
改成分片跑：61 + 117 个文件，1184 条断言全过。）

### 验收

| | 结果 |
| --- | --- |
| 编译期配对 | 0 / 5 → **5 / 5**，每个都验证过会失败 |
| 单测（分片） | 178 文件 / **1184 全通过** |
| 守卫 | `check:styles` / `check:classes` / `check:shorthand` 全绿 |
| 视觉套件 | **未跑** —— 只加了类型断言，没改任何运行时行为 |

## Round 190 — 第三面镜子，不在那个专门放镜子的文件里

`stylesheetMirror.test.ts` 开头就把原则写清楚了：

> the stylesheet is what the first paint reads, and it cannot call a function — so what needs
> guarding is that they still say the same thing.
> A drift here never fails loudly; it is a frame of the old design on a cold start, which
> reads as the app settling.

它守着两面镜子：调色板、visual style 的令牌。
**第三面不在里面** —— 密度阶梯。

`density.ts` 的 `BASE_PX` 有 13 个 comfortable 值，`globals.css` 的 `:root` 把同样 13 个
写成字面量（因为首帧读的是它，而它不能调用函数）。**一份清单，两个所有者。**
改了一边忘了另一边 = 每次冷启动先按旧行高排版、然后跳一下。

今天 13 个全对。补上守卫，并且和调色板那两面有一处**故意的不同**：

| | 调色板 | 密度 |
| --- | --- | --- |
| 要求 | 只比**块里声明过的**那些 | **13 个全都必须声明** |
| 理由 | dark 块省略一个令牌是**故意继承** light 的 | 密度变量省略了就**没有任何兜底** —— 那是没有样式的首帧，不是过时的首帧 |

所以断言是 `compared === 13`，而不是「比过的都一致」。
**两个方向都验证过**：改一个值 → 报 `css=34px spec=36px`；
从 `globals.css` 删掉一个声明 → 报 `a density variable the stylesheet never declares`。

（`density.ts` 之前一个测试都没有。`--density-row-height` 在 Round 172 拿到了一条视觉断言，
其余 12 个到现在为止没有任何东西看着。）

### 上一轮那个失误，这轮避开了

又要用 `git checkout --` 撤销故意制造的破坏。这次先确认 `density.ts` 和 `globals.css`
**本轮没有我的其他改动**，再撤 —— 所以没有连带损失。

### 验收

| | 结果 |
| --- | --- |
| 单测 | 32 文件 / **213 全通过**（+1）|
| 首帧镜子 | 2 面 → **3 面** |
| 密度变量守卫 | 1 / 13（视觉）→ **13 / 13**（编译期读 CSS 文本）|
| 视觉套件 | **未跑** —— 只加了一条单测 |

## Round 191 —— 第四面镜子：字号阶梯

上一轮补完密度阶梯之后，我没有停在"这面补上了"，而是把**同一个形状**在仓库里搜干净：
哪些 TypeScript 模块会写一个 `globals.css` 也用字面量声明过的 CSS 变量？

| 写者 | 变量数 | 状态 |
| --- | --- | --- |
| `theme/kit/palette.ts` | 调色板 | Round 168 已守 |
| `theme/kit/visualStyle.ts` | style 令牌 | Round 168 已守 |
| `theme/kit/density.ts` | 13 | Round 190 已守 |
| **`theme/kit/typeLadder.ts`** | **11 (`--fs-*`)** | **没有任何东西看着** |
| `chat/ui/ChatSearchOverlay.tsx` | 1 (`--wails-draggable`) | 不是镜子（运行时开关，CSS 侧没有对应字面量清单）|

所以是 **4 面**，不是 3 面 —— 而且第四面的赌注比前三面都大：
调色板漂移是首帧颜色不对，密度漂移是首帧行高不对，**字号漂移会把每一段文字重排**。
11 个值今天全对（`--fs-ui-2xs=11px` … `--fs-display-lg=24px`）。

### 没有写第二个近似副本

`density` 和 `typeLadder` 的守卫逻辑逐字相同：算出 writer 的输出 → 去掉 `--` → 和 `:root`
块里声明的字面量逐个比 → 要求**每一个都被声明过**（不是"比过的都一致"）。
两份 30 行的近似代码，是下一次只改一份的由来。折成一张 `describe.each` 表：

| | 之前 | 之后 |
| --- | --- | --- |
| 守卫的镜子 | 3 面 | **4 面** |
| `stylesheetMirror.test.ts` 里的密度/字号代码 | 1 份 | **1 份**（表驱动，加第五面 = 加一行）|

### 三个方向都验证过

`compared === written.length` 和 `> 10` 的下限各自挡的是不同的错，分开验：

| 故意制造的破坏 | 报什么 |
| --- | --- |
| `globals.css` 的 `--fs-ui-md: 14px` → `15px` | 值不一致 |
| writer 里删掉 `--fs-code` | 撞下限（样式表声明了没人覆盖的变量）|
| writer 里加一个 `--fs-caption`（CSS 里没有） | `a variable the stylesheet never declares` |

破坏前都先确认那两个文件**本轮没有我的其他改动**，再 `git checkout --` 撤 —— 这是 Round 189 交的学费。

### 验收

| | 结果 |
| --- | --- |
| 单测 | 32 文件 / **214 全通过**（+1）|
| 首帧镜子 | 3 面 → **4 面** |
| 字号变量守卫 | 0 / 11 → **11 / 11** |
| typecheck / prettier | 全绿 |
| 视觉套件 | **未跑** —— 本轮零生产代码改动 |

## Round 192 —— 那道谁都看不见的 focus 环

`CLAUDE.md` 写着：键盘 focus 环由 `globals.css` 里**唯一一条全局规则**画。环的周围已经有三个审计
—— 几何（有没有地方画得下）、opt-out（说了"用行状态代替"的有没有兑现）、静态守卫（别在 callsite
自己画一个）。**没有一个问"它到底画出来了吗"。**

于是量了：Tab 走完 10 条 fixture 路线，对每个**没有** opt-out 的控件读 `outline-style` ——

**121 个里 109 个什么都不显示。** 整个键盘可达性指示，全product 范围内没有。

### 根因一：环有一个所有者，"不画环"有二十个

环只说一遍；"鼠标点击时别画环"在 callsite 说了二十遍，形式是 StyleX 里的 `outline: "none"`。

| 规则 | 特异度 | 说什么 |
| --- | --- | --- |
| `globals.css` | (0,4,3) | `outline: 1.5px solid var(--color-focus-ring)` |
| StyleX 原子类 | **(2,2,0)** | `outline: none` |

Tailwind 时代同一句话编译进 `@layer utilities`，被无 layer 的全局规则压住 —— 所以它**当时真的无害**，
`check-interactive-chrome` 还专门用文字祝福过它：*"`focus-visible:outline-none` stays legal"*。
StyleX 把这个关系倒过来了：每条声明带三个 `:not(#\#)`，于是它连着浏览器默认环一起，
把设计自己的环也压掉了。二十处里大部分来自 `Button`，所以它是全局的。

**一道 focus 环在有人去碰键盘之前是不可见的**，所以什么都没有响。

### 根因二：那个守卫的九条规则里八条在 Tailwind 走的时候瞎了

`check-interactive-chrome` 每条 pattern 都是 Tailwind class 正则（`hover:bg-fg/[0.04]`、
`active:scale-96`、`ring-2`、`duration-150`）。它读 1258 个文件、过自己 500 的下限、然后打印
**"hover + selected + press + focus + motion each hold one value"** —— 一句它已经无法核实的话。
只有 `<Icon aria-label>` 那条还在开火。

按 StyleX 形状重新量了另外七条：实质上都是干净的（`":hover": surface.hover` 正是它想要的 ink wash），
所以**瞎掉的这段时间里，只有 focus 这一条真的被违反了 —— 而它被违反得彻底**。

### 根因三：改完才露出来的 —— 视觉台架从来没有过这个 gate

删掉二十处之后，6 个视觉测试红了。不是机器争抢：**`visual/index.html` 没有任何 pre-module bootstrap**。
`data-pointer` 从不设置 → `html:not([data-pointer])` 永远成立 → 每一张截图拍的都是一个
**没有 modality gate 的 app**。而那个名叫
「a mouse-opened menu shows its highlighted item, not a ring around itself」的测试，
自己的注释就写着这个 gate *"was written by nobody"* ——
它一直靠我刚删掉的 callsite `outline: none` 通过，**从没有一次真的走过它点名的那条 gate**。
`vite.visual.config.ts` 的文档串写的是 fixtures "exercise the same visual implementation"。

### 治本

| | 之前 | 之后 |
| --- | --- | --- |
| 画环 | `globals.css` 一条 | 不变 |
| **不画环** | 20 处 callsite（且压过上面那条） | **`globals.css` 一条** `html[data-pointer] :focus-visible` |
| `@layer base` 的 `[data-control="button"]:focus-visible{outline:none}` | 只覆盖 button 这一半 | 删掉（新规则覆盖全部）|
| modality 脚本 | `index.html` 内联，台架没有 | **`public/focus-modality.js`，两个入口都 load** |
| callsite 的 `outline` | 20 处压制 + 1 处真值 | **只剩那 1 处真值**（bare field 标 invalid，`input` 本就被环规则排除）|

有真实理由不要环的（menu popup、question card、modal），理由的正确表达是
`data-chrome-focus` —— 那个有文档、被 `chromeFocus.visual.spec.ts` 守着的 opt-out。它们本来就带着它，
所以 `outline: none` 是纯冗余。

### 三个守卫，每个都验证过会失败

| 守卫 | 挡什么 | 制造的破坏 → 报什么 |
| --- | --- | --- |
| `check:chrome`（新规则，替掉瞎掉的那条） | callsite 压制环 | 四种形状（`outline:"none"` / `:focus-visible` 键 / `outlineStyle` / `outlineWidth: 0`）**全部命中**；真 outline 值和 `outlineOffset` 放过 |
| `focusRing.visual.spec.ts`（新测试，加在已经拥有这道环的文件里） | 环没画出来 | 恢复 `button.tsx` 一行 → **193 个控件静默** |
| `check:bootstrap`（扩了两条） | 台架丢 bootstrap；CSS 少一半 gate | 各自精确报出 |

新测试有**两个**断言，因为"有环"是弱的那个：全 product 的环还必须是**同一个环** ——
style / width / color 的指纹集合大小必须是 1，否则"一条规则画所有环"这个设计已经没了，无论像素长什么样。

### 又踩了 Round 189 那个坑，这次是真的踩进去了

验证守卫要先制造破坏，再 `git checkout --` 撤。`button.tsx` **本轮有我的改动**（就是那行删除），
我却先跑了 `git diff --stat`、看到 "1 deletion"、然后照样 checkout —— 把自己的修复一起撤了。
被随后的 grep 和守卫立刻抓到。此后改用定点 `replace`，不用 checkout。

### 验收

| | 结果 |
| --- | --- |
| 有环的控件（不含 opt-out） | **12 / 121 → 121 / 121** |
| 环的指纹种类 | 未测 → **1** |
| callsite 的环压制 | 20 → **0** |
| pre-module bootstrap 的所有者 | `index.html` 内联（台架没有）→ **1 个文件，2 个入口** |
| `check:chrome` 活着的规则 | 1 / 9 → **2 / 9**（其余 7 条见下）|
| 单测 | **1463 全通过**（21+45+144+55+4 文件分片跑）|
| 视觉套件 | **676 全通过**（9.9m，零失败）|
| typecheck / lint / prettier / knip / 18 个守卫 / build | 全绿 |

### 仍然欠着

`check:chrome` 还有 **7 条规则是 Tailwind 形状、认不出 StyleX**。今天量过它们实质干净，
但守卫不是靠"今天干净"活着的。下一轮把它们按 StyleX 形状重写，每条都验证会失败。

## Round 193 —— 把瞎掉的七条规则重新装上眼睛

上一轮欠着的：`check-interactive-chrome` 九条规则里，除了 focus 那条（上轮重写）和
`<Icon aria-label>`，剩下**七条全是 Tailwind class 正则**。Tailwind 走了之后它们一个都匹配不到，
而守卫照旧读 1258 个文件、打印同一句自信的话。

### 七条按 StyleX / CSS 形状重写

| 规则 | Tailwind 时代的形状 | 现在读什么 |
| --- | --- | --- |
| 手挑的状态色 | `hover:bg-fg/[0.04]` | 状态 key 下的**颜色字面量**（`rgba(` / `color-mix(` / `#…`）|
| surface 台阶当 hover | `hover:bg-surface-3` + `bg-transparent` | `":hover": surface.surfaceN` 且 rest 是 `transparent` / `null` |
| 手写按下量 | `active:scale-95` | `:active": 0.96`（`1` 是 disabled 的取消值，放过）|
| hover 用 opacity 回答 | `hover:opacity-100` | `opacity: { … ":hover" … }` |
| `:has(:focus-visible)` | `has-[:focus-visible]` | `:has(:focus-visible)` |
| 全属性 transition | `transition-all` | `transitionProperty: "all"` / CSS `transition: all` |
| 手写时长 | `duration-150` | `transitionDuration` / `transitionDelay` / CSS 同名的 `\d+ms` |

**七条全部验证过会失败**：种下 7 个违规 → 命中 10 次（`transition: all 200ms`
同时踩时长和属性两条），4 个合法写法全部放过 —— 包括 `default: surface.surface2 → surface3`
这种"本来就有底色、可以升一档"的 soft，和 `":is(:disabled):active": 1`。

### 实质上只有一处真违规

按新形状重量了整棵树：按下量全是 `var(--press-scale)`，时长全是 `motion.*`，
reveal 早就收进 `reveal.ts` 的 `--reveal` 变量，`:has(:focus-visible)` 只出现在**警告它的注释里**。
唯一的真违规：

`TextButton` 的 `negative`：`opacity: { ":hover": 0.8 }` —— 全树唯一一个用 opacity 回答 hover 的，
而且是**同一个组件内的第三种拼法**（`muted` / `faint` 用颜色台阶，`row` 用 ink wash）。

根因不是粗心：`muted` / `faint` 的 hover 是"往 `fg` 提亮"，而 `negative` 本来就在满强度上，
**没有更亮的地方可去** —— 所以有人伸手拿了 opacity。但把警示色调淡，正好削弱它唯一在传达的东西。
改用这个文件里 `link` 已经在用的机制：下划线（`textDecorationColor` transparent → currentColor），
不需要新 token，静止态像素完全不变（`base` 的 `transitionProperty` 本来就含 `text-decoration-color`）。

### 顺手修掉三处"读着像在做事、其实不做事"

| | 问题 |
| --- | --- |
| 注释行 | 两条规则在注释里**拼出了自己禁止的写法**，于是守卫报了自己的文档（2 处误报）。改成：以注释开头的行一律跳过 —— 注释不给任何东西上样式 |
| rule 4 的 `appliesTo` | 还在测两个 Tailwind 拼法，**都已不可能出现**，所以它永远返回 true。删掉，写明它现在管两种形状（reveal 和 dim）|
| 我自己刚留下的重复注释 | 插入新句时没删旧句，同一件事写了两遍 |
| 守卫头部 | 还在讲 focus 环的**上一个**故事（16 处各画一个），极性和现在正好相反。改成两次都讲，并写明八条规则是怎么一起瞎掉的 |

### 时长规则顺带补了 delay

`transitionDelay` 和 CSS `transition-delay` 是同一类值，原规则读不到。树里今天没有字面量
delay，但守卫不是靠"今天没有"活着的 —— 加进去并验证会失败。

### 验收

| | 结果 |
| --- | --- |
| `check:chrome` 活着的规则 | 2 / 9 → **9 / 9**，每条都见过它失败 |
| 真违规 | 1 处（唯一的 opacity hover）→ **0** |
| 一个组件内 hover 的拼法 | 3 种 → **2 种**（颜色台阶 / ink wash；下划线是文本态的同一族）|
| 单测 | **235 全通过**（`src/ui` + settings 分片）|
| 视觉套件 | **676 全通过**（10.0m，零失败）|
| typecheck / lint / prettier / knip / 守卫 | 全绿 |

### 仍然欠着

- **`TextButton tone="negative"` 只有一个消费方，而它从没进过任何截图**：
  它只在插件有 `errCount > 0` 时渲染，而 error 来自 `reportPluginError`（渲染边界的捕获），
  fixture 从不触发。这轮改的正是它的 hover。
- 静态守卫读的是**源码**里的 hover 拼法；**屏幕上**的 hover 没有任何东西看着 ——
  focus 环这一轮刚证明这两件事可以完全不一致。下一轮做 hover 的运行时扫描。

## Round 194 —— 一个不是 wash 的 "ink wash"

上一轮的结论：静态守卫读的是**源码**里的 hover 拼法，屏幕上的 hover 没有任何东西看着 ——
而 focus 环刚证明这两件事可以完全对不上。所以这轮把每个控件都 hover 一遍，diff 计算后的样式。

**184 个控件，34 个毫无反应，14 种不同的答案。** 其中两种是真缺陷：

| 量到的 | 意思 |
| --- | --- |
| `5x  背景 4% → 3%` | **hover 一个已选中的行，把它变淡了** |
| `1x  背景 #eaf0fb → 3% 半透明` | **hover 一个凹陷的行，它丢掉了整个凹陷**，读起来像从表面弹出来了 |

（另外 `41x color` 和 `41x textDecorationColor` 是同一次颜色变化的影子 —— 未设置时它跟随
`currentColor`。`18x borderColor` 同理。不是三种答案，是一种。）

### 根因：机制和它自己的模型矛盾

`CLAUDE.md` 把这两个状态称作 **ink wash** —— 墨水**盖在**已有的东西上。
但它们被写进了 `background-color`，而 `background-color` 只能装一个值 ——
**所以这个 wash 是替换，不是覆盖。一个会替换的 wash 不是 wash。**

这不是写错了顺序。三处站点都把 selected 的 key 写在 `:hover` **后面**，量到的却是 hover 赢：

| 规则 | 特异度 |
| --- | --- |
| `.x…:hover:not(#\#)×3` | (3,2,0) |
| `.x…:is([data-active]):not(#\#)×3` | **(3,2,0)** |

**特异度相同**，于是表里的顺序决定胜负，而 StyleX 把 `:hover` 排在属性条件之后。
在同一个对象里调换书写顺序**改不动它**。

### 治本：把墨水盖在真正在那儿的东西上

产品里只有两种"有底色还要接 hover"的静止态，所以只要两个值，不是组合爆炸：

| 新 token | 构造 | 为什么是这个数 |
| --- | --- | --- |
| `--wash-selected-hover` | `depth-step * 1.75`，透明底 | 两层 wash 叠起来正好是这个 alpha（1-(1-.03)(1-.04)≈.069），**不是挑的** |
| `--color-sunken-hover` | `depth-step * 0.75` 盖在 `--color-sunken` 上 | 就是"hover 的那层墨水，盖在凹陷上" |

两个都用 `--depth-step` 推导，所以每个主题、每个 visual style 自动跟着走。
**这个写法是这张表自己的成语** —— `--app-dock-tabstrip-surface` 早就在把同一层墨水盖在
tabstrip 上（`color-mix(text at depth-step*0.75, over --app-dock-surface)`）。

selected 的那三处用组合 key `":is([data-active]):hover"` —— 特异度 (3,3,0)，**同时压过两者**，
不依赖表里的顺序。凹陷的三处直接换 `":hover": surface.sunkenHover`，两个不同的 key 之间没有争议。

改完重量：

| | 之前 | 之后 |
| --- | --- | --- |
| 不同的背景答案 | 14（含 2 种缺陷） | **5，每一种都是加墨水** |
| 选中行 hover | 4% → **3%** | 4% → **7%** |
| 凹陷行 hover | 凹陷 → 3% 中性 wash | 凹陷 **+ 一层墨水** |
| 透明行 hover（123 个） | 3% wash ✓ | 不变 |
| 填充控件 hover | accent → 深 accent ✓ / surface2 → surface3 ✓ | 不变 |

### 新守卫：`hoverAnswers.visual.spec.ts`

两条断言，一条是缺陷的**精确形状**，一条是答案的**整体形状**：

1. 有底色的控件，hover 后**不能落在那个中性 wash 上** —— 落在那儿就说明 wash 把底色替换掉了。
   （中性 wash 的值是运行时从 `var(--wash-hover)` 探出来的，不是写死在测试里 —— 写死的话主题一动它就说谎。）
2. 不同答案的**数量有上限**（8）。一个手势应该有一小把答案，不是一个站点一个。
   这正是那个"值守卫"存在的理由、也正是它看不见的东西。

**两个方向都验证过会失败**：把凹陷那处换回中性 wash → 报出来；把选中的组合 key 删掉 → 报出来。

### 两个自己的失误

| | |
| --- | --- |
| Python 的 `str.replace("", x)` **是前插** | 验证守卫时用 `''` 当"删掉这行"的替换值，还原时 `replace('', line)` 把那行插到了文件**开头** —— `vertical-tabs.tsx` 被我写坏。typecheck 立刻报出来，随即修好；此后 swap 两侧都要求非空 |
| 第一次验证 A 方向"通过"了 | 我改的是 `SearchResults.tsx`，而它**不在这 5 条路线里** —— 不是守卫的问题，是我选的站点没有覆盖。换成侧栏搜索行（真的被扫到）后立刻报错 |

### 验收

| | 结果 |
| --- | --- |
| hover 的缺陷答案 | 2 种 / 6 个控件 → **0** |
| 不同 hover 答案 | 14 → **5**（上限 8）|
| 屏幕上的 hover 守卫 | 无 → **1 条 spec、2 条断言、各自见过失败** |
| 单测 | **121 全通过** |
| 视觉套件 | **677 全通过**（11.3m，零失败；+1 就是新守卫）|
| typecheck / lint / prettier / knip / 守卫 / | 全绿 |

### 仍然欠着

- **34 / 184 个控件 hover 毫无反应** —— 包括 "Jump to bottom"、"Copy message"、"Tool search"
  和几个工具 chip。textarea 不算（插入符就是反馈）。一个 `<button>` 不回应指针，
  是它不承认自己被指着。这批要一个个看：有的可能确实该静默，有的是
  `Button` 的 `active`（写在 variants 之后，"outranks whichever fill they gave"）把 hover 一起盖掉了。
- 第三处凹陷站点 `SearchResults.tsx` 修了，但**这 5 条路线到不了它** —— 和上一轮那个
  `TextButton tone="negative"` 一样，是改了却没被拍到的东西。

## Round 195 —— 上一轮那个守卫，可能在量错的元素

本来是去查上一轮欠着的"34 个 hover 无反应的控件"。查到的第一件事是：**那个数字不可信，
而且不可信的原因在我上一轮刚提交的守卫里。**

### 三个叠加的测量错误，每一个都让沉默看起来是真的

| | 错在哪 | 后果 |
| --- | --- | --- |
| **索引不稳定** | hover 会**挂载和卸载控件**（hover 一条消息会显出它的操作行），所以 move 之后 `locator(CONTROL).nth(i)` 解析到的**可能是另一个元素** | 拿两个不同的控件在比 |
| **坐标是陈旧的** | 一次 `evaluateAll` 把所有 box 量完，然后连续 move 很多次 —— 后面的坐标读的是**已经被前面的 hover 改过**的布局 | 指针落在别处 |
| **没有验证指针到没到** | 只比样式，不问 `:hover` 到底命中了没有 | 布局从光标底下移开的控件，报告成"忽略指针的控件" |

用 `elementFromPoint` 一验就露了：那批"沉默"控件里大多数，采样中心点上的元素是**完全无关的节点**
—— `Timeline` 那个点上是一个关闭图标的 `svg`，`Explorer` 那个点上是 header 里的一个 `a`。
（顺带：修掉"每次读静止态前先把指针停走"之后，34 立刻变成 25 —— 有 9 个是被上一个控件的
hover 污染的，设置面板的 segmented tab 和开关其实一直是好的。）

### 重写

| | 之前 | 之后 |
| --- | --- | --- |
| 身份 | `nth(i)`，索引 | **`data-hover-probe="i"` 属性**，DOM 重排也认得 |
| 坐标 | 一次量完全部 | **每个控件在指针停走后单独量**（那才是静止态的 box）|
| 到达 | 不问 | **`node.matches(":hover")`**，没到达就不计入 |
| 到不了的控件 | 混进"没反应"里 | **单独报出来** |

没有用 `locator.hover()`：它会等元素"能接收指针事件"，而被别的东西盖住的控件**永远等不到**，
于是整条 walk 挂在它上面而不是把它报出来 —— 实测就是 140s 超时。

**下限现在挂在"到达"上**（`:hover` 命中该元素本身），所以一条瞄着陈旧坐标的 walk **过不了这个下限**。
之前那句 `> 100` 是拿错配的元素对凑出来的。

| | 之前（不可信） | 之后（每一个都验证过指针到达）|
| --- | --- | --- |
| 真正 hover 到的控件 | 声称 > 100 | **140** |
| 指针根本没到的控件 | 混在"沉默"里 | **17，单独报出** |

### 两个方向重新验证过

把凹陷那处换回中性 wash → 报错。删掉 selected 的组合 key → 报错。

**第二次验证我第一次做错了**：想"破坏"组合 key，写成 `":is([data-active]):not(#nope):hover"` ——
`:not(#nope)` 是**提高**特异度，规则照样赢，于是"通过"了。真正删掉那一行才报错。
特异度的直觉在这套 StyleX 上错了两次了，这次是我自己的验证栽在同一件事上。

### 验收

| | 结果 |
| --- | --- |
| 守卫比较的元素 | 索引解析（可能错配）→ **属性绑定** |
| 指针到达 | 未验证 → **每个控件都验证** |
| 视觉套件 | **677 全通过**（11.5m，含争抢；零失败）|
| typecheck / lint / prettier / knip | 全绿 |
| 生产代码改动 | **零** —— 本轮只修测量 |

### 仍然欠着（现在的数字是可信的了）

- **17 个控件指针到不了它们静止态的中心点**。`elementFromPoint` 指向别的节点：
  `Copy message` / `Edit message` 上面盖着同一个 div。**一个指针到不了的控件，是一个点不到的控件** ——
  这个要单独查清楚：到底是 fixture 的那个状态下确实被盖住，还是产品里真的点不着。
- 剩下那些"真的对指针没反应"的控件要拿 `DESKTOP_UI_POLISH.md` 的规则量：
  它写的是 *"Hover is reserved for dense lists, sidebar rows, **icon buttons**, and controls
  where it improves scanability"*，并且明确把"给每个按钮都加 hover 背景"列为反面清单。
  所以**不是 21 个缺陷** —— 是要按这条规则一个个判。已知一处明确对不上：
  同一条 dock tabstrip 里，`Plan`（active，吃到上一轮的组合 key）有反应，
  `Explorer` / `File preview` / `Timeline` 没有。一个组件里的兄弟控件答案不一致，这条规则不背书。

## Round 196 —— 17 个"点不到的控件"，全部是我的测量错

上一轮把那 17 个"指针到不了"的控件单独报了出来，说要判定是 fixture 状态如此、还是产品真点不着。
**答案是第三种：都不是。17 个全部是测量错误，而且是三个各自不同的错。**

| 类别 | 证据 | 是谁 |
| --- | --- | --- |
| 本来就没显示 | `pointer-events: none` + `opacity: 0` | `Jump to bottom` ×3（滚到底部时它就该不在）|
| 被祖先 hover 揭示的 | 静止态 `visibility: hidden`，遮挡物**是它的祖先** | `Copy` / `Edit message` |
| **在 scroller 里滚出了可视区** | 自己可见、可命中，而那一点上画的是别的节点 | 其余 13 个 |

第三类是最有意思的：**一个控件自己 rect 的中心，不一定是它被画出来的地方。**
在 scroller 里滚出去之后，它的 viewport rect 仍然落在窗口里，
于是指针去的那一点上显示的是完全别的东西 —— `Timeline` 那点上是个关闭图标的 `svg`。
`focusRing.visual.spec.ts` 早就为它自己写过这条经验（"scrolled out of its own container
reports a 1000px cut and is not a defect"），这一条只是没有传过来。

### 三处修正

| | 之前 | 之后 |
| --- | --- | --- |
| 瞄哪一点 | 控件自己 rect 的中心 | **rect 与每一个会裁剪的祖先求交**之后的中心 |
| 没显示的控件 | 算进"对指针没反应" | `pointer-events: none` / `opacity: 0` / `input[type=range]` 直接跳过 |
| 揭示型控件 | 报成"到不了" | 移动之后再**补一个 1px 的微移** —— 到达和被 hover 不是一回事：`visibility: hidden` 的控件不可命中，移动把它揭示出来了，但它自己的 `:hover` 要等下一个指针事件才重算 |
| "有没有显示"什么时候判 | 移动之前 | **移动之后** —— 对揭示型控件来说，"有没有显示"正是这次移动决定的事。移动前问，分不出「dock tab 的 × 只是还没被揭示」和「消息操作在一棵 hidden 子树里」|

`input[type=range]` 的排除写在**这个 spec 里**，不写进共享的 `CONTROL` ——
那份清单还有两个审计在读，对它们来说滑块的 input 是个实打实的控件。
（Base UI 的 `Slider.Root` 把 input 当无障碍表面，指针面是 `Track` / `Thumb`。）

### 结果

| | Round 194 | Round 195 | **Round 196** |
| --- | --- | --- | --- |
| 声称 hover 到的控件 | > 100（错配的对） | 140（已验证到达） | **156** |
| 指针"到不了"的 | 混在沉默里 | 17 | **0** |

### 四个方向都验证过会失败

| 制造的破坏 | 报什么 |
| --- | --- |
| 凹陷行换回中性 wash | 替换了底色 |
| 删掉 selected 的组合 key | 替换了底色 |
| **两次移动都瞄偏 4000px** | `hovered 0, never arrived 156` → 撞到达下限 |

**第三条我第一次又做错了**：只把第一次移动瞄偏，忘了后面那个 1px 微移会把指针**移回目标上**，
于是 walk 照样到达、测试照样通过。两次都瞄偏才真的报错。
（这一轮里我自己的验证错了两次 —— 上一轮是 `:not(#nope)` 反而提高特异度。
**验证守卫这件事本身也需要被验证。**）

### 验收

| | 结果 |
| --- | --- |
| 视觉套件 | **677 全通过**（10.5m，零失败）|
| typecheck / lint / prettier / knip | 全绿 |
| 生产代码改动 | **零** —— 连续第二轮只修测量 |

### 仍然欠着

现在这份"对指针没反应"的名单终于可信了，可以拿 `DESKTOP_UI_POLISH.md` 的规则去判：
*"Hover is reserved for dense lists, sidebar rows, **icon buttons**, and controls where it
improves scanability"*，而"给每个按钮都加 hover 背景"是它明确的反面清单。
所以这不是"改 N 个缺陷"，是一条条对规则。已知一处明确对不上：
同一条 dock tabstrip 里 `Plan`（active）有反应，`Explorer` / `File preview` / `Timeline` 没有 ——
**一个组件里的兄弟控件答案不一致，这条规则不背书任何读法。**

## Round 197 —— 那个还没人看的元素：真正上色的那一个

名单可信之后（Round 196），拿 `DESKTOP_UI_POLISH.md` 的规则去判。**先纠正我上一轮的说法：**
`Explorer` / `File preview` / `Timeline` / `Copy message` / `Jump to bottom` **都是有反应的** ——
它们进名单是 scroller 裁剪和揭示型控件那两个测量错造成的。所以"同一条 tabstrip 里三个 tab 没反应"
这个说法是错的，我上一轮报错了。

把每个 tab 单独量一遍，真实的形状是**全产品一致的一条**：

| 分组 | 未选中 | **选中的那个** |
| --- | --- | --- |
| 设置侧栏（`vertical-tabs`）| 透明 → 3% ✓ | 4% → **7% ✓**（Round 194 修的）|
| segmented ×5 组 | `fgMuted → fg` ✓ | **什么都不变** |
| dock tabstrip | `fgMuted → fg` ✓ | **什么都不变** |

**一个分组里被选中的那个，是全产品唯一对指针毫无反应的控件。** 原因统一：
选中态是把文字挪到 `color.fg`，而 hover 去的也正是 `fg` —— 选中的那个**已经没地方可去了**。
设置侧栏之所以逃掉，只因为 Round 194 给它在**背景**通道上补了 selected+hover 的合成值。

### 但 dock tab 是个更严重的东西，而且我的守卫看不见它

`role="tab"` 的元素是一个内层 label，背景在**外层 wrapper** 上。量 wrapper 才看到真相：

```
ACTIVE "Plan1/3"   oklab(0.979564 …)  ->  color(srgb … / 0.03)
```

**选中的 dock tab 一被指到就丢掉它那层"抬起来"的填充，掉回中性 wash** ——
和 Round 194 修的是同一个缺陷、同一个根因（`:hover` 与 `:is([data-active])` 特异度相同、
hover 排在后面）。它躲过了上一轮的守卫，因为**守卫只量匹配 `CONTROL` 的元素，
而真正上色的是一个 wrapper**。

治本同上：一个从 `--dock-tab-active-surface` 推导出来的合成值，
`color-mix(text at depth-step*0.75, over 它自己的填充)` —— 和
`--app-dock-tabstrip-surface`、`--color-sunken-hover` 同一个构造。加组合 key `":is([data-active]):hover"`。

改完：`oklab(0.9796) → oklab(0.9574)`（它自己的填充，压上那层墨水），不再掉到 wash。

### 守卫的洞：往上走

"替换了底色"这条断言现在**从控件往上走祖先链**（最多 6 层，遇到第一个 scroller 停 ——
再往上是页面家具，不是这个控件的盒子）。而且这段检查放在**控件自己那个提前 return 之前** ——
dock tab 的 label 根本不变，`if (after.fill === before.rest) continue;` 正是把下面那个 wrapper
藏起来的东西。

**验证过**：把 dock tab 的组合 key 撤掉 → 守卫报错（之前它一声不出）。

### 验收

| | 结果 |
| --- | --- |
| 选中态被 hover 替换掉的填充 | 1 处（dock tab wrapper）→ **0** |
| 守卫看的元素 | 只有控件本身 → **控件 + 6 层祖先** |
| 单测 | **121 全通过** |
| 视觉套件 | **677 全通过**（10.3m，零失败）|
| typecheck / lint / prettier / knip / 8 个守卫 | 全绿 |

### 仍然欠着

- **5 个 segmented 分组里被选中的那个仍然对指针没反应**。它和别的不一样：选中态是一个
  `motion/react` 驱动的、会在 tab 之间移动的 **chip**（`position: absolute; inset: 0`，
  不透明 `surface.canvas`，是**选中那个 tab 的子元素**）。所以给 tab 自己的背景上墨水会被 chip 盖住 ——
  墨水得上在 **chip** 上。`reveal.ts` 已经有这个成语（宿主发布自定义属性、目标读它），
  因为 StyleX 没有后代选择器。这是一个可见控件的设计改动，单独一轮做。
- 剩下的沉默项按那条规则还要一个个判：3 个开关（`span[checkbox]`）、
  2 个工具汇总展开按钮、1 个 goal 按钮。文本输入框（4 个）不算 —— 插入符就是反馈。

## Round 198 —— chip 接住那层墨水，以及一个靠"编译缓存热不热"决定覆盖率的守卫

### 1. segmented 选中项终于回应指针

选中的那个 tab 的文字已经在 `color.fg` 上，而 hover 去的也是 `fg` —— **没地方可去**。
它有的是 **chip**：`position: absolute; inset: 0`，不透明，而且是**选中那个 tab 的子元素**。
所以墨水上在 tab 自己的背景上会被 chip 盖住，得上在 chip 上；而 StyleX 没有后代选择器。

用 `reveal.ts` 已有的成语：**宿主发布自定义属性，目标读它**，兜底值就是静止值。

```
tab:  "--segment-chip-fill": { default: surface.canvas, ":hover": surface.surface2 }
chip: backgroundColor: `var(--segment-chip-fill, ${surface.canvas})`
```

台阶用的是 `Button` 的 `raised` 变体**早就在用**的那一档（不透明抬起面：canvas → surface2），
所以**没有给交互模型增加任何新值，也没加 token**。静止态像素完全不变（tab 默认发布的就是静止值）。

实测：4 个选中的 segmented tab 全部从 `rgb(255,255,255)` → `oklab(0.943423 …)`。

### 2. 那个守卫的覆盖率，取决于 vite 的转换缓存热不热

编辑完文件跑第一遍：`hovered 131`。再跑：`156`。连跑三遍：`156 / 156 / 156`。
**编辑任何文件之后的第一遍都是 131** —— 视觉配置是 `reuseExistingServer: false`，
每次都起一个新的 dev server，冷缓存下第一条路线在 `data-visual-ready` 亮起时**挂载得更少**。

**16% 的覆盖率差，而且它照样通过**（下限是 100）。这正是我这几轮反复撞见的那种失败：
守卫没坏，只是少看了 25 个控件，然后打印同一句话。

治本：不等一个固定的时间，**等控件数量不再变**（两次读数相同才往下走）。
改完冷跑和热跑都是 **156**。

### 3. "指针没到达"从日志变成断言

`unreachable` 之前只是打印。现在它是断言 —— 因为有意思的数字不是"hover 到了多少个"，
而是**有没有漏掉**：一个指针到不了的控件就是一个没人点得到的控件，
而这个审计有三轮花在其实是"漏掉"的读数上。

**单独验证过它会失败**（不是被下限顶掉的）：只把 `span` 的瞄点打偏 →
`hovered 153, never arrived on 3` → 下限过了（153 > 100），**是这条断言把那 3 个抓出来的**。

### 验收

| | 结果 |
| --- | --- |
| 对指针无反应的选中态 | segmented ×4 + dock ×1 → **0** |
| 交互模型里的值 | **没有新增**（复用 `raised` 的那一档）|
| 守卫覆盖率 | 131 或 156，看缓存 → **恒定 156** |
| "指针没到达" | 日志 → **断言，且单独验证过** |
| 单测 | **121 全通过** |
| 视觉套件 | **677 全通过**（9.9m，零失败）|
| typecheck / lint / prettier / knip / 8 个守卫 | 全绿 |

### 仍然欠着

最后三类沉默项**它们的祖先也不回应**（量过）：工具汇总展开按钮、goal 按钮、3 个开关
（`span[role=checkbox]`）。这三类要拿 `DESKTOP_UI_POLISH.md` 那条规则一个个判 ——
它写的是 hover 只保留给 dense lists、sidebar rows、icon buttons 和"有助于扫读"的控件，
并且把"给每个按钮都加 hover 背景"列为反面清单。**所以判断结果可能是"就该沉默"**，
那也要写下来，而不是默认它是缺陷。

## Round 199 —— 把判断本身写进守卫，而不是留在日志里

前几轮一直在往日志里记"这些控件对指针没反应，要拿规则判"。这轮把判断做完，
并且**判断结果本身变成断言** —— 留在日志里的判断，下一个人不会读。

### 先把"回应"的定义修对

之前只读控件自己。但 segmented 选中项是通过**子元素** chip 回应的，
dock tab 是通过**父元素** wrapper 回应的 —— 只读控件自己，这两个都会被算成沉默。
定义改成：**控件自己 + 最多 12 个子孙 + 最多 6 层祖先**里，
任何一个可见属性变了，就算回应了。

按这个定义重量，沉默名单从 15 缩到 **8**，而且**工具汇总展开按钮掉出了名单** ——
它其实一直在通过自己的 chevron 回应。（我在 Round 197/198 报过它，那是窄定义下的误判。）

### 剩下 8 个的判定

| | 判定 | 理由 |
| --- | --- | --- |
| 4 个文本框（3 个 composer + 设置搜索） | **就该沉默** | 插入符就是"键盘在这里" |
| 3 个开关（`span[role=checkbox]`） | **就该沉默** | 点下去拨杆会滑，那就是它的反馈；而且 `DESKTOP_UI_POLISH.md` 把"给每个按钮都加 hover 背景"列为反面清单 |
| 1 个 goal 摘要按钮 | **缺陷** | 见下 |

### goal 那条 bar 上，四个控件里三个回应、最宽的那个不回应

```
"Pursuing goal · Get the desktop suite"   answers=false
"Clear goal"                              answers=true
"Pause goal"                              answers=true
"Edit goal"                               answers=true
```

**一个面里兄弟控件答案不一致，那条规则不背书任何读法。**

根因：它用 `variant="bare"`，而 `bare` 明确写着 `":hover": "transparent"` ——
"No box at all: the button IS its text"。这个选择对：这一行读起来是**内容**
（它自己的注释就说 "The objective is CONTENT"），给它一个 wash 等于给它一个不该有的盘子。
换 `ghost` 会带上 padding 和圆角，那是布局改动。

所以用 Round 193 给 `TextButton` 定下的那个机制：**下划线**
（`textDecorationColor` transparent → currentColor），而且只在 `:is(:enabled)` 时 ——
因为"不能编辑时它保留自己的墨色和光标"正是这一行被要求遵守的规则。静止态像素不变。

**顺带露出一个既有缺陷**：`Button.base` 的 `transitionProperty` 里**没有 `text-decoration-color`**，
所以 `variant="link"` 自己的下划线一直是硬切的（而 `TextButton` 的会渐变）。
那个列表是"一条声明就是整份清单，callsite 只能替换不能追加"，
所以补在 `base` 上 —— 一个所有者，`link` 和 goal 摘要一起修好。

### 新断言：沉默是允许的，但名单是封闭的

```
MAY_ANSWER_NOTHING = 'input, textarea, [role="switch"], [role="checkbox"]'
```

这**不是**"所有控件都必须回应" —— 那正好是 `DESKTOP_UI_POLISH.md` 的反面清单。
它是：**一个什么都不回应的控件，必须是那种"反馈在别处"的控件**。
其余每一个安静下来的，最后都被证明是缺陷：选中行、选中的 segmented tab、
active dock tab、以及这条 goal bar 上最宽的那个。

**两个方向都验证过**：

| 制造的破坏 | 报什么 |
| --- | --- |
| 撤掉 goal 摘要的下划线 | 精确点名 `<button> "Pursuing goalGet the desktop s"` |
| 从白名单里去掉 `[role=checkbox]` | 3 个开关被抓出来 —— 证明是**白名单**在允许它们，不是测量恰好看不见 |

### 验收

| | 结果 |
| --- | --- |
| 对指针无反应且无正当理由的控件 | 1 → **0** |
| "回应"的判定范围 | 只有控件自己 → **自己 + 子孙 + 祖先** |
| 这条策略住在哪 | 日志里的一段话 → **一条断言 + 一份写明理由的白名单** |
| `Button variant="link"` 的下划线 | 硬切 → **跟着 `--dur-*` 渐变** |
| 单测 | **542 全通过**（`src/ui` + chat 分片）|
| 视觉套件 | **677 全通过**（10.0m，零失败）|
| typecheck / lint / prettier / knip / 8 个守卫 | 全绿 |

### 这一段（Round 194–199）的收尾

从"量一下 hover"开始，六轮里改对的东西：

| | |
| --- | --- |
| 真缺陷 | 选中行被 hover 变淡、凹陷行丢掉凹陷、active dock tab 丢掉抬起、选中 segmented tab 毫无反应、goal 摘要毫无反应、`link` 下划线硬切 |
| **我自己的测量错误** | 索引不稳定（比了两个不同元素）、坐标陈旧、没验证指针到达、把 scroller 裁剪当成沉默、把揭示型控件当成沉默、覆盖率取决于编译缓存热不热、"回应"只看控件自己 |

**测量错误比真缺陷还多一个。** 每一轮的教训是同一句：
一个守卫说"没问题"之前，先证明它会说"有问题"。

## Round 200 —— 键盘走的路：一个没找到缺陷的轮次，和一条被我砍掉的断言

hover 那条线已经收敛（守卫封闭、名单为空），继续挖会变成"给不需要的控件加 hover"。
换一个面：**焦点环有三个审计（有没有地方画、画不画得出来、opt-out 有没有兑现），
但没有一个问路径**。环画在对的控件上、顺序是错的，键盘用户照样会丢掉自己的位置。

### 结果：这条线上没有缺陷

四条路线、45 次 Tab，**Tab 顺序完全跟着阅读顺序**；
`Shift+Tab` 也**逐个原路退回**，0 处分歧。

这是一个正当的结果，不该包装成发现。但**过程里的东西值得留下** ——
因为这个测量在说出这句话之前，错了四次。

### 四次测量错误

| 错在哪 | 症状 |
| --- | --- |
| **比 viewport 坐标** | 焦点会滚动它所在的容器，所以在一个滚动列表里往下走，下一站的 y 反而更小 —— 侧栏的行和两条设置行被报成"顺序反了"，其实什么都没反 |
| **不排除跨 band 的元素** | 一个满高的 pane resizer 报的是页面顶部，于是"从窗口里最后一个控件跳上去 676px" |
| **`BACKWARD_X = 40` 这个阈值** | 见下 —— **这条最严重** |
| **把"往下且往左"当成倒退** | 表单里每一行都比上一行更靠左，于是每一对连续设置行都被举报 |

前两条修法：位置读**最近的 scroller 的内容坐标**（焦点滚动不会移动它），
只在两站共享同一个 scroller 时比较，跨 band 的元素干脆不比。

### 第三条：我自己的魔法阈值让守卫瞎了

我一开始写的是"往上 24px / 往左 40px 才算倒退"。
拿真实破坏去验证 —— 把一行三个图标按钮在屏幕上反向、DOM 顺序不动 —— **守卫没反应**。
因为相邻两个 24px 控件之间的跳跃大约 32px，**正好躲在那个用来容忍布局噪声的阈值下面**。

治本：**不要阈值，用几何关系**。

| | 判据 |
| --- | --- |
| 往上倒退 | 下一站**整个**在上一站上方（盒子不重叠），**且**没有往右开新列 |
| 往左倒退 | 两站**竖直方向有重叠**（同一行），**且**下一站**整个**在上一站左边 |

"同一行"也是重叠关系，不是 12px。这样就没有需要调的数了。

改完再验证：把 composer footer 在屏幕上反向 → **报出来了**，
而且报的跳跃是 **6px** —— 原来那个 40px 的阈值永远抓不到它。

### 砍掉了 `Shift+Tab` 那条断言

它在四条路线上都成立，但我**三次尝试用产品代码把它破坏掉，全部失败**：
segmented 的成员走 roving tabindex，所以同一组的两个成员**永远不是相邻的 Tab 站**；
`tabIndex` 传进去被 Base UI 覆盖；focus 重定向也没生效。

**一个没人见过它失败的断言不是守卫。** 而且它要守的那个承诺，
设计系统本来就在守（`CLAUDE.md`：绝不手写 roving tabindex，一律 Base UI）。
所以它不进代码，只把测量结果写在这个文件的注释里。

### 验收

| | 结果 |
| --- | --- |
| 键盘路径的守卫 | 无 → **1 条（阅读顺序），已见过它失败** |
| 断言里的魔法数字 | 3 个（24 / 40 / 12）→ **0** |
| 找到的缺陷 | **0** —— 这条线是干净的 |
| 单测 | **50 全通过** |
| 视觉套件 | **678 全通过**（10.2m，零失败；+1 是新守卫）|
| typecheck / lint / prettier / knip / 8 个守卫 | 全绿 |
| 生产代码改动 | **零** |

### 仍然欠着

45 次 Tab 在 dock-light 这条路线上只走到 38 站就没了 ——
`Clear goal` / `Pause goal` / `Edit goal` **根本没被走到**。
所以这条守卫覆盖的是每条路线的**前 45 个 Tab 站**，不是全部。
要么加步数，要么承认它是抽样 —— 目前是后者，写在这里而不是假装它是全量。

## Round 201 —— 抽样的守卫会漏掉它没走到的地方

上一轮末尾写着"45 次 Tab 是抽样，不是全量"。这轮把它做成全量，然后连着掉出三个东西：
**一个真缺陷、一个我上一轮埋的测量错、以及一个我 Round 192 就埋下的假绿。**

### 1. 走到环闭合，而不是走 45 步

先量：各条路线的 Tab 循环分别在 24 / 50 / 22 / 56 / 40 次按键时闭合。
**45 对其中两条不够** —— dock 路线要 50，narrative 要 56。

所以不再用常数，而是**按到焦点离开文档、再从头走一圈为止**。
中间踩了两个坑：

| 坑 | 症状 |
| --- | --- |
| 用"标签派生的 key"判断环闭合 | dock 路线上两个控件的 key 撞了，walk **走了 1 站就结束** |
| `document.activeElement === document.body` 当成"在顶部" | **它不是。** Chromium 的 sequential focus starting point 是另一份状态：dock 路线从那里按 Tab 会先访问**最后两个**控件、离开文档、然后才从第一个重新进入 |

第二个坑造成了一次假报警：跨过"离开文档"那个边界比较，读出 **984px 向左跳**（窗口两端的两个 chrome 按钮）。
治法：rewind 阶段**先按再判**，按到离开文档为止；下一次按键才是真正的第一站。

还有一个：dedup 也不能用标签 key —— goal bar 那三个图标按钮**标签全是空的**，
key 完全相同，于是三个里两个被当重复丢掉。**而那正是"顺序反了"会体现出来的一组。**
改成按元素本身打标记。

**覆盖：99 站 → 138 站 → 182 站。**

### 2. 真缺陷：收起侧栏的那个按钮是整个 shell 最后一个 Tab 站

```
19  y=680  "Switch to dark"     ← 侧栏底部
20  y= 10  "Hide sidebar"       ← 标题栏，窗口最顶上
```

键盘用户要走完**每一个会话行**，才能拿到那个"收起侧栏"的按钮。
根因：`app-shell.tsx` 里它渲染在 `AgentSidebar` **之后** —— 挨着它所控制的东西，
而不是按它出现的位置。它是 `position: absolute` 且有显式 `z-index: var(--layer-chrome-control)`，
**所以布局和绘制顺序都不依赖兄弟顺序，只有 Tab 顺序依赖**。移到侧栏之前。

### 3. 我 Round 192 埋的假绿：那条"全产品只有一个环"的断言是 flaky 的

Tab 顺序一变，`focusRing` 的 paint 守卫红了，报"有两种环指纹"。查下去：

- 指纹一 `solid 1px oklab(…/0.5)` —— **就是设计的环**。声明是 `1.5px`，
  Chromium 在这个设备像素比下**报成 1px**。
- 指纹二 `solid 3px currentcolor` —— 一个瞬态，**大约三次里出现一次**。

也就是说这条断言**一直靠运气在过**（实测 pass / fail / pass），
我的改动只是改了概率。**比较计算后的字符串，比的是像素取整和瞬态，不是设计。**

治本：改成断言**规则身份** —— 一个画出环的控件，
匹配它的、设置了 `outline*` 的**作者规则只能是 globals.css 那一对**（两半都同时 gate 在
modality 属性和 `:focus-visible` 上，全表没有别的规则这样）。
这才是"一条规则画所有环"的字面意思，而且对像素取整和瞬态免疫。

改完立刻稳定地**红了 3/3**，抓到一个真东西：

`.agent-seam-rail { outline: none }` —— pane resizer 上一条**死声明**。
特异度 (0,1,0) 对全局规则的 (0,4,3)，**它从来没有压制过任何东西**；
而这个元素自己的注释写着 *"Arrow keys resize, so a keyboard user needs to see this one"*。
一条读起来像在做事、其实不做事、还和自己的意图矛盾的声明。
（同一块里的 `border: 0` / `padding: 0` / `background: transparent` 对 `div role="separator"`
都是初始值，一并删掉。）

**顺带暴露一个守卫盲区**：`check:chrome` 的 focus 规则**豁免 `styles/globals.css`** ——
这对全局规则本身是对的，但也意味着 globals.css 里针对**单个组件**的压制没人看。
这条运行时断言正好补上那个缺口。

### 三个方向都验证过

| 制造的破坏 | 报什么 |
| --- | --- |
| goal 那三个按钮在屏幕上反向 | 两条 **8px** 跳跃（前两轮两次都看不见：先是覆盖不到，后是标签 key 撞了）|
| 侧栏按钮挪回原处 | 那条 **644px** 向上跳 |
| `outline: none` 放回 seam rail | 精确点名 `.agent-seam-rail` |

### 验收

| | 结果 |
| --- | --- |
| 键盘路径覆盖 | 45 步抽样 → **走到环闭合**（182 站 / 5 条路线）|
| Tab 顺序缺陷 | 1 → **0** |
| 死的 `outline` 声明 | 1 → **0** |
| flaky 的断言 | 1（Round 192 起）→ **0**，换成规则身份 |
| 单测 | **121 全通过** |
| 视觉套件 | **678 全通过**（14.8m，零失败）|
| typecheck / lint / prettier / knip / 9 个守卫 | 全绿 |

### 一句话

上一轮我把"这是抽样"写进了日志。**写下来不等于安全** ——
把它变成全量之后，立刻掉出一个真缺陷和两个假绿。

## Round 202 —— 那个盲区里还坐着一条一模一样的死声明

上一轮末尾报了一个盲区：`check:chrome` 的 focus 规则**豁免整个 `styles/globals.css`**，
而全局规则只需要两条豁免。收窄它，第一下就掉出**同一条死声明的孪生兄弟**。

### 1. 两个 resizer，一样的死声明

```
.agent-seam-rail    { … outline: none }   ← Round 201 删掉的
.agent-pane-resizer { … outline: none }   ← 一直坐在那里
```

两个都是同一个 `ResizeHandle`（`SeparatorPrimitive` 渲染的 `div role="separator"`），
特异度 (0,1,0) 对全局规则的 (0,4,3) —— **从来没压制过任何东西**，
而这个组件自己的注释写着 *"Arrow keys resize, so a keyboard user needs to see this one"*。
（同块的 `border: 0` / `padding: 0` / `background: transparent` 对 `div` 都是初始值，一并删。）

**为什么 Round 201 只抓到一个？** 因为 paint 守卫当时还是 **45 步抽样** ——
右侧那个 resizer 是第 **47** 站。上一轮我刚在 `keyboardPath` 里把抽样改成走满一圈，
**却没有把同一个修正带给 `focusRing`**。同一个错误，同一天，两个文件。

### 2. 静态豁免从"整个文件"收窄到"那两条规则"

线性扫描器现在跟踪**当前选择器**（记住带 `{` 的那一行 —— prettier 折行后最具体的部分还在那里），
focus 规则的豁免条件变成：*选择器里含 `:focus-visible` 或 `[data-pointer]`*。

收窄之后它立刻又报了一条 —— **我自己的注释**。`/* */` 块的**续行**以正文开头，
不以注释标记开头，所以我在 Round 193 加的"跳过注释行"覆盖不到它。
于是那条解释这条规则为何存在的段落，被这条规则举报了。补上块注释状态跟踪。

### 3. 走满一圈这件事，现在只有一个所有者

`keyboardPath` 和 `focusRing` 都需要同一套 walk 协议，而它的三个微妙点**每一个都以"覆盖更少但照样报成功"的方式失败**：

| 微妙点 | 错了会怎样 |
| --- | --- |
| `activeElement === body` **不等于**"在顺序顶部"（Chromium 的 sequential-focus starting point 是另一份状态）| 跨过"离开文档"的边界比较，读出假的 984px 向左跳 |
| 身份必须是**元素本身** | 标签派生的 key 撞了：一次 walk 走 1 站就结束；goal bar 三个空标签图标按钮塌成 1 站 |
| 走到**环闭合**，不是固定步数 | 45 步对两条路线不够，第二个 resizer 就在第 47 站 |

抽成 `visual/tabWalk.ts` 的 `eachTabStop(page, visit)`，两个 spec 各自只提供"读什么"。
重构后 `keyboardPath` 仍是 **182 站**，行为不变。

### 4. 顺带修掉一个我自己的超时预算

全套跑完 `hoverAnswers` **红了 —— 141s 撞它自己 140s 的天花板**。
单独跑：**冷跑 141s，热跑 43s**。视觉配置每次起新 dev server，所以第一遍要付转换成本。
**Round 198 我把这个冷缓存从"覆盖率"里拿掉了，却留在了时钟上。**
三个 spec 现在统一按**每条路线**给预算，并且按**冷跑**的数字定，不是热跑的 ——
天花板是给"出事了"用的，不是给达标用的。

### 三个方向都验证过

| 制造的破坏 | 报什么 |
| --- | --- |
| `outline: none` 放回 `.agent-pane-resizer` | 静态：`globals.css:1126`；运行时：`<div> "Resize right workspace" .agent-pane-resizer` —— **45 步的时候它一声不出** |
| 收窄后的豁免 | 全局那两条规则仍然放行（clean tree 全绿）|
| 块注释跟踪 | 关掉就会举报自己的文档 |

### 验收

| | 结果 |
| --- | --- |
| 死的 `outline` 声明 | 1 → **0**（两个 resizer 都干净）|
| `check:chrome` 的豁免范围 | 整个 globals.css → **两条 focus 规则** |
| paint 守卫覆盖 | 45 步抽样 → **走满一圈** |
| walk 协议的所有者 | 2 份（其中一份是旧的）→ **1 份** |
| 超时预算 | 冷跑会撞天花板 → **按冷跑定，×2 余量** |
| 单测 | **121 全通过** |
| 视觉套件 | **678 全通过**（10.7m，零失败）|
| typecheck / lint / prettier / knip / 9 个守卫 | 全绿 |

### 一句话

上一轮我在一个文件里把"抽样"改成"全量"，**没把同一个修正带到旁边那个文件** ——
于是同一个缺陷在同一天躲过了同一个守卫的孪生兄弟。
修正一个测量错误时，要问的是"还有谁在犯同一个错"。

## Round 203 —— 第三份手写的 Tab 走法，而且它是二次的

上一轮抽出了 `tabWalk.ts`，两个环审计接上了。`chromeFocus` 是**第三份**，
而且它比前两份错得更多：

| | 它原来怎么做 |
| --- | --- |
| 步数 | 40 步/路线 —— 而它的路线里 dock-light 的循环要 **50** 才闭合 |
| 身份 | `class + text` 派生的 key —— 图标控件标签为空会塌成一个 |
| 起点 | 假设"从 body 按一下 Tab 落在顺序顶部" —— **不成立**（Round 201 的结论）|
| 拍完之后 | blur 掉，然后**按 step+1 次 Tab 数回去** —— **二次复杂度**，而且数回去的前提正是上面那条不成立的假设 |

治法：接上 `eachTabStop`，拍完之后把焦点**还给那个元素本身**
（先给它打个标记，再 `focus()`），走法变成线性。
这和文件头"用真 Tab、不用 `element.focus()`"不矛盾 ——
**视觉状态已经拍完了，还回来的只是位置**。这一点写进了注释。

顺带把两段过期的文件头注释改对（"四条路线、每条四十次 Tab……"描述的是已经不存在的机制），
并给它加了 opt-out 数量的下限：**一次没遇到任何 opt-out 的 walk，不守任何承诺**。

### 验证：这条断言在重构之后还能开火

它不是新断言 —— 文件头记着它历史上抓到过三个真缺陷（question card、header diff stat、
active dock tab 的假 stand-in）。所以要验的不是"它有没有用"，而是**我的重构有没有把它弄瞎**。

试了三次才验出来，而这三次本身就是信息：

| 拿掉什么 | 结果 |
| --- | --- |
| `Button` 的 `":is([data-chrome-focus]):focus-visible": surface.hover` | **照样通过** —— 走到的 12 个 opt-out 都不是靠它 |
| `navigation-row` 的 `":focus-visible": surface.hover` | **照样通过** —— 这些行还会通过 `reveal.host` 的 `:focus-within` **显出自己的操作按钮** |
| 上面那条 **加上** `reveal.ts` 的三个 `:focus-within` | **报出来了**，点名两个控件 |

也就是说这 12 个 opt-out 的"另一种表达"**不止一层**，
而这条断言问的就是"到底有没有任何一层" —— 它没瞎。

（走到的 12 个全是侧栏的会话/项目行加 `Switch to dark`、`Back to app`。
`question` 和 `dock-light` 两条路线一个都没有：question card 是 `tabIndex={-1}`，
dock tab 用的是 `data-focus-inset` 而不是 `data-chrome-focus`。）

### 验收

| | 结果 |
| --- | --- |
| 手写的 Tab 走法 | 3 份 → **1 份**（`tabWalk.ts`）|
| `chromeFocus` 覆盖 | 40 步抽样 → **走满一圈**（12 个 opt-out，下限 8）|
| 走法复杂度 | **二次 → 线性** |
| 耗时 | 24.5s（覆盖更多，且不再需要"只在空闲机器上才过"的预算）|
| 视觉套件 | **678 全通过**（10.4m，零失败）|
| typecheck / lint / prettier / knip / 9 个守卫 | 全绿 |
| 生产代码改动 | **零** |

### 一句话

上一轮的教训是"还有谁在犯同一个错"。答案是**第三个文件**，
而且它把同一个错误犯得更彻底 —— 它连"数回去"的正确性都建立在那个不成立的假设上。

## Round 204 —— 关掉最后一个面板，键盘用户当场失去位置

键盘这条线上还剩一个没人问的问题：焦点**离开**之后去哪。
三个环审计都只看"焦点在某处时长什么样"，没有一个问"拿着焦点的那个东西被删掉之后呢"。

按键盘量了三种破坏性动作：

| 动作 | 焦点从 → 到 | 判定 |
| --- | --- | --- |
| dock tab 上按 Delete | `Plan1/3` → `Timeline` | ✓ ARIA 的答案 |
| 侧栏切换按 Enter | `Hide sidebar` → `Expand sidebar` | ✓ 同一个按钮换了标签 |
| **关掉最后一个 dock tab** | `Explorer` → **`<body>`** | **缺陷** |

### 根因：`?.focus()` 打在空值上，是静默的

```ts
rootRef.current
  ?.querySelector<HTMLElement>('[role="tab"][data-active]')
  ?.focus({ preventScroll: true });
```

"移到新的活动 tab"在还有 tab 时是对的。**关掉最后一个面板之后没有任何 tab** ——
这个链式调用什么都不做、也什么都不报，于是焦点掉回 `<body>`。
键盘用户不会收到任何提示：**他下一次按 Tab 会从整个文档的顶部重新开始**，
这是"让人失去位置"最彻底的一种方式。

### 治本：焦点去哪必须在拿着它的东西被删掉**之前**决定

量了一圈空 dock 之后还剩什么：`agent-dock-tabs` 的**父元素**是 `agent-surface-header`，
里面 tablist 之外唯一的可聚焦元素是 **`Browse panels`** —— 而且它**活过了空 dock**。

它同时满足两件事：**在 Tab 顺序里紧邻**，而且**正是关掉最后一个面板之后一个人想要的东西**。
（`.agent-dock-row` 不行 —— 量出来它包着整个聊天面板，"第一个可聚焦元素"会落到 transcript 里。）

所以 fallback 在 close 之前从活的 DOM 里取好，rAF 之后按
「活动 tab → 任意剩余 tab → 那个重开控件」的顺序落焦。改完：`Explorer` → **`Browse panels`**。

### 新守卫

放在 `keyboardPath.visual.spec.ts`（它拥有"键盘去哪"这个问题）。
断言在**每一次关闭**上，而不是只看最后一次 —— 因为"哪一次关闭把 dock 清空"
不该是这个测试关心的事，fixture 的 tab 数会变。三条下限一起挡住"什么都没做也能过"：
关闭次数 > 3、dock 最终确实空了、以及焦点一次都没掉出文档。

**验证过会失败**：把 restore 改回"只找活动 tab" → 报
`closing "Explorer" left focus on <body>`。

### 验收

| | 结果 |
| --- | --- |
| 焦点掉出文档的动作 | 1 → **0** |
| 覆盖"焦点离开之后"的守卫 | 无 → **1 条，已见过它失败** |
| 单测 | **350 全通过**（`src/ui` + workspace 分片）|
| 视觉套件 | **679 全通过**（10.5m，零失败；+1 是新守卫）|
| typecheck / lint / prettier / knip / 10 个守卫 | 全绿 |

### 一句话

`?.focus()` 打在空值上不报错，正是这个缺陷能一直活着的原因 ——
可选链把"没有目标"和"已经处理好了"写成了同一个样子。

## Round 205 —— 同一个静默模式：`?.` 把"没找到"和"做完了"写成一个样子

上一轮的根因不是 dock 特有的，是 `?.focus()` 这个形状：
**可选链让"没有目标"和"已经处理好了"在代码里长得一模一样。**
所以把这个形状在仓库里搜了一遍 —— DOM 查询的结果被可选链消费掉的地方，四处。

| 站点 | 判定 |
| --- | --- |
| `context-dock.tsx:183` `querySelectorAll(...).forEach` | ✓ 空集合就是空操作，语义正确 |
| `search-overlay.tsx:124` `?.scrollIntoView` | ✓ 没有 `[aria-selected]` 就是没有东西要滚 |
| `catalog-picker.tsx:411` `?.scrollIntoView` | ✓ 已经包在 `requestAnimationFrame` 里 —— 下一条缺的那一帧，它算进去了 |
| **`navigationStatePort.ts:158`** | **缺陷** |

### 那个说谎的布尔值

```ts
function focusConversationTool(itemId: string): boolean {
  const anchor = document.getElementById(itemId);
  if (!anchor) return false;
  anchor.scrollIntoView?.({ block: "center" });
  anchor.querySelector<HTMLElement>("button")?.focus({ preventScroll: true });
  return true;                      // ← 只要 anchor 在，就报成功
}
```

调用方是这样用它的：

```ts
if (!focusConversationTool(id) && typeof requestAnimationFrame === "function") {
  requestAnimationFrame(() => focusConversationTool(id));    // 重试
}
```

**这个重试的存在，正是因为一个工具的 anchor 和它的按钮不必在同一帧提交。**
而这个函数只要 anchor 在就返回 `true` —— 于是"anchor 已提交、按钮还没有"这一帧
**报告成功，把专为这一帧写的重试取消掉了**。用户看到的是：滚到了那个工具，键盘留在原地。

治本：让这个布尔值说它的调用方在问的那件事 —— **焦点到底落下了没有**。

### 那个 `?.scrollIntoView` 不是噪音，别删

它看起来像多余的防御（`getElementById` 已经收窄成 `HTMLElement`）。
但 jsdom **没有** `scrollIntoView` —— `context-dock.test.tsx` 专门把它从原型上删掉再跑。
所以它是真防御，加了注释说明，没删。**"看起来没用"和"没用"是两件事**，这一条差点被我按上一轮的手法清掉。

### 新测试补的是重试那条分支

已有的测试建的 anchor **自带按钮**，只走了 happy path，重试分支一直没人测 ——
而缺陷正好在那条分支上。新测试建一个**没有按钮**的 anchor，
调用之后确认焦点还在 `<body>`，然后把按钮挂上、放过一帧，确认焦点落到了按钮上。

**验证过会失败**：把契约改回旧的 →
`AssertionError: expected <body> to be <button>`。

### 验收

| | 结果 |
| --- | --- |
| 说谎的布尔契约 | 1 → **0** |
| 覆盖重试分支的测试 | 无 → **1 条，已见过它失败** |
| 误删的防御 | **0**（`?.scrollIntoView` 查清了才留下）|
| 单测 | **351 全通过**（workspace + `src/ui` 分片，+1）|
| 视觉套件 | **679 全通过**（10.8m，零失败）|
| typecheck / lint / prettier / knip / 11 个守卫 | 全绿 |

### 一句话

上一轮修的是一个缺陷，这一轮搜的是**那个形状**。
四处里三处是对的 —— 而"搜一遍"的价值恰恰在这里：
它同时告诉你哪一处该改，和哪三处不该动。

## Round 206 —— 只有交互之后才存在的界面，第三次

先说两条**没有找到缺陷**的线，因为"搜过了、是干净的"和"找到了"一样是结论：

- **被吞掉的 `catch`**：全仓零个空 `catch {}`。逐个读了七处 —— i18n 的两处
  `localStorage` 可能被禁用、`asyncOwnership` 和 `queryClient` 各写明了理由、
  RPC 的两处一处**记日志**一处**区分 abort 和真失败**、`runDigest` 是 JSON 兜底。**全部正当。**
- **没有可访问名字的控件**：hover 扫描里看到三个 `span[checkbox] ""`。查了 axe 的标签集是
  `wcag2a/2aa/21a/21aa/22aa` 全量 —— 名字缺失早就会红。它们用的是 `aria-labelledby`，
  是**我的探针没解析**。清白。

### 真正的缺口：只有交互之后才存在的界面

`closure.visual.spec.ts` 自己记着它学过两次这件事 ——
两个搜索浮层「neither was in this audit until they had a fixture to open them from」，
以及 schedules 表单「reaching it takes a click」。**每次里面都有真违规在等着。**

所以把工作区里一个人能**打开**的东西都打开，跑 axe：右键菜单、composer 三个弹层、
dock 目录、goal 编辑器。第三次也有：

```
target-size: #base-ui-_r_u_  button 22x22  label="Clear goal"
  Safe clickable space has a diameter of 15.6px instead of at least 24px.
target-size: #base-ui-_r_10_ button 22x22  label="Pause goal"
  ... 21.6px instead of at least 24px.
```

### 根因不是那个 22px 的档位

一开始我以为是 `--control-height-xs: 22px`（21 处 IconButton 在用）低于 24px 的底线。
**不是。** WCAG 2.5.8 明确允许小于 24px 的目标**靠间距达标**，axe 实现的就是这一条 ——
所以 22px 这一档本身是合规的，全产品其他地方都靠间距过关。

真正出问题的是这一行：**三个 22px 按钮以 8px 间距挤在一个 `flex: fill` 的宽目标旁边**，
于是这两个的安全点击空间掉到 15.6px 和 21.6px。
（用户选了"只修 goal bar 的间距"，档位不动。）

顺带解释了它为什么一直没被抓到：
`touchTargets.visual.spec.ts` **跑的是粗指针**（`hasTouch`），那里 `@media (pointer: coarse)`
给每个控件 44px 的地板 —— 它只查触摸下的**重叠**，从不查鼠标下的**尺寸**。
而路线审计能看到这一行，只是那个视口下的间距恰好算过了线。**它躲在一个布局巧合后面。**

### 改动与验证

`gs.actions`：`gap: s2 → s3`，并在摘要与操作之间加 `marginInlineStart: s2`。
改完在被审计的视口（1472×900）上 **violations=0**。

**六个交互界面全部纳入审计**，并验证过它们会红：把间距改回 8px → 精确点名 `Clear goal`。

写这六条时踩了一个 Playwright 的坑：一个弹层是 **dialog 里套 listbox**，
`[role=menu], [role=listbox], [role=dialog]` 的并集匹配到**两个**元素，
strict locator 直接拒绝 —— 两条测试报的是定位器错误，不是 axe 违规。加 `.first()`。

### golden

8 张移动：agent 的 `running`/`terminal`/`canceled` 与 workspace 的 `dock-light`，各明暗两版
—— 正好是会显示 goal bar 的那些状态。**看了 diff 图再更新**：226 像素（全图 0.01%），
红色只落在那三个图标和摘要的截断点上，别处一动没动。

### 验收

| | 结果 |
| --- | --- |
| WCAG target-size 违规 | 2 → **0** |
| 纳入审计的交互界面 | 2（搜索浮层、schedules 表单）→ **8** |
| 搜过但干净的线 | `catch` 吞异常、控件缺名 —— **各 0 个缺陷，已记录** |
| 单测 | **542 全通过**（chat + `src/ui` 分片）|
| 视觉套件 | **685 全通过**（11.4m，零失败；+6 是新审计）|
| typecheck / lint / prettier / knip / 9 个守卫 | 全绿 |

### 一句话

`touchTargets` 跑粗指针、路线审计跑静止态 —— 两个守卫各自都对，
**而缺陷正好落在它们之间那条缝里**：鼠标下的尺寸，只在打开某个东西之后才暴露。

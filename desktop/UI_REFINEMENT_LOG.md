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

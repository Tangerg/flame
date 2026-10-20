# Plugin capabilities roadmap

Forward-looking TODO for extending Flame's plugin line beyond Memory and Schedules.
Knowledge base, GitHub, Model router, Guardrails, Web research and Saved usage
reports are now implemented as ordinary first-party backends. The architecture refactor is covered
by `plugin-refactor-progress.md` / `plugin-refactor-handoff.md`; this file is about
*which capabilities to build next as plugins, and in what order.*

## Current state (baseline)

- Plugin architecture is proven. Two deep contract-vs-implementation audits (core
  `runtime/plugin` SDK; `pluginservices` + `mcpconnection`) found no correctness,
  ownership, lifetime, concurrency, or failure-semantics bugs.
- Host-callback parity is closed: a backend reached through the model MCP Tool
  entry now receives the same call-scoped canonical host grant as its public
  `plugins.call` entry, through the single owner `Pool.OpenRPC` (commit
  `a15f2f9e4`). Memory keeps its own narrow model/embedding grant by design.
- Eight first-party backends: `runtime/internal/plugins/memory`,
  `runtime/internal/plugins/schedules`, `runtime/internal/plugins/knowledgebase`,
  `runtime/internal/plugins/github`, `runtime/internal/plugins/modelrouter`,
  `runtime/internal/plugins/guardrails`, `runtime/internal/plugins/web`,
  `runtime/internal/plugins/usagereports`.
- Examples: `examples/plugins/reviews` (backend + typed ops + MCP tools + a
  declared analyzer dependency + board UI; one analyzer business is exposed as
  *both* a typed query and the `analyze_review` Tool), `examples/plugins/composition`
  (pure UI point composition).

## What makes a capability a good plugin (the lens)

A capability belongs in a plugin when, and only when:
1. It owns its own durable data (a private store), not core Session/Run/Goal state.
2. It participates in a Run through a *published contract* (tool / context /
   selection / input / observation / points-UI / accept-Run / embedding) or
   provides standalone typed operations.
3. When it needs host capability it uses a callback (model, embedding, call a
   declared dependency, accept a durable Run) — it never rebuilds a core state
   machine.
4. It is NOT a core authority (see boundary below).

## Must stay in core (do NOT plugin-ize)

Session, Run, Segment, Item, Goal, Plan, Interrupt, execution, persistence,
recovery, provider/model **construction**, compaction, the delivery endpoint,
idempotency, command routing, and the **accounting totals**. Note the distinction:
model *selection* is a published contract a plugin may contribute; model
*construction and cost accounting* are core.

## Available contracts / mechanisms (what a plugin can already use)

- Model-facing tools: `RegisterTools`; inside a tool, `CallbackFromContext` →
  `ConnectCallback` → canonical RPC (`CallPlugin` to a declared dependency, core
  ops). Tool arg transformation: `ToolProposalContract`.
- Context contribution: `ModelContextContract` (`flame.model-context.v2`).
- Model selection: `ModelSelectionContract` (`flame.model-selection.v2`).
- Request transformation: `ModelInputContract`. Transparent observation:
  `ModelObserverContract`.
- Embeddings: `flame.model/embed-texts.v2` (`NewTextEmbeddingCallback`,
  `CallTextEmbedding`). Independent model exchange: `flame.model/selected-call.v1`
  / `selected-stream.v1` (`CallSelectedModel`, `StreamSelectedModel`).
- Accept a durable Run from a backend: the schedules pattern (canonical RPC).
- Composition / UI: `context.compose` with collection / selection / decoration;
  consumer-defined points; `placement: export` components; `workspaceView`.
- Private store: `plugin/sqlite` (original-input admission, nullable settlement,
  atomic mutation/receipt).

## Candidate capabilities (prioritized)

### Tier 1 — UI-light, high reuse, start here
- [x] **Knowledge base / RAG over user docs.** Nearly the same shape as Memory:
      embedding callback + private vector store + `ModelContextContract` to inject
      retrieved chunks. Best "third plugin" to prove the pattern generalizes.
- [x] **First external connector (pick one SaaS, e.g. Lark or GitHub).** MCP tools
      + a private credential store (+ optional context contribution). This is the
      largest product-value category; it also validates "same business usable from
      UI command *and* model Tool" (the parity just closed in `a15f2f9e4`).
- [x] **Model router.** A `ModelSelectionContract` contributor that picks
      provider/model per role/budget (cheap model for simple tasks).
- [x] **Guardrails / safety.** `ModelInputContract` (e.g. PII redaction) or
      `ModelObserverContract` (output classification). Transparent, no new core.

### Tier 2 — more UI, or needs a new mechanism
- [x] **Web search / fetch / browse** (+ cache store) — tool plugin.
- [ ] **Code execution / sandbox** — tool plugin owning its execution environment.
- [x] **Usage / cost reports, daily / weekly summaries.** A projection plugin: it
      *reads* core accounting facts and owns its own report store + UI. Accounting
      totals stay core. Good vehicle to also measure composition/callback latency.
- [ ] **Event-driven watchers / triggers (webhook, file-watch → start a Run).**
      Same "accept a durable Run" shape as schedules, but event- not time-driven.
      **Gap:** there is no external-event *ingress* contract today — how an external
      event reaches the plugin backend process is unbuilt. This one needs a new,
      root-cause ingress mechanism first; do not bolt it onto tool/callback.

## Recommended sequencing

1. Start with Tier 1, UI-light first (RAG backend, a connector's tools, model
   router, guardrails). These barely touch the frontend, so they do not thrash
   against a concurrent UI refactor.
2. Use one of them to establish a performance baseline for the composition /
   callback hot path (point resolution + ESM load + backend callbacks +
   decoration). Measurements and reproducible workloads are recorded below.
3. Do plugin UIs *after* the shared design-system component APIs have settled.
4. Only build the event-ingress contract when a real webhook/event need appears.

### Reference-guided implementation direction

The user directed the remaining design to the local `study/codex-server` reference
(snapshot `7c073196a1`, location in `DEVELOPMENT.md`). Its implementation separates
execution target, permission policy and platform enforcement. Docker and Seatbelt
are therefore not competing choices at one boundary. Neither remaining candidate
is complete; their boxes stay unchecked until implementation and verification.

- Code execution: reuse Flame's existing native confinement mechanism for the
  actual execution platform. The current macOS `sandbox.Workspace` owns a temporary
  copy, scrubs the child environment, confines writes, hides the configured home,
  denies network access and bounds output. Cancellation/timeouts are distinct from
  CPU, memory and process-count quotas, which this mechanism does not supply.
  Unsupported confinement must fail closed. The plugin owns its private code
  environment and command outcomes; core Run execution remains core. A container
  execution target is a separate future requirement, not a prerequisite invented
  for this candidate. Reference: `sandboxing/src/manager.rs` platform selection and
  `exec-server/src/process_sandbox.rs` enforcement refusal.
- Event ingress: use workspace file changes as the first concrete source.
  Codex's `fs/watch` → `fs/changed` → `fs/unwatch` protocol supplies a useful
  connection-scoped subscription/lifetime precedent. Its `fs_watch.rs` only emits
  notifications; starting a Turn is a separate canonical operation. Flame needs
  its own published watch subscription mechanism, with host-owned path admission,
  source lifetime and bounded delivery. The plugin owns trigger rules, accepted
  event facts and captured dispatch intent; core owns canonical Run acceptance,
  lifecycle and accounting. Coalesced OS changes are observations, not a durable
  event log; accepted plugin dispatch intent must survive interruption/restart.
  Watch initialization failures must be explicit rather than adopting Codex's
  noop fallback. No external webhook listener or Tool/callback ingress shortcut
  is required for this source.

## Workflow: dedicated worktree, sync from main anytime

The plugin line runs in its own worktree while `main` carries the concurrent UI
refactor.

- Worktree: `git worktree add ../flame-plugins -b plugins`. It does Go backends
  (`runtime/...`) + **new** plugin UI files
  (`desktop/frontend/src/plugins/builtin/<name>/...`) + examples. It **consumes**
  shared design-system components; it does **not** edit them. This file separation
  keeps syncs conflict-free.
- Sync from `main` anytime, and often — at green commits (frontend check passing):
  `git fetch && git rebase main` (or merge if the branch is shared). Run the
  frontend check + plugin tests after each sync. Because the branch stays current,
  the eventual back-merge to `main` is effectively a fast-forward.
- Stop signal for UI work: if a sync breaks plugin UI compilation (component props
  changed), the UI refactor is churning APIs, not just tokens — pause plugin UI
  work and advance only backend plugins until the API seam settles.

### Execution loop

Work one candidate per round, in priority order (Tier 1 before Tier 2):

1. Pick the next unchecked candidate.
2. Read its target contract, the closest existing plugin/example, and tests.
3. State the design (owner, contract, store, host callbacks), then implement it
   root-cause — no compatibility patches, breaking allowed.
4. Verify (see below) and commit on the `plugins` branch via an explicit path.
5. Check the box here and record anything learned.
6. Sync from `main`, then go to 1.

Continue round by round until every candidate is done, or the user stops. For any
candidate that needs a new core mechanism (e.g. the event-ingress gap), design and
build that mechanism root-cause rather than hacking it onto an existing contract;
if its scope is genuinely ambiguous, surface it instead of guessing.

## Working constraints (per AGENTS.md / PROJECT_RULES)

- Root-cause only; no compatibility patches; breaking changes allowed to fix a
  faulty contract or remove a wrong design — migrate every in-scope consumer in
  the same batch.
- One fact, one owner. Name the owner before changing any representation of a fact.
- Prefer behavior-rich owners over procedural orchestration around data structs;
  config/wire/projection structs stay data unless they own a real invariant.
- Comments: default to none; add only *why* at critical data structures,
  interfaces, non-obvious algorithms, or pitfalls. Do not narrate what the code
  does. The existing plugin-line comments are already disciplined — do not churn
  them, and do not manufacture changes to look productive.
- Reply to the user in Chinese; keep code, identifiers, errors, and repo docs in
  English.

## Verification

- Build: `go build ./runtime/...`
- Adapter suites: `go test ./runtime/internal/adapter/integration/{mcpconnection,pluginservices}/... -count=1`
- Plugin SDK: `go test ./runtime/plugin/... -count=1`
- Reviews example module: `(cd examples/plugins/reviews/backend && go test ./... -count=1)`
- Callback-heavy integration (slow, builds real backends):
  `go test ./runtime/internal/bootstrap/ -run 'Memory|Schedule|Review|TestInstalledToolEntryReaches|Callback' -count=1`
- The runtime LSP/gopls index has shown stale errors this cycle; trust `go build`
  / `go test`, not the editor diagnostics.

## Recent plugin-line commits (context)

- `a15f2f9e4` fix: route installed model Tool calls through the one host-grant owner.
- `342293b3a` refactor: tighten the installed-tool preparation path (lower complexity).
- `fe169cd65` refactor: share one callback HTTP client instead of per-call transports.

## Completed rounds and contract lessons

Commit IDs below identify authored revisions and may be rewritten when this
worktree rebases onto `main`; subjects and changed owners identify each round.

- `c5a031a74` upgrades all 30 Scope modules to v0.53.0 in Runtime, CLI and
  Reviews. Build/vet and the changed dependency consumers passed. The broad CLI
  run had one executable-command timeout; its isolated regression passed.
- `d3c0407ae` repairs two pre-existing acceptance fixtures that assembled an
  Endpoint without production's required callback publication. The failures
  reproduced with the original Scope dependencies; the unchanged business
  assertions pass with the complete fixture composition.
- `b9229530d` implements Knowledge base. The private owner atomically publishes
  document revisions, Unicode-safe chunks, vectors and command receipts. Only
  a newly admitted import may embed; missing settlement cannot authorize replay.
  Deleted document IDs are retired so a stale revision cannot match a reimport.
  Typed search, MCP search and model context share the same retrieval use case.
- `df62f776a` implements GitHub's read-only issue/list/file operations and matching
  MCP tools. The existing private installation secret-input store owns the current
  token; the backend receives one immutable launch value rather than creating a
  competing credential database. Its private database owns credential-qualified
  conditional responses. Every read revalidates upstream authority; rejection
  never returns cached private content. Public access is explicit absence, while
  a configured invalid token fails without a public fallback.
- GitHub's Link header may use its canonical numeric repository route even when
  the caller used `owner/name`. Only its next page number is projected. Subsequent
  requests remain bound to the authored repository/filter; no Link, download URL
  or redirect gains HTTP routing authority. The fixed-origin client bounds
  responses/files and preserves explicit authorization/rate/resource refusals.
  Source contracts and API version are linked in `plugins/github/README.md`.
- GitHub verification passed: Runtime build, backend/resource/credential/cache
  tests, typed/Tool success and refusal parity, actual installed secret lifecycle
  and model refusal parity, first-party initialization, vet and architecture gates.
  HTTP tests are deterministic and credential-independent; installed acceptance
  deliberately refuses an invalid resource before HTTP. Live GitHub requests and
  real-account permissions have not been qualified. The catalog initialization
  regression now checks all supplied packages rather than freezing their count.
- Embedding v2 returns the actual vector space with its vectors. Dimensions or a
  separately read embedding role cannot prove vector compatibility. Memory and
  all in-repository callback consumers migrated together; external backends must
  rebuild for `TextEmbeddings` and the v2 method. Private Memory storage is unchanged.
- Context v2 projects `userMessage` from the original request before adding host
  trailers. The complete provider-visible request may end with a user-role
  deferred-tool catalog; a retrieval plugin must not parse private markers or
  mistake that trailer for the user's question. Rebuild context contributors and
  explicitly select the v2 composition; finish old waiting work with its old build.
- Verification passed: Runtime build, SDK and adapter suites, Knowledge base
  domain/receipt/entry tests, installed stdio retrieval with a local embedding
  provider, context consumer regressions, affected callback-heavy Memory/Tool
  integration, Reviews, vet and architecture/public-author compilation checks.
  The installed regression compares typed and model Tool retrieval, requires the
  original user query to reach embedding, and checks that injected material does
  not become authored Session history. No live SaaS/provider credentials were used.
- `3c1e245cf` implements Model router. A private immutable policy owns one rule
  per actual calling use, optional captured-envelope ceilings, revision transitions
  and atomic command receipts. Runtime's selection query now publishes interaction,
  utility or independent use and the caller's existing limits. Utility text bytes
  and independent encoded-request bytes remain distinct units; interaction invents
  no byte envelope. Model construction, capacity, caller-budget enforcement and
  pricing remain core. Missing policy matches preserve the original proposal;
  publisher or selected-provider failures never authorize fallback.
- Selection v2 replaces v1. Rebuild contributors and explicitly choose the v2
  role; no compatibility alias or old-query interpretation remains. Accepted Runs
  retain their exact selection and Session preferences remain separately owned.
  Policy changes apply to future queries rather than accepted command retries.
- Model-router verification passed: Runtime build/vet, model/SDK/private-policy
  tests, changed Run-opening and auxiliary/independent model consumers,
  pluginservices, architecture gates, durable command/reopen/refusal checks and
  an installed stdio acceptance covering provider construction, Run identity,
  Session preference and accepted retry after policy replacement. A shared Go
  cache lost compiled dependency files during one rerun; the final installed
  acceptance passed with a task-local GOCACHE, without clearing shared caches.
- `22fe473ab` implements Guardrails through the existing input decoration contract.
  A private policy admits at most 32 exact protected phrases and atomically owns
  policy revision/command receipts. Decoded Scope JSON string tokens include
  composed messages, Tool schemas, options and metadata; a match refuses before
  physical dispatch. Allowed wire requests remain unchanged. Inspection returns
  revision/count, refusals expose no protected value, and the backend retains no
  inspected requests. Exact phrases are the declared scope; arbitrary PII,
  binary media, application encodings and numeric classification are not inferred.
  Provider dispatch, authored conversation, final budgets and accounting stay core.
- Guardrails verification passed: Runtime build/vet, private policy and JSON
  boundary tests, typed mutation/query/input entries with restart and command
  replay, architecture gates and installed input composition. The installed
  regression proves allowed work completes, protected work fails before provider
  dispatch, original user history remains intact, and failure output discloses
  no protected phrase. No new core mechanism or host callback was needed.
- `31a228eb9` tightens model-router restoration: a missing rule list is corrupt,
  rather than an empty pass-through policy. Guardrails applies the same ownership
  distinction to its phrase list. Explicit empty arrays remain valid owner facts.
  Model-router backend/domain regressions passed alongside the changed guardrail
  packages; no unrelated broad suite was repeated.
- `eeab9933d` moves existing Jina/Tavily construction and credential configuration
  out of core into Web research. Typed operations and MCP search/fetch/browse share
  one owner. Search and fetch always query their providers; browse reads an exact
  immutable private capture by content-qualified ID and Unicode offset. Changed
  source content receives a different identity; eviction or corruption never
  authorizes a silent refetch or substitution. Logical archive retention is
  bounded to 32 distinct captures / 64 MiB, with two-MiB source documents.
- Web migration removes Runtime YAML web keys and the former FLAME credential
  overlays; configure the package's optional secret inputs instead. Provider
  redirects retain the prior same-origin restriction and response framing limit.
  Historical built-in references and normalized result schemas remain readable
  because persisted facts retain their actual original issuer. Their constructors,
  current dispatch metadata and normalization producer are removed, with no
  executable alias or retargeting. The method-selectable HTTP capability retains
  its distinct host-allowlist policy.
- Web verification passed: Runtime build/vet, archive identity/Unicode/reopen/
  eviction/corruption tests, typed/Tool success parity, definite failed read,
  provider response/redirect/credential boundaries, config migration, current
  constructor/metadata coverage and architecture gates. Installed stdio acceptance
  proves parent API-key variables cannot become installation authority and model
  and typed missing-credential refusals agree. No live provider request was used.
  The old descriptor test now compares current executable metadata with current
  constructors; a separate regression retains historical read contracts.
- Main's Memory UI commit `e339c70bf` is rebased into this worktree. Both `plugins`
  check and the complete desktop frontend check passed after this sync, including
  tests, published boundaries, resources and production bundle checks. No plugin
  UI compilation stop was triggered. Backend rounds still avoid shared UI edits.
- `3cbefe94b` implements Saved usage reports: a private archive and ordinary
  workspace component capture, inspect and explicitly remove daily/weekly core
  responses. Core now publishes the actual UTC observation time and relative
  lower bound from one clock reading. These are rolling retained-evidence queries,
  not calendar settlement or a transaction cutoff. Accounting, prices and unknown
  coverage remain core-owned; the archive preserves the returned response.
- Capture admission precedes the public accounting callback. Only newly admitted
  work can observe core; publication and receipt settlement are atomic. Recovery
  retains the original identity. Capture receipts contain headers, not duplicate
  report bodies, so explicit deletion removes the saved accounting payload.
  Replaying old capture/removal receipts cannot resurrect or target later reports.
  The archive refuses at 64 reports / one MiB each instead of silently evicting
  user-saved evidence. The UI preserves missing values, zero and exact JSON numbers.
- The executable binds core's public summary type into the archive envelope and
  generated contract. The plugin RPC package does not import core's protocol or
  define a competing accounting structure. Existing live Usage remains a separate
  read projection over the same core owner, without a new total or price rule.
- Reports verification passed: Runtime build, changed usage/protocol/backend
  tests and vet, dependency gates, real installed stdio capture/read/restart/delete
  acceptance, frontend typecheck/lint, changed wire/component tests, published/UI
  boundaries, and plugin resource reproducibility. After main's `4e54d4f8e` native
  zoom/dock sync, full frontend and plugin checks had already passed; no shared UI
  file was changed by this round. Broader unchanged suites were not repeated.

## Composition and callback baseline

Apple M4, darwin/arm64, Go 1.27, default 10 CPUs; median of three 300 ms benchmark
runs. These are local measurements, not latency guarantees. Reproduce with:

```sh
go test ./runtime/internal/application/integration/plugins -run '^$' -bench '^BenchmarkInstalledPointResolution$' -benchmem -benchtime=300ms -count=3
go test ./runtime/plugin -run '^$' -bench '^BenchmarkTextEmbeddingCallback$' -benchmem -benchtime=300ms -count=3
```

| Boundary | Workload | Median time | Allocation per operation |
| --- | --- | --- | --- |
| Point collection resolution | 1 / 8 / 32 members | 5.29 / 14.80 / 65.85 µs | 4.53 / 18.1 / 80.4 KB |
| Point decoration resolution | 1 / 8 / 32 members | 5.73 / 17.33 / 75.78 µs | 4.53 / 18.1 / 80.3 KB |
| Embedding callback grant + invocation | 1 × 1,024-byte text, 1,536 dimensions | 1.63 ms | 1.60 MB |
| Embedding callback grant + invocation | 16 × 1,024-byte texts, 1,536 dimensions | 16.97 ms | 21.8 MB |

Point resolution uses real contract/applicability/order admission over an
in-memory installation snapshot; it excludes persistence I/O and rendered frames.
Callbacks use the real loopback grant/transport, contract admission and result
decoding with deterministic vectors; they exclude provider latency/accounting I/O.
No speculative optimization was added.

### Browser realization baseline

Apple M4, Chromium 149 headless, 10 reported hardware threads, viewport
1,280 × 577 CSS pixels at device scale 1. Nine fresh-page samples, with the host
modules loaded before measurement; the browser process and filesystem caches
were warm. The 1,029-byte prebuilt Provider ESM from the independent composition
example was served uncompressed over loopback with `Cache-Control: no-store`.

| Boundary | Workload | Median | Sample range |
| --- | --- | --- | --- |
| First plugin module acquisition + ESM evaluation | One Provider module, warm host | 3.5 ms | 2.9–4.5 ms |
| Warm decoration DOM ready | 1 / 8 / 32 nested frames | 0.9 / 1.0 / 1.8 ms | 0.8–1.1 / 0.9–1.2 / 1.7–2.0 ms |
| Warm decoration + two animation frames | 1 / 8 / 32 nested frames | 32.5 / 32.7 / 32.7 ms | 31.9–32.8 / 32.2–32.9 / 31.3–32.8 ms |

The fixture uses production `ComponentModules`, `ComponentCompositions`,
`RenderingComponent`, shared-module import maps and contribution lifetimes.
Metadata and lease admission return deterministic in-memory facts, so this sample
does not include Runtime RPC, core point admission, backend callbacks, provider
latency, full workbench startup or native WebView behavior. Core resolution and
callback baselines above measure different boundaries; their values are not one
end-to-end request. One module resource was acquired in each page, shared by all
frames; repeated module reads were below the 0.1 ms timer resolution except one
0.1 ms sample. Warm DOM samples followed the first single-frame mount. The probe
checked both frame count and outer-to-inner nesting of the current sample.

DOM readiness is observed mutation completion. Two animation frames provide an
additional browser scheduling horizon, not a paint or interaction-latency
guarantee; below-viewport frames are still part of this DOM workload. The roughly
33 ms frame horizon must not be interpreted as JavaScript execution time.

Reproduce from `desktop/frontend`:

```sh
npm run perf:composition
```

In another terminal, use a fresh page for each sample:

```sh
agent-browser --session flame-composition open http://127.0.0.1:4176/visual/composition.html
agent-browser --session flame-composition wait --fn '!!window.compositionBaseline'
agent-browser --session flame-composition eval --stdin <<'JS'
(async () => {
  const cold = await window.compositionBaseline.measure(1);
  const warm = [];
  for (const count of [1, 8, 32]) warm.push(await window.compositionBaseline.measure(count));
  return {cold, warm};
})()
JS
```

Repeat the fresh-page sequence nine times and compare medians/ranges. The fixture
has no installed Runtime, model calls or user data. Close the isolated browser
session and stop the preview when finished. The observed browser tree and a local
screenshot confirmed ordinary nested Provider frames; no performance threshold
was added as a correctness test. Fixture build, frontend typecheck, focused lint,
format and Knip checks passed. No production host path was changed.

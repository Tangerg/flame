# Plugin capabilities roadmap

Forward-looking TODO for extending Flame's plugin line beyond the two shipped
first-party plugins (`memory`, `schedules`). The architecture refactor is covered
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
- Two shipped first-party plugins: `runtime/internal/plugins/memory`,
  `runtime/internal/plugins/schedules`.
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
- Model selection: `ModelSelectionContract` (`flame.model-selection.v1`).
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
- [ ] **Knowledge base / RAG over user docs.** Nearly the same shape as Memory:
      embedding callback + private vector store + `ModelContextContract` to inject
      retrieved chunks. Best "third plugin" to prove the pattern generalizes.
- [ ] **First external connector (pick one SaaS, e.g. Lark or GitHub).** MCP tools
      + a private credential store (+ optional context contribution). This is the
      largest product-value category; it also validates "same business usable from
      UI command *and* model Tool" (the parity just closed in `a15f2f9e4`).
- [ ] **Model router.** A `ModelSelectionContract` contributor that picks
      provider/model per role/budget (cheap model for simple tasks).
- [ ] **Guardrails / safety.** `ModelInputContract` (e.g. PII redaction) or
      `ModelObserverContract` (output classification). Transparent, no new core.

### Tier 2 — more UI, or needs a new mechanism
- [ ] **Web search / fetch / browse** (+ cache store) — tool plugin.
- [ ] **Code execution / sandbox** — tool plugin owning its execution environment.
- [ ] **Usage / cost reports, daily / weekly summaries.** A projection plugin: it
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
   decoration) — currently unmeasured.
3. Do plugin UIs *after* the shared design-system component APIs have settled.
4. Only build the event-ingress contract when a real webhook/event need appears.

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

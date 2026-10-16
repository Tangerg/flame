# Unified plugin refactor: next-session handoff

Updated 2026-10-16 (Asia/Shanghai).

- Latest frontend dependency change: `d06e02636` (KaTeX resolution/lockfile).
- Latest backend/CLI test change: `4bb2481a3` (probe framing/steer diagnostics).
- Reviewed branch HEAD before this documentation refresh: `dc7bdad65`.

Continue from the current branch HEAD; later documentation commits are expected.
Do not checkout/reset back to any checkpoint. Each verification below retains its
own code, dependency and artifact scope.
This document replaces the long chronological handoff. Historical detail remains
in `docs/plugin-refactor-progress.md` and Git history. The handoff update itself
changes documentation only and does not extend any verification scope.

## Start here

- Reuse `/Users/tangerg/.codex/worktrees/unified-plugins/flame`, branch
  `codex/unified-plugins`. Always set the command working directory explicitly.
  The desktop app may otherwise start in the primary checkout.
- Do not modify `/Users/tangerg/Desktop/flame`, recreate this worktree from main,
  push, merge into main, tag, publish, or change the daily Runtime/app.
- The worktree was clean at the reviewed HEAD before this refresh. Preserve the stash:
  `WIP on main: c443a1a8 refactor(runtime): finish the json/v2 migration across delivery and tooling`.
  Never use bare `git stash`.
- Read `AGENTS.md`, `PROJECT_RULES.md`, `DESIGN_PHILOSOPHY.md`, `DEVELOPMENT.md`,
  `REFACTORING.md` and the README of every module being changed. Then read this
  document and the progress document's Current stage / Next / Proposal acceptance
  coverage sections. Repository state wins over older session summaries.
- Reply in Chinese. Keep source, comments, errors and repository documentation in
  English. Comments explain why.
- Root repairs only. Breaking changes are authorized; migrate all affected
  consumers and delete the old path together. One fact, one owner. No compatibility
  patches, dual writes, silent fallback, provider replay, weaker assertions or
  longer success deadlines.
- The user lifted the UI/E2E/screenshot prohibition on 2026-10-15 and authorized
  Computer Use. Older no-UI instructions in historical notes/goal text are stale.
  Use only isolated profiles, Runtime/data directories and identified app windows.
- Do not touch daily Runtime port 17171 or launch the daily-ID production bundle
  `com.tangerg.flame`. Global Wails beta.25 is preserved; Desktop pins beta.29.
- Task npm/build/commit commands use `HUSKY=0`. Preserve shared hooks/configuration.
  Stage explicit paths, run the actual manual hooks below, and commit each verified
  slice. The user's no-push instruction overrides DEVELOPMENT.md's push step.

```sh
cd /Users/tangerg/.codex/worktrees/unified-plugins/flame
git status --short --branch
git log -3 --oneline
```

## Actual completion boundary

Core functionality has matching implementation and scenario evidence. There is no
currently identified unimplemented P01-P48 feature. The proposal audit is
**48 Evidenced / 0 Partial / 0 Open**; all **four review groups are Evidenced**.
Those are evidence counts, not an overall percentage or a 100% claim.

The remaining required work is, in priority order:

1. Establish the historical safe-steer post-Resume timeout cause, or preserve its
   explicit unresolved status when a matching failure cannot be obtained.
2. Establish the historical MCP initial-discovery timeout cause, with the same
   evidence standard. Do not conflate it with safe-steer.
3. The KaTeX advisory chain is remediated in the selected dependency tree (see
   Priority 3). The recorded production-only npm audit reports no known dependency
   vulnerabilities; it is not proof that the product has no vulnerabilities.
   Braces remains open: no supported patched release was available at the last
   check. Keep the audited build exposure and removal conditions explicit.
4. Reconcile final acceptance against changes actually made and retain explicit
   unexecuted/risk boundaries. Do not rerun every old gate just to restate status,
   or describe missing evidence as a pass.

These are stability, security and acceptance tasks. They may require production
code repairs. Current passing tests do not establish that only paperwork remains.

## Priority 1: safe-steer

### What actually failed

Historical source reference: `366ffff6a`. The retained original output names
`steer_receipt_integration_test.go:144`, safe 15.70s / outer 26.75s. This is the
unchanged 10s wait for the second provider request **after Resume succeeds with a
new Segment**. No failed-interval Runtime log, goroutine dump or test directory
survives; the user explicitly confirms none is available. Do not ask again for
those same artifacts.

Resume acceptance precedes detached execution activation. Relevant current paths:

- `cli/internal/adapter/runtimebinding/steer_receipt_integration_test.go`
- `runtime/internal/application/agent/runs/resume.go`
- `runtime/internal/application/agent/runs/coordinator.go`:
  `segmentStartup.activate` / `beginExecution` / `activateExecutor`
- `runtime/internal/adapter/run/execution/interaction_accepted_restore.go`
- `runtime/internal/adapter/run/execution/interaction_continuation.go`
- `runtime/internal/adapter/run/execution/interaction_dispatch.go`
- `runtime/internal/adapter/run/execution/interaction_durability.go`

### Already established; do not rediscover or mislabel

- Immutable-schema compilation cost and exact Deployment reconstruction had
  independently reproduced defects and repairs. The per-group MCP capture repair
  is `981c4c915`. A historical safe control reproduces the Deployment refusal in
  2.78s, not the unexplained 10s waiter. Neither explains the latter.
- A fully pinned historical overlay traced activation, answer delivery/proof,
  process resumption, actual Shell execution, commits/pump acknowledgement,
  next-model context, applied steer, Model start and provider iteration. Normal
  post-Resume timing reaches the second HTTP request in 173.4ms. Passing traces,
  six-pair repetitions and parallel/full gates do not identify the failed interval.
- Four targeted boundary failures (activation, Tool, steer commit, pre-dispatch)
  yield the same caller timeout while the Run has already failed/lost in
  0.44-1.05s. These are intentional failing controls, not historical reproductions.
- `4bb2481a3` adds a fifth distinct observation: remove Tools only from the test
  fixture's decoded second request, after HTTP arrival. The Run completes, but
  the fixture does not classify the request as second and the original 10s
  assertion fails. This demonstrates a classification blind spot, not a product
  repair or proof that the historical request had that shape.
- Current failure-only diagnostics retain client phases, every provider arrival's
  elapsed time/stream flag/Tool count/Tool-result IDs, goroutines and authoritative
  Run outcome. Failure inspection has its own 2s bound after the original failure;
  it cannot extend the 10s success window. Original assertions and event loops
  remain unchanged. Auxiliary title requests can have zero Tools.

### Next useful experiment

The 2026-10-16 ownership audit (progress doc, "Deadline-path ownership audit")
examined the receipt handshake's cancellation boundaries, pump acknowledgements
and activation resolution. It found no reachable orphan/deadlock in the inspected
normal path. This is a scoped code-review finding, not a proof covering all error,
cancellation, panic or blocked-callback paths, and not a historical diagnosis.
Choose a new falsifiable ownership/order hypothesis before running a bounded
experiment. Correlate Resume acknowledgement with activation,
Tool dispatch/result, durable commit receipt, physical model admission, actual HTTP
arrival and fixture classification. Keep the production state and client observer
separate. On failure, retain the live owning Run state and matched stage entries /
returns before teardown. A completed/failed/lost Run is not a live stalled Run.

Do not replace the diagnosis with another successful repetition, synthetic delay,
unmeasured build contention, Deployment drift or an injected failure. If no
matching failure is captured, leave the historical cause open rather than making
a speculative production change.

## Priority 2: MCP startup discovery

### Actual failure and boundary

The retained no-overlay parallel execution/bootstrap gate based on `523b2e917`
exits 1 after 551.760s: five failures, 380 top-level passes, no skips. One edited
SIGKILL cut and four Memory/Schedules reply-loss cases project `connecting` beyond
an existing 10s discovery prerequisite. Parent stacks show SDK `Client.discover` /
`AsyncCall.Await` waiting for the first `server/discover` response. This is before
Flame capability verification, admission, setup, activation and Core callbacks.
The original child/backend interval is missing.

The normalized actual failed stack and candidate hashes remain available with:

```sh
git show 6b3a175e8:docs/plugin-refactor-mcp-startup-evidence.md
```

Relevant current paths:

- `runtime/internal/bootstrap/plugin_acceptance_mcp_test.go`: `poMCPTools`
- `runtime/internal/bootstrap/managed_services_test.go`
- `runtime/internal/bootstrap/testdata/managed-message-loss/main.go`
- `runtime/internal/bootstrap/managed_message_loss_test.go`
- `runtime/internal/adapter/run/execution/schedule_recovery_product_crash_test.go`
- `runtime/internal/infra/integration/mcp/config.go`, `attempt.go`, `reconnect.go`
- `runtime/internal/plugins/memory/main.go`, `schedules/main.go`

### Already tested or ruled out as an explanation

- Whole/fragmented frame, held directory lease/release and stdin EOF + SIGTERM
  controls identify expected boundaries. A held lease delays pre-transport startup;
  release yields a reply in about 16/13ms. EOF during that pre-stdio wait does not
  itself cancel startup. This is not proof that a lease caused the old failure or
  authorization for an unrelated lifecycle rewrite.
- Cold pre-main and Go init traces show startup delay but no reproduced 10s stall.
  A 454.684s startup/proxy/9s-snapshot overlay passes, with 39 normal discovery
  traces and no eligible discovery-blocked child. It is not a failure explanation.
- Expected go-sdk version/checksum is the same at historical/current references:
  `v1.8.1-scope.1`, `h1:8V8k8aeqbEDgLlH3T4+DZnf0DM1dSOGofkAXb3dZjKM=`.
  The archive already reads discovery metadata before acquiring the session state
  mutex. Do not credit that existing lock-order fix to the Scope upgrade. This
  audits expected source, not the old child's actual execution image.
- New actual-client controls: oversized response returns EOF in 29.883ms; backend
  exit 17 returns EOF in 33.406ms. Neither reaches the unchanged 10s client bound.
  Whole/fragmented small replies connect in 940.135/27.428ms. Existing hard-stop
  retirement errors are recorded separately, not claimed as clean exits.
- A separate real defect is now fixed in `4bb2481a3`: the test proxy's Scanner
  default rejected legitimate frames above 64 KiB. Both scanners derive their
  budget from the SDK-owned 16 MiB constant. Default bootstrap regression proves
  80 KiB discovery requests/replies preserve bytes; original scanners fail both
  cases. This fixes the fixture, not the historical discover deadline.

### Next useful experiment

The 2026-10-16 ownership audit found that the child acquires its data lease and
opens SQLite before serving stdio. In the inspected supersession path, a new
attempt waits on the recorded outgoing-session retirement before launch. No lease
overlap was found in that path. This does not prove every error/cancellation or
process-descendant path safe, establish external contention, or recover the old
child's execution interval. Process exit/join must not be described as proof that
a Go defer executed. The historical 10s value is the captured test's readiness
bound, not a universal SDK handshake default.
Choose a concrete hypothesis and a bounded, owned process graph. Correlate parent,
proxy and backend PID/session with spawn, pre-main, lease acquisition, schema /
registration completion, stdio entry, discover receive, reply write and proxy
forward. Capture metadata/byte counts, not private payloads or credentials. A
parent pipe-read stack alone cannot select the missing child phase. Inspect an
eligible live child before teardown; do not qualify killed/unclosed CPU profiles.

For diagnostic overlays, propagate GOFLAGS to nested go builds. A command-line-only
`-overlay` does not modify the proxy built by its test helper; that already
produced an invalid passing negative control. Never mutate the published module
cache or copy Scope/SDK internals into Flame. Repair the proven owner only.

## Priority 3: npm advisory chains

At the recorded post-repair check (`d06e02636`, 2026-10-16), npm audit reports
**4 high / 0 low** overall and **no known vulnerabilities in the production-only
dependency audit**. The remaining four entries represent one braces advisory
chain. These are audit/database results for the selected graph, not a product-wide
security guarantee. This documentation refresh does not rerun npm audit.

| Chain | Status and verification boundary |
| --- | --- |
| rehype-katex / micromark-extension-math -> katex | **Remediated in the selected graph.** `overrides: { "katex": "$katex" }` selects the app's declared KaTeX range; the lockfile and installed tree resolve one 0.19.0 for rendering and CSS, replacing both 0.16.47 copies. This explicitly overrides the integrations' declared `^0.16.0` ranges; upstream packages have not thereby declared 0.19 compatibility. The app owns compatibility verification and future maintenance of this resolution policy. The recorded full frontend check, markdown tests, render smoke and npm ci reproduction support the tested scope. An additional read-only check verified all three import paths resolve 0.19.0 and exercised the installed remark-parse -> remark-math -> remark-rehype -> rehype-katex HAST pipeline: inline, display and trust-gadget inputs, each with/without deliberate inherited trust, pass all six cases without unsafe links. That check executes no browser or native app. Version unification alone is not a compatibility proof; reconsider the override when upstream supported ranges change. |
| StyleX PostCSS 0.19.1 -> fast-glob 3.3.3 -> micromatch 4.0.8 -> braces 3.0.3 | **Open; no supported patched release at the last check.** The audited path is a build devDependency using two repository-owned shallow globs; no attacker-controlled nested-pattern ingress was found in that path. This is scoped exposure analysis, not proof that untrusted input can never arrive. There is no verified supported direct upgrade for this chain at the checkpoint; it is not correct to claim that no code migration could remove it. Recheck upstream fixes and, if justified, evaluate a complete integration migration with its actual build consumers. Removal requires a qualified dependency chain without the vulnerable version. Build-only exposure, an ignored advisory or a forced downgrade does not remediate the vulnerability. |

Sources: [braces advisory](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm),
[KaTeX advisory](https://github.com/advisories/GHSA-238p-pmpm-9mq7).
Recheck upstream braces state before each session; this observation is dated.

## Verification already performed

Do not repeat these solely to restate status, and do not broaden their scope.

| Source / scope | Actual result |
| --- | --- |
| Product source `6d5e2427d`; original four-module parallel race, -p 2, -count=1, package timeout 900s | 174 packages finished: 159 tested pass, 15 no-test. 9,187 test/subtest passes. Six paid DeepSeek cases plus helper conditionally skipped; not counted as passed. Bootstrap 375.810s, delegated 3.03s, safe/yolo 2.82/2.72s, lost Core opening 4.46s, all eight execution SIGKILL cuts pass. |
| Same product source; five workspace modules | go mod verify, vet/build pass. Generation plus existing formatter reproduces tracked bytes. Desktop Go race 40 passes. Runtime/first-party distribution builds. |
| Independent authors at `6d5e2427d` | Reviews/Tool-proposal author with GOWORK=off, no replace, isolated archives/cache, offline verify/race/vet/build passes. Public component archives independently build Usage and match emitted bytes; clean offline reinstall passes. These are not registry publications. |
| Shared/frontend qualification at `c44671017`, before the KaTeX dependency change | Shared full check 364 tests; Desktop complete check 357 files / 2,429 tests plus architecture/API/resource/bundle guards. These historical results keep that source/dependency scope. |
| Frontend dependency graph at `d06e02636` | Recorded complete npm run check passes, including full vitest and architecture/API/resource/bundle/module guards. Markdown subset: 10 files / 96 tests. npm ci reproduces one KaTeX 0.19.0; recorded production-only audit reports no known vulnerabilities. These were not rerun by this documentation refresh. |
| Read-only follow-up at `dc7bdad65` | Lockfile/npm ls and three import paths confirm one 0.19.0. Six installed HAST math-pipeline cases (inline/display/trust gadget, with/without inherited trust) pass. This does not execute the React browser/native surface or replace the full frontend gate. |
| Native artifacts qualified before `d06e02636` | Private Wails beta.29 arm64 packaging/signature/macOS 13 floor and actual isolated original-command recovery pass for the earlier built artifacts. They retain the original item/receipt, replay-trap and zero-Run/model evidence. The control alone uses a bootstrap-address overlay. The packaged frontend predates the KaTeX change: these results do not qualify a newly built post-upgrade native artifact. Fresh native packaging/targeted math rendering remains unexecuted. |
| Backend/CLI slice `4bb2481a3`; complete bootstrap/runtimebinding parallel race | 417.252/40.909s, 235/342 test/subtest passes (153/204 top-level), zero failures/skips. All original bounds/assertions retained. |
| Same backend/CLI slice; focused bootstrap/execution/runtimebinding parallel race | 68.516/50.624/10.093s; response-loss recovery, safe/yolo, lost Core opening and all eight SIGKILL cuts pass (cut parent 48.35s). This precedes only stricter cleanup assertions in the new frame regression, which the complete bootstrap run then covers. |
| Same backend/CLI slice; local checks | Vet passes bootstrap/runtimebinding/proxy; gofmt, diff, manual lint-staged/commitlint pass. No new whole-product/frontend/native gate is claimed for this test-only slice. |

The frontend dependency change does not invalidate historical Go-only results,
but it changes the packaged JavaScript payload. Before claiming current native
acceptance, build an isolated artifact with the new graph and qualify its math
rendering. Do not carry old native results forward as a fresh pass. Backend full
race, paid providers and native gates were not rerun for the KaTeX slice; this
refresh adds no new execution of them.

Original full backend command, to use again when a relevant final change warrants it:

```sh
go test -json -race -p 2 ./runtime/... ./runtime/localruntime/... ./cli/... ./examples/plugins/reviews/backend/... -count=1 -timeout=900s
```

The eight SIGKILL cuts belong to execution, which invokes bootstrap's crash helper.
Bootstrap alone does not execute them. For a production change affecting recovery,
run whole bootstrap, the relevant recovery/steer/schedule cases and those cuts;
do not replace the original parallel gate with a serial pass. Frontend changes
follow the repository's full check gate; native changes require native proof.

## Evidence and resources

Task evidence root (verified present at handoff, but OS temporary storage can be
cleaned; check existence before depending on it):

```text
/var/folders/d5/z_zd9f0n35v9d5lclp27rdv40000gn/T/flame-ui-acceptance-81o3iszt/evidence
```

Important files under that root:

- `steer-provider-classification-summary.json`, full intentionally failing
  `steer-provider-classification-control.jsonl`.
- `mcp-proxy-frame-controls.json`, `mcp-sdk-provenance-audit.json`.
- `mcp-proxy-final-frame-red-inherited.jsonl`; command-line-only negative control
  without inherited GOFLAGS is invalid and retained separately.
- `deadline-boundary-gates-summary.json`,
  `deadline-boundary-complete-packages-race.jsonl`,
  `deadline-boundary-focused-race.jsonl`.
- `deadline-control-*.txt`: retained temporary control/overlay source copies.
  Executables and working overlays were removed; recreate owned files explicitly
  before using any copied overlay's former absolute paths.
- `npm-deadline-followup-summary.json`, both audit JSONs,
  `npm-upstream-deadline-followup.json`, `npm-katex-prototype-control.json`.
- `final-corrected-source-gates.json`, `final-source-core-gates.json`,
  `final-source-independent-author.json`, `final-component-author.json`.
- `final-native-original-command-recovery.json`,
  `final-native-production-package.json`, `final-native-control-package.json`.

Screenshots live under:
`/Users/tangerg/.codex/visualizations/2026/10/14/01a13b1e-2bac-7272-9e13-68d9dde5283f`.
The actual recovered native screenshot is `plugin-native-original-recovered.png`.
Private evidence may contain expired credentials/callback references. Read only
needed metadata; never print whole request packets or copy credentials into docs.

All invoked jobs and task Runtime/native/provider handles are terminal. Task
listeners were released. Latest temporary control executables/overlays/npm cache
were removed. Author caches/proxies/dependencies and copied installations were
cleaned. Retain the worktree, normal dependencies/build outputs, private evidence
and intentionally unknown uninstalled Web command. Do not delete unknown facts
or task data to manufacture a clean state. If reopening UI qualification, create
new isolated lifetimes and verify the target before launch.

## Breaking-data and unexecuted boundaries

- Selected Scope graph: thirty modules at v0.51.1. This is the qualified version,
  not a claim that it remains the latest available release.
- Core protocol is `2026-10-15.2`; backend protocol is
  `io.github.tangerg.flame/backend/2026-10-15.3`; export schema is 3.
  Waiting checkpoints require per-group mcp_manifests and tool_proposals.
- Existing journal records now require method metadata. Preserve former records
  and reconcile with their original client/binary before a fresh-profile upgrade.
  No compatibility reader, automatic conversion, receipt copying or replay exists.
- Paid DeepSeek cases, other OS/architectures, real second-machine TLS and registry /
  deployment publication are unqualified. Publication is outside current permission.
- The dedicated graphical private invocationConflict entrance remains unexecuted;
  actual Tool/CLI identity conflict and graphical revision-conflict evidence have
  their own scopes in `examples/plugins/reviews/README.md`. Do not infer every
  refusal class from them.
- A post-KaTeX native package and targeted native math rendering have not been
  qualified. Earlier native evidence belongs to the pre-upgrade artifacts.
- An additional native return to the Memory list after successful original recovery
  was not executed because the Mac relocked. The successful recovery screenshot
  and exact receipt comparison remain valid; do not infer the extra navigation.
- Core owns Run/execution/accounting; backends own private business facts/receipts.
  Pending unknown routes do not expire into permission to execute again.

## Close each slice

Inspect status and real callers -> state a falsifiable hypothesis -> implement the
smallest complete owner repair -> focused verification -> proportionate final
gates -> explicit-path commit -> update progress -> clean owned resources.
Keep failed controls, failed gates, skips and unexecuted cases distinct from passes.
If the historical interval remains unavailable and no causal reproduction exists,
report that limit honestly; do not declare the overall refactor 100%.

Manual hooks (run from desktop/frontend after explicit staging):

```sh
HUSKY=0 npx --no-install lint-staged --no-stash --no-hide-partially-staged
HUSKY=0 npx --no-install commitlint --edit /absolute/path/to/owned-message.txt
```

Docs/Go-only paths can legitimately match no frontend lint-staged tasks. Still run
Go formatting/diff checks appropriate to source changes. Do not alter hooks to
make a check pass.

Read-only original inputs:

- `/Users/tangerg/Desktop/docs/Flame-Unified-Plugin-Proposal.html`
- `/Users/tangerg/Desktop/docs/Flame-Unified-Plugin-Proposal-Review-2026-10-13.md`

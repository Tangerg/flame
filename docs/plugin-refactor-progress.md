# Plugin refactor progress

## Current stage

Stage 1 complete: installation-owned selection and desired enablement.
Stage 2a complete: public backend wire binding and refusal/uncertainty semantics implemented, verified and committed.
Stage 2b is next: durable original operation binding before a generic host mutation entrance. The full proposal remains open.

Working checkout: /Users/tangerg/.codex/worktrees/unified-plugins/flame.
Branch: codex/unified-plugins.
Starting commit: 5262aadc002d40d415212ead3b311005c1c6a62f.
Stage 1 commit: 869d4304.
Incoming local main: 61645c2561b174042aa157aef21cc00cfba67a86.
Integration commit: f5e4bdbc140ad4d24b6b723ffdfd908cdff515a3.
Stage 2a commit: c4706b6b.
Stage 2a main synchronization: git merge main reported already up to date at 61645c2561b174042aa157aef21cc00cfba67a86.
Canonical progress file: docs/plugin-refactor-progress.md in this checkout. The user-requested /Users/tangerg/Desktop/flame/docs/plugin-refactor-progress.md points to this same file; it is not an independently edited copy.

Read-only design inputs:

- /Users/tangerg/Desktop/docs/Flame-Unified-Plugin-Proposal.html
- /Users/tangerg/Desktop/docs/Flame-Unified-Plugin-Proposal-Review-2026-10-13.md
- /Users/tangerg/Desktop/docs/Flame-Durable-Improvement-Measures-2026-10-12.md
- /Users/tangerg/Desktop/docs/Scope-Flame-Agent-Architecture-and-Upgrade-Report-2026-10-12.md

## Completed

- Read current repository instructions, design rationale, development and refactoring rules.
- Created an isolated managed checkout at the current committed main baseline; no product source changes in the shared checkout.
- Revalidated the proposal and review against current installation, delivery and dependency ownership.
- Classified existing UI/E2E checks as prohibited by the explicit task constraint; their omission must remain visible in verification reports.
- Replaced unapproved/approved/enabled installation state with installation-owned desired enablement across Domain, persistence, Delivery, generated contracts, Go/TypeScript bindings, CLI and graphical consumers. Removed approve/revoke operations, errors and current workflow instructions without compatibility aliases or decoding.
- Selection preserves desired enablement, drops secrets whose recipient changes and atomically refuses missing required configuration for an enabled candidate. Tests cover preserved preference, rejected selection, stale drafts and exact release authority.
- Same-release disable/re-enable remains available while execution holds a dependency; replacement, configuration and uninstall still require quiescence. Standing model-tool rules retain their separate source-qualified owner.
- Default distribution initialization still happens once and preserves user choices. Bounded directory enumeration now uses the actual installation capacity and preserves I/O, cancellation and close failures.
- Corrected generated TypeScript method-policy vocabulary to derive from the Delivery registry, including the existing owner-committed Run stream policy. Added a generator regression instead of another handwritten policy list.
- Extended exact-schema admission coverage for the former installation trust column, including retained-data checks. Incompatible directories remain refused rather than converted.
- Existing frontend fixtures were mechanically migrated to the removed APIs/shape and obsolete approval UI cases deleted. No new frontend UI assertions or test execution were added.
- Committed the complete first slice and merged local main without conflict. The incoming change reconciles already-accepted Interaction answer Signals as success. Integrated Runtime/CLI module checks and execution races passed.
- Fixed the existing commit-msg hook's assumption that COMMIT_EDITMSG is always relative. Linked worktree commits supply an absolute path; the hook now preserves it while using the same commitlint rules.
- Added runtime/plugin in the existing Runtime module as the one public typed MCP backend binding. Memory, Schedules, their actual host callers and the independent Reviews backend use it; former private method/result/client/tool registration implementations were removed, not wrapped.
- Unrecognized storage/commit/transport/cancellation errors remain protocol failures. Only explicitly classified business refusals become a known Failure. Memory/Schedules classifiers no longer label every unknown error internal as a business result.
- Closed method envelopes reject missing input, unknown members and malformed command identity before mutation. Replies reject ambiguous/missing outcomes and preserve an explicit nullable value separately from a missing reply.
- Centralized logical Tool metadata in the public binding. Existing Tool receipt namespaces remain unchanged; model arguments and transport IDs cannot originate that identity.
- Reviews now exposes typed list/update backend methods and its existing model Tools through the same private store. Tests prove shared receipt replay across entrances, typed revision/identity conflicts and preserved uncertainty for unexpected storage failure.
- Verified an external Reviews author with GOWORK=off, no local replace and a temporary module archive containing only the public SDK files plus module metadata, without Runtime internal source. Tidy/test/vet/build all passed in that isolated offline fixture. This proves the backend binding subset, not the full future SDK or a published release.

## In progress

- The generic host operation entrance remains unopened until original-binding persistence, retention, quiescence and receipt reconciliation are implemented.
- Next owner boundary: Runtime captures immutable routing/input/origin/parent/target evidence before backend dispatch; private backend receipts remain the sole business outcome. Derive held releases from unresolved commands at the same installation admission serialization point, rather than adding a refcount or a host business-result cache.
- The confirmed pre-existing runs.getAcceptance frontend consumer-gate failure remains tracked for a real reconciliation caller; no dummy call or weaker source coverage was added.

## Next

1. Implement durable original command binding and typed installed-operation admission, including receipt-only reconciliation after response loss, Runtime restart, disable/re-enable, update, uninstall and provider replacement. Then finish the single-source Go/schema/TS authoring route. Re-read proposal sections 6-12 and 17-18 plus the review's two API-freeze gaps before implementation.
2. Prove real managed callbacks and cancelable streaming decoration without a graphical relay.
3. Replace the installed guest carrier with public component loading and establish the Reviews operation/board/analyzer slice.
4. Migrate Memory, Schedules and remaining contributions through the same public contracts, remove superseded paths, and prove independent author parity.
5. Complete requirement-level audit and final clean delivery.

## Decision ledger

| Date | Decision | Reason | Rejected alternative |
| --- | --- | --- | --- |
| 2026-10-13 | Keep external proposal and review read-only; record corrections in owning repository design documentation and this ledger | The task explicitly identifies read-only inputs; corrections must not rewrite review evidence | Editing the external inputs |
| 2026-10-13 | Use one managed checkout and one canonical progress file, exposed at the requested path by a symlink | Isolate product changes from concurrent user work without creating competing progress copies | Editing product files in main or maintaining two progress ledgers |
| 2026-10-13 | Preserve the existing Go-owned contract-to-schema/TypeScript direction for the public authoring slice | The existing generator does not consume JSON Schema to produce Go; reuse one structural owner and existing generation mechanisms | Parallel handwritten Go/Schema/TS or a new custom IDL compiler |
| 2026-10-13 | Remove separate release approval; retain selected digest and desired enablement | Explicit installation/update is the trust decision; enablement remains a separate product preference | Renaming approval, automatic tool approval or a legacy state decoder |
| 2026-10-13 | Initial install remains disabled until explicitly enabled/configured; selection preserves desired enablement and refuses an enabled candidate missing retained required inputs before mutation | Keep user preference while preserving construction invariants and credential-recipient binding; disable/configure/enable is explicit when credentials cannot survive | Silently reusing executable secrets, implicitly resetting user preference or publishing invalid configuration |
| 2026-10-13 | Permit disable/re-enable of the same selected code while its execution dependency is held; retain quiescence for selection, configuration and uninstall | Recovery must be able to restore availability of the frozen release without replacing its code or inputs | Treating every availability transition as release replacement or making withdrawal irreversible while work waits |
| 2026-10-13 | Derive generated method-policy unions from registered Delivery metadata | The ownerRunStream policy already existed in main but the handwritten TypeScript union could not represent it | Adding a second manually synchronized vocabulary or dropping the registered policy |
| 2026-10-13 | Enumerate the default distribution with an owned, bounded directory reader | The required architecture gate exposed an unbounded directory read; the installation owner supplies the actual maximum | Ignoring the gate or choosing an unrelated arbitrary bound |
| 2026-10-13 | Reuse installed local development dependencies in the isolated checkout with local package links | Offline installation lacked cached packages; no source, shared config or credentialed service needed mutation | Sharing a node_modules symlink that resolves old contracts or running install scripts against shared state |
| 2026-10-13 | Preserve Git's absolute commit-message path in linked worktrees and resolve relative paths before changing directory | The existing hook blocked a valid local commit by prefixing an already-absolute path | Disabling hooks, weakening commitlint or changing shared Git configuration |
| 2026-10-13 | Publish the existing backend method binding in runtime/plugin and migrate all actual consumers together | A demonstrated public author boundary should have one implementation; independent Reviews must use the same method and Tool semantics as bundled backends | Keeping a private richer binding beside a public imitation, or creating a separate SDK module |
| 2026-10-13 | Only explicitly classified business errors are known refusals; empty classification preserves protocol failure | The former wrapper encoded every internal/commit failure as a business response, which could hide uncertainty | Treating all errors as definite failures or silently retrying a possibly committed effect |
| 2026-10-13 | Host unresolved-command evidence owns immutable routing, not mutation outcome; only the original owner's valid response discharges its dependency | No-Run commands need original provider/release retention without a competing business receipt | Client-current proxies, execution-only dependency holds, mutable provider retargeting, or host success fabrication |
| 2026-10-13 | Prove the current public backend subset through an isolated file-proxy SDK archive while its release metadata is still pending | Independent build evidence must not rely on Runtime internal source, workspace privilege or local replace; a temporary snapshot must not masquerade as a published artifact | Overwriting a cached published module, claiming an old release contains the new SDK, or publishing before the final authorization boundary |
| 2026-10-13 | Run backend, contract, failure and relevant race checks; do not write/run frontend UI, E2E or screenshot checks | Explicit task verification constraint | Running the composite frontend gate without inspecting its test components |
| 2026-10-13 | Merge current local main into the feature branch after each verified slice; do not push, publish, modify shared main history or use credentialed services during ordinary slices | The amended task explicitly requires incoming main synchronization; external/shared changes and credentials still require authorization | Deferring integration until final delivery or treating code-edit authorization as production authorization |

## Review closure

Status is open until code and appropriate verification prove the requirement.

| Review item | Status | Implementation/evidence |
| --- | --- | --- |
| Baseline calibration and accurate old/new comparison | Resolved in planning | Current stage and decisions; external inputs preserved |
| Runtime/Scope single authority and private business fact ownership | Open; preserve throughout | runtime delivery, run execution, private plugin owners |
| Remove business-specific service/view knowledge from substrate | Open | plugin declaration and public contribution contracts |
| Public operation shared by Tool, CLI, UI and other plugins | Backend typed/Tool owner sharing verified; generic host/CLI/UI/other-plugin admission open | runtime/plugin; examples/plugins/reviews/backend/server.go; server_test.go |
| Preserve origin, parent identity and concrete target | Open | shared call envelope and execution context |
| Public React/TS components and shared module instances | Open | public SDK and installed component loader |
| Consumer-owned collection/selection/decoration and plugin-defined points | Open | public point contracts and Reviews analyzer |
| Real callback, remote next, backpressure, cancellation and inner evidence | Open | managed link and real-process backend tests |
| No-Run unresolved human command pins original binding and recovery evidence | Open; required before public mutation | delivery/persistence, release retention, owner receipt reconciliation |
| One executable contract generation source | Go source direction settled; public backend types/build verified; Go/schema/TS feature generation open | runtime/plugin; existing Go contract generator; independent archive fixture |
| Typed business refusal versus transport uncertainty across entrances | Public Go/backend and Reviews typed/Tool semantics verified; generic host/CLI/TS projections open | runtime/plugin/method.go; method_test.go; Memory/Schedules classifiers; Reviews server_test.go |
| Recovery entrance independent of failed optional UI | Open | startup/plugin management and CLI |
| Strict public protocol requirements without internal revision coupling | Open | public SDK handshake and contract tests |
| Explicit candidate update, withdrawal, quiescence and uninstall behavior | Stage 1 enablement/update semantics verified; generic provider and unresolved-command lifecycle open | runtime/internal/domain/integration/plugin/installation.go; runtime/internal/application/integration/plugins/installations.go; owning tests |
| Reject old host encodings without data mutation | Installation shape covered; full plugin/public contract cutover open | runtime/internal/infra/sqlite/plugins_test.go; schema_admission_test.go; runtime/README.md |
| Preserve Memory ledger, watermark, curation and receipts | Open | public Memory capability and backend tests |
| One Schedules wake/dispatch lifetime and retained Run acceptance | Open | public background role and Schedules tests |
| Preserve package integrity, credentials, source identity and core authority checks | Open; baseline required | existing admission and policy regression checks |
| New acceptance coverage for unresolved mutation/update/provider replacement | Open | real-process contract/regression tests |
| Standalone bundled/non-bundled author parity | Open | independent public consumer builds |
| Deletion of replaced APIs, carriers, schemas, examples and documentation | Open; audit every slice | final source and generated-contract search |

## Proposal acceptance coverage

Full P01-P48 completion remains open. Stage 1 supplies installation-state, default-initialization and incompatible-storage evidence; it does not prove the new public platform. Grouping names the evidence required, not a reduced scope.

| Cases | Required evidence | Status |
| --- | --- | --- |
| P01-P09 | Author parity, selected roles, plugin-defined points, declaration failure, provisional resources and available management | Open |
| P10-P20 | Reentrant host callback, installed model decorators, frozen ordering, admitted transformed input, command identity, stale next, preserved inner evidence, stream cancellation/backpressure and actual model identity | Open |
| P21-P28 | Shared operation entrances, receipts/conflicts, scheduling acceptance/capture, independent Run lifetime and Memory owner invariants | Open |
| P29-P35 | Shared React binding, recoverable UI failure, target capture, shared resource owner, applicability, capacity and appearance owner | Open; UI/E2E execution excluded by user |
| P36-P41 | Generation safety, waiting dependencies, retained data, generic history, remote environment and headless host calls | Open |
| P42-P48 | Portable packages, strict current contracts, non-mutating old-state refusal, recovery, shutdown, diagnostics and final deletion | Open |

## Verification

Stage 2a verification:

- Passed: public SDK and Memory/managed-model regressions, including -race; independent Reviews go test -race ./..., go vet ./... and go build ./....
- Passed: current SDK package GOWORK=off GOPROXY=off go test ./plugin. The isolated external Reviews author fixture passed GOWORK=off tidy/test/vet/build with only public SDK source and no local replace or Runtime internal source.
- Passed: real stdio Reviews installation, lost response, Runtime/backend restart, process retirement and CLI observation via go test ./internal/bootstrap -run TestReviewBackend -count=1.
- Passed: CLI full test/vet/build. Runtime's initial full suite passed all behavioral packages but exposed the intentional new public-package baseline; after updating that exact baseline and external consumer fixture, full internal/arch, vet and build passed. The final full Runtime go test ./... also passed.
- Resolved: the moved method's same-package model test callers initially referred to the deleted private functions; they now consume the public package. The public-package gate retains exact checking and dependency rejection, with plugin intentionally admitted as a transport boundary.
- Pending release graph, failed as expected: GOPROXY=off GOSUMDB=off go mod tidy in the actual Reviews module cannot find runtime/plugin in its currently named published Runtime version. Workspace and isolated current-source archive builds pass. Update published dependency metadata when the final SDK-containing Runtime version is available; no cached published module was overwritten or released in this slice.
- Not run: frontend checks in this Go/backend-only slice; no frontend files, UI/E2E/screenshot checks or new UI tests were added. The pre-existing runs.getAcceptance consumer-gate failure remains open.

- Passed before incoming main integration: runtime and CLI go test ./..., go vet ./... and go build ./.... Runtime's architecture/dependency checks run as part of its Go suite.
- Passed after incoming main integration: runtime and CLI go test ./..., go vet ./... and go build ./.... Runtime internal/arch and internal/dependency and CLI internal/arch passed with those suites.
- Passed: go generate ./... and the shared Runtime TypeScript npm run check, including 284 contract tests, types, lint, formatting and knip.
- Passed: targeted -race checks for plugin Domain, plugin Application, SQLite, package admission, execution and bootstrap. After final bounded-reader/cancellation and duplicate-case cleanup, focused distribution/default-initialization and withdrawal races also passed.
- Passed after final changes: SQLite exact-schema/former-installation-shape races, and the complete integrated execution package -race suite (167.235 seconds).
- Passed: frontend typecheck, lint, format, knip, locales, plugin message-frame contracts, circular/context/published-boundary/layer/port checks, HMR/bootstrap/style/design-system checks, tracked plugin resource rebuild verification, production bundle/CSS resolution and bundle-size gates. These static/build/contract checks do not execute the graphical UI.
- Failed, confirmed pre-existing on local main 61645c2561b174042aa157aef21cc00cfba67a86: check:api-consumers reports runs.getAcceptance has no non-test frontend consumer. Its seven checker regression tests pass. The CLI inventory now accurately identifies external reconciliation; the frontend source-consumer requirement remains open for a real use case.
- Resolved preparation/check failures: offline npm ci lacked cached dependencies; isolated copies of existing dependencies restored builds. Plugin resources initially lacked rolldown in this checkout, then passed after isolated dependency setup. Formatting drift was repaired. The pre-existing missing ownerRunStream TypeScript union and CLI API inventory entry were corrected at their owners.
- Not run: frontend UI, E2E, browser/native and screenshot checks, including the composite frontend npm run check whose test phase would violate the task. Native script fixtures were updated but their execution is not claimed.
- External proposal and review hashes still match their read-only baseline. No production/user data was opened, converted or deleted.
- Passed: changed Go files gofmt -l, git diff --check and feature commitlint using the owning frontend configuration. The managed checkout initially lacked ignored hook dispatchers; the existing dispatcher files were copied into this checkout without changing shared Git configuration.
- Resolved commit failure: the restored commit-msg hook prefixed an absolute linked-worktree message path with PWD. The corrected hook passed shell syntax validation, accepted a valid message and rejected an invalid message through both absolute and relative paths. Its temporary message fixtures were removed.

### Round cleanup

- Stage 2a: the temporary SDK file proxy, independent author checkout and isolated module cache were removed by their owning temporary-directory lifetime. Backend/unit/integration checks have all returned; no task check process is running. No frontend build output was generated in this slice. The worktree is committed and clean after main synchronization; the active checkout and necessary development dependencies remain for Stage 2b.
- Removed task-generated frontend/Icons/UI dist output, generated Icons/UI build sources and Runtime contract Vitest cache. Removed the task's offline-install debug log if present. All invoked checks have returned terminal success/failure; no task check process remains running.
- Kept the active managed checkout, formal code/tests/docs and isolated development dependencies for the continuing refactor. Shared product source and Git configuration were not edited. Before the next frontend verification, rebuild Icons and UI to restore their generated prerequisites.
- No remote push, publication, main-history mutation, production operation or credentialed action was performed.

## Unresolved questions

- Define durable unresolved command binding/retention at the actual command owner before generic plugin mutation is exposed.
- Settle typed refusal/result and public contract generation details through the independent Reviews consumer.
- Verify the selected MCP/SDK custom method and streaming primitives using real processes.
- Select and implement actual module/resource loading for Web and Wails, including a management recovery entrance.
- Close the existing runs.getAcceptance source-consumer gate with an actual reconciliation caller and appropriate contract evidence.
- Select the actual SDK-containing published Runtime version for independent application module metadata after the complete public surface exists. The isolated current-source archive proof does not claim published artifact availability.

## Spillover risks

- Public installation/protocol/storage changes break external compiled clients. Rebuild all in-repository consumers; report any externally used SDK/module contracts.
- Protocol 2026-10-13.1 removes approve/revoke and replaces installation state with required enabled. All external compiled Runtime clients must rebuild. No externally deployed clients were inventoried or modified in this isolated task.
- Existing incompatible user data is retained separately and refused; no conversion or deletion is authorized.
- Concurrent user work in main is outside this checkout. Final integration must revalidate the then-current shared branch.
- The active worktree is required work, not disposable scratch space. Remove only owned scratch artifacts between slices; retire the checkout after safe final delivery.

## Deliberately excluded scope

- Frontend UI, E2E and screenshot test execution or additions.
- Marketplace, untrusted-code sandbox, arbitrary workflow DSL, automatic compatibility, live provider migration and a second execution engine, as deferred by the proposal.
- Production operations, user-data deletion, destructive Git actions, credentialed services or publishing without the required authorization.

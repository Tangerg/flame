# Plugin refactor progress

## Current stage

Stage 1 complete: installation-owned selection and desired enablement.
Stage 2a complete: public backend wire binding and refusal/uncertainty semantics implemented, verified and committed.
Stage 2b complete: existing Memory/Schedules human commands retain durable original binding and reconcile through owner receipt lookup.
Stage 2c complete: Go-owned feature methods generate Schema, TypeScript data bindings and operation reference artifacts; Reviews consumes the same immutable contracts. Independent author and regeneration-drift checks passed; local commit/main synchronization is being recorded. The generic host operation entrance and full proposal remain open.

Working checkout: /Users/tangerg/.codex/worktrees/unified-plugins/flame.
Branch: codex/unified-plugins.
Starting commit: 5262aadc002d40d415212ead3b311005c1c6a62f.
Stage 1 commit: 869d4304.
Incoming local main: 61645c2561b174042aa157aef21cc00cfba67a86.
Integration commit: f5e4bdbc140ad4d24b6b723ffdfd908cdff515a3.
Stage 2a commit: c4706b6b.
Stage 2b commit: 1ea49486d7f3385e11fef7e395d43c901ca0b29d.
Stage 2b main synchronization: git merge main reported already up to date at 61645c2561b174042aa157aef21cc00cfba67a86.
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

- Added immutable command routing in the plugin Domain and SQLite: identity, semantic method, input digest, original installation/release/server and preparation/dispatch-admission/observation evidence. Observation preserves routing and never stores a host business outcome.
- Moved installation admission into the Application owner and injected the same boundary into command routing and execution. Quiescence/reclamation derive the union of unresolved commands, live execution and pending checkpoints; no refcount or competing held set was added.
- Migrated actual Memory add/update/review/delete, Schedule create/update/delete and manual preparation from current-provider mutation calls to original-bound dispatch. After dispatch admission, redelivery only queries the original backend's receipt. Missing/malformed/unreachable evidence remains unknown; no resend or provider retargeting occurs.
- Backends now atomically persist both known success and business refusal envelopes. Refusal rolls back mutation before saving its receipt in the same transaction; unknown storage/commit errors do not fabricate a receipt. Removed the former bare-result encoding and added non-mutating refusal coverage for that private schema.
- Published the receipt lookup/fingerprint/reply primitives in the existing public backend binding. Strict request admission, nullable/missing outcome separation and zero/empty success round trips are covered.
- Real-process regressions close and reopen the host database after response/request loss. They prove retained quiescence across restart, blocked release selection/configuration/uninstall, same-release disable/re-enable recovery, missing receipt without resubmission and old identity refusal after a replacement provider is enabled. Existing Schedules captured Run acceptance/replay remains intact.
- Shared admission preserves standard cancellation plus custom cause and refuses inactive/foreign execution assemblies. Deterministic concurrency coverage proves an installation change cannot pass a pending command-binding write.
- Removed speculative lifetime-count limits from both routing and private receipts before delivery. Retained history cannot silently evict an identity or permanently exhaust normal mutation usage after 4096 commands.

- Added public NewMethod[P,R] in the existing runtime/plugin package. Semantic method identity, query/command kind, target meaning and prose are declared once; Scope derives immutable structural schemas from the owner's existing Go types. Real client/server calls use those same compiled schemas.
- Required original input members are checked before business execution, rather than re-encoding Go zero values. Output schema violations remain protocol failures, preserving effect uncertainty. Method identity mode cannot be relabeled by its caller. Descriptor mutation cannot change the active method.
- Reviews now owns both method contracts in backend/contracts.go and binds its typed entrances from those contracts. Removed separate ListMethod/UpdateMethod name declarations; Tool descriptions and input schemas derive from the same owner. Status vocabulary is shared by validation and Scope schema inference; output outcome vocabulary remains typed.
- Extended the existing Runtime contractgen with a feature-input mode. It consumes public method descriptors and reuses Scope's standard Schema derivation plus the existing Schema-to-TypeScript emitter. Generated standard Schema preserves all numeric/cross-field keywords; TypeScript remains a structural data projection. Local definition references are rewritten without provider selection, external schema loading or a custom IDL.
- Generated Reviews schema.json, methods.generated.json, API_REFERENCE.md and TypeScript method input/output maps from go generate. Backend contract tests compare generated artifact admission with the actual method schemas. The TypeScript contract fixture rejects unknown status/outcome and a missing inspected revision; it runs no UI.
- Added the actual public plugin package to go-api.json generation and the external public-binding compile proof. No new Go module/public package or dependency was added. SDK construction, raw admission, descriptor independence, invalid output, generic reference, nullable/map projection, duplicate contract and source/output collision regressions pass.

## In progress

- Generic installed-operation admission, contribution declarations, plugin-defined points, target/origin/parent capture and provider selection still need the verified method/routing owners. Generated backend metadata is not an installed capability or an alternate registration authority.
- Public TypeScript installed-call/component bindings, callbacks and decorators remain open. The generated TypeScript artifacts supply concrete data maps, not a completed host client SDK.
- The confirmed pre-existing runs.getAcceptance frontend consumer-gate failure remains tracked for a real reconciliation caller; no dummy call or weaker source coverage was added.

## Next

1. Implement current consumer-owned contribution declarations and generic typed installed-operation admission, reusing the verified method schemas and original-binding/receipt owner. Include contract/target/origin/parent evidence, generic provider selection and independent Reviews receipt lookup; remove apiVersion:1 and closed business service routing with its consumers. Re-read proposal sections 6-12 and 17-18 plus the review's remaining API-freeze gaps before the next slice.
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
| 2026-10-13 | Dispatch admission is durably recorded before the network call; subsequent retries only read the original receipt | Transport loss or process death cannot prove whether the effect happened; the original binding survives availability/provider changes | Resending on absent receipt or selecting the current provider |
| 2026-10-13 | Store typed success and known refusal in the private receipt transaction; roll back refused mutation with a savepoint | A known refusal must replay the original decision while unknown commit/storage failures retain uncertainty | Bare success-only results or host business-result caching |
| 2026-10-13 | Keep historical command identities without an arbitrary lifetime-count cap or automatic eviction | Existing private receipts did not cap total historical usage; adding 4096 as a permanent product quota would break ordinary use, while eviction permits identity retargeting | Unmeasured permanent history quotas or expiration into a new provider call |
| 2026-10-13 | Use immutable Go-owned method contracts and Scope's existing schema derivation for authoring; reuse Runtime's TS emitter | Actual Reviews server, Go caller, Tool schemas and generated consumers need one structural owner | Parallel hand-maintained DTO/Schema/TS, a second SDK module or a custom IDL compiler |
| 2026-10-13 | Keep descriptor and generated files as projections; validate raw input and output at the bound method | Typed decoding can hide absent fields; artifact edits must not change a running method or create another registration path | Revalidating only a re-encoded DTO or classifying malformed output as a known refusal |
| 2026-10-13 | Keep target meaning declared by the contract; capture concrete target/origin/parent at generic host admission next | Current Reviews uses installation-private facts; target metadata alone is not captured invocation authority | Pretending backend descriptors already implement installed provider/context binding |


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
| No-Run unresolved human command pins original binding and recovery evidence | Existing Memory/Schedules boundary verified; generic/public operation application open | application/integration/plugins/commands.go; domain/integration/plugin/command.go; infra/sqlite/plugin_commands.go; bootstrap/plugin_command_recovery_test.go |
| One executable contract generation source | Go/Scope Schema/TypeScript data and method-doc generation verified with real Reviews; contribution/component binding generation still open | runtime/plugin/contract.go; cmd/contractgen/feature.go; examples/plugins/reviews/backend/contracts.go; generated contracts and contract tests |
| Typed business refusal versus transport uncertainty across entrances | Public Go/backend and Reviews typed/Tool semantics plus generated TS outcome vocabulary verified; generic host/CLI/TS call projections open | runtime/plugin/method.go; method_test.go; Memory/Schedules classifiers; Reviews server_test.go |
| Recovery entrance independent of failed optional UI | Open | startup/plugin management and CLI |
| Strict public protocol requirements without internal revision coupling | Open | public SDK handshake and contract tests |
| Explicit candidate update, withdrawal, quiescence and uninstall behavior | Stage 1 enablement/update and Stage 2b unresolved-command lifecycle verified; generic provider selection open | runtime/internal/domain/integration/plugin/installation.go; runtime/internal/application/integration/plugins/installations.go; owning tests |
| Reject old host encodings without data mutation | Installation/command-route host shapes and private bare-result receipt refusal verified; full public cutover open | runtime/internal/infra/sqlite/plugins_test.go; schema_admission_test.go; runtime/README.md |
| Preserve Memory ledger, watermark, curation and receipts | Open | public Memory capability and backend tests |
| One Schedules wake/dispatch lifetime and retained Run acceptance | Open | public background role and Schedules tests |
| Preserve package integrity, credentials, source identity and core authority checks | Open; baseline required | existing admission and policy regression checks |
| New acceptance coverage for unresolved mutation/update/provider replacement | Existing real commands verified, including database reopening and distinct installation replacement; generic/UI coverage open | bootstrap/managed_services_test.go; plugin_command_recovery_test.go; application/integration/plugins/commands_test.go |
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

Stage 2c verification:

- Passed: complete Runtime and CLI go test ./..., go vet ./... and go build ./..., including architecture/dependency gates and real installed Reviews backend lifecycle tests.
- Passed: independent Reviews complete go test -race ./..., go vet ./... and go build ./...; generated artifact admission is checked against the immutable backend contracts.
- Passed: public SDK and contractgen full targeted -race suites, including raw request rejection before business execution and invalid-output uncertainty. Later generator safety/reference tests were also checked with -race and vet.
- Passed: Runtime go generate ./... and Reviews go generate ./.... A second generation preserved SHA-256 for the tracked Runtime catalog/Schema/OpenRPC/reference/API inventory and all formal Reviews artifacts.
- Passed: shared Runtime TypeScript npm run check, including typecheck, lint, format, all 284 contract tests and knip. Reviews client/contract-check.ts compiled with strict tsc; its negative assertions preserve closed status/outcome and required revision. These checks execute no frontend UI.
- Passed: an isolated independent Reviews author with its own generated package contracts built with GOWORK=off, no replace and a temporary SDK-only module archive, no Runtime internal source, offline module proxy and isolated module cache. Tidy/test/vet/build passed, including the generated-file regression. This fixture is not a published Runtime version.
- Not run: graphical UI/E2E/screenshot tests, explicitly excluded. The composite frontend gate remains unrun; its previously confirmed runs.getAcceptance consumer gap remains open. No SDK release was published or claimed available in the old dependency version.


Stage 2b verification:

- Passed: Runtime and CLI complete go test ./..., go vet ./... and go build ./..., including architecture/dependency gates. Independent Reviews complete go test -race ./..., go vet ./... and go build ./... passed.
- Passed: public SDK outside the workspace with GOWORK=off GOPROXY=off GOSUMDB=off go test ./plugin.
- Passed: focused public receipt/strict request/empty-success, atomic success/refusal rollback, immutable route/phase/history and former host/private schema tests. Real-process response-loss, absent-receipt and replacement-provider tests passed with actual host SQLite close/reopen.
- Passed: targeted -race for SDK receipt, Application commands/shared admission, private receipts, SQLite routing/schema admission, execution dependency/assembly boundaries and real Memory recovery/replacement plus Schedule manual replay. Schedule create/dispatch-loss races also passed.
- Resolved: private envelope encoding initially omitted empty Ack/zero values under JSON v2 omitempty. The shared Result uses omitzero and a zero-value regression; the formerly failing CLI lifecycle now passes. Removing speculative capacity left one Memory classifier reference; it was removed and full checks rerun successfully.
- Initial Runtime default-initialization check exceeded its existing 10-second availability deadline during the earlier full run. The unchanged check passed standalone and in the final full suite; no timeout/expectation was weakened and no unproven cause is claimed.
- Not run: frontend UI/E2E/screenshot checks, as explicitly excluded; frontend static/build checks were not repeated because this slice changes only Go/backend behavior and owning documentation. Runtime protocol catalog did not change, so go generate was unnecessary. The pre-existing frontend consumer gate and independent published SDK version remain open.

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

- Stage 2c: all invoked verification processes returned terminal success. TemporaryDirectory removed the task-owned independent SDK proxy/checkouts/module cache. Formal generated Reviews contracts/data bindings remain as product artifacts. Task-generated Runtime TypeScript test cache was removed; active checkout/development dependencies remain for the continuing refactor.

- Stage 2b: all invoked verification processes returned terminal success after repairs. Test-owned temporary databases/packages and helper processes were closed/removed by their owning test cleanup. No task scratch script or verification log was saved. No UI build output, shared configuration, remote publication or user-data operation was introduced. The slice was committed, incoming local main was already up to date, and the active checkout and necessary dependencies remain for the full refactor.

- Stage 2a: the temporary SDK file proxy, independent author checkout and isolated module cache were removed by their owning temporary-directory lifetime. Backend/unit/integration checks have all returned; no task check process is running. No frontend build output was generated in this slice. The worktree is committed and clean after main synchronization; the active checkout and necessary development dependencies remain for Stage 2b.
- Removed task-generated frontend/Icons/UI dist output, generated Icons/UI build sources and Runtime contract Vitest cache. Removed the task's offline-install debug log if present. All invoked checks have returned terminal success/failure; no task check process remains running.
- Kept the active managed checkout, formal code/tests/docs and isolated development dependencies for the continuing refactor. Shared product source and Git configuration were not edited. Before the next frontend verification, rebuild Icons and UI to restore their generated prerequisites.
- No remote push, publication, main-history mutation, production operation or credentialed action was performed.

## Unresolved questions

- Extend the verified durable original-binding/retention owner to generic operation descriptors and public entrances before opening new plugin mutation. Current closed service binding is not the final contribution substrate.
- Finish generic call/error projections and public TypeScript client/component bindings through Reviews; current Go-owned generation supplies structural contracts and preserves typed outcome vocabulary.
- Verify the selected MCP/SDK custom method and streaming primitives using real processes.
- Select and implement actual module/resource loading for Web and Wails, including a management recovery entrance.
- Close the existing runs.getAcceptance source-consumer gate with an actual reconciliation caller and appropriate contract evidence.
- Select the actual SDK-containing published Runtime version for independent application module metadata after the complete public surface exists. The isolated current-source archive proof does not claim published artifact availability.

## Spillover risks

- New schema-bound methods tighten feature input/output admission. Params and Result now retain unexported original wire evidence, so external unkeyed literals require migration; keyed usage and real consumers are updated. Separate Reviews ListMethod/UpdateMethod exports were removed without aliases.
- Public installation/protocol/storage changes break external compiled clients. Rebuild all in-repository consumers; report any externally used SDK/module contracts.
- Protocol 2026-10-13.1 removes approve/revoke and replaces installation state with required enabled. All external compiled Runtime clients must rebuild. No externally deployed clients were inventoried or modified in this isolated task.
- Existing incompatible user data is retained separately and refused; no conversion or deletion is authorized. Stage 2b adds plugin_commands to the exact host schema and replaces private Memory/Schedules commands.result with a value/refusal reply envelope. Existing directories must remain untouched and be replaced with fresh directories for this pre-release cutover.
- Concurrent user work in main is outside this checkout. Final integration must revalidate the then-current shared branch.
- The active worktree is required work, not disposable scratch space. Remove only owned scratch artifacts between slices; retire the checkout after safe final delivery.

## Deliberately excluded scope

- Frontend UI, E2E and screenshot test execution or additions.
- Marketplace, untrusted-code sandbox, arbitrary workflow DSL, automatic compatibility, live provider migration and a second execution engine, as deferred by the proposal.
- Production operations, user-data deletion, destructive Git actions, credentialed services or publishing without the required authorization.

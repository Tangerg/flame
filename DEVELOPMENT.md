# Flame development workflow

This document owns repository execution details. [`AGENTS.md`](AGENTS.md) owns stable rules, [`DESIGN_PHILOSOPHY.md`](DESIGN_PHILOSOPHY.md) owns design rationale, and [`REFACTORING.md`](REFACTORING.md) owns the structural refactoring method.

## Required reading

Before changing architecture, read the root instructions, design philosophy, refactoring guide, this workflow, and the nearest module instructions. Follow the module architecture index for the affected contract; do not preload unrelated historical material.

| Change | Additional source of truth |
| --- | --- |
| CLI behavior or structure | `cli/ARCHITECTURE.md` |
| Runtime behavior or structure | `runtime/doc/README.md` and its ordered baseline |
| Runtime Domain behavior | `runtime/doc/DOMAIN_MODEL.md` |
| Protocol or binding | `runtime/doc/API.md`, `runtime/doc/TRANSPORT.md`, and `runtime/contract/` |
| Local deployment handoff | `runtime/localruntime/README.md` and its architecture document if present |

## Active work boundary

- Change only `cli`, `runtime`, `runtime/localruntime`, and repository documents required by those modules.
- Treat all `desktop` changes as user-owned concurrent work. Do not edit, format, stage, revert, generate into, or include them in a commit.
- The current Runtime and CLI simplification is authorized to make breaking changes. Do not leave a compatibility package or parallel call path after migrating in-scope consumers.
- If scheduled build-cache cleanup removes artifacts, rebuild them and continue; do not investigate the cleanup as a product failure.

## Evidence and references

Start structural work from production callers, dynamic entrypoints, persistence, protocol registration, generated artifacts, and lifecycle ownership. Directory counts and static-tool output identify candidates but do not justify a change.

Reference repositories supply evidence, not dependencies or layouts to copy:

| Reference | Relevant evidence |
| --- | --- |
| `/Users/tangerg/Desktop/scope` | public Go APIs, rich models, package discipline, released framework/provider contracts |
| `/Users/tangerg/Desktop/study/codex-server` | protocol lifecycle, interruption, steering, compaction, and recovery mechanisms |
| `/Users/tangerg/Desktop/grok-build` | CLI/TUI interaction density, composer behavior, Goal/Plan presentation, and terminal details |
| `/Users/tangerg/Desktop/opencode` | provider discovery, exact model identity, credential precedence, endpoints, and capability projection |

Record what evidence a reference provides and which Flame invariant owns the resulting decision. Do not import multi-client, cloud, daemon, plugin, or compatibility architecture without a Flame requirement.

## Change workflow

1. Inspect `git status` and identify unrelated work before every batch.
2. Trace the real entrypoint through composition, use case, domain owner, adapter, persistence, and projection.
3. Search callers, serialized names, configuration keys, registries, generated artifacts, tests, and documentation in the blast radius.
4. Run the narrowest useful baseline. Record a pre-existing failure instead of attributing it to the change.
5. Repair one ownership boundary and delete the former contract end to end.
6. Search again for retired symbols and spellings.
7. Run the narrow decisive check, then proportionate module and workspace verification.
8. Inspect the complete diff, stage explicit paths, and verify no `desktop` path is staged.
9. Commit one independently revertible ownership change and push it. Never force-push or rewrite user commits.

## Verification

Choose checks by risk rather than ritual:

| Change | Required evidence |
| --- | --- |
| Domain invariant | owner tests plus caller-visible rejection |
| Application transaction | exact durable winner and failure ordering |
| Storage shape | strict round trip, malformed-state rejection, and schema baseline |
| Protocol or binding | catalog, generation, strict validation, and cross-binding parity |
| CLI routing | fresh-root in-memory command tests and captured output |
| TUI input or rendering | deterministic application/render test; real PTY for terminal contracts |
| Goroutine, stream, cancellation, or shutdown | deterministic lifecycle test and targeted race run |
| Package or interface removal | surviving consumer tests and a dependency-direction guard |

For affected Go modules, run `go test`, `go vet`, `go build`, `go mod tidy`, generation, architecture checks, and `git diff --check` in proportion to the change. Use `GOWORK=off` for standalone module verification after dependency or module-boundary changes. Do not add race, fuzz, multi-client, multi-server, or cross-platform matrices unless the changed owner requires them.

## Live provider verification

`/Users/tangerg/Desktop/flame/runtime/config/config.yaml` is authorized for bounded live DeepSeek verification. Load it only through production configuration code. Never print or copy its credential into logs, fixtures, snapshots, diffs, documentation, environment dumps, or command output.

Keep live scenarios explicit and bounded. Record model identity, scenario, outcome, and duration without recording secrets or full private prompts. The ordinary offline suite must remain deterministic and credential-independent.

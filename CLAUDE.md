# CLAUDE.md — Flame monorepo context

Flame is a Wails v3 desktop agent application backed by the Flame Runtime Protocol.
This repository is the canonical owner of the application and contains four Go
modules joined by the root `go.work`:

- `runtime`: application backend, protocol, domain model, persistence, and executable;
- `runtime/localruntime`: strict local deployment handoff shared by Runtime and Desktop;
- `desktop`: Wails shell and React/TypeScript client.
- `cli`: Cobra command surface and oolong terminal client over the embedded Runtime.

The modules consume released `github.com/Tangerg/scope/...` libraries as external
dependencies. Flame-owned code must use `github.com/Tangerg/flame/...` module paths
and the `Flame`/`flame`/`FLAME` product vocabulary. Do not add compatibility aliases,
dual paths, or former product names.

Preserve the existing domain-driven and clean dependency direction. Domain models own
their invariants, application packages own use-case ordering and transactions, adapters
translate external semantics, delivery owns protocol projection, and bootstrap remains
the only composition root. Wails services stay thin and must not absorb Runtime logic.

Before changing a module, read its local `CLAUDE.md`. Keep Wails Go, CLI, and frontend
runtime versions exact and aligned. Validate changed Go modules with test, vet, and build;
validate the frontend with `npm run check`; run the production Wails task for desktop
release changes. Commit only generated contracts that match their canonical generators.

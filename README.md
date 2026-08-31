# Flame

Flame is a native desktop agent application built with Wails v3, React, TypeScript,
and a Go runtime. Its Runtime Protocol models durable sessions, runs, goals, plans,
interrupts, recovery, and threshold-gated context compaction as one coherent product
lifecycle.

Repository rules live in [`AGENTS.md`](AGENTS.md). [`DESIGN_PHILOSOPHY.md`](DESIGN_PHILOSOPHY.md) explains the shared Runtime and CLI design, [`REFACTORING.md`](REFACTORING.md) defines the structural method, and [`DEVELOPMENT.md`](DEVELOPMENT.md) owns execution and verification details.

## Repository layout

| Path | Responsibility |
| --- | --- |
| `runtime/` | Domain model, application use cases, adapters, SQLite persistence, protocol, and Runtime executable |
| `runtime/localruntime/` | Strict local Runtime-to-Desktop deployment handoff |
| `desktop/` | Wails v3 shell and React/TypeScript desktop client |
| `cli/` | Cobra command surface, one-shot client, and interactive oolong TUI |

The root `go.work` is the single local workspace for all four Go modules. Flame-owned
module paths live under `github.com/Tangerg/flame`; reusable agent and provider libraries
remain versioned dependencies from `github.com/Tangerg/scope`.

## Development

Requirements: Go 1.27, Node.js 22.12 or newer, npm, and Wails CLI
`v3.0.0-beta.15`.

```sh
cd desktop/frontend
npm ci
npm run check

cd ..
wails3 dev
```

Run the Go verification matrix from the repository root:

```sh
go test ./runtime/... ./runtime/localruntime/... ./desktop ./cli/...
go vet ./runtime/... ./runtime/localruntime/... ./desktop ./cli/...
go build ./runtime/... ./runtime/localruntime/... ./desktop ./cli/...
```

Build the native production application from `desktop/` with:

```sh
wails3 task build
```

Runtime architecture and protocol authority live in `runtime/doc/`; frontend
architecture and visual rules live in `desktop/frontend/`.

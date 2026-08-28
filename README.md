# Flame

Flame is a native desktop agent application built with Wails v3, React, TypeScript,
and a Go runtime. Its Runtime Protocol models durable sessions, runs, goals, plans,
interrupts, recovery, and threshold-gated context compaction as one coherent product
lifecycle.

## Repository layout

| Path | Responsibility |
| --- | --- |
| `runtime/` | Domain model, application use cases, adapters, SQLite persistence, protocol, and Runtime executable |
| `runtime/localruntime/` | Strict local Runtime-to-Desktop deployment handoff |
| `desktop/` | Wails v3 shell and React/TypeScript desktop client |

The root `go.work` is the single local workspace for all three Go modules. Flame-owned
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
go test ./runtime/... ./runtime/localruntime/... ./desktop
go vet ./runtime/... ./runtime/localruntime/... ./desktop
go build ./runtime/... ./runtime/localruntime/... ./desktop
```

Build the native production application from `desktop/` with:

```sh
wails3 task build
```

Runtime architecture and protocol authority live in `runtime/doc/`; frontend
architecture and visual rules live in `desktop/frontend/`.

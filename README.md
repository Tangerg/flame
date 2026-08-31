# Flame

Flame is a local agent product with a Go Runtime, a Cobra/Oolong CLI, and a Wails desktop client.

Runtime owns durable product semantics and exposes the same behavior through an in-process Go binding and the Runtime Protocol. CLI and Desktop are consumers; Scope supplies the agent framework and provider libraries.

## Repository

| Path | Responsibility |
| --- | --- |
| `runtime/` | Domain model, use cases, execution adapters, persistence, protocol, and Go binding |
| `runtime/localruntime/` | Strict local Runtime credential handoff |
| `cli/` | Command routing, one-shot output, terminal interaction, and CLI-local authoring state |
| `desktop/` | Wails host and graphical presentation |

Read [`AGENTS.md`](AGENTS.md) before changing the repository. Current design and workflow are documented in [`DESIGN_PHILOSOPHY.md`](DESIGN_PHILOSOPHY.md), [`REFACTORING.md`](REFACTORING.md), and [`DEVELOPMENT.md`](DEVELOPMENT.md).

## Development

The root `go.work` contains the Runtime, local Runtime handoff, CLI, and Desktop Go modules. The repository currently targets Go 1.27.

```sh
go test ./runtime/... ./runtime/localruntime/... ./cli/...
go vet ./runtime/... ./runtime/localruntime/... ./cli/...
go build ./runtime/... ./runtime/localruntime/... ./cli/...
```

Desktop frontend commands and Wails build instructions live under `desktop/`; Desktop is outside the current Runtime and CLI refactoring boundary.

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

Each module's own `README.md` carries its boundaries and module instructions.

## Documents

Read [`AGENTS.md`](AGENTS.md) before changing the repository. Each document below has one job, so a reader
knows where to look and a writer knows where to add.

| Document | Holds |
| --- | --- |
| [`AGENTS.md`](AGENTS.md) | Engineering conventions that apply to every change |
| [`PROJECT_RULES.md`](PROJECT_RULES.md) | Rules that apply only to this repository |
| [`DESIGN_PHILOSOPHY.md`](DESIGN_PHILOSOPHY.md) | Why Flame is shaped the way it is |
| [`REFACTORING.md`](REFACTORING.md) | How to make a structural change and prove it |
| [`DEVELOPMENT.md`](DEVELOPMENT.md) | Active scope, workflow, and verification commands |
| [`runtime/doc/ARCHITECTURE.md`](runtime/doc/ARCHITECTURE.md) | Current Runtime boundaries |
| [`docs/`](docs/) | Comparisons against reference runtimes, and the Scope adoption record |

## Development

The root `go.work` contains the Runtime, local Runtime handoff, CLI, and Desktop Go modules. The repository currently targets Go 1.27.

```sh
go test ./runtime/... ./runtime/localruntime/... ./cli/...
go vet ./runtime/... ./runtime/localruntime/... ./cli/...
go build ./runtime/... ./runtime/localruntime/... ./cli/...
```

Desktop frontend commands and Wails build instructions live under `desktop/`; Desktop is outside the current Runtime and CLI refactoring boundary.

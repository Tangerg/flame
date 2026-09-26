# Flame

Flame is an agent product with a Go Runtime, a Cobra/Oolong CLI, a shared Web/Wails application, and a VS Code extension.

Runtime owns durable product semantics and exposes the same behavior through an in-process Go binding and the Runtime Protocol. Clients own interaction and presentation; Scope supplies the agent framework and provider libraries.

## Repository

| Path | Responsibility |
| --- | --- |
| `runtime/` | Domain model, use cases, execution adapters, persistence, protocol, and local/remote Go bindings |
| `runtime/contract/typescript/` | Generated contract and the shared TypeScript HTTP client |
| `runtime/localruntime/` | Strict local Runtime credential handoff |
| `cli/` | Command routing, one-shot output, terminal interaction, and CLI-local authoring state |
| `desktop/` | Shared browser/desktop presentation and the Wails host |
| `ide/` | Native VS Code interaction over the shared Runtime client |

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

Graphical-client commands and Wails build instructions live under [`desktop/`](desktop/README.md). IDE commands and packaging live under [`ide/`](ide/README.md).

## Share a Runtime

Start one standalone Runtime with its own provider configuration and data directory. Every attached client names its base URL, such as `http://127.0.0.1:17171`; the shared clients append the generated `/v2/rpc` path. Runtime owns execution, credentials for providers, and filesystem paths. A remote client's environment does not reconfigure the running Runtime.

```sh
go run ./cli --runtime-url http://127.0.0.1:17171
go run ./cli --runtime-url http://127.0.0.1:17171 run --workspace /server/project "Review this project"
```

Supply the existing Runtime bearer token through `FLAME_RUNTIME_TOKEN` for CLI, the connection dialog for Web, or the IDE connection command. The token never belongs in the URL. A CLI with no configured endpoint owns an embedded Runtime. An explicit remote connection failure never starts a local substitute.

Closing an attached view releases that client's transport and projections. Explicit cancellation remains a Runtime command; one-shot CLI interruption cancels the Run it is driving. Only the process or host that opened a Runtime owns its shutdown. Multiple processes using the same database are not substitutes for attaching to the same live Runtime.

To serve the graphical application from the Runtime's origin, build `desktop/frontend`, then set `server.webDirectory` (or `FLAME_SERVER_WEBDIRECTORY`) to the absolute path of its `dist` directory. Static assets are public; `/v2/rpc` retains its token gate. Browser clients choose paths on the Runtime host instead of treating a local file picker as a remote workspace.

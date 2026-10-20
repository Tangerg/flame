# Flame

<p align="center"><img src="assets/brand/flame-mark.png" alt="Flame" width="144" /></p>

Flame is an agent product with a Go Runtime, a terminal CLI, and one React application for Browser and Wails. Runtime owns durable Session, Run, Goal, Plan, execution, recovery and provider semantics. Clients own interaction and presentation. Scope supplies the agent framework and provider libraries.

## Repository

| Path                                  | Owner                                                                     |
| ------------------------------------- | ------------------------------------------------------------------------- |
| `runtime/`                            | Semantic core, protocol, server, local Go binding and contract generators |
| `apps/cli/`                           | Scriptable `flame` command and interactive terminal                       |
| `apps/web/`                           | Shared Browser/Wails product UI                                           |
| `apps/desktop/`                       | Native Wails host and macOS packaging                                     |
| `packages/runtime-client/go/`         | Remote Go HTTP/SSE client                                                 |
| `packages/runtime-client/typescript/` | `@flame/runtime-client`, generated wire projection and client             |
| `packages/plugin-sdk/`                | Go backend authoring API                                                  |
| `packages/component-sdk/`             | `@flame/component-sdk`, installed React component authoring API           |
| `packages/local-runtime/`             | Process roots, token handoff and companion-distribution lookup            |
| `packages/ui/`, `packages/icons/`     | Shared controls/tokens and authored optical icon masters                  |
| `plugins/`                            | First-party backend/component/package sources and plugin build tools      |
| `examples/plugins/`                   | Independent authoring and component-composition consumers                 |
| `assets/brand/`                       | Original brand artwork and guide                                          |
| `tooling/`, `tests/integration/`      | Workspace coordination and public product integration                     |

## Development

Read [AGENTS.md](AGENTS.md) and [PROJECT_RULES.md](PROJECT_RULES.md). [DEVELOPMENT.md](DEVELOPMENT.md) owns the toolchain and command workflow. From this directory:

```sh
make setup
make generate
make check
make check-native
make check-author
```

Use `make dev-web` or `make dev-desktop` for the same Web product. `make build-runtime`, `make build-cli` and `make package-desktop` create deliveries under `out/`. Keep each executable with its companion `plugins/` directory. Browser output is `apps/web/dist`; native embed staging is generated in `apps/desktop/assets/web`.

The root Go module contains Runtime, CLI, backend SDKs and first-party plugins. `go.work` additionally includes Desktop, local-runtime and the independent Reviews backend. Root npm workspaces use one lock and one install. No former module/package names or source-layout aliases are provided.

## Connect to a Runtime

CLI without an endpoint opens an embedded Runtime. Explicit remote connections use the base URL and `FLAME_RUNTIME_TOKEN`; a failed remote connection remains an error.

```sh
./out/cli/darwin-arm64/flame --runtime-url http://127.0.0.1:17171
./out/cli/darwin-arm64/flame --runtime-url http://127.0.0.1:17171 run --workspace /server/project "Review this project"
```

Runtime appends and serves its generated `/v2/rpc` route. Provider configuration and filesystem paths belong to the Runtime host. Closing a client releases its transport and views; explicit cancellation is a Runtime command. Only the host that opened a Runtime owns shutdown.

To serve Web from that Runtime origin, build Web and set `server.webDirectory` (or `FLAME_SERVER_WEBDIRECTORY`) to the absolute `apps/web/dist` path. Static assets are public; RPC retains its token gate.

## Documentation

- [Workspace ownership and dependency direction](docs/architecture/workspace.md)
- [Public consumers and source-distribution rules](docs/reference/consumers.md)
- [Runtime architecture](runtime/docs/ARCHITECTURE.md)
- [CLI architecture](apps/cli/ARCHITECTURE.md)
- [Web architecture](apps/web/ARCHITECTURE.md)
- [Design rationale](DESIGN_PHILOSOPHY.md) and [structural-change method](REFACTORING.md)
- [Brand guide](assets/brand/README.md)

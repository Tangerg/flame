# Flame

<p align="center"><img src="assets/brand/flame-mark.png" alt="Flame" width="144" /></p>

Flame is an agent product with a Go Runtime, a terminal CLI, and one React application for Browser and Wails. Runtime owns durable Session, Run, Goal, Plan, execution, recovery and provider semantics. Clients own interaction and presentation. Scope supplies the agent framework and provider libraries.

## Repository

| Path                               | Owner                                                                          |
| ---------------------------------- | ------------------------------------------------------------------------------ |
| `qimu/`                            | Independent heterogeneous plugin framework, Go Host, drivers, storage and SDKs |
| `runtime/`                         | Semantic core, HTTP delivery, local Go binding and Host contract generator     |
| `apps/cli/`                        | Scriptable `flame` command and interactive terminal                            |
| `apps/web/`                        | Shared Browser/Wails product UI                                                |
| `apps/desktop/`                    | Native Wails host and macOS packaging                                          |
| `packages/go-sdk/`                 | Flame Go contracts, remote client and product-specific plugin author APIs      |
| `packages/runtime-client/`         | `@flame/runtime-client`, generated wire projection and client                  |
| `packages/backend-sdk/`            | `@flame/backend-sdk`, TypeScript backend authoring API                         |
| `packages/component-sdk/`          | `@flame/component-sdk`, installed React component authoring API                |
| `packages/local-runtime/`          | Process roots, token handoff and companion-distribution lookup                 |
| `packages/ui/`, `packages/icons/`  | Shared controls/tokens and authored optical icon masters                       |
| `plugins/`                         | First-party backend/component/package sources and plugin build tools           |
| `examples/plugins/`                | Independent authoring and component-composition consumers                      |
| `assets/brand/`                    | Original brand artwork and guide                                               |
| `design/branding/`                 | Brand mark exploration history; shipped artwork lives in `assets/brand/`       |
| `tooling/`, `tooling/integration/` | Workspace coordination and public product integration                          |

## Development

Read [AGENTS.md](AGENTS.md) and [PROJECT_RULES.md](PROJECT_RULES.md). [DEVELOPMENT.md](DEVELOPMENT.md) owns the toolchain and command workflow. From this directory:

```sh
make setup
make generate
make check
make check-native
make check-author
```

Use `make dev-web` or `make dev-desktop` for the same Web product. `make build-runtime`, `make build-cli` and `make package-desktop` create deliveries under `out/`. A Runtime-hosting distribution includes its sibling `qimu-runner`, `plugins/` directory and pinned `runtimes/node/<os>-<arch>/` companion. Ship these together; managed plugins do not search for a system Node executable. Desktop attaches to a Runtime rather than hosting plugin processes. Browser output is `apps/web/dist`; native embed staging is generated in `apps/desktop/assets/web`.

The repository root owns `go.work` and has no `go.mod`. Qimu, Go SDK, local-runtime, Runtime, CLI, Desktop, first-party plugins, tooling and the independent Reviews backend own their Go dependency graphs. Root npm workspaces use one lock and one install. See [workspace ownership](docs/architecture/workspace.md) for module boundaries.

The Qimu integration is a breaking storage cutover. Start the new distribution
with a fresh product root; `localruntime.DataDirectory` selects `flame-qimu.db`
for Qimu framework records and Flame business references in one SQLite database.
Retain the previous binary and data together for unresolved work. No historical
plugin-state reader or automatic conversion is supplied.

## Connect to a Runtime

CLI defaults to a remote Runtime at `http://127.0.0.1:17171`. Select `--runtime-mode embedded` to host a Runtime in the CLI process. Remote connections use the base URL and `FLAME_RUNTIME_TOKEN`; a failed remote connection remains an error.

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

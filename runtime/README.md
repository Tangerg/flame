# Flame Runtime

Flame Runtime is the local product backend for Flame. It owns durable agent semantics and exposes the same behavior through an in-process Go binding and the Runtime Protocol.

Runtime is not another agent framework. Scope owns process execution, strategies, tools, and provider libraries; Runtime adapts those capabilities to Flame's Session, Run, Goal, Plan, persistence, recovery, and protocol model.

## Public surfaces

- The module-root `runtime.Runtime` is the concrete in-process binding.
- `protocol` contains binding-neutral requests, responses, events, errors, and validation.
- `contract` contains generated machine-readable protocol artifacts and the generated API reference.
- `localruntime` contains the strict local credential-file handoff used by Runtime hosts.

All Runtime operations enter one delivery endpoint. The Go binding avoids JSON and HTTP encoding but uses the same admission, capability, idempotency, Application, error, and event semantics as the HTTP binding.

## Open an in-process Runtime

```go
rt, err := runtime.Open(ctx, runtime.Config{
	DataDirectory:        dataDirectory,
	DefaultWorkspacePath: workspace,
})
if err != nil {
	return err
}
defer rt.Close()

session, err := rt.CreateSession(ctx, protocol.CreateSessionRequest{
	Workspace: &protocol.WorkspaceRef{Path: workspace},
}, runtime.CommandOptions{IdempotencyKey: requestID + ":session"})
if err != nil {
	return err
}
```

Hosts must close each Runtime they open. Protocol errors support `errors.Is` against public sentinel errors and `errors.As` to `protocol.ProblemError` for structured recovery information.

## Develop

```sh
GOWORK=off go test ./...
GOWORK=off go vet ./...
GOWORK=off go build ./...
go generate ./...
```

The default suite is offline. Module rules live in [`AGENTS.md`](AGENTS.md); current boundaries live in [`doc/ARCHITECTURE.md`](doc/ARCHITECTURE.md).

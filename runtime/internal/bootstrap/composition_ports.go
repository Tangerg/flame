package bootstrap

// TerminalResource is a process-owned adapter whose Close call is one-shot:
// once Close returns, the resource has reached its final state even when it
// reports a diagnostic. Instance bounds and joins the call itself, so adapters do
// not need a second timeout or retry layer.
type TerminalResource interface {
	Close() error
}

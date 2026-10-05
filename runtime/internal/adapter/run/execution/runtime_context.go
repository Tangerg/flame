package execution

// RuntimeContextKind names one kind of context the Runtime itself authors
// into a model request. The model needs one stable framing to tell such
// context from user input; it is Flame's own and borrows no other product's
// control vocabulary.
type RuntimeContextKind string

const (
	RuntimeContextDeferredTools  RuntimeContextKind = "deferred-tools"
	RuntimeContextRecalledMemory RuntimeContextKind = "recalled-memory"
	RuntimeContextRetainedShells RuntimeContextKind = "retained-shells"
	RuntimeContextSessionGoal    RuntimeContextKind = "session-goal"
	RuntimeContextSessionPlan    RuntimeContextKind = "session-plan"
)

// RuntimeContextOpening is the line that opens one kind of Runtime-authored
// context, so a reader can recognize that kind without parsing its body.
func RuntimeContextOpening(kind RuntimeContextKind) string {
	return `<flame-context kind="` + string(kind) + `">`
}

// FrameRuntimeContext wraps body in the single framing for Runtime-authored
// context.
func FrameRuntimeContext(kind RuntimeContextKind, body string) string {
	return RuntimeContextOpening(kind) + "\n" + body + "\n</flame-context>"
}

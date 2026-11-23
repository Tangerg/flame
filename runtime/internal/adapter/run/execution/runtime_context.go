package execution

type RuntimeContextKind string

const (
	RuntimeContextDeferredTools  RuntimeContextKind = "deferred-tools"
	runtimeContextRecalledMemory RuntimeContextKind = "recalled-memory"
	RuntimeContextRetainedShells RuntimeContextKind = "retained-shells"
	RuntimeContextSessionGoal    RuntimeContextKind = "session-goal"
	RuntimeContextSessionPlan    RuntimeContextKind = "session-plan"
)

func RuntimeContextOpening(kind RuntimeContextKind) string {
	return `<flame-context kind="` + string(kind) + `">`
}

func FrameRuntimeContext(kind RuntimeContextKind, body string) string {
	return RuntimeContextOpening(kind) + "\n" + body + "\n</flame-context>"
}

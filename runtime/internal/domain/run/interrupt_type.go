package run

// Spelled once beside the state it explains so the frozen capability set can be
// handed straight to the executor; executors may not infer a kind from prompt
// fields.
type InterruptType string

const (
	InterruptApproval InterruptType = "approval"
	InterruptQuestion InterruptType = "question"
)

func (k InterruptType) Valid() bool {
	return k == InterruptApproval || k == InterruptQuestion
}

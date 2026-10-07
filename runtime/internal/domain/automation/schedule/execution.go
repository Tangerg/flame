package schedule

import (
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

// Execution is the immutable instruction set captured by a firing. It is not a
// partial Schedule: lifecycle timestamps, cursor, and revision deliberately do
// not exist on this value.
type Execution struct {
	title          string
	instructions   string
	cwd            string
	modelSelection modelref.Selection
}

// ExecutionSnapshot is the persistence representation of [Execution].
type ExecutionSnapshot struct {
	Title          string
	Instructions   string
	CWD            string
	ModelSelection modelref.Selection
}

// Execution returns the immutable instructions a manual or cron firing runs.
func (s Schedule) Execution() Execution {
	return Execution{
		title: s.title, instructions: s.instructions, cwd: s.cwd,
		modelSelection: s.modelSelection,
	}
}

// RestoreExecution reconstructs a durable firing snapshot without pretending
// it is a complete Schedule aggregate.
func RestoreExecution(snapshot ExecutionSnapshot) (Execution, error) {
	value := Execution{
		title: snapshot.Title, instructions: snapshot.Instructions, cwd: snapshot.CWD,
		modelSelection: snapshot.ModelSelection,
	}
	if err := value.Validate(); err != nil {
		return Execution{}, err
	}
	return value, nil
}

// Validate checks the complete captured execution value. The trigger is not
// part of it: a firing already exists, so the cron that produced it has no
// further say over what runs.
func (e Execution) Validate() error {
	return validateInstructions(e.instructions)
}

// Snapshot returns the complete persistence representation.
func (e Execution) Snapshot() ExecutionSnapshot {
	return ExecutionSnapshot{
		Title: e.title, Instructions: e.instructions, CWD: e.cwd,
		ModelSelection: e.modelSelection,
	}
}

func (e Execution) Title() string                      { return e.title }
func (e Execution) Instructions() string               { return e.instructions }
func (e Execution) CWD() string                        { return e.cwd }
func (e Execution) ModelSelection() modelref.Selection { return e.modelSelection }

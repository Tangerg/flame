package schedule

import (
	"fmt"

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
	cron           string
}

// ExecutionSnapshot is the persistence representation of [Execution].
type ExecutionSnapshot struct {
	Title          string
	Instructions   string
	CWD            string
	ModelSelection modelref.Selection
	Cron           string
}

// Execution returns the immutable instructions a manual or cron firing runs.
func (s Schedule) Execution() Execution {
	return Execution{
		title: s.title, instructions: s.instructions, cwd: s.cwd,
		modelSelection: s.modelSelection, cron: s.cron,
	}
}

// RestoreExecution reconstructs a durable firing snapshot without pretending
// it is a complete Schedule aggregate.
func RestoreExecution(snapshot ExecutionSnapshot) (Execution, error) {
	value := Execution{
		title: snapshot.Title, instructions: snapshot.Instructions, cwd: snapshot.CWD,
		modelSelection: snapshot.ModelSelection, cron: snapshot.Cron,
	}
	if err := value.Validate(); err != nil {
		return Execution{}, err
	}
	return value, nil
}

// Validate checks the complete captured execution value.
func (e Execution) Validate() error {
	if err := e.modelSelection.Validate(); err != nil {
		return fmt.Errorf("schedule: execution model selection: %w", err)
	}
	if e.instructions == "" {
		return ErrInstructionsRequired
	}
	if e.cron == "" {
		return ErrCronRequired
	}
	return ValidateCron(e.cron)
}

// Snapshot returns the complete persistence representation.
func (e Execution) Snapshot() ExecutionSnapshot {
	return ExecutionSnapshot{
		Title: e.title, Instructions: e.instructions, CWD: e.cwd,
		ModelSelection: e.modelSelection, Cron: e.cron,
	}
}

func (e Execution) Title() string                      { return e.title }
func (e Execution) Instructions() string               { return e.instructions }
func (e Execution) CWD() string                        { return e.cwd }
func (e Execution) ModelSelection() modelref.Selection { return e.modelSelection }
func (e Execution) Cron() string                       { return e.cron }

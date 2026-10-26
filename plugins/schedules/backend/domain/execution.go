package schedule

import (
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
)

// Capturing instructions independently prevents later definition edits or deletion from changing pending work.
type Execution struct {
	title          string
	instructions   string
	cwd            string
	modelSelection modelchoice.Selection
}

type ExecutionSnapshot struct {
	Title          string
	Instructions   string
	CWD            string
	ModelSelection modelchoice.Selection
}

func (s Schedule) Execution() Execution {
	return Execution{
		title: s.title, instructions: s.instructions, cwd: s.cwd,
		modelSelection: s.modelSelection,
	}
}

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

func (e Execution) Validate() error {
	return validateInstructions(e.instructions)
}

func (e Execution) Snapshot() ExecutionSnapshot {
	return ExecutionSnapshot{
		Title: e.title, Instructions: e.instructions, CWD: e.cwd,
		ModelSelection: e.modelSelection,
	}
}

func (e Execution) Title() string                         { return e.title }
func (e Execution) Instructions() string                  { return e.instructions }
func (e Execution) CWD() string                           { return e.cwd }
func (e Execution) ModelSelection() modelchoice.Selection { return e.modelSelection }

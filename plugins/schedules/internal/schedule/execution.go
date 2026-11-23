package schedule

import (
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
)

type Execution struct {
	title          string
	instructions   string
	cwd            string
	modelSelection flamemodel.Selection
}

type ExecutionSnapshot struct {
	Title          string               `json:"title"`
	Instructions   string               `json:"instructions"`
	CWD            string               `json:"cwd"`
	ModelSelection flamemodel.Selection `json:"modelSelection"`
}

func (s Schedule) execution() Execution {
	return Execution{
		title: s.title, instructions: s.instructions, cwd: s.cwd,
		modelSelection: s.modelSelection,
	}
}

func restoreExecution(snapshot ExecutionSnapshot) (Execution, error) {
	value := Execution{
		title: snapshot.Title, instructions: snapshot.Instructions, cwd: snapshot.CWD,
		modelSelection: snapshot.ModelSelection,
	}
	if err := value.validate(); err != nil {
		return Execution{}, err
	}
	return value, nil
}

func (e Execution) validate() error {
	return validateInstructions(e.instructions)
}

func (e Execution) snapshot() ExecutionSnapshot {
	return ExecutionSnapshot{
		Title: e.title, Instructions: e.instructions, CWD: e.cwd,
		ModelSelection: e.modelSelection,
	}
}

func (e Execution) Title() string                        { return e.title }
func (e Execution) Instructions() string                 { return e.instructions }
func (e Execution) CWD() string                          { return e.cwd }
func (e Execution) ModelSelection() flamemodel.Selection { return e.modelSelection }

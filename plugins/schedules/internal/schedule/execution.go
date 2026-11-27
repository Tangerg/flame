package schedule

import (
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
)

type Execution struct {
	projectID      string
	title          string
	instructions   string
	cwd            string
	modelSelection flamemodel.Selection
}

type ExecutionSnapshot struct {
	ProjectID      string               `json:"projectId,omitempty"`
	Title          string               `json:"title"`
	Instructions   string               `json:"instructions"`
	CWD            string               `json:"cwd"`
	ModelSelection flamemodel.Selection `json:"modelSelection"`
}

func (s Schedule) execution() Execution {
	return Execution{
		projectID: s.projectID, title: s.title, instructions: s.instructions, cwd: s.cwd,
		modelSelection: s.modelSelection,
	}
}

func restoreExecution(snapshot ExecutionSnapshot) (Execution, error) {
	value := Execution{
		projectID: snapshot.ProjectID, title: snapshot.Title, instructions: snapshot.Instructions, cwd: snapshot.CWD,
		modelSelection: snapshot.ModelSelection,
	}
	if err := value.validate(); err != nil {
		return Execution{}, err
	}
	return value, nil
}

func (e Execution) validate() error {
	if _, err := identity.ParseOptionalProject(e.projectID); err != nil {
		return err
	}
	return validateInstructions(e.instructions)
}

func (e Execution) snapshot() ExecutionSnapshot {
	return ExecutionSnapshot{
		ProjectID: e.projectID, Title: e.title, Instructions: e.instructions, CWD: e.cwd,
		ModelSelection: e.modelSelection,
	}
}

func (e Execution) ProjectID() string                    { return e.projectID }
func (e Execution) Title() string                        { return e.title }
func (e Execution) Instructions() string                 { return e.instructions }
func (e Execution) CWD() string                          { return e.cwd }
func (e Execution) ModelSelection() flamemodel.Selection { return e.modelSelection }

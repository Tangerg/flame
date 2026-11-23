package modelref

import flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"

type Role struct {
	selection flamemodel.Selection
}

func NewRole(provider, model string) (Role, error) {
	selection, err := flamemodel.New(provider, model)
	if err != nil {
		return Role{}, err
	}
	return Role{selection: selection}, nil
}

func (r Role) Configured() bool { return r.selection.Configured() }

func (r Role) Provider() string { return r.selection.Provider() }

func (r Role) Model() string { return r.selection.Model() }

func (r Role) Selection() flamemodel.Selection { return r.selection }

func (r Role) String() string { return r.selection.String() }

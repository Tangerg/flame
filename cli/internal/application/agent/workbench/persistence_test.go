package workbench

import (
	"errors"

	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/statefile"
)

func OpenDirectory(directory string, config Config) (*Store, error) {
	persistence, err := statefile.Open(directory)
	if err != nil {
		return nil, err
	}
	return Open(persistence, config)
}

type removeFailurePersistence struct {
	Persistence
	name    string
	enabled bool
}

func (r *removeFailurePersistence) Remove(name string) error {
	if r.enabled && name == r.name {
		return errors.New("injected remove failure")
	}
	return r.Persistence.Remove(name)
}

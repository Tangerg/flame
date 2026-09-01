package workbench

import "github.com/Tangerg/flame/cli/internal/adapter/filesystem/statefile"

func OpenDirectory(directory string, config Config) (*Store, error) {
	persistence, err := statefile.Open(directory)
	if err != nil {
		return nil, err
	}
	return Open(persistence, config)
}

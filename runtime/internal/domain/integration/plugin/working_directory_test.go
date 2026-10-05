package plugin

import (
	"errors"
	"testing"
)

func TestWorkingDirectorySpellingsHaveOneReading(t *testing.T) {
	for dir, want := range map[string]WorkingDirectory{
		"":                    {base: ReleaseBase},
		"${PLUGIN_ROOT}":      {base: ReleaseBase},
		"./work":              {base: ReleaseBase, path: "work"},
		"${PLUGIN_ROOT}/work": {base: ReleaseBase, path: "work"},
		"${PLUGIN_DATA}":      {base: DataBase},
		"${PLUGIN_DATA}/a/b":  {base: DataBase, path: "a/b"},
	} {
		got, err := Server{Dir: dir}.WorkingDirectory()
		if err != nil || got != want {
			t.Errorf("working directory %q = %+v, %v; want %+v", dir, got, err, want)
		}
	}
	for _, dir := range []string{"/abs", "work", "./", "./../escape", "${PLUGIN_DATA}/../x", "${PLUGIN_ROOT}x", "${HOME}/x"} {
		if _, err := (Server{Dir: dir}).WorkingDirectory(); !errors.Is(err, ErrInvalid) {
			t.Errorf("working directory %q = %v, want invalid", dir, err)
		}
	}
}

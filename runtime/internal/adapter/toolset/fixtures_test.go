package toolset

import (
	json "encoding/json/v2"
	"testing"
)

func editArguments(t *testing.T, path, oldString, newString string) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"path": path, "old_string": oldString, "new_string": newString,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

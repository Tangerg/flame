package persistence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// The old authority and Scope continuation formats cannot be interpreted as
// Frame records. Refusing the old root preserves their original recovery owner
// instead of silently initializing an unrelated database beside live evidence.
func checkLegacyLayout(directory string) error {
	for _, name := range []string{"flame-qimu.db", "flame.db"} {
		_, err := os.Lstat(filepath.Join(directory, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("persistence: inspect earlier storage: %w", err)
		}
		return fmt.Errorf("persistence: earlier database %s requires its original Runtime for recovery; this Frame release cannot convert its authority or continuations; preserve that deployment and select a fresh FLAME_HOME", name)
	}
	return nil
}

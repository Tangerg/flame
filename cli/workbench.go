package main

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/statefile"
	"github.com/Tangerg/flame/cli/internal/application/workbench"
)

func workbenchFactory(base string) func(string) (*workbench.Store, error) {
	return func(endpoint string) (*workbench.Store, error) {
		directory := targetStateDirectory(base, endpoint)
		if strings.TrimSpace(directory) == "" {
			return workbench.OpenMemory(workbench.Config{})
		}
		persistence, err := statefile.Open(directory)
		if err != nil {
			return nil, err
		}
		return workbench.Open(persistence, workbench.Config{})
	}
}

func targetStateDirectory(base, endpoint string) string {
	if base == "" || endpoint == "" {
		return base
	}
	identity := sha256.Sum256([]byte(endpoint))
	return filepath.Join(base, "targets", hex.EncodeToString(identity[:]))
}

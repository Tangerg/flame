package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const runtimeConfigDirectoryEnvironment = "FLAME_RUNTIME_CONFIG_DIR"

// runtimeConfigDirectories returns the sole process-owned Runtime config source.
// An explicit directory replaces the durable-data default; process cwd is never
// an ambient configuration source.
func runtimeConfigDirectories(runtimeDirectory string) ([]string, error) {
	if configured := strings.TrimSpace(os.Getenv(runtimeConfigDirectoryEnvironment)); configured != "" {
		if !filepath.IsAbs(configured) {
			return nil, fmt.Errorf("%s must be an absolute path", runtimeConfigDirectoryEnvironment)
		}
		return []string{filepath.Clean(configured)}, nil
	}
	return []string{filepath.Clean(runtimeDirectory)}, nil
}

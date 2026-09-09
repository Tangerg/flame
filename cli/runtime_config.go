package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const runtimeConfigDirectoryEnvironment = "FLAME_RUNTIME_CONFIG_DIR"

// runtimeConfigDirectories returns the explicitly configured Runtime config
// source, or nothing when the process did not name one. Empty leaves the
// default to the Runtime data directory, which this package does not resolve;
// process cwd is never an ambient configuration source either way.
func runtimeConfigDirectories() ([]string, error) {
	configured := strings.TrimSpace(os.Getenv(runtimeConfigDirectoryEnvironment))
	if configured == "" {
		return nil, nil
	}
	if !filepath.IsAbs(configured) {
		return nil, fmt.Errorf("%s must be an absolute path", runtimeConfigDirectoryEnvironment)
	}
	return []string{filepath.Clean(configured)}, nil
}

package main

import (
	"os"
	"strings"
)

const runtimeConfigDirectoryEnvironment = "FLAME_RUNTIME_CONFIG_DIR"

// runtimeConfigDirectories returns the explicitly configured Runtime config
// source, or nothing when the process did not name one. Empty leaves the
// default to the Runtime data directory, which this package does not resolve;
// process cwd is never an ambient configuration source either way. Runtime
// validates these paths only if the process selects the embedded binding.
func runtimeConfigDirectories() []string {
	configured := strings.TrimSpace(os.Getenv(runtimeConfigDirectoryEnvironment))
	if configured == "" {
		return nil
	}
	return []string{configured}
}

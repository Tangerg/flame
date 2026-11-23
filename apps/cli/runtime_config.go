package main

import (
	"os"
	"strings"
)

const runtimeConfigDirectoryEnvironment = "FLAME_RUNTIME_CONFIG_DIR"

func runtimeConfigDirectories() []string {
	configured := strings.TrimSpace(os.Getenv(runtimeConfigDirectoryEnvironment))
	if configured == "" {
		return nil
	}
	return []string{configured}
}

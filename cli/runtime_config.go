package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const runtimeConfigDirectoryEnvironment = "FLAME_RUNTIME_CONFIG_DIR"

// runtimeConfigDirectories returns the explicit config source when configured.
// Otherwise a config beside the runtime's durable data wins, with the current
// Flame worktree's development config as a source-checkout fallback.
func runtimeConfigDirectories(runtimeDirectory string) ([]string, error) {
	if configured := strings.TrimSpace(os.Getenv(runtimeConfigDirectoryEnvironment)); configured != "" {
		if !filepath.IsAbs(configured) {
			return nil, fmt.Errorf("%s must be an absolute path", runtimeConfigDirectoryEnvironment)
		}
		return []string{filepath.Clean(configured)}, nil
	}

	directories := make([]string, 0, 2)
	runtimeDirectory = filepath.Clean(runtimeDirectory)
	directories = append(directories, runtimeDirectory)

	if development, ok := discoverDevelopmentRuntimeConfigDirectory(); ok && development != runtimeDirectory {
		directories = append(directories, development)
	}
	return directories, nil
}

func discoverDevelopmentRuntimeConfigDirectory() (string, bool) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", false
	}
	return developmentRuntimeConfigDirectory(workingDirectory)
}

func developmentRuntimeConfigDirectory(start string) (string, bool) {
	directory, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		goWorkspace := filepath.Join(directory, "go.work")
		runtimeModule := filepath.Join(directory, "runtime", "go.mod")
		configFile := filepath.Join(directory, "runtime", "config", "config.yaml")
		if regularFile(goWorkspace) && regularFile(runtimeModule) && regularFile(configFile) {
			return filepath.Dir(configFile), true
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", false
		}
		directory = parent
	}
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

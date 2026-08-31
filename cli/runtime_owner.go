package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/cli/internal/cmd"
	"github.com/Tangerg/flame/cli/internal/runtimeembedded"
)

func newRuntimeOwnerAt(flameHome string) (runtimeOwner, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve runtime home: %w", err)
	}
	runtimeDirectory := filepath.Join(filepath.Clean(flameHome), "runtime")
	configDirectories, err := runtimeConfigDirectories(runtimeDirectory)
	if err != nil {
		return nil, err
	}
	return runtimeembedded.NewOwner(runtimeembedded.Config{
		DataDirectory: runtimeDirectory, UserHomePath: userHome,
		ConfigDirectories: configDirectories, ClientVersion: cmd.Version(),
	}), nil
}

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	"github.com/Tangerg/flame/runtime/internal/bootstrap"
	"github.com/Tangerg/flame/runtime/internal/config"
)

type runtimePaths struct {
	userHome             string
	defaultWorkspacePath string
	dataDirectory        localruntime.DataDirectory
	firstPartyDirectory  string
	runnerExecutable     string
	launchDirectory      string
}

func resolveRuntimePaths() (runtimePaths, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return runtimePaths{}, fmt.Errorf("runtime: locate user home: %w", err)
	}
	launchDirectory, err := os.Getwd()
	if err != nil {
		return runtimePaths{}, fmt.Errorf("runtime: locate launch directory: %w", err)
	}
	productRoot, err := localruntime.ProductRoot()
	if err != nil {
		return runtimePaths{}, fmt.Errorf("runtime: %w", err)
	}
	dataDirectory, err := localruntime.DataDirectoryUnder(productRoot)
	if err != nil {
		return runtimePaths{}, fmt.Errorf("runtime: %w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return runtimePaths{}, fmt.Errorf("runtime: locate distribution executable: %w", err)
	}
	firstPartyDirectory, err := localruntime.FirstPartyDirectory(executable)
	if err != nil {
		return runtimePaths{}, err
	}
	runnerExecutable, err := localruntime.RunnerExecutable(executable)
	if err != nil {
		return runtimePaths{}, err
	}
	return runtimePaths{
		firstPartyDirectory:  firstPartyDirectory,
		runnerExecutable:     runnerExecutable,
		userHome:             userHome,
		defaultWorkspacePath: userHome,
		dataDirectory:        dataDirectory,
		launchDirectory:      launchDirectory,
	}, nil
}

func bootstrapRuntime(ctx context.Context) (_ *bootstrap.Instance, _ config.Settings, _ runtimePaths, err error) {
	paths, err := resolveRuntimePaths()
	if err != nil {
		return nil, config.Settings{}, runtimePaths{}, err
	}
	instance, settings, err := bootstrapRuntimeWithBuildID(ctx, paths, bootstrap.ExecutableBuildID)
	return instance, settings, paths, err
}

func bootstrapRuntimeWithBuildID(
	ctx context.Context,
	paths runtimePaths,
	buildIdentity func() (string, error),
) (*bootstrap.Instance, config.Settings, error) {
	buildID, err := buildIdentity()
	if err != nil {
		return nil, config.Settings{}, err
	}
	return bootstrap.OpenInstance(ctx, bootstrap.InstanceConfig{
		UserHome:             paths.userHome,
		DefaultWorkspacePath: paths.defaultWorkspacePath,
		DataDirectory:        paths.dataDirectory.Path(),
		FirstPartyDirectory:  paths.firstPartyDirectory,
		RunnerExecutable:     paths.runnerExecutable,
		ConfigDirectories: []string{
			filepath.Join(paths.launchDirectory, "config"),
			paths.dataDirectory.Path(),
		},
		BuildID: buildID,
		ServerInfo: protocol.ServerInfo{
			Name:    identity.ProductName,
			Version: resolvedVersion(),
		},
	})
}

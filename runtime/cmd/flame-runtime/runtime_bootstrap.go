package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	"github.com/Tangerg/flame/runtime/internal/bootstrap"
	"github.com/Tangerg/flame/runtime/internal/config"
)

const flameHomeEnvironment = "FLAME_HOME"

// runtimePaths is the executable's single snapshot of host paths. User Home,
// default workspace, durable data, and launch directory are distinct facts even
// when defaults make some of them descendants or aliases; inner runtime rings
// consume these values and never rediscover them.
type runtimePaths struct {
	userHome             string
	defaultWorkspacePath string
	dataDirectory        localruntime.DataDirectory
	firstPartyDirectory  string
	launchDirectory      string
}

func resolveRuntimePaths() (runtimePaths, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return runtimePaths{}, fmt.Errorf("runtime: locate user home: %w", err)
	}
	if userHome == "" {
		return runtimePaths{}, errors.New("runtime: user home is unavailable")
	}
	if !filepath.IsAbs(userHome) {
		return runtimePaths{}, errors.New("runtime: user home must be absolute")
	}
	userHome = filepath.Clean(userHome)
	launchDirectory, err := os.Getwd()
	if err != nil {
		return runtimePaths{}, fmt.Errorf("runtime: locate launch directory: %w", err)
	}
	if !filepath.IsAbs(launchDirectory) {
		return runtimePaths{}, errors.New("runtime: launch directory must be absolute")
	}
	productRoot, err := localruntime.ResolveProductRoot(userHome, os.Getenv(flameHomeEnvironment))
	if err != nil {
		return runtimePaths{}, fmt.Errorf("runtime: resolve %s: %w", flameHomeEnvironment, err)
	}
	dataDirectory, err := localruntime.DataDirectoryUnder(productRoot)
	if err != nil {
		return runtimePaths{}, fmt.Errorf("runtime: resolve %s: %w", flameHomeEnvironment, err)
	}
	executable, err := os.Executable()
	if err != nil {
		return runtimePaths{}, fmt.Errorf("runtime: locate distribution executable: %w", err)
	}
	firstPartyDirectory, err := localruntime.FirstPartyDirectory(executable)
	if err != nil {
		return runtimePaths{}, err
	}
	return runtimePaths{
		firstPartyDirectory:  firstPartyDirectory,
		userHome:             userHome,
		defaultWorkspacePath: userHome,
		dataDirectory:        dataDirectory,
		launchDirectory:      filepath.Clean(launchDirectory),
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

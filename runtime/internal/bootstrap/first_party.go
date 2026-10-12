package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginpackage"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
)

func initializeFirstPartyPlugins(ctx context.Context, stores *persistence.Bundle, directory string) error {
	packages, err := pluginpackage.New(filepath.Join(stores.DataDirectory, "plugins", "releases"), stores.PluginReleases)
	if err != nil {
		return err
	}
	return plugins.Initialize(ctx, stores.Installations, packages, func() ([]string, error) {
		entries, err := os.ReadDir(directory)
		if err != nil {
			return nil, fmt.Errorf("runtime: read first-party distribution %q: %w", directory, err)
		}
		var sources []string
		for _, entry := range entries {
			if !entry.IsDir() {
				return nil, fmt.Errorf("runtime: first-party distribution contains non-directory %q", entry.Name())
			}
			sources = append(sources, filepath.Join(directory, entry.Name()))
		}
		return sources, nil
	})
}

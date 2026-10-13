package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

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
		distribution, err := os.Open(directory)
		if err != nil {
			return nil, fmt.Errorf("runtime: read first-party distribution %q: %w", directory, err)
		}
		defer distribution.Close()
		var sources []string
		for {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			entries, err := distribution.ReadDir(64)
			for _, entry := range entries {
				if !entry.IsDir() {
					return nil, fmt.Errorf("runtime: first-party distribution contains non-directory %q", entry.Name())
				}
				sources = append(sources, filepath.Join(directory, entry.Name()))
			}
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("runtime: read first-party distribution %q: %w", directory, err)
			}
		}
		slices.Sort(sources)
		return sources, nil
	})
}

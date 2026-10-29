package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/cancellation"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginpackage"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func initializeFirstPartyPlugins(ctx context.Context, stores *persistence.Bundle, host *qimu.Host, packages *pluginpackage.Releases, directory string) error {
	return plugins.Initialize(ctx, host, stores.Installations, packages, func() ([]string, error) {
		return distributionSources(ctx, directory)
	}, stores.ProviderSelections, []plugins.DefaultProvider{
		{Role: "flame.model-provider.v1", Package: "flame.providers", Contribution: "model-provider"},
		{Role: "memory", Package: "flame.memory", Contribution: "memory-provider"},
		{Role: "schedules", Package: "flame.schedules", Contribution: "schedules-provider"},
		{Role: "flame.code-intelligence.v1", Package: "flame.codeintel", Contribution: "code-intelligence"},
		{Role: "flame.skill-learning.v1", Package: "flame.skills", Contribution: "skill-learning"},
	})
}

func distributionSources(ctx context.Context, directory string) (sources []string, err error) {
	if err := cancellation.Err(ctx); err != nil {
		return nil, err
	}
	root, err := os.Open(directory)
	if err != nil {
		return nil, fmt.Errorf("runtime: open plugin distribution: %w", err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	entries, readErr := root.ReadDir(plugin.MaxInstallations + 1)
	if errors.Is(readErr, io.EOF) {
		readErr = nil
	}
	if err := errors.Join(readErr, cancellation.Err(ctx)); err != nil {
		return nil, fmt.Errorf("runtime: read plugin distribution: %w", err)
	}
	if len(entries) == 0 || len(entries) > plugin.MaxInstallations {
		return nil, errors.New("runtime: plugin distribution requires a bounded catalog")
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			return nil, fmt.Errorf("runtime: plugin distribution contains non-directory %q", entry.Name())
		}
		sources = append(sources, filepath.Join(directory, entry.Name()))
	}
	slices.Sort(sources)
	return sources, nil
}

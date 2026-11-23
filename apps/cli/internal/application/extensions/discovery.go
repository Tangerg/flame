package extensions

import (
	"context"
	"fmt"
	"slices"
)

type Source interface {
	ID() string
	Discover(context.Context) (SourceResult, error)
}

type SourceResult struct {
	Plugins []Plugin
	Issues  []error
}

type SourceIssue struct {
	Source string
	Err    error
}

type Discovery struct {
	Plugins []Plugin
	Issues  []SourceIssue
}

func Discover(ctx context.Context, sources ...Source) (Discovery, error) {
	var result Discovery
	for i, source := range sources {
		if err := context.Cause(ctx); err != nil {
			return Discovery{}, err
		}
		if source == nil {
			result.Issues = append(result.Issues, SourceIssue{Source: fmt.Sprintf("source-%d", i+1), Err: fmt.Errorf("extensions: plugin source %d is nil", i+1)})
			continue
		}
		discovered, err := source.Discover(ctx)
		if cause := context.Cause(ctx); cause != nil {
			return Discovery{}, cause
		}
		if err != nil {
			result.Issues = append(result.Issues, SourceIssue{Source: source.ID(), Err: fmt.Errorf("discover plugins from %q: %w", source.ID(), err)})
			continue
		}
		for _, issue := range discovered.Issues {
			if issue != nil {
				result.Issues = append(result.Issues, SourceIssue{Source: source.ID(), Err: issue})
			}
		}
		for _, plugin := range discovered.Plugins {
			result.Plugins = append(result.Plugins, clonePlugin(plugin))
		}
	}
	result.Plugins = slices.Clip(result.Plugins)
	result.Issues = slices.Clip(result.Issues)
	if err := context.Cause(ctx); err != nil {
		return Discovery{}, err
	}
	return result, nil
}

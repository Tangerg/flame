package bootstrap

import (
	"context"
)

func terminalResources(resources []terminalResource) []*teardownStep {
	steps := make([]*teardownStep, 0, len(resources))
	for _, resource := range resources {
		if resource == nil {
			continue
		}
		steps = append(steps, terminalStep(func(context.Context) error {
			return resource.Close()
		}))
	}
	return steps
}

func terminalClosers(closers []func() error) []*teardownStep {
	steps := make([]*teardownStep, 0, len(closers))
	for _, closeFn := range closers {
		if closeFn == nil {
			continue
		}
		steps = append(steps, terminalStep(func(context.Context) error {
			return closeFn()
		}))
	}
	return steps
}

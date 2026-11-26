package bootstrap

import (
	"context"
	"errors"

	"github.com/Tangerg/frame"
	framehost "github.com/Tangerg/frame/host"
	"github.com/Tangerg/frame/process"
)

func ownProcessRunner(owner frame.Lifetime, driver *process.Driver, closeRunner func(context.Context) error) (frame.ResourceHandle, error) {
	return owner.Own(frame.Resource{
		Name: "managed process runner",
		Close: func(ctx context.Context) (frame.ReleaseState, error) {
			if err := driver.Recover(ctx); err != nil {
				return frame.ResourceUnconfirmed, err
			}
			if err := closeRunner(ctx); err != nil {
				return frame.ResourceUnconfirmed, err
			}
			return frame.ResourceReleased, nil
		},
	})
}

type runnerProgramSource struct {
	source process.Source
	owner  frame.Lifetime
	runner frame.ResourceHandle
}

func (s *runnerProgramSource) Program(ctx context.Context, binding framehost.UnitBinding) (process.Program, error) {
	lease, err := s.owner.Own(frame.Resource{
		Name:  "process runner use",
		Close: func(context.Context) (frame.ReleaseState, error) { return frame.ResourceUnconfirmed, nil },
	}, s.runner)
	if err != nil {
		return process.Program{}, err
	}
	program, err := s.source.Program(ctx, binding)
	releaseContent := program.ReleaseContent
	// The process driver releases the content dependency only after it has
	// the original process exit proof. That boundary also ends use of the runner.
	program.ReleaseContent = func() (err error) {
		defer func() { err = errors.Join(err, lease.ConfirmReleased(nil)) }()
		if releaseContent == nil {
			return nil
		}
		return releaseContent()
	}
	return program, err
}

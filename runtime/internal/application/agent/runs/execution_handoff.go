package runs

import (
	"context"
	"errors"
	"fmt"
)

type stagedExecutionHandoff struct {
	releases ExecutionReleaser
	ref      ExecutorRef
	owned    bool
}

func ownStagedExecution(releases ExecutionReleaser, ref ExecutorRef) *stagedExecutionHandoff {
	return &stagedExecutionHandoff{releases: releases, ref: ref, owned: true}
}

func (s *stagedExecutionHandoff) transfer() {
	if !s.owned {
		panic("runs: staged execution ownership transferred more than once")
	}
	s.owned = false
}

func (s *stagedExecutionHandoff) abandon(
	ctx context.Context,
	cause error,
	description string,
) error {
	if s == nil || !s.owned {
		return cause
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
	defer cancel()
	s.owned = false
	if err := s.releases.Release(cleanupCtx, s.ref); err != nil {
		return errors.Join(
			cause,
			fmt.Errorf("runs: release %s %q: %w", description, s.ref.ExecutorID, err),
		)
	}
	return cause
}

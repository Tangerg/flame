package workspace

import (
	"context"
	"errors"
	"time"

	"github.com/samber/lo"
)

type idleSkillSweeper interface {
	SweepIdle(ctx context.Context, now time.Time, inactiveSince time.Time) ([]string, []string, error)
}

type SkillMaintenance struct {
	sweeper      idleSkillSweeper
	observations *AuthoredWatch
}

func NewSkillMaintenance(sweeper idleSkillSweeper, observations *AuthoredWatch) (*SkillMaintenance, error) {
	if lo.IsNil(sweeper) {
		return nil, errors.New("workspace: idle skill sweeper is required")
	}
	if lo.IsNil(observations) {
		return nil, errors.New("workspace: idle skill authored observation is required")
	}
	return &SkillMaintenance{sweeper: sweeper, observations: observations}, nil
}

func (s *SkillMaintenance) ArchiveIdle(ctx context.Context, now time.Time, inactiveSince time.Time) ([]string, error) {
	archived, identities, err := s.sweeper.SweepIdle(ctx, now, inactiveSince)
	s.observations.PublishSkillChanges(identities)
	return archived, err
}

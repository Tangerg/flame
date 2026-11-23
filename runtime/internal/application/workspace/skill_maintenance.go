package workspace

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/samber/lo"
)

type idleSkillSweeper interface {
	SweepIdle(ctx context.Context, now time.Time, inactiveSince time.Time) ([]string, []string, error)
}

type SkillMaintenance struct {
	sweeper       idleSkillSweeper
	observations  *AuthoredWatch
	invalidations invalidation.Publish
}

func NewSkillMaintenance(sweeper idleSkillSweeper, observations *AuthoredWatch, invalidations invalidation.Publish) (*SkillMaintenance, error) {
	if lo.IsNil(sweeper) {
		return nil, errors.New("workspace: idle skill sweeper is required")
	}
	if lo.IsNil(observations) {
		return nil, errors.New("workspace: idle skill authored observation is required")
	}
	if invalidations == nil {
		return nil, errors.New("workspace: idle skill invalidations are required")
	}
	return &SkillMaintenance{sweeper: sweeper, observations: observations, invalidations: invalidations}, nil
}

func (s *SkillMaintenance) ArchiveIdle(ctx context.Context, now time.Time, inactiveSince time.Time) ([]string, error) {
	archived, identities, err := s.sweeper.SweepIdle(ctx, now, inactiveSince)
	if len(identities) > 0 {
		s.observations.Accept(AuthoredChange{Resource: AuthoredSkills, Identities: identities})
		s.invalidations.Notify(invalidation.Notice{Resource: invalidation.Skills})
	}
	return archived, err
}

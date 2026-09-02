package maintenance

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	defaultSkillArchiveAfter         = 30 * 24 * time.Hour
	defaultSkillArchiveCheckInterval = 6 * time.Hour
)

// SkillArchivePolicyValues tunes idle-Skill archival. Nil selects a named
// default; present non-positive durations are invalid.
type SkillArchivePolicyValues struct {
	// ArchiveAfter is the inactivity before an agent-authored skill is archived.
	ArchiveAfter *time.Duration
	// CheckInterval is the minimum wall-clock interval between Run-boundary
	// archival checks, bounding their cost across a busy Session.
	CheckInterval *time.Duration
}

type skillArchivePolicy struct {
	archiveAfter  time.Duration
	checkInterval time.Duration
}

func newSkillArchivePolicy(values SkillArchivePolicyValues) (skillArchivePolicy, error) {
	archiveAfter, err := positiveDurationOrDefault(values.ArchiveAfter, defaultSkillArchiveAfter, "archive after")
	if err != nil {
		return skillArchivePolicy{}, err
	}
	checkInterval, err := positiveDurationOrDefault(values.CheckInterval, defaultSkillArchiveCheckInterval, "check interval")
	if err != nil {
		return skillArchivePolicy{}, err
	}
	return skillArchivePolicy{archiveAfter: archiveAfter, checkInterval: checkInterval}, nil
}

// idleSkillArchiver is the Application capability consumed by this scheduling
// adapter. It keeps the persistence mechanism and invalidation semantics out of
// Run maintenance.
type idleSkillArchiver interface {
	ArchiveIdle(ctx context.Context, now time.Time, archiveAfter time.Duration) ([]string, error)
}

// IdleSkillArchiver archives inactive agent-authored Skills at Run boundaries,
// checking at most once per CheckInterval. The managed Skill library is
// user-scoped, so the check is process-wide rather than per Session. The first
// Run after start performs a check, avoiding a startup-time filesystem mutation.
type IdleSkillArchiver struct {
	skills idleSkillArchiver
	policy skillArchivePolicy
	now    func() time.Time

	mu        sync.Mutex
	lastCheck time.Time
}

// NewIdleSkillArchiver builds a Run-boundary scheduler over the required
// Application Skill-curation capability.
func NewIdleSkillArchiver(skills idleSkillArchiver, values SkillArchivePolicyValues) (*IdleSkillArchiver, error) {
	if nilDependency(skills) {
		return nil, errors.New("idle skill archiver: skill curator is required")
	}
	policy, err := newSkillArchivePolicy(values)
	if err != nil {
		return nil, err
	}
	return &IdleSkillArchiver{
		skills: skills,
		policy: policy,
		now:    time.Now,
	}, nil
}

// ArchiveIfDue archives eligible Skills unless the previous check occurred
// within CheckInterval. The rate-limit window advances even when nothing is
// archived, so a busy Session does not evaluate the library after every Run.
func (i *IdleSkillArchiver) ArchiveIfDue(ctx context.Context) error {
	if i == nil {
		return nil
	}
	now := i.now()
	i.mu.Lock()
	if !i.lastCheck.IsZero() && now.Sub(i.lastCheck) < i.policy.checkInterval {
		i.mu.Unlock()
		return nil
	}
	i.lastCheck = now
	i.mu.Unlock()
	archived, err := i.skills.ArchiveIdle(ctx, now, i.policy.archiveAfter)
	recordArchivedIdleSkills(ctx, len(archived))
	return err
}

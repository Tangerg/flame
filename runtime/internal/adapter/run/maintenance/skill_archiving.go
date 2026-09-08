package maintenance

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	skillArchiveAfter         = 30 * 24 * time.Hour
	skillArchiveCheckInterval = 6 * time.Hour
)

// idleSkillArchiver is the Application capability consumed by this scheduling
// adapter. It keeps the persistence mechanism and invalidation semantics out of
// Run maintenance.
type idleSkillArchiver interface {
	ArchiveIdle(ctx context.Context, now time.Time, archiveAfter time.Duration) ([]string, error)
}

// IdleSkillArchiver archives inactive agent-authored Skills at Run boundaries,
// checking at most once per six-hour window. The managed Skill library is
// user-scoped, so the check is process-wide rather than per Session. The first
// Run after start performs a check, avoiding a startup-time filesystem mutation.
type IdleSkillArchiver struct {
	skills idleSkillArchiver
	now    func() time.Time

	mu        sync.Mutex
	lastCheck time.Time
}

// NewIdleSkillArchiver builds a Run-boundary scheduler over the required
// Application Skill-curation capability.
func NewIdleSkillArchiver(skills idleSkillArchiver) (*IdleSkillArchiver, error) {
	if nilDependency(skills) {
		return nil, errors.New("idle skill archiver: skill curator is required")
	}
	return &IdleSkillArchiver{
		skills: skills,
		now:    time.Now,
	}, nil
}

// ArchiveIfDue archives eligible Skills unless the previous check occurred
// within the check interval. The rate-limit window advances even when nothing is
// archived, so a busy Session does not evaluate the library after every Run.
func (i *IdleSkillArchiver) ArchiveIfDue(ctx context.Context) error {
	now := i.now()
	i.mu.Lock()
	if !i.lastCheck.IsZero() && now.Sub(i.lastCheck) < skillArchiveCheckInterval {
		i.mu.Unlock()
		return nil
	}
	i.lastCheck = now
	i.mu.Unlock()
	archived, err := i.skills.ArchiveIdle(ctx, now, skillArchiveAfter)
	recordArchivedIdleSkills(ctx, len(archived))
	return err
}

package workspace

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
)

type fakeIdleSkillSweeper struct {
	archived []string
	err      error
	now      time.Time
}

func (f *fakeIdleSkillSweeper) SweepIdle(_ context.Context, now time.Time, _ time.Duration) ([]string, []string, error) {
	f.now = now
	identities := make([]string, len(f.archived))
	for index, name := range f.archived {
		identities[index] = "/skills/" + name + "/SKILL.md"
	}
	return f.archived, identities, f.err
}

func TestIdleSkillArchiveNotifiesEveryCommittedPartialSweep(t *testing.T) {
	wantErr := errors.New("usage metadata unavailable")
	sweeper := &fakeIdleSkillSweeper{archived: []string{"old-skill"}, err: wantErr}
	var notices []invalidation.Notice
	maintenance, err := NewSkillMaintenance(sweeper, nil, func(notice invalidation.Notice) {
		notices = append(notices, notice)
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)

	archived, err := maintenance.ArchiveIdle(t.Context(), now, 30*24*time.Hour)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ArchiveIdle error = %v, want %v", err, wantErr)
	}
	if len(archived) != 1 || archived[0] != "old-skill" {
		t.Fatalf("ArchiveIdle archived = %v", archived)
	}
	if !sweeper.now.Equal(now) {
		t.Fatalf("sweep time = %s, want %s", sweeper.now, now)
	}
	if len(notices) != 1 || notices[0].Resource != invalidation.Skills {
		t.Fatalf("notices = %+v, want one Skills invalidation", notices)
	}

	sweeper.archived = nil
	sweeper.err = nil
	if _, err := maintenance.ArchiveIdle(t.Context(), now.Add(time.Hour), 30*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if len(notices) != 1 {
		t.Fatalf("empty sweep published notices = %+v", notices)
	}
}

func TestSkillMaintenanceRequiresSweeper(t *testing.T) {
	var missing *fakeIdleSkillSweeper
	for _, sweeper := range []IdleSkillSweeper{nil, missing} {
		if maintenance, err := NewSkillMaintenance(sweeper, nil, nil); err == nil || maintenance != nil {
			t.Fatalf("NewSkillMaintenance = (%v, %v), want construction failure", maintenance, err)
		}
	}
}

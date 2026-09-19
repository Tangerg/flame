package maintenance

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeIdleSkillArchiver struct {
	calls int
	seen  []time.Time
}

type countingIdleSkillArchiver struct{ calls atomic.Int32 }

func (c *countingIdleSkillArchiver) ArchiveIdle(context.Context, time.Time, time.Duration) ([]string, error) {
	c.calls.Add(1)
	return nil, nil
}

func (f *fakeIdleSkillArchiver) ArchiveIdle(_ context.Context, now time.Time, _ time.Duration) ([]string, error) {
	f.calls++
	f.seen = append(f.seen, now)
	return nil, nil
}

func mustNewIdleSkillArchiver(t *testing.T, skills idleSkillArchiver) *IdleSkillArchiver {
	t.Helper()
	archiver, err := NewIdleSkillArchiver(skills)
	if err != nil {
		t.Fatal(err)
	}
	return archiver
}

func TestIdleSkillArchiverRateLimitsChecks(t *testing.T) {
	store := &fakeIdleSkillArchiver{}
	skillArchiver := mustNewIdleSkillArchiver(t, store)
	base := time.Unix(1_700_000_000, 0)
	skillArchiver.now = func() time.Time { return base }

	// First call fires (lastCheck is zero — stands in for the boot sweep).
	if err := skillArchiver.ArchiveIfDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Within CheckInterval: skipped.
	if err := skillArchiver.ArchiveIfDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 {
		t.Fatalf("calls within the window = %d, want 1", store.calls)
	}
	// Past CheckInterval: fires again.
	skillArchiver.now = func() time.Time { return base.Add(skillArchiveCheckInterval + time.Second) }
	if err := skillArchiver.ArchiveIfDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if store.calls != 2 {
		t.Fatalf("calls after the window = %d, want 2", store.calls)
	}
}

func TestIdleSkillArchiverAdmitsOneConcurrentSweepPerWindow(t *testing.T) {
	skills := &countingIdleSkillArchiver{}
	archiver := mustNewIdleSkillArchiver(t, skills)
	archiver.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	const contenders = 32
	var group sync.WaitGroup
	group.Add(contenders)
	for range contenders {
		go func() {
			defer group.Done()
			if err := archiver.ArchiveIfDue(t.Context()); err != nil {
				t.Errorf("ArchiveIfDue error = %v", err)
			}
		}()
	}
	group.Wait()
	if got := skills.calls.Load(); got != 1 {
		t.Fatalf("concurrent sweeps = %d, want 1", got)
	}
}

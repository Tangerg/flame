package project

import (
	"errors"
	"regexp"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

var ErrBackgroundCapacity = errors.New("project background work is paused or its admission limit was reached")

var pullRequestRef = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+#[1-9][0-9]{0,14}(@[0-9a-f]{40}:[0-9a-f]{40})?$`)

type BackgroundPolicy struct {
	Revision     int64
	Paused       bool
	MaxActive    int
	MaxDailyRuns int
	PullRequests []string
}

func (p BackgroundPolicy) Validate() error {
	if p.Revision < 0 || p.Revision >= 1<<53-1 || p.MaxActive < 1 || p.MaxActive > 32 ||
		p.MaxDailyRuns < 1 || p.MaxDailyRuns > 1000 || len(p.PullRequests) > 16 {
		return ErrInvalid
	}
	seen := make(map[string]struct{}, len(p.PullRequests))
	for _, ref := range p.PullRequests {
		if !pullRequestRef.MatchString(ref) || len(ref) > 255 {
			return ErrInvalid
		}
		if _, duplicate := seen[ref]; duplicate {
			return ErrInvalid
		}
		seen[ref] = struct{}{}
	}
	return nil
}

type BackgroundSource struct {
	ProjectID identity.ProjectID
	Kind      string
	ID        string
	Version   string
	Title     string
}

type BackgroundRun struct {
	BackgroundSource
	SessionID  string
	RunID      string
	Status     string
	Outcome    string
	AdmittedAt time.Time
}

type BackgroundActivity struct {
	Policy   BackgroundPolicy
	Active   int
	Admitted int
	Recent   []BackgroundRun
}

func DailyWindow(now time.Time) (time.Time, time.Time) {
	start := now.UTC().Truncate(24 * time.Hour)
	return start, start.Add(24 * time.Hour)
}

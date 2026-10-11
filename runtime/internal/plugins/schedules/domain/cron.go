package schedule

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

const durableTimePrecision = time.Millisecond

func canonicalTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC().Truncate(durableTimePrecision)
}

func ValidateCron(spec string) error {
	if _, err := cron.ParseStandard(spec); err != nil {
		return fmt.Errorf("%w %q: %w", ErrInvalidCron, spec, err)
	}
	return nil
}

// Cron uses UTC unless the expression names a timezone. Caller locations must
// not change its meaning between creation, editing, and restart.
func NextRun(spec string, after time.Time) (time.Time, error) {
	sched, err := cron.ParseStandard(spec)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w %q: %w", ErrInvalidCron, spec, err)
	}
	next := sched.Next(after.UTC())
	if next.IsZero() {
		return time.Time{}, fmt.Errorf("%w %q: no future occurrence", ErrInvalidCron, spec)
	}
	return canonicalTime(next), nil
}

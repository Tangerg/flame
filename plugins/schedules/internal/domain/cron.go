package schedule

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

var ErrInvalidTimeZone = errors.New("schedule: invalid time zone")

const durableTimePrecision = time.Millisecond

func canonicalTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC().Truncate(durableTimePrecision)
}

// Recurrence is when a schedule fires: a five-field cron read in one IANA
// time zone. The zone is its own fact, so a cron cannot carry a second one
// through a CRON_TZ prefix, and the meaning cannot drift with the host.
type Recurrence struct {
	cron string
	zone *time.Location
}

func NewRecurrence(spec, timeZone string) (Recurrence, error) {
	if spec == "" {
		return Recurrence{}, errCronRequired
	}
	if strings.HasPrefix(spec, "CRON_TZ=") || strings.HasPrefix(spec, "TZ=") {
		return Recurrence{}, fmt.Errorf("%w %q: the time zone is set separately", ErrInvalidCron, spec)
	}
	if _, err := cron.ParseStandard(spec); err != nil {
		return Recurrence{}, fmt.Errorf("%w %q: %w", ErrInvalidCron, spec, err)
	}
	if timeZone == "" || timeZone == "Local" {
		return Recurrence{}, fmt.Errorf("%w %q: an IANA zone name is required", ErrInvalidTimeZone, timeZone)
	}
	zone, err := time.LoadLocation(timeZone)
	if err != nil {
		return Recurrence{}, fmt.Errorf("%w %q: %w", ErrInvalidTimeZone, timeZone, err)
	}
	return Recurrence{cron: spec, zone: zone}, nil
}

func (r Recurrence) Cron() string     { return r.cron }
func (r Recurrence) TimeZone() string { return r.zone.String() }

func (r Recurrence) next(after time.Time) (time.Time, error) {
	sched, err := cron.ParseStandard(r.cron)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w %q: %w", ErrInvalidCron, r.cron, err)
	}
	next := sched.Next(after.In(r.zone))
	if next.IsZero() {
		return time.Time{}, fmt.Errorf("%w %q: no future occurrence", ErrInvalidCron, r.cron)
	}
	return canonicalTime(next), nil
}

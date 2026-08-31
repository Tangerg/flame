package sessions

import (
	"errors"
	"time"
)

var ErrInvalidUsageSummaryPeriod = errors.New("sessions: usage summary period is invalid")

// UsageSummaryPeriod is either all durable history or the preceding positive number
// of calendar days. Its zero value is the named all-time period; numeric zero
// never crosses the Application boundary as a disable switch.
type UsageSummaryPeriod struct {
	recent bool
	days   int
}

func AllTimeUsage() UsageSummaryPeriod { return UsageSummaryPeriod{} }

func RecentUsageDays(days int) (UsageSummaryPeriod, error) {
	if days <= 0 {
		return UsageSummaryPeriod{}, ErrInvalidUsageSummaryPeriod
	}
	return UsageSummaryPeriod{recent: true, days: days}, nil
}

// Since resolves the lower bound at one UTC observation. All-time returns a
// zero time; recent periods subtract whole calendar days from now.
func (p UsageSummaryPeriod) Since(now time.Time) (time.Time, error) {
	if !p.recent {
		if p.days != 0 {
			return time.Time{}, ErrInvalidUsageSummaryPeriod
		}
		return time.Time{}, nil
	}
	if p.days <= 0 {
		return time.Time{}, ErrInvalidUsageSummaryPeriod
	}
	return now.UTC().AddDate(0, 0, -p.days), nil
}

func (p UsageSummaryPeriod) Days() (int, bool) { return p.days, p.recent }

package usage

import (
	"errors"
	"time"
)

var ErrInvalidSummaryPeriod = errors.New("usage: summary period is invalid")

// SummaryPeriod is either all durable history or the preceding positive number
// of calendar days. Its zero value is the named all-time period; numeric zero
// never crosses the Application boundary as a disable switch.
type SummaryPeriod struct {
	recent bool
	days   int
}

func AllTime() SummaryPeriod { return SummaryPeriod{} }

func RecentDays(days int) (SummaryPeriod, error) {
	if days <= 0 {
		return SummaryPeriod{}, ErrInvalidSummaryPeriod
	}
	return SummaryPeriod{recent: true, days: days}, nil
}

// Since resolves the lower bound at one UTC observation. All-time returns a
// zero time; recent periods subtract whole calendar days from now.
func (p SummaryPeriod) Since(now time.Time) (time.Time, error) {
	if !p.recent {
		if p.days != 0 {
			return time.Time{}, ErrInvalidSummaryPeriod
		}
		return time.Time{}, nil
	}
	if p.days <= 0 {
		return time.Time{}, ErrInvalidSummaryPeriod
	}
	return now.UTC().AddDate(0, 0, -p.days), nil
}

func (p SummaryPeriod) Days() (int, bool) { return p.days, p.recent }

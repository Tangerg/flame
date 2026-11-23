package sessions

import (
	"errors"
	"time"
)

var errInvalidUsageSummaryPeriod = errors.New("sessions: usage summary period is invalid")

type UsageSummaryPeriod struct {
	recent bool
	days   int
}

func AllTimeUsage() UsageSummaryPeriod { return UsageSummaryPeriod{} }

func RecentUsageDays(days int) (UsageSummaryPeriod, error) {
	if days <= 0 {
		return UsageSummaryPeriod{}, errInvalidUsageSummaryPeriod
	}
	return UsageSummaryPeriod{recent: true, days: days}, nil
}

func (p UsageSummaryPeriod) Since(now time.Time) (time.Time, error) {
	if !p.recent {
		if p.days != 0 {
			return time.Time{}, errInvalidUsageSummaryPeriod
		}
		return time.Time{}, nil
	}
	if p.days <= 0 {
		return time.Time{}, errInvalidUsageSummaryPeriod
	}
	return now.UTC().AddDate(0, 0, -p.days), nil
}

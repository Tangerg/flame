package agent

import "errors"

type usageSummaryPeriodKind uint8

const (
	allTimeUsagePeriod usageSummaryPeriodKind = iota + 1
	recentDaysUsagePeriod
)

// UsageSummaryPeriod is either all runtime history or a positive recent-day window.
// Its zero value is invalid, so every caller chooses the report scope.
type UsageSummaryPeriod struct {
	kind usageSummaryPeriodKind
	days int
}

func AllTimeUsage() UsageSummaryPeriod { return UsageSummaryPeriod{kind: allTimeUsagePeriod} }

func RecentUsageDays(days int) (UsageSummaryPeriod, error) {
	if days <= 0 {
		return UsageSummaryPeriod{}, errors.New("usage summary recent days must be positive")
	}
	return UsageSummaryPeriod{kind: recentDaysUsagePeriod, days: days}, nil
}

func (p UsageSummaryPeriod) Days() (int, bool, error) {
	if err := p.Validate(); err != nil {
		return 0, false, err
	}
	return p.days, p.kind == recentDaysUsagePeriod, nil
}

func (p UsageSummaryPeriod) Validate() error {
	switch p.kind {
	case recentDaysUsagePeriod:
		if p.days <= 0 {
			return errors.New("usage summary recent days must be positive")
		}
		return nil
	case allTimeUsagePeriod:
		if p.days != 0 {
			return errors.New("usage summary all-time period carries days")
		}
		return nil
	default:
		return errors.New("usage summary period kind is unknown")
	}
}

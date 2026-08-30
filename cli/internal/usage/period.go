package usage

import "errors"

type summaryPeriodKind uint8

const (
	allTimePeriod summaryPeriodKind = iota + 1
	recentDaysPeriod
)

// SummaryPeriod is either all runtime history or a positive recent-day window.
// Its zero value is invalid, so every caller chooses the report scope.
type SummaryPeriod struct {
	kind summaryPeriodKind
	days int
}

func AllTime() SummaryPeriod { return SummaryPeriod{kind: allTimePeriod} }

func RecentDays(days int) (SummaryPeriod, error) {
	if days <= 0 {
		return SummaryPeriod{}, errors.New("usage summary recent days must be positive")
	}
	return SummaryPeriod{kind: recentDaysPeriod, days: days}, nil
}

func (p SummaryPeriod) Days() (int, bool, error) {
	if err := p.Validate(); err != nil {
		return 0, false, err
	}
	return p.days, p.kind == recentDaysPeriod, nil
}

func (p SummaryPeriod) Validate() error {
	switch p.kind {
	case recentDaysPeriod:
		if p.days <= 0 {
			return errors.New("usage summary recent days must be positive")
		}
		return nil
	case allTimePeriod:
		if p.days != 0 {
			return errors.New("usage summary all-time period carries days")
		}
		return nil
	default:
		return errors.New("usage summary period kind is unknown")
	}
}

package sqlite

import "fmt"

// scheduleFiringState is the single codec for the private SQLite lifecycle of
// a durable schedule occurrence.
type scheduleFiringState string

const (
	scheduleFiringPending  scheduleFiringState = "pending"
	scheduleFiringAccepted scheduleFiringState = "accepted"
)

func restoreScheduleFiringState(raw string) (scheduleFiringState, error) {
	value := scheduleFiringState(raw)
	switch value {
	case scheduleFiringPending, scheduleFiringAccepted:
		return value, nil
	default:
		return "", fmt.Errorf("sqlite: unknown schedule firing state %q", raw)
	}
}

func (s scheduleFiringState) databaseValue() string { return string(s) }

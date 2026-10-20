// Package instant defines the exact time range shared by durable lifecycle
// facts and their nanosecond ordering coordinates.
package instant

import (
	"fmt"
	"math"
	"time"
)

// Validate accepts an absent time or an instant that survives Unix nanosecond
// encoding unchanged. The owning aggregate decides whether absence is valid.
func Validate(values ...time.Time) error {
	for _, value := range values {
		if !value.IsZero() && (value.Before(time.Unix(0, math.MinInt64)) || value.After(time.Unix(0, math.MaxInt64))) {
			return fmt.Errorf("instant: %s is outside the exact Unix nanosecond range", value.Format(time.RFC3339Nano))
		}
	}
	return nil
}

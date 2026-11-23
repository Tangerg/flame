package instant

import (
	"fmt"
	"math"
	"time"
)

func Validate(values ...time.Time) error {
	for _, value := range values {
		if !value.IsZero() && (value.Before(time.Unix(0, math.MinInt64)) || value.After(time.Unix(0, math.MaxInt64))) {
			return fmt.Errorf("instant: %s is outside the exact Unix nanosecond range", value.Format(time.RFC3339Nano))
		}
	}
	return nil
}

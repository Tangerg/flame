// Package toolarg owns model-facing Tool argument normalization shared by the
// Runtime toolset adapters.
package toolarg

import "fmt"

// PositiveInt resolves an optional positive integer. Nil selects fallback;
// present values must be strictly positive and no greater than maximum when a
// positive maximum is supplied. This keeps JSON absence distinct from numeric
// zero all the way to the Tool boundary.
func PositiveInt(value *int, fallback, maximum int, field string) (int, error) {
	if fallback <= 0 {
		return 0, fmt.Errorf("tool argument %s: default must be positive", field)
	}
	if maximum > 0 && fallback > maximum {
		return 0, fmt.Errorf("tool argument %s: default exceeds maximum", field)
	}
	if value == nil {
		return fallback, nil
	}
	if *value <= 0 {
		return 0, fmt.Errorf("tool argument %s must be positive", field)
	}
	if maximum > 0 && *value > maximum {
		return 0, fmt.Errorf("tool argument %s must not exceed %d", field, maximum)
	}
	return *value, nil
}

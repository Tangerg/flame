package persistence

import "fmt"

const (
	storedFalse = 0
	storedTrue  = 1
)

func boolToInt(value bool) int {
	if value {
		return storedTrue
	}
	return storedFalse
}

func boolFromInt(value int64) (bool, error) {
	switch value {
	case storedFalse:
		return false, nil
	case storedTrue:
		return true, nil
	default:
		return false, fmt.Errorf("sqlite: invalid stored boolean %d", value)
	}
}

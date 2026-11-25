package workbench

import (
	"errors"
)

type Capacity struct {
	value int
}

func newCapacity(value int) (Capacity, error) {
	if value <= 0 {
		return Capacity{}, errors.New("workbench capacity is not positive")
	}
	return Capacity{value: value}, nil
}

func (c Capacity) Validate() error {
	if c.value <= 0 {
		return errors.New("workbench capacity is not positive")
	}
	return nil
}

func resolveCapacity(configured *Capacity, fallback int) (int, error) {
	if configured == nil {
		defaultCapacity, err := newCapacity(fallback)
		if err != nil {
			return 0, err
		}
		return defaultCapacity.value, nil
	}
	if err := configured.Validate(); err != nil {
		return 0, err
	}
	return configured.value, nil
}

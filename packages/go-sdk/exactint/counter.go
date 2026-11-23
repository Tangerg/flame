package exactint

import "errors"

const Maximum = 1<<53 - 1

var (
	ErrOutOfRange   = errors.New("exact integer counter is outside the supported range")
	ErrExhausted    = errors.New("exact integer counter is exhausted")
	ErrNotSuccessor = errors.New("exact integer counter does not advance exactly once")
)

type Counter struct{ value uint64 }

func Restore(value uint64) (Counter, error) {
	if value > Maximum {
		return Counter{}, ErrOutOfRange
	}
	return Counter{value: value}, nil
}

func First() Counter { return Counter{value: 1} }

func (c Counter) Value() uint64 { return c.value }

func (c Counter) IsZero() bool { return c.value == 0 }

func (c Counter) Next() (Counter, error) {
	if c.value > Maximum {
		return Counter{}, ErrOutOfRange
	}
	if c.value == Maximum {
		return Counter{}, ErrExhausted
	}
	return Counter{value: c.value + 1}, nil
}

func Follows(previous, next uint64) error {
	current, err := Restore(previous)
	if err != nil {
		return err
	}
	want, err := current.Next()
	if err != nil {
		return err
	}
	actual, err := Restore(next)
	if err != nil {
		return err
	}
	if actual != want {
		return ErrNotSuccessor
	}
	return nil
}

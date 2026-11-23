package plan

import "fmt"

type Replacement struct {
	expected Version
	state    State
}

func NewReplacement(expected Version, state State) (Replacement, error) {
	replacement := Replacement{expected: expected, state: state}
	if err := replacement.Validate(); err != nil {
		return Replacement{}, err
	}
	return replacement, nil
}

func (r Replacement) ExpectedVersion() Version { return r.expected }

func (r Replacement) State() State { return r.state }

func (r Replacement) Validate() error {
	if err := r.expected.AdvancesTo(r.state); err != nil {
		return fmt.Errorf("plan: invalid replacement: %w", err)
	}
	return nil
}

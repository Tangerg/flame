package goal

import "fmt"

type Replacement struct {
	expected Version
	state    Goal
}

func NewReplacement(expected Version, state Goal) (Replacement, error) {
	replacement := Replacement{expected: expected, state: state.Clone()}
	if err := replacement.validate(); err != nil {
		return Replacement{}, err
	}
	return replacement, nil
}

func (r Replacement) ExpectedVersion() Version { return r.expected }

func (r Replacement) State() Goal { return r.state.Clone() }

func (r Replacement) validate() error {
	if err := r.expected.AdvancesTo(r.state); err != nil {
		return fmt.Errorf("goal: invalid replacement: %w", err)
	}
	return nil
}

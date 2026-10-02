package approval

import (
	"fmt"
	"path"
)

type SubjectType string

const (
	SubjectAll   SubjectType = "all"
	SubjectExact SubjectType = "exact"
	SubjectGlob  SubjectType = "glob"
)

// Subject keeps the caller's literal command or path distinct from an explicitly
// authored pattern. Metacharacters in a remembered invocation never widen it.
type Subject struct {
	Type  SubjectType
	Value string
}

func InvocationSubject(value string) Subject {
	if value == "" {
		return Subject{Type: SubjectAll}
	}
	return Subject{Type: SubjectExact, Value: value}
}

func (s Subject) Validate() error {
	switch s.Type {
	case SubjectAll:
		if s.Value != "" {
			return fmt.Errorf("%w: whole-tool subject cannot carry a value", ErrInvalidRule)
		}
	case SubjectExact, SubjectGlob:
		if s.Value == "" {
			return fmt.Errorf("%w: %s subject requires a value", ErrInvalidRule, s.Type)
		}
		if s.Type == SubjectGlob {
			if _, err := path.Match(s.Value, ""); err != nil {
				return fmt.Errorf("%w: invalid subject glob: %w", ErrInvalidRule, err)
			}
		}
	default:
		return fmt.Errorf("%w: unknown subject type %q", ErrInvalidRule, s.Type)
	}
	return nil
}

func (s Subject) matches(value string) bool {
	switch s.Type {
	case SubjectAll:
		return true
	case SubjectExact:
		return s.Value == value
	case SubjectGlob:
		// path.Match's * does not cross /; ** has no special meaning.
		ok, err := path.Match(s.Value, value)
		return err == nil && ok
	default:
		return false
	}
}

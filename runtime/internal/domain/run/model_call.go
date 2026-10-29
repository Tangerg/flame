package run

import "fmt"

// ModelCallPurpose distinguishes conversation authority from independent consumption.
type ModelCallPurpose uint8

const (
	ModelConversation ModelCallPurpose = iota
	ModelIndependent
)

func (p ModelCallPurpose) Validate() error {
	if p != ModelConversation && p != ModelIndependent {
		return fmt.Errorf("run: invalid model call purpose %d", p)
	}
	return nil
}
func (p ModelCallPurpose) String() string {
	switch p {
	case ModelConversation:
		return "conversation"
	case ModelIndependent:
		return "independent"
	default:
		return ""
	}
}
func ParseModelCallPurpose(value string) (ModelCallPurpose, error) {
	switch value {
	case ModelConversation.String():
		return ModelConversation, nil
	case ModelIndependent.String():
		return ModelIndependent, nil
	default:
		return 0, fmt.Errorf("run: invalid model call purpose %q", value)
	}
}

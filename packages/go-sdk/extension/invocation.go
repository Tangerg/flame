package extension

import (
	"errors"

	framecall "github.com/Tangerg/frame/call"
)

// Flame owns the closed product origin and target vocabulary.
const (
	Human             framecall.Origin = "human"
	Model             framecall.Origin = "model"
	Automatic         framecall.Origin = "automatic"
	ScopeCall                          = "call"
	ScopeInstallation                  = "installation"
)

// ValidateInvocation admits Flame's product evidence. Frame accepts other
// consumers' origin vocabularies without granting them Flame authority.
func ValidateInvocation(invocation framecall.Invocation) error {
	if err := invocation.Validate(); err != nil {
		return err
	}
	switch invocation.Origin {
	case Human, Model, Automatic:
		return nil
	default:
		return errors.New("extension: unknown Flame invocation origin")
	}
}

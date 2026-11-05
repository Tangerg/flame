package extension

import (
	"errors"

	framemcp "github.com/Tangerg/frame/process/mcp"
)

// Flame owns the closed product origin and target vocabulary.
const (
	Human             framemcp.Origin = "human"
	Model             framemcp.Origin = "model"
	Automatic         framemcp.Origin = "automatic"
	ScopeCall                         = "call"
	ScopeInstallation                 = "installation"
)

// ValidateInvocation admits Flame's product evidence. Frame accepts other
// consumers' origin vocabularies without granting them Flame authority.
func ValidateInvocation(invocation framemcp.Invocation) error {
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

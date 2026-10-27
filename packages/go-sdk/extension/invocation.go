package extension

import (
	"errors"

	"github.com/Tangerg/flame/qimu/portable"
)

// Flame owns the closed product origin and target vocabulary.
const (
	Human             portable.Origin = "human"
	Model             portable.Origin = "model"
	Automatic         portable.Origin = "automatic"
	ScopeCall                         = "call"
	ScopeInstallation                 = "installation"
)

// ValidateInvocation admits Flame's product evidence. Qimu accepts other
// consumers' origin vocabularies without granting them Flame authority.
func ValidateInvocation(invocation portable.Invocation) error {
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

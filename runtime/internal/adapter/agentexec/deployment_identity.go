package agentexec

import (
	"fmt"

	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

const maximumDeploymentIdentityCharacters = 256

// deploymentIdentity is one exact, bounded input to an Agent Framework
// deployment digest. It stays private because agentexec is its only owner and
// consumer; implementation and configuration are distinguished by their
// fields and digest positions, not by duplicate exported wrapper types.
type deploymentIdentity struct {
	text string
}

func parseDeploymentIdentity(kind, text string) (deploymentIdentity, error) {
	if err := runtimeidentity.ValidateText(text, maximumDeploymentIdentityCharacters); err != nil {
		return deploymentIdentity{}, fmt.Errorf("%s %w", kind, err)
	}
	return deploymentIdentity{text: text}, nil
}

func (i deploymentIdentity) String() string { return i.text }

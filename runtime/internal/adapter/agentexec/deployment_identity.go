package agentexec

import (
	"fmt"
	"unicode"
	"unicode/utf8"
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
	if text == "" {
		return deploymentIdentity{}, fmt.Errorf("%s is empty", kind)
	}
	if !utf8.ValidString(text) {
		return deploymentIdentity{}, fmt.Errorf("%s is not valid UTF-8", kind)
	}
	if characters := utf8.RuneCountInString(text); characters > maximumDeploymentIdentityCharacters {
		return deploymentIdentity{}, fmt.Errorf(
			"%s has %d characters, maximum is %d", kind, characters, maximumDeploymentIdentityCharacters,
		)
	}
	for _, character := range text {
		if unicode.IsSpace(character) || !unicode.IsPrint(character) {
			return deploymentIdentity{}, fmt.Errorf("%s contains whitespace or a non-printing character", kind)
		}
	}
	return deploymentIdentity{text: text}, nil
}

func (i deploymentIdentity) String() string { return i.text }

package mutation

import (
	"crypto/rand"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

// NewCommandID allocates a fresh mutation intent. Callers retain it for every
// retry of that intent; only a new intent receives a new identity.
func NewCommandID() agent.CommandID {
	var entropy [agent.CommandIDEntropyBytes]byte
	rand.Read(entropy[:])
	return agent.NewCommandID(entropy)
}

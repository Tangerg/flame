package mutation

import (
	"crypto/rand"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
)

// NewCommandID allocates a fresh mutation intent. Callers retain it for every
// retry of that intent; only a new intent receives a new identity.
func NewCommandID() authoring.CommandID {
	var entropy [authoring.CommandIDEntropyBytes]byte
	rand.Read(entropy[:])
	return authoring.NewCommandID(entropy)
}

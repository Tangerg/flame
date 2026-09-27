package mutation

import (
	"crypto/rand"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
)

// NewCommandID allocates a fresh mutation intent. Callers retain it for every
// retry of that intent; only a new intent receives a new identity.
func NewCommandID() replay.CommandID {
	var entropy [replay.CommandIDEntropyBytes]byte
	rand.Read(entropy[:])
	return replay.NewCommandID(entropy)
}

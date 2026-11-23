package mutation

import (
	"crypto/rand"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
)

func NewCommandID() authoring.CommandID {
	var entropy [authoring.CommandIDEntropyBytes]byte
	rand.Read(entropy[:])
	return authoring.NewCommandID(entropy)
}

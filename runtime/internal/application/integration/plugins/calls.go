package plugins

import (
	"context"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/jsoninput"
)

type CallRequest struct {
	Installation resourceid.InstallationID
	Digest       fingerprint.Digest
	Contribution plugin.ContributionID
	Contract     *capability.Contract
	Method       string
	Input        jsoninput.Input
}

// Call retains an admitted consumer contract until its concrete invocation ends.
// Preparation performs no backend I/O or business effect.
type Call interface {
	Kind() capability.MethodKind
	Invoke(context.Context) (jsoninput.Input, error)
}

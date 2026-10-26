package plugins

import (
	"context"
	"iter"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/fingerprint"
	"github.com/Tangerg/flame/packages/go-sdk/jsoninput"
	"github.com/Tangerg/flame/packages/go-sdk/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type CallRequest struct {
	Installation resourceid.InstallationID
	Digest       fingerprint.Digest
	Contribution plugin.ContributionID
	Contract     *capability.Contract
	Method       string
	Input        jsoninput.Input
	Target       capability.Target
}

// Call retains an admitted consumer contract until its concrete invocation ends.
// Preparation performs no backend I/O or business effect.
type Call interface {
	Kind() capability.MethodKind
	Invoke(context.Context) (jsoninput.Input, error)
}

type StreamKind string

const (
	StreamChunk  StreamKind = "chunk"
	StreamResult StreamKind = "result"
)

type StreamEvent struct {
	Kind  StreamKind
	Value jsoninput.Input
}

type StreamCall interface {
	Kind() capability.MethodKind
	Stream(context.Context) iter.Seq2[StreamEvent, error]
}

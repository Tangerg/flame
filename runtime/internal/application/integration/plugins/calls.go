package plugins

import (
	"context"
	"iter"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type CallRequest struct {
	Installation identity.InstallationID
	Digest       identity.Digest
	Contribution plugin.ContributionID
	Contract     *extension.Capability
	Method       string
	Input        jsonvalue.Value
	Target       extension.Target
}

// Call retains an admitted consumer contract until its concrete invocation ends.
// Preparation performs no backend I/O or business effect.
type Call interface {
	Kind() extension.MethodKind
	Invoke(context.Context) (jsonvalue.Value, error)
}

type StreamKind string

const (
	StreamChunk  StreamKind = "chunk"
	StreamResult StreamKind = "result"
)

type StreamEvent struct {
	Kind  StreamKind
	Value jsonvalue.Value
}

type StreamCall interface {
	Kind() extension.MethodKind
	Stream(context.Context) iter.Seq2[StreamEvent, error]
}

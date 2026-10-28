package plugins

import (
	"context"
	"iter"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type CallRequest struct {
	Installation identity.InstallationID
	Digest       identity.Digest
	Contribution plugin.ContributionID
	Contract     *portable.Capability
	Method       string
	Input        jsonvalue.Value
	Target       portable.Target
}

// Call retains an admitted consumer contract until its concrete invocation ends.
// Preparation performs no backend I/O or business effect.
type Call interface {
	Kind() portable.MethodKind
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
	Kind() portable.MethodKind
	Stream(context.Context) iter.Seq2[StreamEvent, error]
}

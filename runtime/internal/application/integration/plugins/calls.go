package plugins

import (
	"context"
	"iter"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/frame/portable"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type CallRequest struct {
	Installation identity.InstallationID
	Digest       framevalue.Digest
	Contribution plugin.ContributionID
	Contract     *plugincontract.Capability
	Method       string
	Input        framevalue.JSON
	Target       portable.Target
}

// Call retains an admitted consumer contract until its concrete invocation ends.
// Preparation performs no backend I/O or business effect.
type Call interface {
	Kind() portable.MethodKind
	Invoke(context.Context) (framevalue.JSON, error)
}

type StreamKind string

const (
	StreamChunk  StreamKind = "chunk"
	StreamResult StreamKind = "result"
)

type StreamEvent struct {
	Kind  StreamKind
	Value framevalue.JSON
}

type StreamCall interface {
	Kind() portable.MethodKind
	Stream(context.Context) iter.Seq2[StreamEvent, error]
}

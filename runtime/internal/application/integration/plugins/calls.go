package plugins

import (
	"context"
	"iter"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	"github.com/Tangerg/frame"
	framemcp "github.com/Tangerg/frame/process/mcp"
	framevalue "github.com/Tangerg/frame/value"
)

type CallRequest struct {
	Installation identity.InstallationID
	Digest       framevalue.Digest
	Contribution plugin.ContributionID
	Contract     *plugincontract.Capability
	Method       string
	Input        framevalue.JSON
	Target       framemcp.Target
}

// Call retains an admitted consumer contract until its concrete invocation ends.
// Preparation performs no backend I/O or business effect.
type Call interface {
	Kind() frame.MethodKind
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
	Kind() frame.MethodKind
	Stream(context.Context) iter.Seq2[StreamEvent, error]
}

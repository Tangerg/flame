package execution

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/samber/lo"
)

// toolResultOffloader is the narrow write capability the observer needs to
// persist a body after its candidate preview has proven worth evicting. nil
// disables eviction.
type toolResultOffloader interface {
	Stage(ctx context.Context, stage transcript.ToolResultStage) error
}

// toolResultPreviewBytes bounds the head+tail preview left inline once a body is
// offloaded, so the candidate preview keeps at most this many body bytes plus
// the retrieval marker. The observer rejects the candidate if that fixed marker
// makes it no smaller than the original body.
const toolResultPreviewBytes = 2000

// ToolResultOffload evicts Tool results longer than Threshold bytes into Store
// behind a bounded preview. A nil offload disables eviction.
type ToolResultOffload struct {
	Store     toolResultOffloader
	Threshold int
}

// toolResultReaderName is the one Tool that reads an offloaded body back; its
// own output is never offloaded again.
const toolResultReaderName = string(builtincontract.ReadToolResult)

// toolResultOffloadPolicy is enabled exactly when it holds a store.
type toolResultOffloadPolicy struct {
	store     toolResultOffloader
	threshold int
}

type toolResultOffloadIdentity struct {
	Threshold  int    `json:"threshold"`
	ReaderName string `json:"readerName"`
}

func newToolResultOffloadPolicy(offload *ToolResultOffload) (toolResultOffloadPolicy, error) {
	if offload == nil {
		return toolResultOffloadPolicy{}, nil
	}
	if lo.IsNil(offload.Store) {
		return toolResultOffloadPolicy{}, errors.New("execution: Tool-result offload requires a store")
	}
	if offload.Threshold <= 0 {
		return toolResultOffloadPolicy{}, errors.New("execution: Tool-result offload threshold must be positive")
	}
	return toolResultOffloadPolicy{store: offload.Store, threshold: offload.Threshold}, nil
}

func (p toolResultOffloadPolicy) enabled() bool { return p.store != nil }

// evicts reports whether output from toolName is offloaded in sessionID.
func (p toolResultOffloadPolicy) evicts(sessionID, toolName string) bool {
	return p.enabled() && sessionID != "" && toolName != toolResultReaderName
}

func (p toolResultOffloadPolicy) identity() *toolResultOffloadIdentity {
	if !p.enabled() {
		return nil
	}
	return &toolResultOffloadIdentity{Threshold: p.threshold, ReaderName: toolResultReaderName}
}

func evictToolResult(
	ctx context.Context,
	policy toolResultOffloadPolicy,
	sessionID string,
	toolName string,
	output string,
) (string, *transcript.ToolResultRef) {
	if !policy.evicts(sessionID, toolName) || len(output) <= policy.threshold {
		return output, nil
	}
	id := transcript.ToolResultID(rand.Text())
	preview := renderToolResultPreview(output, string(id), min(toolResultPreviewBytes, policy.threshold))
	if len(preview) >= len(output) {
		return output, nil
	}
	if err := policy.store.Stage(ctx, transcript.ToolResultStage{
		ID: id, SessionID: sessionID, Body: output,
	}); err != nil {
		slog.WarnContext(ctx, "execution: persist offloaded tool result", "session.id", sessionID, "tool.name", toolName, "error", err)
		return output, nil
	}
	return preview, &transcript.ToolResultRef{ID: id}
}

package agentexec

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/toolresult"
)

// toolResultOffloader is the narrow write capability the observer needs to
// persist a body after its candidate preview has proven worth evicting. nil
// disables eviction.
type toolResultOffloader interface {
	Stage(ctx context.Context, stage toolresult.Stage) error
}

// toolResultPreviewBytes bounds the head+tail preview left inline once a body is
// offloaded, so the candidate preview keeps at most this many body bytes plus
// the retrieval marker. The observer rejects the candidate if that fixed marker
// makes it no smaller than the original body.
const toolResultPreviewBytes = 2000

// ToolResultOffloadPolicyValues is the construction boundary for optional
// Tool-result eviction. An omitted threshold disables eviction; a present
// threshold must be positive and names the exact read-back Tool.
type ToolResultOffloadPolicyValues struct {
	Threshold  *int
	ReaderName string
}

type toolResultOffloadPolicy struct {
	enabled    bool
	threshold  int
	readerName string
}

type toolResultOffloadIdentity struct {
	Threshold  int    `json:"threshold"`
	ReaderName string `json:"readerName"`
}

func newToolResultOffloadPolicy(values ToolResultOffloadPolicyValues) (toolResultOffloadPolicy, error) {
	if values.Threshold == nil {
		if strings.TrimSpace(values.ReaderName) != "" {
			return toolResultOffloadPolicy{}, errors.New("agentexec: disabled Tool-result offload cannot name a reader Tool")
		}
		return toolResultOffloadPolicy{}, nil
	}
	if *values.Threshold <= 0 {
		return toolResultOffloadPolicy{}, errors.New("agentexec: Tool-result offload threshold must be positive")
	}
	if strings.TrimSpace(values.ReaderName) == "" || values.ReaderName != strings.TrimSpace(values.ReaderName) {
		return toolResultOffloadPolicy{}, errors.New("agentexec: Tool-result reader name is required without surrounding whitespace")
	}
	return toolResultOffloadPolicy{enabled: true, threshold: *values.Threshold, readerName: values.ReaderName}, nil
}

func (p toolResultOffloadPolicy) identity() *toolResultOffloadIdentity {
	if !p.enabled {
		return nil
	}
	return &toolResultOffloadIdentity{Threshold: p.threshold, ReaderName: p.readerName}
}

func evictToolResult(
	ctx context.Context,
	store toolResultOffloader,
	policy toolResultOffloadPolicy,
	sessionID string,
	toolName string,
	output string,
) (string, *toolresult.Ref) {
	if store == nil || !policy.enabled || len(output) <= policy.threshold ||
		toolName == policy.readerName || sessionID == "" {
		return output, nil
	}
	id := toolresult.NewID()
	preview := renderToolResultPreview(
		output,
		string(id),
		policy.readerName,
		min(toolResultPreviewBytes, policy.threshold),
	)
	if len(preview) >= len(output) {
		return output, nil
	}
	if err := store.Stage(ctx, toolresult.Stage{
		ID: id, SessionID: sessionID, ToolName: toolName, Body: output,
	}); err != nil {
		return output, nil
	}
	return preview, &toolresult.Ref{ID: id}
}

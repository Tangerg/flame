package agentexec

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	corechat "github.com/Tangerg/scope/core/chat"
)

// CompactionResult reports one completed Run-boundary compaction sweep.
type CompactionResult struct {
	summary        string
	messagesBefore int
	messagesAfter  int
}

// NewCompactionResult constructs an observable summary compaction. The zero
// value means that no summary rewrite occurred.
func NewCompactionResult(summary string, messagesBefore, messagesAfter int) (CompactionResult, error) {
	canonicalSummary := strings.TrimSpace(summary)
	if canonicalSummary == "" {
		return CompactionResult{}, fmt.Errorf("agentexec: compaction summary is empty")
	}
	if canonicalSummary != summary {
		return CompactionResult{}, fmt.Errorf("agentexec: compaction summary is not canonical")
	}
	if messagesBefore <= 0 || messagesAfter <= 0 {
		return CompactionResult{}, fmt.Errorf(
			"agentexec: invalid compaction message counts %d -> %d",
			messagesBefore,
			messagesAfter,
		)
	}
	return CompactionResult{
		summary:        canonicalSummary,
		messagesBefore: messagesBefore,
		messagesAfter:  messagesAfter,
	}, nil
}

func (r CompactionResult) Compacted() bool {
	return r.summary != ""
}

func (r CompactionResult) Summary() string {
	return r.summary
}

func (r CompactionResult) MessageCounts() (before, after int) {
	return r.messagesBefore, r.messagesAfter
}

// RunMaintenance owns best-effort housekeeping after a clean Interaction but
// before its terminal fact closes the live Segment stream.
type RunMaintenance interface {
	Maintain(ctx context.Context, input RunMaintenanceInput) RunMaintenanceResult
}

// RunMaintenanceInput is the finished root Interaction's maintenance context.
type RunMaintenanceInput struct {
	SessionID      string
	CWD            string
	ModelSelection modelref.Selection
	Options        corechat.Options
	ToolCalls      int
	PreCompact     func(context.Context) bool
}

// RunMaintenanceResult reports independent best-effort failures without
// rewriting the already-produced assistant response.
type RunMaintenanceResult struct {
	Compaction CompactionResult
	Errors     []error
}

// InteractionLifecycleHooks owns the Runtime lifecycle events that are not
// part of Tool authorization or prompt composition.
type InteractionLifecycleHooks interface {
	BeforeCompaction(ctx context.Context, sessionID, cwd string) bool
	NotifyWaiting(ctx context.Context, sessionID, cwd string)
	NotifyStopped(ctx context.Context, sessionID, cwd, reason string)
}

func (i *interactionSession) maintainCompletedRoot() {
	if i.maintenance == nil || i.start.SessionID == "" {
		return
	}
	toolCalls := i.accounting.toolCallCount()
	preCompact := func(ctx context.Context) bool {
		return i.lifecycleHooks == nil || i.lifecycleHooks.BeforeCompaction(
			ctx, i.start.SessionID, i.start.CWD,
		)
	}
	result := i.maintenance.Maintain(i.lifetime.execution, RunMaintenanceInput{
		SessionID:      i.start.SessionID,
		CWD:            i.start.CWD,
		ModelSelection: i.start.ModelSelection,
		Options:        executionOptions(i.start.ModelSelection, i.start.Options),
		ToolCalls:      toolCalls,
		PreCompact:     preCompact,
	})
	for _, err := range result.Errors {
		if err != nil {
			trace.SpanFromContext(i.lifetime.execution).RecordError(
				fmt.Errorf("agentexec: Run maintenance: %w", err),
			)
		}
	}
	if result.Compaction.Compacted() {
		messagesBefore, messagesAfter := result.Compaction.MessageCounts()
		i.lifetime.send(runs.ExecutorEvent{
			Member: runs.ExecutorMember{MemberID: i.processRootID().String()},
			Payload: runs.CompactionBoundary{
				Summary:        result.Compaction.Summary(),
				MessagesBefore: messagesBefore,
				MessagesAfter:  messagesAfter,
			},
		})
	}
}

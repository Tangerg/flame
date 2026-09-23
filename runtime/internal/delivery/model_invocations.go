package delivery

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/protocol"
)

func (s *Handler) ListModelInvocations(ctx context.Context, in protocol.ListModelInvocationsRequest) (*protocol.Page[protocol.ModelInvocation], error) {
	if _, err := s.GetRun(ctx, protocol.GetRunRequest{RunID: in.RunID}); err != nil {
		return nil, err
	}
	limit, err := requestedPageLimit(in.Limit)
	if err != nil {
		return nil, wirePageError(err)
	}
	page, err := s.queries.ListModelInvocationPage(ctx, in.RunID, in.Cursor, limit)
	if err != nil {
		return nil, wirePageError(err)
	}
	rows := make([]protocol.ModelInvocation, len(page.Rows))
	for index, row := range page.Rows {
		rows[index] = protocol.ModelInvocation{FirstOutputLatencyMillis: row.FirstOutputLatencyMillis, CallID: row.CallID, RunID: in.RunID, SegmentID: row.SegmentID, State: protocol.ModelInvocationState(row.State), StartedAt: row.StartedAt, SettledAt: row.FinishedAt}
		if usage := row.Usage; usage != nil {
			owned := accounting.CloneReportedUsage(*usage)
			rows[index].Usage = &protocol.ModelInvocationUsage{
				InputTokens: owned.InputTokens, OutputTokens: owned.OutputTokens,
				CacheReadTokens: owned.CacheReadInputTokens, CacheWriteTokens: owned.CacheWriteInputTokens,
				ReasoningTokens: owned.ReasoningTokens,
			}
		}
	}
	return protocol.NewPageWithCursor(rows, page.NextCursor), nil
}

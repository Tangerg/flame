package delivery

import (
	"context"
	json "encoding/json/v2"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
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
		var err error
		rows[index], err = presentModelInvocation(in.RunID, row)
		if err != nil {
			return nil, err
		}
	}
	return protocol.NewPageWithCursor(rows, page.NextCursor), nil
}

func presentModelInvocation(runID string, row runs.ModelInvocationCommit) (protocol.ModelInvocation, error) {
	out := protocol.ModelInvocation{FirstOutputLatencyMillis: row.FirstOutputLatencyMillis, CallID: row.CallID, RunID: runID, SegmentID: row.SegmentID, State: protocol.ModelInvocationState(row.State), StartedAt: row.StartedAt, SettledAt: row.FinishedAt}
	if row.KnownResponse != nil {
		encoded, err := json.Marshal(row.KnownResponse)
		if err != nil {
			return protocol.ModelInvocation{}, err
		}
		out.KnownResponse = string(encoded)
	}
	if usage := row.Usage; usage != nil {
		owned := accounting.CloneReportedUsage(*usage)
		out.Usage = &protocol.ModelInvocationUsage{
			InputTokens: owned.InputTokens, OutputTokens: owned.OutputTokens,
			CacheReadTokens: owned.CacheReadInputTokens, CacheWriteTokens: owned.CacheWriteInputTokens,
			ReasoningTokens: owned.ReasoningTokens,
		}
	}
	return out, nil
}

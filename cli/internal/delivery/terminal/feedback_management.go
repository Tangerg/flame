package terminal

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/runtime/protocol"
)

func (a *app) RecordFeedback(argument string) error {
	if a.feedback == nil {
		return errors.New("this runtime composition has no feedback service")
	}
	ratingText, note, _ := strings.Cut(strings.TrimSpace(argument), " ")
	rating, err := parseFeedbackRating(ratingText)
	if err != nil {
		return errors.New("usage: /feedback <positive|negative> [note]")
	}
	runID, itemID := latestAssistantTarget(a.execution.conversation.Blocks())
	request := protocol.FeedbackRequest{
		SessionID: a.session.current.ID, RunID: runID, ItemID: itemID,
		Rating: rating, Text: strings.TrimSpace(note),
	}
	if err := request.ValidateWire(); err != nil {
		return err
	}
	a.status.note("recording feedback")
	if !a.runApplicationOperation(feedbackOperation, false,
		func(ctx context.Context) (protocol.FeedbackRequest, error) {
			return request, a.feedback.Record(ctx, request)
		},
		func(recorded protocol.FeedbackRequest, err error) {
			if err != nil {
				a.message("record feedback failed: " + err.Error())
				return
			}
			target := "session"
			if recorded.ItemID != "" {
				target = "assistant item " + shortIdentity(recorded.ItemID)
			} else if recorded.RunID != "" {
				target = "run " + shortIdentity(recorded.RunID)
			}
			a.message("feedback recorded · " + string(recorded.Rating) + " · " + target)
		},
	) {
		return errors.New("another feedback operation is running")
	}
	return nil
}

func parseFeedbackRating(value string) (protocol.FeedbackRating, error) {
	rating := protocol.FeedbackRating(strings.TrimSpace(value))
	if err := (protocol.FeedbackRequest{Rating: rating}).ValidateWire(); err != nil {
		return "", err
	}
	return rating, nil
}

func latestAssistantTarget(blocks []agent.Block) (string, string) {
	for index := len(blocks) - 1; index >= 0; index-- {
		block := blocks[index]
		if block.Kind == agent.BlockAssistant && block.Status != agent.BlockStatusRunning {
			return block.RunID, block.ID
		}
	}
	return "", ""
}

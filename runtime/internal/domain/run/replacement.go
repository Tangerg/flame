package run

import (
	"errors"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
)

type Replacement struct {
	expected Run
	state    Run
}

func (r Run) Replace() Replacement {
	return Replacement{expected: r, state: r}
}

func (r Replacement) advance(state Run, err error) (Replacement, error) {
	if err != nil {
		return Replacement{}, err
	}
	return Replacement{expected: r.expected, state: state}, nil
}

func (r Replacement) AdvanceProgress(metrics Metrics, contextTokens int64, updatedAt time.Time) (Replacement, error) {
	return r.advance(r.state.AdvanceProgress(metrics, contextTokens, updatedAt))
}

func (r Replacement) Resume(segmentID identity.SegmentID, resumedAt time.Time) (Replacement, error) {
	return r.advance(r.state.Resume(segmentID, resumedAt))
}

func (r Replacement) CancelWaiting(detail string, finishedAt time.Time, messageMark MessageMark) (Replacement, error) {
	return r.advance(r.state.CancelWaiting(detail, finishedAt, messageMark))
}

func (r Replacement) RecoverLost(loss Loss, finishedAt time.Time, messageMark MessageMark) (Replacement, error) {
	return r.advance(r.state.RecoverLost(loss, finishedAt, messageMark))
}

func (r Replacement) RebaseConversation(compaction conversation.Compaction) (Replacement, error) {
	return r.advance(r.state.RebaseConversation(compaction))
}

func (r Replacement) WithMessageMark(messageMark int) (Replacement, error) {
	return r.advance(r.state.WithMessageMark(messageMark))
}

func (r Replacement) Validate() error {
	if r.expected.ID().IsZero() {
		return errors.New("run: replacement carries no Run")
	}
	return nil
}

func (r Replacement) Expected() Run { return r.expected }

func (r Replacement) State() Run { return r.state }

package conversation

import (
	"iter"
	"slices"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type EventStream = iter.Seq2[RunEvent, error]

// Event IDs are opaque tokens scoped to SegmentID; callers may return them but
// must never parse or order them.
type SegmentStream struct {
	RunID       string
	SegmentID   string
	UserItemID  string
	HeadEventID string
	Snapshot    *SessionSnapshot
	Events      EventStream
}

type SubscribeRun struct {
	SessionID    string
	RunID        string
	SegmentID    string
	AfterEventID string
	Snapshot     bool
}

type InterruptAnswer struct {
	ItemID string
	Answer Answer
}

type ResumeRun struct {
	CommandID authoring.CommandID
	RunID     string
	Answers   []InterruptAnswer
	Message   *authoring.Message
	Input     []protocol.ContentBlock
}

func (r ResumeRun) Equal(other ResumeRun) bool {
	if r.CommandID != other.CommandID || r.RunID != other.RunID || (r.Message == nil) != (other.Message == nil) || !slices.Equal(r.Input, other.Input) {
		return false
	}
	if r.Message != nil && !r.Message.Equal(*other.Message) {
		return false
	}
	return slices.EqualFunc(r.Answers, other.Answers, func(left, right InterruptAnswer) bool {
		return left.ItemID == right.ItemID && answerEqual(left.Answer, right.Answer)
	})
}

func (r ResumeRun) Clone() ResumeRun {
	r.Input = slices.Clone(r.Input)
	answers := r.Answers
	r.Answers = make([]InterruptAnswer, len(answers))
	for index, response := range answers {
		r.Answers[index] = InterruptAnswer{
			ItemID: response.ItemID,
			Answer: CloneAnswer(response.Answer),
		}
	}
	if r.Message != nil {
		message := r.Message.Clone()
		r.Message = &message
	}
	return r
}

type CancelRun struct {
	CommandID authoring.CommandID
	RunID     string
	Reason    string
}

package agent

import (
	"iter"
	"slices"
)

// EventStream is the ordered event stream of one run segment. A stream yields
// at most one non-nil error and then stops.
type EventStream = iter.Seq2[RunEvent, error]

// SegmentStream is an opened segment together with its event stream. Event IDs
// are opaque tokens scoped to SegmentID; callers may retain and return them but
// must never parse or order them.
type SegmentStream struct {
	RunID       string
	SegmentID   string
	UserItemID  string
	HeadEventID string
	Events      EventStream
}

type StartRun struct {
	CommandID CommandID
	SessionID string
	Message   Message
	Options   RunOptions
}

func (s StartRun) Clone() StartRun {
	s.Message = s.Message.Clone()
	s.Options = s.Options.Clone()
	return s
}

func (s StartRun) Equal(other StartRun) bool {
	return s.CommandID == other.CommandID && s.SessionID == other.SessionID &&
		s.Message.Equal(other.Message) && s.Options.Equal(other.Options)
}

// SubscribeRun rebinds one exact segment. AfterEventID is an opaque checkpoint
// previously accepted from that segment. Empty means attach at its current head.
type SubscribeRun struct {
	RunID        string
	SegmentID    string
	AfterEventID string
}

// InterruptAnswer pairs a response with the pending item it answers. A resume
// consumes the complete waiting set atomically.
type InterruptAnswer struct {
	ItemID string
	Answer Answer
}

type ResumeRun struct {
	CommandID CommandID
	RunID     string
	Answers   []InterruptAnswer
	Message   *Message
}

// Equal reports whether two resume commands carry the same complete decision
// set. Answer order is semantic because the command consumes the runtime's
// ordered interaction set atomically.
func (r ResumeRun) Equal(other ResumeRun) bool {
	if r.CommandID != other.CommandID || r.RunID != other.RunID || (r.Message == nil) != (other.Message == nil) {
		return false
	}
	if r.Message != nil && !r.Message.Equal(*other.Message) {
		return false
	}
	return slices.EqualFunc(r.Answers, other.Answers, func(left, right InterruptAnswer) bool {
		return left.ItemID == right.ItemID && AnswerEqual(left.Answer, right.Answer)
	})
}

// Clone detaches every mutable answer and optional message owned by a resume
// command. Delivery adapters may retain the clone across retries or process
// restarts without sharing the interaction editor's draft state.
func (r ResumeRun) Clone() ResumeRun {
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
	CommandID CommandID
	RunID     string
	Reason    string
}

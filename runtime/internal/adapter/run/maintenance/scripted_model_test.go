package maintenance

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Tangerg/scope/core/chat"
)

type scriptedReply struct {
	text string
	err  error
}

type scriptedModel struct {
	mu       sync.Mutex
	replies  []scriptedReply
	requests []*chat.Request
}

func durationPointer(value time.Duration) *time.Duration { return &value }

func (s *scriptedModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, request)
	if len(s.replies) == 0 {
		return nil, errors.New("scripted model exhausted")
	}
	reply := s.replies[0]
	s.replies = s.replies[1:]
	if reply.err != nil {
		return nil, reply.err
	}
	message := chat.NewAssistantMessage(chat.NewTextPart(reply.text))
	return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
}

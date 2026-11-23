package scheduling

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/plugins/schedules/internal/schedule"
	framecall "github.com/Tangerg/frame/call"
	"github.com/samber/lo"
)

type WorkerStore interface {
	Due(context.Context, time.Time, int) ([]schedule.Schedule, error)
	Claim(context.Context, schedule.Claim) (bool, error)
	Pending(context.Context, time.Time, string, int) ([]schedule.Occurrence, error)
}

type ManualQueue interface {
	PendingManual(context.Context, identity.RunID, int) ([]schedule.RunRequest, error)
}

type Scanner struct {
	mu            sync.Mutex
	store         WorkerStore
	manual        ManualQueue
	afterManualID identity.RunID
	newIdentity   func() (identity.SessionID, identity.RunID, error)
	afterDue      time.Time
	afterID       string
}

func NewScanner(store WorkerStore, manual ManualQueue, newIdentity func() (identity.SessionID, identity.RunID, error)) (*Scanner, error) {
	for _, required := range []struct {
		name  string
		value any
	}{{"due store", store}, {"manual queue", manual}, {"Run identity", newIdentity}} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("schedules: scanner %s is required", required.name)
		}
	}
	return &Scanner{store: store, manual: manual, newIdentity: newIdentity}, nil
}

// Manual retries and pending occurrences take bounded shares of each dispatch
// batch, so newly due schedules always keep at least a quarter of it.
const (
	manualBatch  = schedule.MaximumDispatchBatch / 4
	pendingBatch = schedule.MaximumDispatchBatch / 2
)

func (s *Scanner) Scan(ctx context.Context, now time.Time, invocation framecall.Invocation) ([]schedule.RunRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := extension.ValidateInvocation(invocation); err != nil {
		return nil, err
	}
	manual, err := s.manual.PendingManual(ctx, s.afterManualID, manualBatch)
	if err != nil {
		return nil, err
	}
	if len(manual) < manualBatch {
		s.afterManualID = identity.RunID{}
	} else {
		s.afterManualID = manual[len(manual)-1].RunID()
	}
	pending, err := s.store.Pending(ctx, s.afterDue, s.afterID, pendingBatch)
	if err != nil {
		return nil, err
	}
	if len(pending) < pendingBatch {
		s.afterDue = time.Time{}
		s.afterID = ""
	} else {
		last := pending[len(pending)-1]
		s.afterDue = last.DueAt()
		s.afterID = last.ID()
	}
	requests := append([]schedule.RunRequest(nil), manual...)
	for _, occurrence := range pending {
		requests = append(requests, occurrence.RunRequest())
	}
	due, err := s.store.Due(ctx, now, schedule.MaximumDispatchBatch-len(requests))
	if err != nil {
		return nil, err
	}
	for _, scheduled := range due {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		sessionID, runID, err := s.newIdentity()
		if err != nil {
			return nil, err
		}
		claim, err := scheduled.Claim(sessionID, runID, now, invocation)
		if err != nil {
			return nil, err
		}
		claimed, err := s.store.Claim(ctx, claim)
		if err != nil {
			return nil, err
		}
		if claimed {
			requests = append(requests, claim.Occurrence().RunRequest())
		}
	}
	return requests, nil
}

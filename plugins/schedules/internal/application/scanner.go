package schedules

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/samber/lo"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
)

// Claim must persist both cursor and occurrence so returned work survives restart.
type WorkerStore interface {
	Due(context.Context, time.Time, int) ([]schedule.Schedule, error)
	Claim(context.Context, schedule.Claim) (bool, error)
	Pending(context.Context, time.Time, string, int) ([]schedule.Occurrence, error)
}

type ManualQueue interface {
	PendingManual(context.Context, string, int) ([]schedule.RunRequest, error)
}

// Process cursors rotate pending pages; losing a cursor cannot lose durably captured work.
type Scanner struct {
	mu                     sync.Mutex
	store                  WorkerStore
	manual                 ManualQueue
	afterManualID          string
	newSessionID, newRunID func() string
	afterDue               time.Time
	afterID                string
}

func NewScanner(store WorkerStore, manual ManualQueue, sessionID, runID func() string) (*Scanner, error) {
	for _, required := range []struct {
		name  string
		value any
	}{{"due store", store}, {"manual queue", manual}, {"Session identity", sessionID}, {"Run identity", runID}} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("schedules: scanner %s is required", required.name)
		}
	}
	return &Scanner{store: store, manual: manual, newSessionID: sessionID, newRunID: runID}, nil
}
func (s *Scanner) Scan(ctx context.Context, now time.Time, invocation extension.Invocation) ([]schedule.RunRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := invocation.Validate(); err != nil {
		return nil, err
	}
	manual, err := s.manual.PendingManual(ctx, s.afterManualID, schedule.MaximumDispatchBatch/4)
	if err != nil {
		return nil, err
	}
	if len(manual) < schedule.MaximumDispatchBatch/4 {
		s.afterManualID = ""
	} else {
		s.afterManualID = manual[len(manual)-1].RunID()
	}
	pending, err := s.store.Pending(ctx, s.afterDue, s.afterID, schedule.MaximumDispatchBatch/2)
	if err != nil {
		return nil, err
	}
	if len(pending) < schedule.MaximumDispatchBatch/2 {
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
		claim, err := scheduled.Claim(s.newSessionID(), s.newRunID(), now, invocation)
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

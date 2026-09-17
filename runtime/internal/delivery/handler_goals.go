package delivery

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/automation/goals"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/protocol"
)

// Goal operations drive an objective until completion, blocking, Run failure,
// or an explicit user stop.

type goalUseCases interface {
	Start(ctx context.Context, sessionID, objective string, selection modelref.Selection, capabilities run.Capabilities) (goal.Goal, error)
	UpdateObjective(ctx context.Context, sessionID, objective string, caller run.Capabilities) (goal.Goal, error)
	Clear(ctx context.Context, sessionID string) error
	Resume(ctx context.Context, sessionID string, caller run.Capabilities) (goal.Goal, error)
	Stop(ctx context.Context, sessionID string) (goal.Goal, error)
	Current(ctx context.Context, sessionID string) (goal.Goal, bool, error)
}

// UpdateGoal revises only the current objective (goals.update).
func (s *Handler) UpdateGoal(ctx context.Context, in protocol.UpdateGoalRequest) (*protocol.Goal, error) {
	caller, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	g, err := s.goals.UpdateObjective(ctx, in.SessionID, in.Objective, caller)
	if uncovered, ok := errors.AsType[*goals.InsufficientCapabilitiesError](err); ok {
		return nil, capabilityGap(uncovered.Missing)
	}
	if err != nil {
		return nil, mapGoalErr(err)
	}
	return presentGoal(g)
}

// ClearGoal removes the current objective and stops its drive (goals.clear).
func (s *Handler) ClearGoal(ctx context.Context, in protocol.GoalRequest) error {
	return mapGoalErr(s.goals.Clear(ctx, in.SessionID))
}

// StartGoal opens and begins driving a goal for the session (goals.start).
func (s *Handler) StartGoal(ctx context.Context, in protocol.StartGoalRequest) (*protocol.Goal, error) {
	selection, err := modelref.NewWithReasoningEffort(in.Provider, in.Model, in.ReasoningEffort)
	if err != nil {
		return nil, mapGoalErr(err)
	}
	capabilities, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	g, err := s.goals.Start(ctx, in.SessionID, in.Objective, selection, capabilities)
	if err != nil {
		return nil, mapGoalErr(err)
	}
	return presentGoal(g)
}

// GetGoal returns the session's goal, or a nil result when it has none (goals.get).
func (s *Handler) GetGoal(ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
	g, ok, err := s.goals.Current(ctx, in.SessionID)
	if err != nil {
		return nil, mapGoalErr(err)
	}
	if !ok {
		return nil, nil
	}
	return presentGoal(g)
}

// StopGoal pauses the session's goal and stops the loop (goals.stop).
func (s *Handler) StopGoal(ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
	g, err := s.goals.Stop(ctx, in.SessionID)
	if err != nil {
		return nil, mapGoalErr(err)
	}
	return presentGoal(g)
}

// ResumeGoal re-activates a paused or blocked goal (goals.resume).
func (s *Handler) ResumeGoal(ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
	caller, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	g, err := s.goals.Resume(ctx, in.SessionID, caller)
	if uncovered, ok := errors.AsType[*goals.InsufficientCapabilitiesError](err); ok {
		return nil, capabilityGap(uncovered.Missing)
	}
	if err != nil {
		return nil, mapGoalErr(err)
	}
	return presentGoal(g)
}

func mapGoalErr(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, goals.ErrGoalActive), errors.Is(err, goals.ErrGoalOwned):
		return fmt.Errorf("%w: a goal is already active for this session — stop it first", protocol.ErrSessionBusy)
	case errors.Is(err, goals.ErrNoGoal):
		return fmt.Errorf("%w: no goal for this session", protocol.ErrInvalidParams)
	case errors.Is(err, goal.ErrNotResumable):
		return fmt.Errorf("%w: this goal is not resumable", protocol.ErrInvalidParams)
	case errors.Is(err, goal.ErrNotEditable):
		return fmt.Errorf("%w: this goal is finishing and cannot be edited", protocol.ErrInvalidParams)
	case modelref.IsInvalid(err):
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	case errors.Is(err, goal.ErrInvalid):
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	case errors.Is(err, modelref.ErrUnsupported):
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	default:
		return err
	}
}

func presentGoal(g goal.Goal) (*protocol.Goal, error) {
	status, ok := presentGoalStatus(g.Status())
	if !ok {
		return nil, fmt.Errorf("goals: unsupported status %q", g.Status())
	}
	reason, err := presentGoalReason(g.Reason())
	if err != nil {
		return nil, err
	}
	selection, used := g.ModelSelection(), g.Used()
	w := protocol.Goal{
		SessionID:       g.SessionID(),
		Objective:       g.Objective(),
		Status:          status,
		Reason:          reason,
		Provider:        selection.Provider(),
		Model:           selection.Model(),
		ReasoningEffort: selection.ReasoningEffort(),
		Used:            protocol.GoalUsage{Runs: used.Runs, CostUSD: used.Cost.OptionalUSD(), Steps: used.Steps},
		CreatedAt:       g.CreatedAt(),
		UpdatedAt:       g.UpdatedAt(),
	}
	return &w, nil
}

func presentGoalStatus(status goal.Status) (protocol.GoalStatus, bool) {
	switch status {
	case goal.StatusActive:
		return protocol.GoalActive, true
	case goal.StatusPaused:
		return protocol.GoalPaused, true
	case goal.StatusBlocked:
		return protocol.GoalBlocked, true
	case goal.StatusComplete:
		return protocol.GoalCompleting, true
	default:
		return "", false
	}
}

func presentGoalReason(reason goal.Reason) (*protocol.GoalReason, error) {
	var code protocol.GoalReasonCode
	switch reason.Code() {
	case goal.ReasonNone:
		return nil, nil
	case goal.ReasonStoppedByUser:
		code = protocol.GoalReasonStoppedByUser
	case goal.ReasonRuntimeRestarted:
		code = protocol.GoalReasonRuntimeRestarted
	case goal.ReasonRunStartFailed:
		code = protocol.GoalReasonRunStartFailed
	case goal.ReasonAwaitingInput:
		code = protocol.GoalReasonAwaitingInput
	case goal.ReasonTerminalOutcomeMissing:
		code = protocol.GoalReasonTerminalOutcomeMissing
	case goal.ReasonRunNotCompleted:
		code = protocol.GoalReasonRunNotCompleted
	case goal.ReasonBlockedByModel:
		code = protocol.GoalReasonBlockedByModel
	default:
		return nil, fmt.Errorf("goals: unsupported reason code %q", reason.Code())
	}
	return &protocol.GoalReason{Code: code, Detail: reason.Detail()}, nil
}

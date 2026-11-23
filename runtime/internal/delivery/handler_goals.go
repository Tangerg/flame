package delivery

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/goals"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

func (s *Handler) UpdateGoal(ctx context.Context, in protocol.UpdateGoalRequest) (*protocol.Goal, error) {
	sessionID, parseErr := sessionParameter(in.SessionID)
	if parseErr != nil {
		return nil, parseErr
	}
	caller, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	g, err := s.goals.UpdateObjective(ctx, sessionID, in.Objective, caller)
	if uncovered, ok := errors.AsType[*run.InsufficientCapabilitiesError](err); ok {
		return nil, capabilityGap(uncovered.Missing)
	}
	if err != nil {
		return nil, mapGoalErr(err)
	}
	return presentGoal(g)
}

func (s *Handler) ClearGoal(ctx context.Context, in protocol.GoalRequest) error {
	sessionID, parseErr := sessionParameter(in.SessionID)
	if parseErr != nil {
		return parseErr
	}
	return mapGoalErr(s.goals.Clear(ctx, sessionID))
}

func (s *Handler) StartGoal(ctx context.Context, in protocol.StartGoalRequest) (*protocol.Goal, error) {
	sessionID, parseErr := sessionParameter(in.SessionID)
	if parseErr != nil {
		return nil, parseErr
	}
	selection, err := flamemodel.NewWithReasoningEffort(in.Provider, in.Model, in.ReasoningEffort)
	if err != nil {
		return nil, mapGoalErr(err)
	}
	capabilities, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	g, err := s.goals.Start(ctx, sessionID, in.Objective, selection, capabilities)
	if err != nil {
		return nil, mapGoalErr(err)
	}
	return presentGoal(g)
}

func (s *Handler) GetGoal(ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
	sessionID, parseErr := sessionParameter(in.SessionID)
	if parseErr != nil {
		return nil, parseErr
	}
	g, ok, err := s.goals.Current(ctx, sessionID)
	if err != nil {
		return nil, mapGoalErr(err)
	}
	if !ok {
		return nil, nil
	}
	return presentGoal(g)
}

func (s *Handler) StopGoal(ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
	sessionID, parseErr := sessionParameter(in.SessionID)
	if parseErr != nil {
		return nil, parseErr
	}
	g, err := s.goals.Stop(ctx, sessionID)
	if err != nil {
		return nil, mapGoalErr(err)
	}
	return presentGoal(g)
}

func (s *Handler) ResumeGoal(ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
	sessionID, parseErr := sessionParameter(in.SessionID)
	if parseErr != nil {
		return nil, parseErr
	}
	caller, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	g, err := s.goals.Resume(ctx, sessionID, caller)
	if uncovered, ok := errors.AsType[*run.InsufficientCapabilitiesError](err); ok {
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
	case errors.Is(err, plugin.ErrUnavailable):
		return wirePluginError(err)
	case errors.Is(err, goals.ErrGoalActive):
		return protocol.NewFailure(protocol.ErrSessionBusy, err.Error(), err)
	case errors.Is(err, goals.ErrGoalConflict):
		return protocol.NewFailure(protocol.ErrRevisionConflict, "the goal changed while the command was being prepared", err)
	case errors.Is(err, goals.ErrNoGoal),
		errors.Is(err, goal.ErrNotResumable),
		errors.Is(err, goal.ErrNotEditable),
		flamemodel.IsInvalid(err),
		errors.Is(err, goal.ErrInvalid),
		errors.Is(err, flamemodel.ErrUnsupported):
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	default:
		return mapModelError(err)
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
		SessionID:       g.SessionID().String(),
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
	if original := g.Snapshot().Author; original != (extension.ContributionSource{}) {
		w.Author = new(original)
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
	presented := &protocol.GoalReason{Code: code}
	if outcome, ok := reason.RunOutcome(); ok {
		presented.RunOutcome = protocol.RunOutcomeType(outcome)
	}
	if explanation, ok := reason.Explanation(); ok {
		presented.Explanation = explanation
	}
	return presented, nil
}

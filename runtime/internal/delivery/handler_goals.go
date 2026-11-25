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
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

func (s *Handler) UpdateGoal(ctx context.Context, in protocol.UpdateGoalRequest) (*protocol.Goal, error) {
	return applyGoalCommand(func(complete operation.Reply[goal.Goal]) error { return s.updateGoal(ctx, in, complete) })
}

func (s *Handler) commitUpdateGoal(ctx context.Context, in protocol.UpdateGoalRequest, outcome atomicOutcome) error {
	return outcome.settleRefusal(ctx, s.updateGoal(ctx, in, outcome.goal()))
}

func (s *Handler) updateGoal(ctx context.Context, in protocol.UpdateGoalRequest, complete operation.Reply[goal.Goal]) error {
	sessionID, parseErr := sessionParameter(in.SessionID)
	if parseErr != nil {
		return parseErr
	}
	caller, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return err
	}
	return mapGoalErr(s.goals.UpdateObjective(ctx, sessionID, in.Objective, caller, complete))
}

func (s *Handler) ClearGoal(ctx context.Context, in protocol.GoalRequest) error {
	return s.clearGoal(ctx, in, operation.DirectReply(new(struct{})))
}

func (s *Handler) commitClearGoal(ctx context.Context, in protocol.GoalRequest, outcome atomicOutcome) error {
	return outcome.settleRefusal(ctx, s.clearGoal(ctx, in, outcome.acknowledgement()))
}

func (s *Handler) clearGoal(ctx context.Context, in protocol.GoalRequest, complete operation.Reply[struct{}]) error {
	sessionID, parseErr := sessionParameter(in.SessionID)
	if parseErr != nil {
		return parseErr
	}
	return mapGoalErr(s.goals.Clear(ctx, sessionID, complete))
}

func (s *Handler) StartGoal(ctx context.Context, in protocol.StartGoalRequest) (*protocol.Goal, error) {
	return applyGoalCommand(func(complete operation.Reply[goal.Goal]) error { return s.startGoal(ctx, in, complete) })
}

func (s *Handler) commitStartGoal(ctx context.Context, in protocol.StartGoalRequest, outcome atomicOutcome) error {
	return outcome.settleRefusal(ctx, s.startGoal(ctx, in, outcome.goal()))
}

func (s *Handler) startGoal(ctx context.Context, in protocol.StartGoalRequest, complete operation.Reply[goal.Goal]) error {
	sessionID, parseErr := sessionParameter(in.SessionID)
	if parseErr != nil {
		return parseErr
	}
	selection, err := flamemodel.NewWithReasoningEffort(in.Provider, in.Model, in.ReasoningEffort)
	if err != nil {
		return mapGoalErr(err)
	}
	capabilities, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return err
	}
	return mapGoalErr(s.goals.Start(ctx, sessionID, in.Objective, selection, capabilities, complete))
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

func (s *Handler) StopGoal(ctx context.Context, in protocol.GoalRequest) error {
	return s.stopGoal(ctx, in, operation.DirectReply(new(struct{})))
}

func (s *Handler) commitStopGoal(ctx context.Context, in protocol.GoalRequest, outcome atomicOutcome) error {
	return outcome.settleRefusal(ctx, s.stopGoal(ctx, in, outcome.acknowledgement()))
}

func (s *Handler) stopGoal(ctx context.Context, in protocol.GoalRequest, complete operation.Reply[struct{}]) error {
	sessionID, parseErr := sessionParameter(in.SessionID)
	if parseErr != nil {
		return parseErr
	}
	return mapGoalErr(s.goals.Stop(ctx, sessionID, complete))
}

func (s *Handler) ResumeGoal(ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
	return applyGoalCommand(func(complete operation.Reply[goal.Goal]) error { return s.resumeGoal(ctx, in, complete) })
}

func (s *Handler) commitResumeGoal(ctx context.Context, in protocol.GoalRequest, outcome atomicOutcome) error {
	return outcome.settleRefusal(ctx, s.resumeGoal(ctx, in, outcome.goal()))
}

func (s *Handler) resumeGoal(ctx context.Context, in protocol.GoalRequest, complete operation.Reply[goal.Goal]) error {
	sessionID, parseErr := sessionParameter(in.SessionID)
	if parseErr != nil {
		return parseErr
	}
	caller, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return err
	}
	return mapGoalErr(s.goals.Resume(ctx, sessionID, caller, complete))
}

func applyGoalCommand(command func(operation.Reply[goal.Goal]) error) (*protocol.Goal, error) {
	var result goal.Goal
	if err := command(operation.DirectReply(&result)); err != nil {
		return nil, err
	}
	return presentGoal(result)
}

func (o atomicOutcome) goal() operation.Reply[goal.Goal] {
	return atomicReply(o.complete, func(result goal.Goal) ([]byte, error) {
		view, err := presentGoal(result)
		if err != nil {
			return nil, err
		}
		return encodeAtomicResult(o.method, view)
	})
}

func mapGoalErr(err error) error {
	if err == nil {
		return nil
	}
	if uncovered, ok := errors.AsType[*run.InsufficientCapabilitiesError](err); ok {
		return capabilityGap(uncovered.Missing)
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

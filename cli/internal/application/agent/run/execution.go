package run

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/cli/internal/application/mutation"
	"github.com/Tangerg/flame/cli/internal/application/retry"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
)

const cancellationTimeout = 5 * time.Second

type Renderer interface {
	Begin(conversation.Run, prompt.RunOptions) error
	Render(conversation.RunEvent) error
	Reconcile(conversation.SessionSnapshot) error
	Close() error
}

type SessionReader interface {
	GetSession(context.Context, string) (conversation.SessionSnapshot, error)
}

type Lifecycle interface {
	PrepareInput(context.Context, prompt.Message) ([]protocol.ContentBlock, error)
	StartRun(context.Context, prompt.StartRun) (conversation.SegmentStream, error)
	ResumeRun(context.Context, conversation.ResumeRun) (conversation.SegmentStream, error)
	SubscribeRun(context.Context, conversation.SubscribeRun) (conversation.SegmentStream, error)
	SteerRun(context.Context, prompt.SteerRun) (protocol.SteerRunResponse, error)
	CancelRun(context.Context, conversation.CancelRun) (conversation.RunCancellation, error)
}

type Runtime interface {
	Lifecycle
	SessionReader
}

type Invocation struct {
	Runtime    Runtime
	Renderer   Renderer
	Start      prompt.StartRun
	ApproveAll bool

	ReplayPolicy mutation.ReplayPolicy
}

// Execute drives one stable Run across as many Segments as its interrupts
// require. Ambiguous mutation acknowledgements reuse the same command identity
// only while the runtime's advertised replay retention still owns it.
func Execute(ctx context.Context, invocation Invocation) (runErr error) {
	if invocation.Runtime == nil {
		return errors.New("one-shot run requires a runtime")
	}
	if invocation.Renderer == nil {
		return errors.New("one-shot run requires a renderer")
	}
	if invocation.Start.CommandID == "" {
		invocation.Start.CommandID = mutation.NewCommandID()
	}
	if err := invocation.Start.Validate(); err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, invocation.Renderer.Close()) }()
	observationCtx, releaseObservation := context.WithCancel(ctx)
	defer releaseObservation()
	invocation.Start = invocation.Start.Clone()
	if invocation.Start.Input == nil {
		prepared, err := invocation.Runtime.PrepareInput(ctx, invocation.Start.Message)
		if err != nil {
			return fmt.Errorf("prepare one-shot input: %w", err)
		}
		invocation.Start.Input = prepared
	}

	startReplay, err := invocation.ReplayPolicy.NewGuard()
	if err != nil {
		return fmt.Errorf("prepare one-shot start replay guard: %w", err)
	}
	opened, err := openRun(observationCtx, invocation.Runtime, invocation.Start,
		mutation.FreshReplayAdmission(invocation.ReplayPolicy, startReplay))
	if err != nil {
		if receipt, accepted := conversation.AcceptedMutationReceipt(err); accepted {
			opened = receipt
		} else {
			return err
		}
	}
	var watcher *cancellationWatcher
	if opened.RunID != "" {
		watcher = watchCancellation(ctx, invocation.Runtime, opened.RunID, invocation.ReplayPolicy)
		defer func() { runErr = errors.Join(runErr, watcher.Finish()) }()
	}
	if err != nil {
		return err
	}
	if validateStartErr := opened.ValidateStart(); validateStartErr != nil {
		return fmt.Errorf("start run: %w", validateStartErr)
	}
	// Only the identity is known here; the selected model and every other Run
	// fact arrive from Runtime with segment.started.
	run := conversation.Run{ID: opened.RunID, SessionID: invocation.Start.SessionID, Lineage: conversation.RootRunLineage()}
	if beginErr := invocation.Renderer.Begin(run, invocation.Start.Options); beginErr != nil {
		return beginErr
	}

	return drive(observationCtx, invocation, opened)
}

func openRun(
	ctx context.Context,
	runtime Runtime,
	command prompt.StartRun,
	admit mutation.Admission,
) (conversation.SegmentStream, error) {
	return mutation.ConfirmAdmitted(
		ctx, mutation.AcknowledgementBackoff(), admit,
		func(ctx context.Context) (conversation.SegmentStream, error) { return runtime.StartRun(ctx, command) },
	)
}

type cancellationWatcher struct {
	exit   chan struct{}
	result chan error
}

func watchCancellation(
	ctx context.Context,
	runtime Lifecycle,
	runID string,
	replayPolicy mutation.ReplayPolicy,
) *cancellationWatcher {
	watcher := &cancellationWatcher{exit: make(chan struct{}), result: make(chan error, 1)}
	go func() {
		select {
		case <-watcher.exit:
		case <-ctx.Done():
		}
		if ctx.Err() == nil {
			watcher.result <- nil
			return
		}
		watcher.result <- cancelRequestedRun(ctx, runtime, runID, replayPolicy)
	}()
	return watcher
}

func (c *cancellationWatcher) Finish() error {
	close(c.exit)
	return <-c.result
}

func cancelRequestedRun(
	ctx context.Context,
	runtime Lifecycle,
	runID string,
	replayPolicy mutation.ReplayPolicy,
) error {
	commandID := mutation.NewCommandID()
	cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cancellationTimeout)
	defer cancel()
	replayGuard, err := replayPolicy.NewGuard()
	if err != nil {
		return fmt.Errorf("prepare requested run cancellation replay guard: %w", err)
	}
	result, err := mutation.ConfirmAdmitted(
		cancelCtx, mutation.AcknowledgementBackoff(), mutation.FreshReplayAdmission(replayPolicy, replayGuard),
		func(ctx context.Context) (conversation.RunCancellation, error) {
			return runtime.CancelRun(ctx, conversation.CancelRun{
				CommandID: commandID, RunID: runID, Reason: "CLI execution canceled",
			})
		},
	)
	if errors.Is(err, conversation.ErrRunFinished) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cancel requested run %s: %w", runID, err)
	}
	if err := result.ValidateTarget(runID); err != nil {
		return fmt.Errorf("cancel requested run %s: %w", runID, err)
	}
	return nil
}

func drive(ctx context.Context, invocation Invocation, opened conversation.SegmentStream) error {
	driver := executionDriver{
		invocation: invocation, openedRunID: opened.RunID,
		conversation: conversation.New(), current: opened,
	}
	return driver.run(ctx)
}

type executionDriver struct {
	invocation   Invocation
	openedRunID  string
	conversation *conversation.Conversation
	current      conversation.SegmentStream
	failures     int
}

func (e *executionDriver) run(ctx context.Context) error {
	for {
		followed := consume(e.current.Events, e.conversation, e.invocation.Renderer)
		if followed.err == nil && e.conversation.RunID() == e.current.RunID {
			switch e.conversation.Phase() {
			case conversation.Idle:
				return errorForOutcome(e.conversation.Outcome())
			case conversation.Waiting:
				if err := e.resume(ctx, e.conversation.Interrupts(), e.current.RunID); err != nil {
					return err
				}
				continue
			}
		}
		cause := followed.err
		if cause == nil {
			cause = fmt.Errorf("%w: segment stream ended without a terminal event", conversation.ErrDisconnected)
		}
		if followed.applied > 0 {
			e.failures = 0
		}
		continued, err := e.reconnect(ctx, cause)
		if !continued {
			return err
		}
	}
}

func (e *executionDriver) resume(ctx context.Context, interrupts []conversation.Interrupt, runID string) error {
	answers, err := unattendedAnswers(interrupts, e.invocation.ApproveAll, e.invocation.Start.SessionID)
	if err != nil {
		return err
	}
	commandID := mutation.NewCommandID()
	command := conversation.ResumeRun{CommandID: commandID, RunID: runID, Answers: answers}
	replayGuard, err := e.invocation.ReplayPolicy.NewGuard()
	if err != nil {
		return fmt.Errorf("prepare one-shot resume replay guard: %w", err)
	}
	continued, err := mutation.ConfirmAdmitted(
		ctx, mutation.AcknowledgementBackoff(), mutation.FreshReplayAdmission(e.invocation.ReplayPolicy, replayGuard),
		func(ctx context.Context) (conversation.SegmentStream, error) {
			return e.invocation.Runtime.ResumeRun(ctx, command)
		},
	)
	if err != nil {
		return err
	}
	if err := validateContinuation(continued, e.openedRunID); err != nil {
		return err
	}
	e.current = continued
	e.failures = 0
	return nil
}

func (e *executionDriver) reconnect(ctx context.Context, cause error) (bool, error) {
	for {
		e.failures++
		delay, shouldRetry, policyErr := retry.ReconnectDelay(e.failures, cause)
		if policyErr != nil {
			return false, policyErr
		}
		if !shouldRetry {
			return false, cause
		}
		if err := retry.Wait(ctx, delay); err != nil {
			return false, err
		}
		rebound, err := e.invocation.Runtime.SubscribeRun(ctx, conversation.SubscribeRun{
			RunID: e.current.RunID, SegmentID: e.current.SegmentID, AfterEventID: e.conversation.Checkpoint(),
		})
		if err == nil {
			if validateSubscriptionErr := rebound.ValidateSubscription(); validateSubscriptionErr != nil {
				return false, fmt.Errorf("subscribe run: %w", validateSubscriptionErr)
			}
			e.current = rebound
			return true, nil
		}
		if !RecoveryRequired(err) {
			cause = err
			continue
		}
		recovered, recoveryErr := RecoverSegment(ctx, e.invocation.Runtime, e.invocation.Start.SessionID, e.current.RunID)
		if recoveryErr != nil {
			if !RecoveryRequired(recoveryErr) {
				cause = recoveryErr
			}
			continue
		}
		return e.installRecovery(ctx, recovered)
	}
}

func (e *executionDriver) installRecovery(ctx context.Context, recovered Recovery) (bool, error) {
	if err := e.invocation.Renderer.Reconcile(recovered.Snapshot); err != nil {
		return false, err
	}
	if err := restoreRecoveredConversation(e.conversation, recovered); err != nil {
		return false, err
	}
	switch recovered.Run.Status {
	case protocol.RunStatusFinished:
		return false, errorForOutcome(recovered.Run.Outcome)
	case protocol.RunStatusWaiting:
		if err := e.resume(ctx, recovered.Snapshot.Interrupts, recovered.Run.ID); err != nil {
			return false, err
		}
	case protocol.RunStatusRunning:
		e.current = recovered.Stream
	}
	return true, nil
}

func restoreRecoveredConversation(projection *conversation.Conversation, recovered Recovery) error {
	if recovered.Run.Status == protocol.RunStatusRunning {
		return projection.RestoreAttachedSnapshot(recovered.Snapshot, recovered.Stream)
	}
	return projection.RestoreSnapshot(recovered.Snapshot)
}

func validateContinuation(stream conversation.SegmentStream, runID string) error {
	return stream.ValidateResume(runID, nil)
}

type followResult struct {
	err     error
	applied int
}

func consume(stream conversation.EventStream, projection *conversation.Conversation, renderer Renderer) followResult {
	var followed followResult
	for event, streamErr := range stream {
		if streamErr != nil {
			followed.err = streamErr
			break
		}
		result, err := projection.ApplyRunEvent(event)
		if err != nil {
			followed.err = fmt.Errorf("accept runtime event %s: %w", event.EventID, err)
			break
		}
		if !result.Applied {
			continue
		}
		followed.applied++
		if err := renderer.Render(event); err != nil {
			followed.err = err
			break
		}
		if _, finished := event.Event.(conversation.SegmentFinished); finished && event.RunID == projection.RunID() {
			// The root boundary follows every member's terminal projection.
			// Conversation now owns the complete outcome or pending set.
			return followed
		}
	}
	return followed
}

type outcomeError struct{ outcome conversation.Outcome }

func (o *outcomeError) Error() string {
	if detail := o.outcome.Explanation(); detail != "" {
		return "run " + string(o.outcome.Status) + ": " + detail
	}
	return "run " + string(o.outcome.Status)
}

func errorForOutcome(outcome conversation.Outcome) error {
	if outcome.Status == protocol.OutcomeCompleted {
		return nil
	}
	return &outcomeError{outcome: outcome}
}

func unattendedAnswers(interrupts []conversation.Interrupt, approveAll bool, sessionID string) ([]conversation.InterruptAnswer, error) {
	answers := make([]conversation.InterruptAnswer, 0, len(interrupts))
	for _, interrupt := range interrupts {
		switch item := interrupt.(type) {
		case conversation.Approval:
			answers = append(answers, conversation.InterruptAnswer{ItemID: item.ItemID, Answer: approvalAnswer(approveAll)})
		case conversation.Question:
			return nil, &interruptRequiredError{title: item.Title, sessionID: sessionID}
		default:
			return nil, errors.New("runtime returned an unknown interrupt")
		}
	}
	return answers, nil
}

func approvalAnswer(approveAll bool) conversation.ApprovalAnswer {
	if approveAll {
		return conversation.ApprovalAnswer{Decision: protocol.ApprovalApprove}
	}
	return conversation.ApprovalAnswer{
		Decision: protocol.ApprovalDeny,
		Reason:   "declined: this run is unattended (rerun with --approve-all to allow it)",
	}
}

type interruptRequiredError struct {
	title     string
	sessionID string
}

func (i *interruptRequiredError) Error() string {
	return fmt.Sprintf("run needs answers to %q; continue it interactively with --session %s", i.title, i.sessionID)
}

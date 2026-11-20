package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

// Resume prepares one complete response set without acquiring an executor. Its
// acceptance transaction consumes the waiting handoff, settles answered Items,
// and resumes the tree. Writer restoration and activation belong to the accepted
// Run's lifecycle, which retains the exact committed Segment identities.
func (c *Coordinator) Resume(ctx context.Context, cmd ResumeCommand) (StartResult, error) {
	cmd = cmd.clone()
	if cmd.Acceptance != nil {
		if err := cmd.Acceptance.Validate(); err != nil {
			return StartResult{}, fmt.Errorf("%w: %w", ErrInvalidInterruptResponse, err)
		}
		accepted, found, err := lookupAcceptedCommand(ctx, c.commandAcceptances, *cmd.Acceptance, CommandResumeOpening)
		if err != nil || found {
			return accepted, err
		}
	}
	sess, runAdmission, err := c.claimResume(ctx, cmd.RunID)
	if err != nil {
		return StartResult{}, err
	}
	defer runAdmission.Release()
	parked, err := c.readParkedTree(ctx, sess.ID(), cmd)
	if err != nil {
		return StartResult{}, err
	}
	prepared, err := c.prepareResume(ctx, sess, parked, cmd.Responses)
	if err != nil {
		return StartResult{}, err
	}
	return c.openResumedSegment(ctx, sess, &runAdmission, parked, prepared, cmd)
}

// parkedTree is the answerable hand-off and the Items and Runs it names, read
// while this command excludes competing resume and cancellation.
type parkedTree struct {
	pending Pending
	items   map[identity.ItemID]transcript.Item
	runs    []rundomain.Run
}

func (c *Coordinator) readParkedTree(ctx context.Context, sessionID identity.SessionID, cmd ResumeCommand) (parkedTree, error) {
	pending, found, err := c.interrupts.LookupOpenInterrupt(ctx, cmd.RunID)
	if err != nil {
		return parkedTree{}, err
	}
	if !found {
		return parkedTree{}, ErrInterruptNotOpen
	}
	// The Items an open interrupt names cannot change while it is open: only
	// the opening transaction answers them.
	items, err := pendingItems(ctx, c.items, pending)
	if err != nil {
		return parkedTree{}, err
	}
	runs, err := c.runs.Tree(ctx, pending.RootRunID)
	if err != nil {
		return parkedTree{}, err
	}
	if err := validatePendingRunTree(pending, runs); err != nil {
		return parkedTree{}, err
	}
	if err := validateResumeInput(pending, runs, cmd); err != nil {
		return parkedTree{}, err
	}
	return parkedTree{pending: pending, items: items, runs: runs}, nil
}

// preparedResume is the acceptance write-set and the executor continuation it
// authorizes, prepared without acquiring an executor.
type preparedResume struct {
	acceptance   ResumeAcceptance
	answers      []InterruptAnswer
	waiting      ExecutionContinuation
	continuation *treeContinuation
}

func (c *Coordinator) prepareResume(ctx context.Context, sess session.Session, parked parkedTree, responses []ResumeResponse) (preparedResume, error) {
	sessionID, pending := sess.ID(), parked.pending
	acceptance, err := prepareResumeAcceptance(sessionID, pending, parked.items, responses)
	if err != nil {
		return preparedResume{}, err
	}
	for _, change := range acceptance.itemChanges {
		item := change.State()
		parked.items[item.ID()] = item
	}
	answers, err := pending.settledAnswers(sessionID, parked.items)
	if err != nil {
		return preparedResume{}, fmt.Errorf("runs: prepare resume acceptance: %w", err)
	}
	ruleCandidates, err := rememberedRuleCandidates(sessionID, sess.Workspace().Path(), pending, parked.items, answers)
	if err != nil {
		return preparedResume{}, fmt.Errorf("runs: prepare remembered rule candidates: %w", err)
	}
	rootContinuation, ok := pending.RootContinuation()
	if !ok {
		return preparedResume{}, errors.New("runs: pending interrupt set has no root continuation")
	}
	checkpoint, err := c.checkpoints.ReadWaitingCheckpoint(ctx, rootContinuation.MemberID)
	if err != nil {
		return preparedResume{}, err
	}
	waiting, err := waitingContinuationFromPending(pending, checkpoint, parked.runs, sess)
	if err != nil {
		return preparedResume{}, fmt.Errorf("runs: prepare waiting continuation: %w", err)
	}
	requests, err := c.continuation.PrepareContinuation(ctx, waiting, answers, ruleCandidates)
	if err != nil {
		return preparedResume{}, err
	}
	for _, request := range requests {
		rule, err := c.approvalRules.PrepareRule(ctx, request)
		if err != nil {
			return preparedResume{}, fmt.Errorf("runs: prepare remembered approval rule: %w", err)
		}
		acceptance.rememberedRules = append(acceptance.rememberedRules, rule)
	}
	continuation, err := treeContinuationForAnswers(pending, parked.runs, parked.items)
	if err != nil {
		return preparedResume{}, err
	}
	return preparedResume{acceptance: acceptance, answers: answers, waiting: waiting, continuation: continuation}, nil
}

// openResumedSegment commits one acceptance transaction that consumes the open
// set, settles its answered Items, resumes the tree and opens the continuation
// Segment. Executor activation is detached and owned by the accepted Run.
func (c *Coordinator) openResumedSegment(
	ctx context.Context,
	sess session.Session,
	runAdmission *ownership.RunAdmission,
	parked parkedTree,
	prepared preparedResume,
	cmd ResumeCommand,
) (StartResult, error) {
	sessionID, waiting := sess.ID(), prepared.waiting
	segmentID := c.segments.newSegmentID()
	var committedInput *CommittedUserInput
	if len(cmd.Input) > 0 {
		committedInput = &CommittedUserInput{ItemID: userMessageItemID(segmentID), Content: transcript.CloneContent(cmd.Input)}
	}
	events, err := c.segments.openSegment(ctx, segmentSpec{
		RunID:        cmd.RunID,
		SegmentID:    segmentID,
		SessionID:    sessionID,
		WorkspaceCWD: sess.Workspace().Path(),
		Isolated:     sess.Isolated(),
		ExecutorID:   waiting.ExecutorID,
		Opening: treeResumption{
			Continuation:     prepared.continuation,
			Input:            cmd.Input,
			ResumeAcceptance: &prepared.acceptance,
			StageExecution: func(stageCtx context.Context) error {
				_, err := c.continuation.StageAcceptedContinuation(stageCtx, waiting)
				return err
			},
		},
		CommandAcceptance: cmd.Acceptance,
		admission:         runAdmission,
		DetachActivation:  true,
		BeginExecution: func(beginCtx context.Context) error {
			return c.continuation.BeginContinuation(
				beginCtx, ExecutorRef{SessionID: sessionID, ExecutorID: waiting.ExecutorID}, prepared.answers, committedInput,
			)
		},
	})
	if err != nil {
		return StartResult{}, err
	}
	// The standing rules committed inside the acceptance; their read model is
	// invalidated only now that the transaction is durable.
	if len(prepared.acceptance.rememberedRules) > 0 {
		c.segments.publications.publishApprovalsChanged()
	}
	for _, continuation := range parked.pending.Continuations {
		if continuation.RunID != parked.pending.RootRunID {
			c.segments.publications.publishRunMoved(sessionID, continuation.RunID)
		}
	}
	c.segments.publications.publishWaitingMoved(sessionID, parked.pending.RootRunID)
	result := StartResult{RunID: cmd.RunID, SegmentID: segmentID, SessionID: sessionID, Events: events}
	if committedInput != nil {
		// The id derives from the Segment as a fresh Run's does, so the client
		// reconciles its optimistic bubble by id rather than by content.
		result.UserItemID = committedInput.ItemID
	}
	return result, nil
}

// rememberedRuleCandidates selects every answered approval that requested a
// standing rule and assembles the authority skeleton the acceptance owner derives
// a rule from. The frozen tool authority comes from the interrupt binding, the
// scope and decision from the accepted answer, and the effective arguments from
// the confirmed edit or the original invocation. The answers follow the
// binding order because both are projected from the settled Items.
func rememberedRuleCandidates(
	sessionID identity.SessionID, projectDir string,
	pending Pending,
	items map[identity.ItemID]transcript.Item,
	answers []InterruptAnswer,
) ([]RememberedRuleCandidate, error) {
	candidates := make([]RememberedRuleCandidate, 0, len(answers))
	for index, answer := range answers {
		resolution := answer.Resolution.Approval()
		if resolution == nil || resolution.RememberScope == "" {
			continue
		}
		binding := pending.Bindings[index]
		effective, err := effectiveApprovalArguments(items[answer.InterruptItemID], *resolution)
		if err != nil {
			return nil, fmt.Errorf("item %q: %w", answer.InterruptItemID, err)
		}
		candidates = append(candidates, RememberedRuleCandidate{
			Request: approval.RememberRequest{
				Scope:             resolution.RememberScope,
				SessionID:         sessionID,
				ProjectDir:        projectDir,
				Tool:              binding.ToolRef,
				SourceFingerprint: binding.SourceFingerprint,
				Decision:          resolution.Decision,
			},
			CallID:             binding.ToolCallID,
			EffectiveArguments: effective,
		})
	}
	return candidates, nil
}

// effectiveApprovalArguments is the exact invocation a remembered rule is derived
// from: the human's confirmed edit when an approval carried one, otherwise the
// original held invocation. A held approval keeps its unexecuted arguments as
// verbatim text, so that form is parsed when no edit replaced it.
func effectiveApprovalArguments(item transcript.Item, resolution approval.Resolution) (tool.Arguments, error) {
	if resolution.Decision == approval.Allow && resolution.EditedArguments != "" {
		return tool.ParseArguments(resolution.EditedArguments)
	}
	call, ok := item.ToolCall()
	if !ok {
		return tool.Arguments{}, errors.New("approval item carries no tool invocation")
	}
	if rejected, ok := call.Invocation.Input.Rejected(); ok {
		return tool.ParseArguments(rejected)
	}
	return call.Invocation.Input.Arguments(), nil
}

func (c *Coordinator) claimResume(ctx context.Context, runID identity.RunID) (session.Session, ownership.RunAdmission, error) {
	addressed, found, err := c.runs.Run(ctx, runID)
	if err != nil {
		return session.Session{}, ownership.RunAdmission{}, err
	}
	if !found {
		return session.Session{}, ownership.RunAdmission{}, ErrInterruptNotOpen
	}
	sessionID := addressed.SessionID()
	sess, err := c.sessionReader.Get(ctx, sessionID)
	if err != nil {
		return session.Session{}, ownership.RunAdmission{}, err
	}
	runAdmission, ok, leaseErr := c.admission.AcquireRun(ctx, sessionID, sess.Workspace().Path())
	if leaseErr != nil {
		return session.Session{}, ownership.RunAdmission{}, leaseErr
	}
	if !ok {
		return session.Session{}, ownership.RunAdmission{}, fmt.Errorf("%w: session %q or working tree %q has a run or mutation in flight", run.ErrSessionBusy, sessionID, sess.Workspace().Path())
	}
	return sess, runAdmission, nil
}

func validateResumeInput(pending Pending, parkedRuns []run.Run, cmd ResumeCommand) error {
	root, ok := parkedRoot(pending.RootRunID, parkedRuns)
	if !ok {
		return errors.New("runs: pending interrupt set has no parked root Run")
	}
	if err := root.CheckResumeCapabilities(cmd.CallerCapabilities); err != nil {
		return fmt.Errorf("runs: run %q: %w", cmd.RunID, err)
	}
	if len(cmd.Input) > 0 {
		if _, err := MaterializeUserMessage(cmd.Input); err != nil {
			return err
		}
	}
	return nil
}

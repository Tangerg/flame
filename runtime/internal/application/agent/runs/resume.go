package runs

import (
	"context"
	"errors"
	"fmt"

	corechat "github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// Resume validates one complete response set, then opens the continuation in one
// acceptance transaction that consumes the waiting hand-off, settles its answered
// Items, and resumes the tree together. The execution context checkpoint is
// retained for that accepted continuation. Staging reconstructs the live/restored
// tree before acceptance without any durable effect; semantic answer submission
// continues behind the Run lifecycle supervisor after the opening commits.
func (c *Coordinator) Resume(ctx context.Context, cmd ResumeCommand) (result StartResult, err error) {
	cmd = cmd.clone()
	addressed, found, err := c.runs.Run(ctx, cmd.RunID)
	if err != nil {
		return StartResult{}, err
	}
	if !found {
		return StartResult{}, ErrInterruptNotOpen
	}
	sessionID := addressed.SessionID()
	sess, err := c.sessionReader.Get(ctx, sessionID)
	if err != nil {
		return StartResult{}, err
	}
	runAdmission, ok, leaseErr := c.admission.AcquireRun(ctx, sessionID, sess.Workspace().Path())
	if leaseErr != nil {
		return StartResult{}, leaseErr
	}
	if !ok {
		return StartResult{}, fmt.Errorf("%w: session %q or working tree %q has a run or mutation in flight", ErrSessionBusy, sessionID, sess.Workspace().Path())
	}
	defer runAdmission.Release()
	// Run identity locates admission; the answerable hand-off and its Items are
	// read only while this command excludes competing resume and cancellation.
	pending, found, err := c.interrupts.LookupOpenInterrupt(ctx, cmd.RunID)
	if err != nil {
		return StartResult{}, err
	}
	if !found {
		return StartResult{}, ErrInterruptNotOpen
	}
	// The Items an open interrupt names cannot change while it is open: only
	// the claim below answers them.
	itemsByID, err := c.pendingItems(ctx, pending)
	if err != nil {
		return StartResult{}, err
	}
	interrupts, err := pending.ProjectInterrupts(itemsByID)
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: project open interrupts: %w", err)
	}
	answers, err := resolveResumeResponses(interrupts, pending.Bindings, cmd.Responses)
	if err != nil {
		return StartResult{}, err
	}
	parkedRuns, err := c.runs.Tree(ctx, pending.RootRunID)
	if err != nil {
		return StartResult{}, err
	}
	if validatePendingRunTreeErr := validatePendingRunTree(pending, parkedRuns); validatePendingRunTreeErr != nil {
		return StartResult{}, validatePendingRunTreeErr
	}

	root, ok := parkedRoot(pending.RootRunID, parkedRuns)
	if !ok {
		return StartResult{}, errors.New("runs: pending interrupt set has no parked root Run")
	}
	if gap := root.Capabilities().MissingFrom(cmd.CallerCapabilities); !gap.IsEmpty() {
		return StartResult{}, &run.InsufficientCapabilitiesError{RunID: cmd.RunID, Missing: gap}
	}
	if len(cmd.Input) > 0 {
		message, err := MaterializeUserMessage(cmd.Input)
		if err != nil {
			return StartResult{}, err
		}
		if err := c.models.AdmitInput(root.ModelSelection(), []corechat.Message{message}); err != nil {
			return StartResult{}, fmt.Errorf("%w: %w", ErrUnsupportedMedia, err)
		}
	}

	acceptance, err := NewResumeAcceptance(sessionID, pending, itemsByID, answers)
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: prepare resume acceptance: %w", err)
	}
	ruleCandidates, err := rememberedRuleCandidates(sessionID, sess.Workspace().Path(), pending, itemsByID, answers)
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: prepare remembered rule candidates: %w", err)
	}
	rootContinuation, ok := pending.RootContinuation()
	if !ok {
		return StartResult{}, errors.New("runs: pending interrupt set has no root continuation")
	}
	checkpoint, err := c.checkpoints.ReadWaitingCheckpoint(ctx, rootContinuation.MemberID)
	if err != nil {
		return StartResult{}, err
	}
	waiting, err := waitingContinuationFromPending(pending, checkpoint, parkedRuns, sess)
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: prepare waiting continuation: %w", err)
	}
	// Staging reconstructs in-memory executor state and commits nothing durable.
	// The open interrupt set is consumed only by the acceptance opening below, so a
	// failure before that commit leaves the Run recoverable as waiting.
	ref, err := c.continuation.StageContinuation(ctx, waiting)
	if err != nil {
		return StartResult{}, err
	}
	staged := ownStagedExecution(c.releases, ref)
	defer func() {
		if err != nil {
			err = staged.abandon(ctx, err, "unaccepted continuation")
			result = StartResult{}
		}
	}()
	// The staged tree validates each confirmed invocation and derives its rule
	// subject; the approval owner validates the frozen source authority. Both run
	// before the acceptance opening, so a changed authority or an unpreparable rule
	// rejects the whole Resume and the staged tree abandons recoverable as waiting.
	if len(ruleCandidates) > 0 {
		requests, deriveErr := c.continuation.DeriveRememberedRules(ctx, ref, ruleCandidates)
		if deriveErr != nil {
			return StartResult{}, deriveErr
		}
		rememberedRules := make([]approval.Rule, 0, len(requests))
		for _, request := range requests {
			rule, prepareErr := c.approvalRules.PrepareRule(ctx, request)
			if prepareErr != nil {
				return StartResult{}, fmt.Errorf("runs: prepare remembered approval rule: %w", prepareErr)
			}
			rememberedRules = append(rememberedRules, rule)
		}
		acceptance, err = acceptance.WithRememberedRules(rememberedRules)
		if err != nil {
			return StartResult{}, err
		}
	}
	segmentID := c.newSegmentID()
	var committedInput *CommittedUserInput
	if len(cmd.Input) > 0 {
		committedInput = &CommittedUserInput{
			ItemID:  userMessageItemID(segmentID),
			Content: transcript.CloneContent(cmd.Input),
		}
	}
	continuation, err := treeContinuationFromPending(pending, parkedRuns, itemsByID)
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: prepare tree continuation: %w", err)
	}
	continuation.approvalCalls = acceptance.approvalCalls()
	replacements, err := acceptance.ItemReplacements()
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: prepare tree continuation: %w", err)
	}
	for _, replacement := range replacements {
		item := replacement.State()
		continuation.items[item.ID()] = item
	}
	// One acceptance transaction consumes the open set, settles its answered Items,
	// resumes the tree, and opens the continuation Segment. Executor activation is
	// detached and owned by the accepted Run lifecycle.
	events, err := c.openSegment(ctx, segmentSpec{
		RunID:            cmd.RunID,
		SegmentID:        segmentID,
		SessionID:        sessionID,
		WorkspaceCWD:     sess.Workspace().Path(),
		Isolated:         sess.Isolated(),
		ExecutorID:       ref.ExecutorID,
		Input:            cmd.Input,
		Continuation:     continuation,
		ResumeAcceptance: &acceptance,
		admission:        &runAdmission,
		DetachActivation: true,
		BeginExecution: func(beginCtx context.Context) error {
			return c.continuation.BeginContinuation(
				beginCtx, ref, answers, committedInput,
			)
		},
	})
	if err != nil {
		return StartResult{}, err
	}
	// The opening committed the acceptance and transferred the staged tree to the
	// Segment lifecycle.
	staged.transfer()
	// The standing rules committed inside the acceptance; their read model is
	// invalidated only now that the transaction is durable.
	if len(acceptance.RememberedRules()) > 0 {
		c.publications.publishApprovalsChanged()
	}
	// The continuation is durably accepted, which consumed the whole open set: the
	// run is running again and nothing in this session is waiting on a person.
	for _, continuation := range pending.Continuations {
		if continuation.RunID == pending.RootRunID {
			continue
		}
		c.publications.publishRunMoved(sessionID, continuation.RunID)
	}
	c.publications.publishWaitingMoved(sessionID, pending.RootRunID)
	result = StartResult{RunID: cmd.RunID, SegmentID: segmentID, SessionID: sessionID, Events: events}
	if committedInput != nil {
		// Named only when there is an item to name: the id is derived from the segment
		// the same way a fresh run derives it, so the client reconciles its optimistic
		// bubble by id rather than by content.
		result.UserItemID = committedInput.ItemID
	}
	return result, nil
}

// rememberedRuleCandidates selects every answered approval that requested a
// standing rule and assembles the authority skeleton the acceptance owner derives
// a rule from. The frozen tool authority comes from the interrupt binding, the
// scope and decision from the accepted answer, and the effective arguments from
// the confirmed edit or the original invocation. Interrupts, bindings, and
// answers share one order, which ResumeAcceptance already validates.
func rememberedRuleCandidates(
	sessionID, projectDir string,
	pending Pending,
	items map[string]transcript.Item,
	answers []InterruptAnswer,
) ([]RememberedRuleCandidate, error) {
	candidates := make([]RememberedRuleCandidate, 0, len(answers))
	for index, answer := range answers {
		open := pending.Interrupts[index]
		if open.Kind() != interrupt.Approval || open.Approval == nil || !open.Approval.Rememberable {
			continue
		}
		if answer.Resolution.RememberScope == "" {
			continue
		}
		binding := pending.Bindings[index]
		effective, err := effectiveApprovalArguments(items[open.ItemID], answer.Resolution)
		if err != nil {
			return nil, fmt.Errorf("item %q: %w", open.ItemID, err)
		}
		candidates = append(candidates, RememberedRuleCandidate{
			Request: approval.RememberRequest{
				Scope:             answer.Resolution.RememberScope,
				SessionID:         sessionID,
				ProjectDir:        projectDir,
				Tool:              binding.ToolRef,
				SourceFingerprint: binding.SourceFingerprint,
				Decision:          approval.DecisionOf(answer.Resolution.Approved),
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
func effectiveApprovalArguments(item transcript.Item, resolution interrupt.Resolution) (tool.Arguments, error) {
	if resolution.Approved && resolution.Arguments != "" {
		return tool.ParseArguments(resolution.Arguments)
	}
	invocation, ok := item.ToolInvocation()
	if !ok {
		return tool.Arguments{}, errors.New("approval item carries no tool invocation")
	}
	if invocation.ArgumentsText != "" {
		return tool.ParseArguments(invocation.ArgumentsText)
	}
	return invocation.Arguments, nil
}

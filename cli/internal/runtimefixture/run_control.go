package runtimefixture

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
)

func (r *Runtime) StartRun(ctx context.Context, in prompt.StartRun) (conversation.SegmentStream, error) {
	if err := in.Validate(); err != nil {
		return conversation.SegmentStream{}, fmt.Errorf("mock: %w", err)
	}
	if err := context.Cause(ctx); err != nil {
		return conversation.SegmentStream{}, err
	}
	build := r.Script
	if build == nil {
		build = DefaultScript
	}
	script, err := buildScriptSafely(build, in.Message.Text)
	if err != nil {
		return conversation.SegmentStream{}, fmt.Errorf("mock: build script: %w", err)
	}

	r.mu.Lock()
	session := r.sessions[in.SessionID]
	if session == nil {
		r.mu.Unlock()
		return conversation.SegmentStream{}, fmt.Errorf("%w: %s", conversation.ErrSessionNotFound, in.SessionID)
	}
	if session.active != "" {
		r.mu.Unlock()
		return conversation.SegmentStream{}, fmt.Errorf("%w: %s", conversation.ErrSessionHasActiveRun, in.SessionID)
	}
	if err := session.requireRevisionCapacity(startRunRevisionChanges(session)); err != nil {
		r.mu.Unlock()
		return conversation.SegmentStream{}, err
	}
	fault, err := r.takeFaultLocked()
	if err != nil {
		r.mu.Unlock()
		return conversation.SegmentStream{}, err
	}
	runID := r.identities.next(runIdentity)
	run := &runState{
		id: runID, sessionID: in.SessionID,
		lineage:  conversation.RootRunLineage(),
		provider: in.Options.Provider, model: in.Options.Model, reasoningEffort: in.Options.ReasoningEffort,
		status:   protocol.RunStatusRunning,
		segments: make(map[string]*segmentState), script: script, answers: make(map[string]conversation.Answer), cancel: make(chan struct{}),
	}
	run.script = namespaceScript(run.script, run.id)
	if run.provider == "" {
		run.provider, run.model = session.meta.Provider, session.meta.Model
		run.reasoningEffort = session.meta.ReasoningEffort
	}
	segment := r.openSegmentLocked(run)
	r.runs[run.id] = run
	r.runOrder = append(r.runOrder, run.id)
	session.active = run.id
	session.runs = append(session.runs, run.id)
	if err := r.setSessionStatusLocked(session, protocol.SessionStatusRunning); err != nil {
		r.mu.Unlock()
		return conversation.SegmentStream{}, err
	}
	if err := r.emitLocked(run, conversation.SegmentStarted{Run: projectRun(run)}); err != nil {
		r.mu.Unlock()
		return conversation.SegmentStream{}, err
	}
	userItemID := r.identities.next(itemIdentity)
	if err := r.emitLocked(run, conversation.BlockCompleted{Block: conversation.Block{
		ID: userItemID, Kind: conversation.BlockUser, Text: in.Message.Text, Attachments: slices.Clone(in.Message.Attachments),
	}}); err != nil {
		r.mu.Unlock()
		return conversation.SegmentStream{}, err
	}
	stream := r.bindSegmentLocked(ctx, run, segment, 0, 0, userItemID, fault)
	r.mu.Unlock()

	go r.play(run, run.script.Prelude, run.script.interrupts())
	return stream, nil
}

func startRunRevisionChanges(session *sessionState) sessionRevisionChanges {
	return sessionStatusRevisionChanges(session, protocol.SessionStatusRunning).
		plus(sessionEventRevisionChange()).
		plus(sessionEventRevisionChange())
}

func (r *Runtime) ResumeRun(ctx context.Context, in conversation.ResumeRun) (conversation.SegmentStream, error) {
	if err := in.Validate(); err != nil {
		return conversation.SegmentStream{}, fmt.Errorf("mock: %w", err)
	}
	if err := context.Cause(ctx); err != nil {
		return conversation.SegmentStream{}, err
	}

	r.mu.Lock()
	prepared, err := r.prepareResumeLocked(in)
	r.mu.Unlock()
	if err != nil {
		return conversation.SegmentStream{}, err
	}
	steps, err := prepared.continueScript()
	if err != nil {
		return conversation.SegmentStream{}, err
	}

	r.mu.Lock()
	stream, err := r.activateResumeLocked(ctx, in.Message, prepared)
	r.mu.Unlock()
	if err != nil {
		return conversation.SegmentStream{}, err
	}

	go r.play(prepared.run, steps, false)
	return stream, nil
}

type resumePreparation struct {
	run        *runState
	answers    []conversation.InterruptAnswer
	allAnswers []conversation.InterruptAnswer
	script     Script
}

func (r resumePreparation) continueScript() ([]Step, error) {
	steps, err := continueSafely(r.script, r.allAnswers)
	if err != nil {
		return nil, fmt.Errorf("mock: continue script: %w", err)
	}
	return steps, nil
}

func (r *Runtime) prepareResumeLocked(in conversation.ResumeRun) (resumePreparation, error) {
	run := r.runs[in.RunID]
	if run == nil {
		return resumePreparation{}, fmt.Errorf("%w: %s", conversation.ErrRunNotFound, in.RunID)
	}
	if err := validateResumeSet(run, in.Answers); err != nil {
		return resumePreparation{}, err
	}
	answers := cloneAnswers(in.Answers)
	allAnswers, err := completeScriptAnswers(run, answers)
	if err != nil {
		return resumePreparation{}, err
	}
	return resumePreparation{run: run, answers: answers, allAnswers: allAnswers, script: run.script}, nil
}

func (r *Runtime) activateResumeLocked(ctx context.Context, message *prompt.Message, prepared resumePreparation) (conversation.SegmentStream, error) {
	run := prepared.run
	if run.status != protocol.RunStatusWaiting {
		return conversation.SegmentStream{}, fmt.Errorf("%w: run %s", conversation.ErrInterruptNotOpen, run.id)
	}
	answeredQuestions, err := r.acceptedQuestionBlocksLocked(run, prepared.answers)
	if err != nil {
		return conversation.SegmentStream{}, err
	}
	approvalEvents := approvalCompletionEvents(run, prepared.answers)
	session := r.sessions[run.sessionID]
	if err := session.requireRevisionCapacity(resumeRunRevisionChanges(session, message, len(answeredQuestions)+len(approvalEvents))); err != nil {
		return conversation.SegmentStream{}, err
	}
	fault, err := r.takeFaultLocked()
	if err != nil {
		return conversation.SegmentStream{}, err
	}
	r.recordAnswersLocked(run, prepared.answers)
	for _, block := range answeredQuestions {
		persistBlock(session, run.id, block)
	}
	run.interrupts = nil
	run.status = protocol.RunStatusRunning
	segment := r.openSegmentLocked(run)
	if err := r.setSessionStatusLocked(session, protocol.SessionStatusRunning); err != nil {
		return conversation.SegmentStream{}, err
	}
	if err := r.emitLocked(run, conversation.SegmentStarted{Run: projectRun(run)}); err != nil {
		return conversation.SegmentStream{}, err
	}
	for _, block := range answeredQuestions {
		if err := r.emitLocked(run, conversation.BlockCompleted{Block: block}); err != nil {
			return conversation.SegmentStream{}, err
		}
	}
	userItemID, err := r.emitResumeMessageLocked(run, message)
	if err != nil {
		return conversation.SegmentStream{}, err
	}
	if err := r.emitAllLocked(run, approvalEvents); err != nil {
		return conversation.SegmentStream{}, err
	}
	return r.bindSegmentLocked(ctx, run, segment, 0, 0, userItemID, fault), nil
}

func resumeRunRevisionChanges(session *sessionState, message *prompt.Message, itemEvents int) sessionRevisionChanges {
	changes := sessionStatusRevisionChanges(session, protocol.SessionStatusRunning).
		plus(sessionEventRevisionChange()).
		plus(sessionEventRevisionChanges(itemEvents))
	if message != nil {
		changes = changes.plus(sessionEventRevisionChange())
	}
	return changes
}

// acceptedQuestionBlocksLocked mirrors the production runtime's resume
// linearization point: accepted answers replace the durable Question items, and
// the continuation segment re-completes each one right after it starts.
func (r *Runtime) acceptedQuestionBlocksLocked(run *runState, answers []conversation.InterruptAnswer) ([]conversation.Block, error) {
	session := r.sessions[run.sessionID]
	accepted := make([]conversation.Block, 0, len(answers))
	for _, response := range answers {
		answer, isQuestionAnswer := response.Answer.(conversation.QuestionAnswer)
		if !isQuestionAnswer {
			continue
		}
		question := findQuestion(run.interrupts, response.ItemID)
		if question == nil {
			return nil, fmt.Errorf("mock: question answer references non-question item %s", response.ItemID)
		}
		block, exists := durableBlock(session, run.id, response.ItemID)
		if !exists || block.Kind != conversation.BlockQuestion || block.Question == nil {
			return nil, fmt.Errorf("mock: question item %s is absent from the durable transcript", response.ItemID)
		}
		answered, err := question.Accept(answer)
		if err != nil {
			return nil, fmt.Errorf("mock: accept question item %s: %w", response.ItemID, err)
		}
		block.Question = &answered
		accepted = append(accepted, block)
	}
	return accepted, nil
}

func (r *Runtime) recordAnswersLocked(run *runState, answers []conversation.InterruptAnswer) {
	for _, response := range answers {
		run.answers[response.ItemID] = conversation.CloneAnswer(response.Answer)
		approval := findApproval(run.interrupts, response.ItemID)
		answer, ok := response.Answer.(conversation.ApprovalAnswer)
		if approval != nil && ok && answer.Remember != "" {
			r.rememberApprovalLocked(run, *approval, answer)
		}
	}
}

func (r *Runtime) emitResumeMessageLocked(run *runState, message *prompt.Message) (string, error) {
	if message == nil {
		return "", nil
	}
	itemID := r.identities.next(itemIdentity)
	if err := r.emitLocked(run, conversation.BlockCompleted{Block: conversation.Block{
		ID: itemID, Kind: conversation.BlockUser, Text: message.Text, Attachments: slices.Clone(message.Attachments),
	}}); err != nil {
		return "", err
	}
	return itemID, nil
}

func completeScriptAnswers(run *runState, provided []conversation.InterruptAnswer) ([]conversation.InterruptAnswer, error) {
	byID := make(map[string]conversation.Answer, len(run.answers)+len(provided))
	maps.Copy(byID, run.answers)
	for _, answer := range provided {
		byID[answer.ItemID] = answer.Answer
	}
	complete := make([]conversation.InterruptAnswer, 0, len(run.script.Interrupts))
	for _, interrupt := range run.script.Interrupts {
		id := conversation.InterruptItemID(interrupt)
		answer, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("mock: script interrupt %s has no answer", id)
		}
		complete = append(complete, conversation.InterruptAnswer{ItemID: id, Answer: conversation.CloneAnswer(answer)})
	}
	return complete, nil
}

func validateResumeSet(run *runState, answers []conversation.InterruptAnswer) error {
	if run.status != protocol.RunStatusWaiting {
		return fmt.Errorf("%w: run %s", conversation.ErrInterruptNotOpen, run.id)
	}
	if len(answers) != len(run.interrupts) {
		return fmt.Errorf("mock: resume answers %d interrupts; waiting set has %d", len(answers), len(run.interrupts))
	}
	byID := make(map[string]conversation.Answer, len(answers))
	for _, answer := range answers {
		byID[answer.ItemID] = answer.Answer
	}
	for _, interrupt := range run.interrupts {
		id := conversation.InterruptItemID(interrupt)
		answer, ok := byID[id]
		if !ok {
			return fmt.Errorf("mock: waiting interrupt %s has no answer", id)
		}
		if err := conversation.ValidateAnswer(interrupt, answer); err != nil {
			return fmt.Errorf("mock: interrupt %s: %w", id, err)
		}
	}
	return nil
}

func findApproval(interrupts []conversation.Interrupt, id string) *conversation.Approval {
	for _, interrupt := range interrupts {
		if approval, ok := interrupt.(conversation.Approval); ok && approval.ItemID == id {
			return &approval
		}
	}
	return nil
}

func findQuestion(interrupts []conversation.Interrupt, id string) *conversation.Question {
	for _, interrupt := range interrupts {
		if question, ok := interrupt.(conversation.Question); ok && question.ItemID == id {
			return &question
		}
	}
	return nil
}

func cloneAnswers(answers []conversation.InterruptAnswer) []conversation.InterruptAnswer {
	out := slices.Clone(answers)
	for i := range out {
		out[i].Answer = conversation.CloneAnswer(out[i].Answer)
	}
	return out
}

func (r *Runtime) CancelRun(ctx context.Context, in conversation.CancelRun) (conversation.RunCancellation, error) {
	if err := in.Validate(); err != nil {
		return conversation.RunCancellation{}, fmt.Errorf("mock: %w", err)
	}
	if err := context.Cause(ctx); err != nil {
		return conversation.RunCancellation{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.runs[in.RunID]
	if run == nil {
		return conversation.RunCancellation{}, fmt.Errorf("%w: %s", conversation.ErrRunNotFound, in.RunID)
	}
	if run.status == protocol.RunStatusFinished {
		return conversation.RunCancellation{}, fmt.Errorf("%w: %s", conversation.ErrRunFinished, run.id)
	}
	if err := r.finishLocked(run, Finish{Outcome: conversation.Outcome{Status: protocol.OutcomeCanceled, Detail: strings.TrimSpace(in.Reason)}}); err != nil {
		return conversation.RunCancellation{}, err
	}
	run.cancelOnce.Do(func() { close(run.cancel) })
	projected := projectRun(run)
	return conversation.RunCancellation{Canceled: projected, Root: projected.Clone()}, nil
}

func (r *Runtime) SteerRun(ctx context.Context, in prompt.SteerRun) (protocol.SteerRunResponse, error) {
	if err := in.Validate(); err != nil {
		return protocol.SteerRunResponse{}, fmt.Errorf("mock: %w", err)
	}
	if err := context.Cause(ctx); err != nil {
		return protocol.SteerRunResponse{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.runs[in.RunID]
	if run == nil {
		return protocol.SteerRunResponse{}, fmt.Errorf("%w: %s", conversation.ErrRunNotFound, in.RunID)
	}
	if run.status != protocol.RunStatusRunning || run.active != in.SegmentID {
		return protocol.SteerRunResponse{}, fmt.Errorf("%w: run %s is not executing segment %s", conversation.ErrStaleSegment, in.RunID, in.SegmentID)
	}
	if err := r.sessions[run.sessionID].requireRevisionCapacity(sessionEventRevisionChange()); err != nil {
		return protocol.SteerRunResponse{}, err
	}
	itemID := r.identities.next(itemIdentity)
	if err := r.emitLocked(run, conversation.BlockCompleted{Block: conversation.Block{
		ID: itemID, Kind: conversation.BlockUser,
		Text: in.Message.Text, Attachments: slices.Clone(in.Message.Attachments),
	}}); err != nil {
		return protocol.SteerRunResponse{}, err
	}
	return protocol.SteerRunResponse{UserItemID: itemID}, nil
}

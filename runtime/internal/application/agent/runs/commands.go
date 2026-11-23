package runs

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/media"
)

var (
	ErrRunAdmissionBusy         = fmt.Errorf("%w: run admission busy", run.ErrSessionBusy)
	errIsolationUnavailable     = errors.New("runs: sandbox isolation unavailable")
	ErrRunNotFound              = errors.New("runs: run not found")
	ErrChildRunNotAllowed       = errors.New("runs: child run control is not allowed")
	ErrRunWaiting               = errors.New("runs: run is waiting for a response")
	ErrRunFinished              = errors.New("runs: run has finished")
	ErrStaleSegment             = errors.New("runs: run is executing a different segment")
	ErrInstallationChanged      = errors.New("runs: installation changed during run assembly")
	ErrInterruptNotOpen         = errors.New("runs: interrupt not open")
	ErrInvalidInterruptResponse = errors.New("runs: invalid interrupt response")
	ErrExecutionClaimed         = errors.New("runs: parked execution already claimed")
	ErrExecutorNotLive          = errors.New("runs: executor not live")
	ErrExecutorParking          = errors.New("runs: executor is publishing a waiting boundary")
	ErrExecutorStateLost        = errors.New("runs: executor state lost")

	ErrInputRequired             = errors.New("runs: input required")
	ErrUnsupportedMedia          = errors.New("runs: unsupported media")
	ErrUnsupportedModelSelection = errors.New("runs: unsupported model selection")
	ErrInvalidRunOptions         = errors.New("runs: invalid run options")
	ErrInvalidCancellationReason = errors.New("runs: invalid cancellation reason")
	ErrInvalidStart              = errors.New("runs: invalid start")
)

type InputBlockError struct {
	Index  int
	Field  transcript.ContentField
	Detail string
	Cause  error
}

func (i *InputBlockError) Error() string {
	return fmt.Sprintf("runs: input block %d: %s", i.Index, i.Detail)
}

func (i *InputBlockError) Unwrap() error { return i.Cause }

func invalidInputContent(index int, err error) error {
	invalid, ok := errors.AsType[*transcript.ContentError](err)
	if !ok {
		return &InputBlockError{Index: index, Field: transcript.ContentKindField, Detail: err.Error(), Cause: ErrUnsupportedMedia}
	}
	cause := ErrUnsupportedMedia
	if invalid.Field == transcript.ContentTextField {
		cause = ErrInputRequired
	}
	return &InputBlockError{Index: index, Field: invalid.Field, Detail: invalid.Detail, Cause: cause}
}

type SessionOpening struct {
	SessionID  identity.SessionID
	Title, CWD string
	RunID      identity.RunID
}

type StartCommand struct {
	SessionID         identity.SessionID
	NewSession        *SessionOpening
	Actor             *operation.Actor
	ModelSelection    flamemodel.Selection
	Options           *corechat.Options
	Capabilities      run.Capabilities
	Input             []transcript.ContentBlock
	GoalIncarnationID goalref.IncarnationID
	Acceptance        *CommandIdentity
}

func (s StartCommand) clone() StartCommand {
	if s.Options != nil {
		options := s.Options.Clone()
		s.Options = &options
	}
	s.Capabilities = s.Capabilities.Clone()
	s.Input = transcript.CloneContent(s.Input)
	if s.Acceptance != nil {
		acceptance := *s.Acceptance
		s.Acceptance = &acceptance
	}
	if s.NewSession != nil {
		s.NewSession = new(*s.NewSession)
	}
	if s.Actor != nil {
		s.Actor = new(*s.Actor)
	}
	return s
}

func MaterializeUserMessage(input []transcript.ContentBlock) (corechat.Message, error) {
	parts := make([]corechat.Part, 0, len(input))
	for index, block := range input {
		if err := block.Validate(); err != nil {
			return corechat.Message{}, invalidInputContent(index, err)
		}
		if block.Kind == transcript.TextContent {
			parts = append(parts, corechat.NewTextPart(block.Text))
			continue
		}
		image, err := media.NewBytes(block.MediaType, block.Bytes)
		if err != nil {
			return corechat.Message{}, &InputBlockError{Index: index, Field: transcript.ContentBytesField, Detail: err.Error(), Cause: ErrUnsupportedMedia}
		}
		parts = append(parts, corechat.NewMediaPart(image))
	}
	if len(parts) == 0 {
		return corechat.Message{}, ErrInputRequired
	}
	message := corechat.NewUserMessage(parts...)
	if err := message.Validate(); err != nil {
		return corechat.Message{}, fmt.Errorf("runs: materialized user message: %w", err)
	}
	return message, nil
}

type ResumeCommand struct {
	RunID              identity.RunID
	Responses          []ResumeResponse
	Acceptance         *CommandIdentity
	Input              []transcript.ContentBlock
	CallerCapabilities run.Capabilities
}

func (r ResumeCommand) clone() ResumeCommand {
	if r.Acceptance != nil {
		r.Acceptance = new(*r.Acceptance)
	}
	r.Responses = slices.Clone(r.Responses)
	for index := range r.Responses {
		if r.Responses[index].Approval != nil {
			r.Responses[index].Approval = new(*r.Responses[index].Approval)
		}
		clonedQuestion := r.Responses[index].Question
		if clonedQuestion != nil {
			clonedQuestion = new(*clonedQuestion)
		}
		if question := clonedQuestion; question != nil {
			question.Answers = transcript.CloneAnswers(question.Answers)
			r.Responses[index].Question = question
		}
	}
	r.Input = transcript.CloneContent(r.Input)
	r.CallerCapabilities = r.CallerCapabilities.Clone()
	return r
}

type CommittedUserInput struct {
	ItemID  identity.ItemID
	Content []transcript.ContentBlock
}

type ResumeResponse struct {
	ItemID   identity.ItemID
	Kind     run.InterruptType
	Approval *ApprovalResponse
	Question *QuestionResponse
}

type ApprovalResponse struct {
	Approved      bool
	Arguments     string
	Reason        string
	RememberScope approval.Scope
}

type QuestionResponse struct {
	Answers [][]string
}

type QuestionAnswerError struct {
	ItemID identity.ItemID
	Index  int
	Detail string
}

func (q *QuestionAnswerError) Error() string {
	if q.Index < 0 {
		return fmt.Sprintf("question item %q answers: %s", q.ItemID, q.Detail)
	}
	return fmt.Sprintf("question item %q answer %d: %s", q.ItemID, q.Index+1, q.Detail)
}

func (q *QuestionAnswerError) Unwrap() error { return ErrInvalidInterruptResponse }

type CancelCommand struct {
	RunID         identity.RunID
	Reason        string
	AllowChildRun bool
	Receipt       CancellationReceipt
}

type CancellationReceipt interface {
	Key() string
	ReserveForRun(ctx context.Context, runID identity.RunID) error
	Refuse(ctx context.Context, reason error) error
	Stage(ctx context.Context, ref ExecutorRef, commit func(context.Context) (CancelResult, error)) error
}

func StageCancellation(
	ctx context.Context,
	receipt CancellationReceipt,
	ref ExecutorRef,
	commit func(context.Context) (CancelResult, error),
) (CancelResult, error) {
	if receipt == nil {
		return commit(ctx)
	}
	var result CancelResult
	err := receipt.Stage(ctx, ref, func(tx context.Context) (CancelResult, error) {
		var err error
		result, err = commit(tx)
		return result, err
	})
	return result, err
}

func CancellationKey(receipt CancellationReceipt) string {
	if receipt == nil {
		return ""
	}
	return receipt.Key()
}

const (
	defaultCancelReason = "user canceled the run"

	MaxCancellationReasonCharacters = 1024
)

func (c CancelCommand) normalizeReason() (CancelCommand, error) {
	c.Reason = strings.TrimSpace(c.Reason)
	if c.Reason == "" {
		c.Reason = defaultCancelReason
	}
	if !utf8.ValidString(c.Reason) || utf8.RuneCountInString(c.Reason) > MaxCancellationReasonCharacters {
		return CancelCommand{}, fmt.Errorf(
			"%w: reason must be valid UTF-8 containing at most %d characters",
			ErrInvalidCancellationReason,
			MaxCancellationReasonCharacters,
		)
	}
	return c, nil
}

type CancelResult struct {
	Run     run.Run
	RootRun *run.Run
}

type SteerCommand struct {
	Identity          *CommandIdentity
	RunID             identity.RunID
	ExpectedSegmentID identity.SegmentID
	Input             []transcript.ContentBlock
}

func (s SteerCommand) clone() SteerCommand {
	s.Input = transcript.CloneContent(s.Input)
	if s.Identity != nil {
		s.Identity = new(*s.Identity)
	}
	return s
}

type SubscribeRequest struct {
	RunID              identity.RunID
	SegmentID          identity.SegmentID
	Cursor             string
	CallerCapabilities run.Capabilities
}

func (s SubscribeRequest) clone() SubscribeRequest {
	s.CallerCapabilities = s.CallerCapabilities.Clone()
	return s
}

type Subscription struct {
	Record     Record
	HeadCursor string
	Events     iter.Seq[Event]
}

type Opening uint8

const (
	OpeningAdmitted Opening = iota
	OpeningReopened
	OpeningRetired
)

type StartResult struct {
	RunID      identity.RunID
	SegmentID  identity.SegmentID
	SessionID  identity.SessionID
	UserItemID identity.ItemID
	Events     iter.Seq[Event]
	Opening    Opening
}

func (r RootExecutionStart) Validate() error {
	if len(r.WorkingContext) == 0 {
		return ErrInputRequired
	}
	for index, message := range r.WorkingContext {
		if err := message.Validate(); err != nil {
			return fmt.Errorf("runs: working context message[%d]: %w", index, err)
		}
	}
	if r.WorkingContext[len(r.WorkingContext)-1].Role != corechat.RoleUser {
		return errors.New("runs: fresh working context must end with the current user message")
	}
	if err := r.ModelSelection.ValidateExact(); err != nil {
		return fmt.Errorf("runs: %w", err)
	}
	if err := (run.Capabilities{
		ChildRuns:      r.ChildRunAdmissionEnabled,
		InterruptTypes: r.InterruptTypes,
	}).Validate(); err != nil {
		return fmt.Errorf("runs: capabilities: %w", err)
	}
	return validateOptions(r.Options)
}

func (r RootExecutionStart) Clone() RootExecutionStart {
	if r.Options != nil {
		options := r.Options.Clone()
		r.Options = &options
	}
	messages := r.WorkingContext
	r.WorkingContext = make([]corechat.Message, len(messages))
	for index, message := range messages {
		r.WorkingContext[index] = message.Clone()
	}
	r.InterruptTypes = slices.Clone(r.InterruptTypes)
	return r
}

func validateOptions(options *corechat.Options) error {
	if options == nil {
		return nil
	}
	if err := options.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidRunOptions, err)
	}
	if options.Model != "" {
		return fmt.Errorf("%w: Options.Model must stay empty; use Provider and Model", ErrInvalidRunOptions)
	}
	if options.ReasoningEffort != "" {
		return fmt.Errorf("%w: Options.ReasoningEffort must stay empty; use ModelSelection", ErrInvalidRunOptions)
	}
	return nil
}

type ActiveRunConflictError struct {
	RunID identity.RunID
	State run.State
}

func (a *ActiveRunConflictError) Error() string {
	return fmt.Sprintf("runs: session already has a %s run %q", a.State, a.RunID)
}

func (r StartResult) Reopened() bool { return r.Opening != OpeningAdmitted }

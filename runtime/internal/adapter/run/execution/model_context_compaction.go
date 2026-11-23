package execution

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
)

var (
	errInvalidModelContextCompaction = errors.New("execution: invalid model-context compaction")
	ErrModelContextDiverged          = errors.New("execution: model context diverged from durable conversation")
)

type ModelContextTokenCalibration struct {
	reported  int64
	estimated int
}

func NewModelContextTokenCalibration(
	reported int64,
	estimated int,
) (ModelContextTokenCalibration, error) {
	if reported <= 0 {
		return ModelContextTokenCalibration{}, fmt.Errorf(
			"%w: reported context tokens must be positive",
			errInvalidModelContextCompaction,
		)
	}
	if estimated <= 0 {
		return ModelContextTokenCalibration{}, fmt.Errorf(
			"%w: estimated context tokens must be positive",
			errInvalidModelContextCompaction,
		)
	}
	return ModelContextTokenCalibration{reported: reported, estimated: estimated}, nil
}

func (m ModelContextTokenCalibration) Validate() error {
	if m == (ModelContextTokenCalibration{}) {
		return nil
	}
	if m.reported <= 0 || m.estimated <= 0 {
		return fmt.Errorf(
			"%w: token calibration requires positive reported and estimated values",
			errInvalidModelContextCompaction,
		)
	}
	return nil
}

func (m ModelContextTokenCalibration) ReportedTokens() int64 { return m.reported }
func (m ModelContextTokenCalibration) EstimatedTokens() int  { return m.estimated }

func (m ModelContextTokenCalibration) Adjustment() int {
	if m == (ModelContextTokenCalibration{}) {
		return 0
	}
	if m.reported > int64(math.MaxInt) {
		return math.MaxInt
	}
	return int(m.reported) - m.estimated
}

type modelContextPersistence uint8

const (
	modelContextTransient modelContextPersistence = iota + 1
	modelContextDurable
)

type modelContextInputTokenCounter interface {
	CountInputTokens(context.Context, *corechat.Request) (int64, error)
}

type ModelContextCompaction struct {
	persistence   modelContextPersistence
	sessionID     identity.SessionID
	selection     flamemodel.Selection
	limits        modelref.TokenLimits
	instructions  []corechat.Message
	candidate     []corechat.Message
	trailer       []corechat.Message
	tools         []toolcontract.Definition
	options       corechat.Options
	calibration   ModelContextTokenCalibration
	counter       modelContextInputTokenCounter
	fits          func(context.Context, *corechat.Request) (bool, error)
	protectedTail int
	preCompact    func(context.Context) (bool, error)
}

// A value rather than parameters because two members are message slices that must
// not be transposable.
type ModelContextCompactionInput struct {
	SessionID    identity.SessionID
	Selection    flamemodel.Selection
	Limits       modelref.TokenLimits
	Instructions []corechat.Message
	Candidate    []corechat.Message
	Trailer      []corechat.Message
	Tools        []toolcontract.Definition
	Options      corechat.Options
	Calibration  ModelContextTokenCalibration
	Counter      modelContextInputTokenCounter
	RequestFits  func(context.Context, *corechat.Request) (bool, error)
	// Counted against Candidate, not against what the compactor folds: a durable
	// compaction reattaches the rest of Candidate afterwards.
	ProtectedTail int
	PreCompact    func(context.Context) (bool, error)
}

func NewDurableModelContextCompaction(input ModelContextCompactionInput) (ModelContextCompaction, error) {
	return newModelContextCompaction(modelContextDurable, input)
}

func newTransientModelContextCompaction(input ModelContextCompactionInput) (ModelContextCompaction, error) {
	return newModelContextCompaction(modelContextTransient, input)
}

func (input ModelContextCompactionInput) validateMessages() error {
	for index := range input.Instructions {
		if input.Instructions[index].Role != corechat.RoleSystem {
			return fmt.Errorf("instruction %d is not a System message", index)
		}
	}
	for _, group := range []struct {
		label    string
		messages []corechat.Message
	}{{"instruction", input.Instructions}, {"candidate message", input.Candidate}, {"trailer message", input.Trailer}} {
		for index := range group.messages {
			if err := group.messages[index].Validate(); err != nil {
				return fmt.Errorf("%s %d: %w", group.label, index, err)
			}
		}
	}
	return nil
}

func (input ModelContextCompactionInput) validate() error {
	if err := input.SessionID.Validate(); err != nil {
		return fmt.Errorf("owning %w", err)
	}
	if err := input.Selection.ValidateExact(); err != nil {
		return err
	}
	if len(input.Candidate) == 0 {
		return errors.New("candidate conversation is empty")
	}
	if input.ProtectedTail < 0 || input.ProtectedTail > len(input.Candidate) {
		return fmt.Errorf("protected tail %d is outside [0,%d]", input.ProtectedTail, len(input.Candidate))
	}
	if err := input.validateMessages(); err != nil {
		return err
	}
	for index := range input.Tools {
		if err := input.Tools[index].Validate(); err != nil {
			return fmt.Errorf("tool definition %d: %w", index, err)
		}
	}
	if err := input.Options.Validate(); err != nil {
		return fmt.Errorf("model options: %w", err)
	}
	if err := validateModelOutputReservation(input.Limits, &input.Options); err != nil {
		return fmt.Errorf("model output reservation: %w", err)
	}
	if input.Counter != nil && lo.IsNil(input.Counter) {
		return errors.New("input token counter is nil")
	}
	return nil
}

func newModelContextCompaction(
	persistence modelContextPersistence,
	input ModelContextCompactionInput,
) (ModelContextCompaction, error) {
	if persistence != modelContextDurable && persistence != modelContextTransient {
		return ModelContextCompaction{}, errInvalidModelContextCompaction
	}
	if err := input.validate(); err != nil {
		return ModelContextCompaction{}, fmt.Errorf("%w: %w", errInvalidModelContextCompaction, err)
	}
	if err := input.Calibration.Validate(); err != nil {
		return ModelContextCompaction{}, err
	}
	frozenTools := make([]toolcontract.Definition, len(input.Tools))
	for index := range input.Tools {
		frozenTools[index] = input.Tools[index].Clone()
	}
	return ModelContextCompaction{
		persistence: persistence,
		sessionID:   input.SessionID,
		limits:      input.Limits, selection: input.Selection,
		instructions:  cloneChatMessages(input.Instructions),
		candidate:     cloneChatMessages(input.Candidate),
		trailer:       cloneChatMessages(input.Trailer),
		tools:         frozenTools,
		options:       input.Options.Clone(),
		calibration:   input.Calibration,
		counter:       input.Counter,
		fits:          input.RequestFits,
		protectedTail: input.ProtectedTail,
		preCompact:    input.PreCompact,
	}, nil
}

func (m ModelContextCompaction) Durable() bool {
	return m.persistence == modelContextDurable
}

func (m ModelContextCompaction) SessionID() identity.SessionID { return m.sessionID }

func (m ModelContextCompaction) ModelSelection() flamemodel.Selection { return m.selection }
func (m ModelContextCompaction) TokenLimits() modelref.TokenLimits    { return m.limits }

func (m ModelContextCompaction) RequestFits(ctx context.Context, request *corechat.Request) (bool, error) {
	if m.fits == nil {
		return true, nil
	}
	return m.fits(ctx, request)
}

func (m ModelContextCompaction) Instructions() []corechat.Message {
	return cloneChatMessages(m.instructions)
}

func (m ModelContextCompaction) Candidate() []corechat.Message {
	return cloneChatMessages(m.candidate)
}

func (m ModelContextCompaction) EphemeralSuffixStart(durable []corechat.Message) (int, error) {
	candidateMessages, err := normalizedModelContextMessages(m.candidate, "Interaction conversation")
	if err != nil {
		return 0, err
	}
	durableMessages, err := normalizedModelContextMessages(durable, "durable conversation")
	if err != nil {
		return 0, err
	}
	if len(candidateMessages) < len(durableMessages) {
		return 0, fmt.Errorf("%w: semantic_message_count=%d/%d",
			ErrModelContextDiverged, len(candidateMessages), len(durableMessages))
	}
	for index := range durableMessages {
		equal, err := sameInteractionMessages(
			[]corechat.Message{candidateMessages[index].message},
			[]corechat.Message{durableMessages[index].message},
		)
		if err != nil {
			return 0, fmt.Errorf("execution: compare durable message %d: %w", index, err)
		}
		if !equal {
			return 0, fmt.Errorf("%w: first_difference=message[%d]", ErrModelContextDiverged, index)
		}
	}
	if len(durableMessages) == 0 {
		return 0, nil
	}
	return candidateMessages[len(durableMessages)-1].sourceEnd, nil
}

type modelContextMessage struct {
	message   corechat.Message
	sourceEnd int
}

func normalizedModelContextMessages(messages []corechat.Message, owner string) ([]modelContextMessage, error) {
	normalized := make([]modelContextMessage, 0, len(messages))
	for index := range messages {
		if err := messages[index].Validate(); err != nil {
			return nil, fmt.Errorf("execution: %s message %d: %w", owner, index, err)
		}
		message := messages[index].Clone()
		if message.Role == corechat.RoleTool && len(message.Metadata) == 0 && len(normalized) > 0 &&
			normalized[len(normalized)-1].message.Role == corechat.RoleTool &&
			len(normalized[len(normalized)-1].message.Metadata) == 0 {
			last := &normalized[len(normalized)-1]
			last.message.Parts = append(last.message.Parts, message.Parts...)
			last.sourceEnd = index + 1
			continue
		}
		normalized = append(normalized, modelContextMessage{message: message, sourceEnd: index + 1})
	}
	return normalized, nil
}

func (m ModelContextCompaction) Trailer() []corechat.Message {
	return cloneChatMessages(m.trailer)
}

func (m ModelContextCompaction) Tools() []toolcontract.Definition {
	tools := make([]toolcontract.Definition, len(m.tools))
	for index := range m.tools {
		tools[index] = m.tools[index].Clone()
	}
	return tools
}

func (m ModelContextCompaction) Options() corechat.Options { return m.options.Clone() }

func (m ModelContextCompaction) TokenEstimateAdjustment() int {
	return m.calibration.Adjustment()
}

func (m ModelContextCompaction) HasInputTokenCounter() bool { return m.counter != nil }

func (m ModelContextCompaction) CountInputTokens(
	ctx context.Context,
	candidate []corechat.Message,
) (int64, error) {
	if m.counter == nil {
		return 0, errors.New("execution: model-context input token counter is unavailable")
	}
	messages := make([]corechat.Message, 0, len(m.instructions)+len(candidate))
	messages = append(messages, cloneChatMessages(m.instructions)...)
	messages = append(messages, cloneChatMessages(candidate)...)
	request := &corechat.Request{
		Messages: messages,
		Tools:    m.Tools(),
		Options:  m.options.Clone(),
	}
	count, err := m.counter.CountInputTokens(ctx, request)
	if err != nil {
		return 0, fmt.Errorf("execution: count model-context input tokens: %w", err)
	}
	if count <= 0 {
		return 0, fmt.Errorf(
			"%w: input token counter returned a non-positive count: %d",
			errInvalidModelContextCompaction,
			count,
		)
	}
	return count, nil
}

func (m ModelContextCompaction) ProtectedTail() int { return m.protectedTail }

func (m ModelContextCompaction) AllowsCompaction(ctx context.Context) (bool, error) {
	if m.preCompact == nil {
		return true, nil
	}
	return m.preCompact(ctx)
}

type ModelContextCompactionResult struct {
	rewrite         *conversation.Compaction
	messages        []corechat.Message
	changed         bool
	summary         string
	messagesBefore  int
	estimatedTokens int
}

func NewModelContextCompactionResult(
	messages []corechat.Message,
	changed bool,
	summary string,
	messagesBefore int,
	estimatedTokens int,
) (ModelContextCompactionResult, error) {
	if len(messages) == 0 {
		return ModelContextCompactionResult{}, fmt.Errorf(
			"%w: result messages are empty",
			errInvalidModelContextCompaction,
		)
	}
	canonicalSummary := strings.TrimSpace(summary)
	if canonicalSummary != summary {
		return ModelContextCompactionResult{}, fmt.Errorf(
			"%w: result summary is not canonical",
			errInvalidModelContextCompaction,
		)
	}
	if canonicalSummary != "" && !changed {
		return ModelContextCompactionResult{}, fmt.Errorf(
			"%w: summarized result must be changed",
			errInvalidModelContextCompaction,
		)
	}
	if messagesBefore < 0 {
		return ModelContextCompactionResult{}, fmt.Errorf(
			"%w: result message count before compaction is negative: %d",
			errInvalidModelContextCompaction,
			messagesBefore,
		)
	}
	if estimatedTokens <= 0 {
		return ModelContextCompactionResult{}, fmt.Errorf(
			"%w: result token estimate must be positive",
			errInvalidModelContextCompaction,
		)
	}
	for index := range messages {
		if err := messages[index].Validate(); err != nil {
			return ModelContextCompactionResult{}, fmt.Errorf(
				"%w: result message %d: %w",
				errInvalidModelContextCompaction,
				index,
				err,
			)
		}
	}
	return ModelContextCompactionResult{
		messages:        cloneChatMessages(messages),
		changed:         changed,
		summary:         canonicalSummary,
		messagesBefore:  messagesBefore,
		estimatedTokens: estimatedTokens,
	}, nil
}

func NewDurableModelContextCompactionResult(messages []corechat.Message, summary string, before, tokens int, rewrite conversation.Compaction) (ModelContextCompactionResult, error) {
	result, err := NewModelContextCompactionResult(messages, true, summary, before, tokens)
	if err != nil {
		return ModelContextCompactionResult{}, err
	}
	if rewrite.ExpectedCount() > before {
		return ModelContextCompactionResult{}, errors.New("execution: durable compaction exceeds its candidate history")
	}
	result.rewrite = new(rewrite)
	return result, nil
}

func (m ModelContextCompactionResult) Rewrite() (conversation.Compaction, bool) {
	if m.rewrite == nil {
		return conversation.Compaction{}, false
	}
	return *m.rewrite, true
}

func (m ModelContextCompactionResult) Messages() []corechat.Message {
	return cloneChatMessages(m.messages)
}

func (m ModelContextCompactionResult) Changed() bool { return m.changed }

func (m ModelContextCompactionResult) Summarized() bool { return m.summary != "" }

func (m ModelContextCompactionResult) Summary() string { return m.summary }

func (m ModelContextCompactionResult) MessageCounts() (before int, after int) {
	return m.messagesBefore, len(m.messages)
}

func (m ModelContextCompactionResult) EstimatedTokens() int { return m.estimatedTokens }

type ModelContextCompactor interface {
	CompactModelContext(
		ctx context.Context,
		request ModelContextCompaction,
	) (ModelContextCompactionResult, error)
}

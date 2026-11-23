package maintenance

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	executionadapter "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	"github.com/Tangerg/scope/core/chat"
)

var (
	errModelContextCannotFit        = errors.New("maintenance: protected model context cannot fit")
	errModelContextCompactionVetoed = errors.New("maintenance: required model context compaction was vetoed")
)

func (c *Compactor) durableHistory(ctx context.Context, request executionadapter.ModelContextCompaction) ([]chat.Message, []chat.Message, int, error) {
	candidate := request.Candidate()
	stored, err := c.store.Read(ctx, request.SessionID())
	if err != nil {
		return nil, nil, 0, fmt.Errorf("maintenance: read model context: %w", err)
	}
	ephemeralStart, err := request.EphemeralSuffixStart(stored)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("maintenance: candidate_messages=%d durable_messages=%d: %w",
			len(candidate), len(stored), err)
	}
	ephemeral := candidate[ephemeralStart:]
	protectedTail := max(request.ProtectedTail()-len(ephemeral), 0)
	if protectedTail > len(stored) {
		return nil, nil, 0, fmt.Errorf("%w: protected durable tail %d exceeds stored history %d",
			executionadapter.ErrModelContextDiverged, protectedTail, len(stored))
	}
	return stored, ephemeral, protectedTail, nil
}

func (c *Compactor) CompactModelContext(
	ctx context.Context,
	request executionadapter.ModelContextCompaction,
) (executionadapter.ModelContextCompactionResult, error) {
	candidate := request.Candidate()
	history, ephemeral, protectedTail := candidate, []chat.Message(nil), request.ProtectedTail()
	if request.Durable() {
		var err error
		history, ephemeral, protectedTail, err = c.durableHistory(ctx, request)
		if err != nil {
			return executionadapter.ModelContextCompactionResult{}, err
		}
	}

	limits := request.TokenLimits()
	options := request.Options()
	trigger, err := c.policy.tokenTrigger(limits, options)
	if err != nil {
		return executionadapter.ModelContextCompactionResult{}, fmt.Errorf(
			"maintenance: resolve model-context token trigger: %w",
			err,
		)
	}
	budget := newModelContextBudget(
		trigger,
		request.Instructions(),
		slices.Concat(ephemeral, request.Trailer()),
		request.Tools(),
		options,
		request.TokenEstimateAdjustment(),
		newModelContextCounter(request),
	)
	budget.fits = request.RequestFits
	plan, err := c.planCompactionWithProtectedTail(ctx, history, budget, protectedTail)
	if err != nil {
		return executionadapter.ModelContextCompactionResult{}, err
	}
	if plan.action == noCompaction {
		return unchangedModelContextResult(candidate, plan.estimatedTokens)
	}
	allowed, err := request.AllowsCompaction(ctx)
	if err != nil {
		return executionadapter.ModelContextCompactionResult{}, fmt.Errorf("maintenance: pre-compaction hooks: %w", err)
	}
	if !allowed {
		return executionadapter.ModelContextCompactionResult{}, errModelContextCompactionVetoed
	}

	replacement, summary, cutoff, prefixAfter, err := c.materializeModelContextPlan(
		ctx,
		request.SessionID(),
		plan,
	)
	if err != nil {
		return executionadapter.ModelContextCompactionResult{}, err
	}
	overBudget, estimatedTokens, err := budget.exceeded(ctx, replacement)
	if err != nil {
		return executionadapter.ModelContextCompactionResult{}, err
	}
	if overBudget {
		return executionadapter.ModelContextCompactionResult{}, errModelContextCannotFit
	}
	effective := slices.Concat(replacement, ephemeral)
	result, err := executionadapter.NewModelContextCompactionResult(
		effective,
		true,
		summary,
		len(candidate),
		estimatedTokens,
	)
	if err != nil {
		return executionadapter.ModelContextCompactionResult{}, err
	}
	if request.Durable() {
		rewrite, err := conversation.NewCompaction(len(history), cutoff, prefixAfter, replacement)
		if err != nil {
			return executionadapter.ModelContextCompactionResult{}, err
		}
		return executionadapter.NewDurableModelContextCompactionResult(effective, summary, len(candidate), estimatedTokens, rewrite)
	}
	return result, nil
}

func unchangedModelContextResult(
	candidate []chat.Message,
	estimatedTokens int,
) (executionadapter.ModelContextCompactionResult, error) {
	return executionadapter.NewModelContextCompactionResult(
		candidate,
		false,
		"",
		len(candidate),
		estimatedTokens,
	)
}

type modelContextCounter executionadapter.ModelContextCompaction

func newModelContextCounter(request executionadapter.ModelContextCompaction) modelContextInputTokenCounter {
	if !request.HasInputTokenCounter() {
		return nil
	}
	return modelContextCounter(request)
}

func (m modelContextCounter) CountInputTokens(
	ctx context.Context,
	messages []chat.Message,
) (int64, error) {
	return executionadapter.ModelContextCompaction(m).CountInputTokens(ctx, messages)
}

func (c *Compactor) materializeModelContextPlan(
	ctx context.Context,
	sessionID identity.SessionID,
	plan compactionPlan,
) (
	replacement []chat.Message,
	summary string,
	cutoff int,
	prefixAfter int,
	err error,
) {
	switch plan.action {
	case summarizeCompaction:
		references, hasReferences, err := modeladapter.RetainedToolResultReferences(plan.older)
		if err != nil {
			return nil, "", 0, 0, fmt.Errorf("maintenance: retain tool-result references: %w", err)
		}
		summary, err := c.summarize(ctx, plan.older)
		if err != nil {
			return nil, "", 0, 0, fmt.Errorf("maintenance: summarize model context: %w", err)
		}
		replacement = make([]chat.Message, 0, 2+len(plan.recent))
		replacement = append(replacement, chat.NewSystemMessage(compactionModelPrefix+summary))
		if hasReferences {
			replacement = append(replacement, references)
		}
		if reminder, ok := liveStateReminder(c.liveState(ctx, sessionID)); ok {
			replacement = append(replacement, reminder)
		}
		replacement = append(replacement, plan.recent...)
		return replacement, summary, plan.cutoff, len(replacement) - len(plan.recent), nil
	default:
		return nil, "", 0, 0, errors.New("maintenance: unsupported model-context compaction plan")
	}
}

func cloneMessages(messages []chat.Message) []chat.Message {
	cloned := make([]chat.Message, len(messages))
	for index := range messages {
		cloned[index] = messages[index].Clone()
	}
	return cloned
}

var _ executionadapter.ModelContextCompactor = (*Compactor)(nil)

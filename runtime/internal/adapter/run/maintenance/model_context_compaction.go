package maintenance

import (
	"context"
	"errors"
	"fmt"
	"slices"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	executionadapter "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/scope/core/chat"
)

var (
	// ErrModelContextDiverged reports that the Interaction candidate no longer
	// begins with the exact durable conversation snapshot.
	ErrModelContextDiverged = errors.New("maintenance: model context diverged from durable conversation")
	// ErrModelContextCannotFit reports that only protected or fixed context is
	// over budget, so no truthful compaction can make the request executable.
	ErrModelContextCannotFit = errors.New("maintenance: protected model context cannot fit")
	// ErrModelContextCompactionVetoed reports that a lifecycle hook blocked a
	// compaction required to keep the next model request inside its budget.
	ErrModelContextCompactionVetoed = errors.New("maintenance: required model context compaction was vetoed")
)

// CompactModelContext reduces the exact mutable context for one imminent model
// call. Durable root history is reconciled and rewritten transactionally;
// transient child history is reduced only in the returned Interaction state.
func (c *Compactor) CompactModelContext(
	ctx context.Context,
	request executionadapter.ModelContextCompaction,
) (executionadapter.ModelContextCompactionResult, error) {
	candidate := request.Candidate()
	history := candidate
	protectedTail := request.ProtectedTail()
	var ephemeral []chat.Message
	if request.Durable() {
		stored, err := c.store.Read(ctx, request.SessionID())
		if err != nil {
			return executionadapter.ModelContextCompactionResult{}, fmt.Errorf("maintenance: read model context: %w", err)
		}
		candidatePrefix, matches, difference, err := request.CompareDurableHistory(stored)
		if err != nil {
			return executionadapter.ModelContextCompactionResult{}, err
		}
		if !matches {
			return executionadapter.ModelContextCompactionResult{}, fmt.Errorf(
				"%w: candidate_messages=%d durable_messages=%d first_difference=%s",
				ErrModelContextDiverged,
				len(candidate),
				len(stored),
				difference,
			)
		}
		history = stored
		ephemeral = candidate[candidatePrefix:]
		// ProtectedTail counts trailing Candidate messages, and the ephemeral
		// suffix is reattached verbatim after the fold, so it already satisfies
		// that many of them. Only what reaches past it has to be protected inside
		// the durable history, which is the sequence this compaction folds.
		protectedTail = max(protectedTail-len(ephemeral), 0)
		if protectedTail > len(history) {
			return executionadapter.ModelContextCompactionResult{}, fmt.Errorf(
				"%w: protected durable tail %d exceeds stored history %d",
				ErrModelContextDiverged,
				protectedTail,
				len(history),
			)
		}
	}

	limits, _, err := modeladapter.LookupTokenLimits(request.ModelSelection())
	if err != nil {
		return executionadapter.ModelContextCompactionResult{}, err
	}
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
		return executionadapter.ModelContextCompactionResult{}, ErrModelContextCompactionVetoed
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
		return executionadapter.ModelContextCompactionResult{}, ErrModelContextCannotFit
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
		if err := c.store.RewriteForCompaction(
			ctx,
			request.SessionID(),
			len(history),
			cutoff,
			prefixAfter,
			replacement...,
		); err != nil {
			return executionadapter.ModelContextCompactionResult{}, fmt.Errorf(
				"maintenance: persist model context compaction: %w",
				err,
			)
		}
	}
	c.contextState.ForgetSessionContext(request.SessionID())
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
	sessionID string,
	plan compactionPlan,
) (
	replacement []chat.Message,
	summary string,
	cutoff int,
	prefixAfter int,
	err error,
) {
	switch plan.action {
	case trimCompaction:
		return plan.trimmed, "", 0, 0, nil
	case summarizeCompaction:
		summary, err := c.summarize(ctx, plan.older)
		if err != nil {
			return nil, "", 0, 0, fmt.Errorf("maintenance: summarize model context: %w", err)
		}
		replacement = make([]chat.Message, 0, 2+len(plan.recent))
		replacement = append(replacement, chat.NewSystemMessage(compactionModelPrefix+summary))
		if c.liveState != nil {
			if reminder, ok := liveStateReminder(c.liveState(ctx, sessionID)); ok {
				replacement = append(replacement, reminder)
			}
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

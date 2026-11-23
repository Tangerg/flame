package maintenance

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
)

type compactionStore interface {
	Read(context.Context, identity.SessionID) ([]chat.Message, error)
}

type Compactor struct {
	store     compactionStore
	client    modeladapter.AuxiliaryResolver
	liveState LiveStateSnapshotter
	policy    compactionPolicy
}

type compactionAction uint8

const (
	noCompaction compactionAction = iota
	summarizeCompaction
)

type compactionPlan struct {
	action          compactionAction
	cutoff          int
	older           []chat.Message
	recent          []chat.Message
	estimatedTokens int
}

func NewCompactor(
	store compactionStore,
	client modeladapter.AuxiliaryResolver,
	liveState LiveStateSnapshotter,
	values CompactionPolicyValues,
) (*Compactor, error) {
	if lo.IsNil(store) {
		return nil, errors.New("compactor: conversation store is required")
	}
	if client == nil || liveState == nil {
		return nil, errors.New("compactor: utility model resolver and live-state snapshotter are required")
	}
	policy, err := newCompactionPolicy(values)
	if err != nil {
		return nil, err
	}
	return &Compactor{
		store: store, client: client, liveState: liveState,
		policy: policy,
	}, nil
}

func (c *Compactor) planCompactionWithProtectedTail(
	ctx context.Context,
	messages []chat.Message,
	budget modelContextBudget,
	protectedTail int,
) (compactionPlan, error) {
	overBudget, estimatedTokens, err := budget.triggered(ctx, messages)
	if err != nil {
		return compactionPlan{}, err
	}
	if !overBudget || len(messages) == 0 {
		return compactionPlan{estimatedTokens: estimatedTokens}, nil
	}
	if protectedTail < 0 || protectedTail > len(messages) {
		return compactionPlan{}, errModelContextCannotFit
	}
	foldableLimit := len(messages) - protectedTail
	protectedOverBudget, _, err := budget.exceeded(ctx, messages[foldableLimit:])
	if err != nil {
		return compactionPlan{}, err
	}
	if protectedOverBudget {
		return compactionPlan{}, errModelContextCannotFit
	}
	if foldableLimit == 0 {
		return compactionPlan{}, errModelContextCannotFit
	}
	cutoff := summaryCutoffWithProtectedTail(messages, protectedTail)
	if cutoff == 0 {
		return compactionPlan{}, errModelContextCannotFit
	}
	// A prefix summary cannot converge when the latest turn alone is over budget, so
	// the whole foldable history is widened.
	recentOverBudget, _, err := budget.exceeded(ctx, messages[cutoff:])
	if err != nil {
		return compactionPlan{}, err
	}
	if recentOverBudget {
		cutoff = foldableLimit
	}
	return compactionPlan{
		action:          summarizeCompaction,
		cutoff:          cutoff,
		older:           messages[:cutoff],
		recent:          messages[cutoff:],
		estimatedTokens: estimatedTokens,
	}, nil
}

func summaryCutoffWithProtectedTail(
	messages []chat.Message,
	protectedTail int,
) int {
	if protectedTail < 0 || protectedTail > len(messages) {
		return 0
	}
	foldable := messages[:len(messages)-protectedTail]
	for index := len(foldable) - 1; index >= 0; index-- {
		if foldable[index].Role == chat.RoleUser {
			if index > 0 {
				return index
			}
			return len(foldable)
		}
	}
	return len(foldable)
}

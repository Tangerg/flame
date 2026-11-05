package maintenance

import (
	"context"
	"errors"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
)

// compactionStore supplies the durable prefix used to compute a proposal.
type compactionStore interface {
	Read(context.Context, string) ([]chat.Message, error)
}

// Compactor reduces the exact context of an imminent model request.
type Compactor struct {
	store     compactionStore
	client    modeladapter.AuxiliaryResolver
	liveState LiveStateSnapshotter // nil = no post-compaction live-state reminder
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

// NewCompactor requires a chat history store and per-call chat-client resolver.
// liveState (nil to disable) snapshots a
// session's still-active process state so an LLM summary rung can remind the
// model of running shells the summary cannot reconstruct.
// The Run fact owner commits the returned proposal before retiring derived authority.
func NewCompactor(
	store compactionStore,
	client modeladapter.AuxiliaryResolver,
	liveState LiveStateSnapshotter,
	values CompactionPolicyValues,
) (*Compactor, error) {
	if lo.IsNil(store) {
		return nil, errors.New("compactor: conversation store is required")
	}
	if client == nil {
		return nil, errors.New("compactor: utility model resolver is required")
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
	// The latest turn is not a license to preserve a suffix that is already over
	// budget by itself. In that case a prefix summary cannot converge, so widen
	// to the complete foldable history. Original arguments and output must reach
	// the summarizer before any lossy reduction is proposed.
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

// summaryCutoffWithProtectedTail preserves the latest foldable user turn and
// the caller-owned exact suffix. When one long turn alone fills the context,
// the complete foldable turn is summarized so the next model request can fit.
// A prior summary can already represent the user turn; its continuation remains
// foldable even though no original User message survives.
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

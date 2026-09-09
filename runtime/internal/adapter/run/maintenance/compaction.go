package maintenance

import (
	"context"
	"errors"

	"github.com/Tangerg/scope/core/chat"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/model"
	"github.com/Tangerg/flame/runtime/internal/dependency"
)

// compactionStore is the worker's narrow conversation use-case view. The
// implementation owns the cross-aggregate transaction that replaces history
// and rebases Run watermarks; this model worker only decides summary content.
// Read transfers decoded messages to the caller. RewriteForCompaction borrows
// messages synchronously and must copy any values it retains.
type compactionStore interface {
	Read(ctx context.Context, sessionID string) ([]chat.Message, error)
	RewriteForCompaction(
		ctx context.Context,
		sessionID string,
		expectedCount int,
		cutoff int,
		replacementPrefix int,
		messages ...chat.Message,
	) error
}

// SessionContextInvalidator retires process-local authority derived from model
// context that a successful compaction replaced.
type SessionContextInvalidator interface {
	ForgetSessionContext(sessionID string)
}

// Compactor reduces the exact context of an imminent model request.
type Compactor struct {
	store        compactionStore
	client       modeladapter.AuxiliaryResolver
	liveState    LiveStateSnapshotter // nil = no post-compaction live-state reminder
	contextState SessionContextInvalidator
	policy       compactionPolicy
}

type compactionAction uint8

const (
	noCompaction compactionAction = iota
	trimCompaction
	summarizeCompaction
)

type compactionPlan struct {
	action          compactionAction
	cutoff          int
	trimmed         []chat.Message
	older           []chat.Message
	recent          []chat.Message
	estimatedTokens int
}

// NewCompactor requires a chat history store and per-call chat-client resolver.
// liveState (nil to disable) snapshots a
// session's still-active process state so an LLM summary rung can remind the
// model of running shells the summary cannot reconstruct. contextState is
// required to retire read-before-write authority after history changes.
func NewCompactor(
	store compactionStore,
	client modeladapter.AuxiliaryResolver,
	liveState LiveStateSnapshotter,
	values CompactionPolicyValues,
	contextState SessionContextInvalidator,
) (*Compactor, error) {
	if dependency.Missing(store) {
		return nil, errors.New("compactor: conversation store is required")
	}
	if client == nil {
		return nil, errors.New("compactor: utility model resolver is required")
	}
	if dependency.Missing(contextState) {
		return nil, errors.New("compactor: session context invalidator is required")
	}
	policy, err := newCompactionPolicy(values)
	if err != nil {
		return nil, err
	}
	return &Compactor{
		store: store, client: client, liveState: liveState,
		contextState: contextState, policy: policy,
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
		return compactionPlan{}, ErrModelContextCannotFit
	}
	foldableLimit := len(messages) - protectedTail
	protectedOverBudget, _, err := budget.exceeded(ctx, messages[foldableLimit:])
	if err != nil {
		return compactionPlan{}, err
	}
	if protectedOverBudget {
		return compactionPlan{}, ErrModelContextCannotFit
	}
	if foldableLimit == 0 {
		return compactionPlan{}, ErrModelContextCannotFit
	}
	cutoff := summaryCutoffWithProtectedTail(messages, protectedTail)
	if cutoff == 0 {
		return compactionPlan{}, ErrModelContextCannotFit
	}
	trimmed, changed := trimForBudgetBefore(messages, cutoff)
	trimmedOverBudget, trimmedEstimate, err := budget.exceeded(ctx, trimmed)
	if err != nil {
		return compactionPlan{}, err
	}
	if changed && !trimmedOverBudget {
		return compactionPlan{
			action:          trimCompaction,
			trimmed:         trimmed,
			estimatedTokens: trimmedEstimate,
		}, nil
	}
	// The latest turn is not a license to preserve a suffix that is already over
	// budget by itself. In that case a prefix summary cannot converge, so widen
	// the deterministic rung first and summarize the complete foldable history
	// only when cheap trimming still cannot make it executable.
	recentOverBudget, _, err := budget.exceeded(ctx, trimmed[cutoff:])
	if err != nil {
		return compactionPlan{}, err
	}
	if recentOverBudget {
		cutoff = foldableLimit
		trimmed, changed = trimForBudgetBefore(messages, cutoff)
		trimmedOverBudget, trimmedEstimate, err = budget.exceeded(ctx, trimmed)
		if err != nil {
			return compactionPlan{}, err
		}
		if changed && !trimmedOverBudget {
			return compactionPlan{
				action:          trimCompaction,
				trimmed:         trimmed,
				estimatedTokens: trimmedEstimate,
			}, nil
		}
	}
	return compactionPlan{
		action:          summarizeCompaction,
		cutoff:          cutoff,
		older:           trimmed[:cutoff],
		recent:          trimmed[cutoff:],
		estimatedTokens: estimatedTokens,
	}, nil
}

// summaryCutoffWithProtectedTail preserves the latest foldable user turn and
// the caller-owned exact suffix. When one long turn alone fills the context,
// the complete foldable turn is summarized so the next model request can fit.
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
	return 0
}

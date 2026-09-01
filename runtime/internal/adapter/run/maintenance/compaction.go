package maintenance

import (
	"context"
	"fmt"
	"math"

	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/adapter/agentexec"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/model"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

// compactionStore is the worker's narrow conversation use-case view. The
// implementation owns the cross-aggregate transaction that replaces history
// and rebases Run watermarks; this model worker only decides summary content.
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

// Compactor is the automatic conversation-history compaction worker. A nil
// Compactor makes [Compactor.CompactIfNeeded] a silent no-op.
type Compactor struct {
	store     compactionStore
	client    modeladapter.AuxiliaryResolver
	liveState LiveStateSnapshotter // nil = no post-compaction live-state reminder
	policy    compactionPolicy
}

type compactionAction uint8

const (
	noCompaction compactionAction = iota
	trimCompaction
	summarizeCompaction
)

type compactionPlan struct {
	action         compactionAction
	required       bool
	messagesBefore int
	cutoff         int
	trimmed        []chat.Message
	older          []chat.Message
	recent         []chat.Message
	inputTokens    int
}

// NewCompactor builds a Compactor over the chat history store and a
// per-call chat-client resolver. liveState (nil to disable) snapshots a
// session's still-active process state so an LLM summary rung can remind the
// model of running shells the summary cannot reconstruct.
func NewCompactor(store compactionStore, client modeladapter.AuxiliaryResolver, liveState LiveStateSnapshotter, values CompactionPolicyValues) (*Compactor, error) {
	policy, err := newCompactionPolicy(values)
	if err != nil {
		return nil, err
	}
	return &Compactor{store: store, client: client, liveState: liveState, policy: policy}, nil
}

// tokenTrigger resolves the token-footprint compaction threshold for a run whose
// model publishes limits. An explicit MaxTokens config chooses the desired
// trigger; otherwise it is window-relative to the RUN's model when known, else
// the default model's catalog fallback, else a coarse fixed fallback. The
// hard input ceiling is the tighter of the provider's independent prompt
// maximum and the total context remaining after an explicit output reservation.
func (c *Compactor) tokenTrigger(limits modelref.TokenLimits, options chat.Options) (int, error) {
	effectiveLimits := limits
	if effectiveLimits.Unknown() {
		effectiveLimits = c.policy.fallbackLimits
	}
	reservation := modelref.OutputReservation{}
	if options.MaxOutputTokens != nil {
		var err error
		reservation, err = modelref.NewOutputReservation(*options.MaxOutputTokens)
		if err != nil {
			return 0, err
		}
	}
	inputLimit, inputLimitKnown, err := effectiveLimits.InputCeiling(reservation)
	if err != nil {
		return 0, err
	}
	contextWindow, contextWindowKnown := effectiveLimits.ContextWindow()

	trigger := defaultCompactMaxTokens
	if c.policy.maxTokensExplicit {
		trigger = c.policy.maxTokens
	} else if contextWindowKnown {
		window := tokenLimitInt(contextWindow)
		whole := window / percentageScale * windowTriggerPct
		fraction := window % percentageScale * windowTriggerPct / percentageScale
		trigger = max(1, saturatedAdd(whole, fraction))
	}
	if inputLimitKnown {
		trigger = min(trigger, tokenLimitInt(inputLimit))
	}
	return trigger, nil
}

func tokenLimitInt(value int64) int {
	if value <= 0 {
		return 0
	}
	if uint64(value) > uint64(math.MaxInt) {
		return math.MaxInt
	}
	return int(value)
}

// CompactIfNeeded inspects sessionID's history. When either trigger
// (message count or complete-request token footprint, see [modelContextBudget]) is
// breached it runs a ladder, cheapest rung first: a non-LLM trim of oversized
// tool-call arguments and old tool-result bodies (see trimForBudgetBefore);
// only if that leaves the footprint over budget is the older slice summarized by
// the LLM and the store rewritten as [summary, recent...]. A trim that suffices
// on its own rewrites history silently and reports no boundary — it drops no
// messages. The returned [agentexec.CompactionResult] carries the semantic
// summary and before/after message counts so callers can chain follow-on work
// (e.g. extraction) and surface an observable boundary event.
//
// No-op (zero result) on a nil receiver (compaction disabled) or an
// empty sessionID.
//
// Important: the summary call goes through chatclient.Client directly
// (no middleware), so it does NOT enter the chat history middleware
// — otherwise the summarisation request itself would be appended
// to the history and trigger another compaction round.
func (c *Compactor) CompactIfNeeded(
	ctx context.Context,
	sessionID string,
	limits modelref.TokenLimits,
	options chat.Options,
	preCompact func(context.Context) bool,
) (agentexec.CompactionResult, error) {
	if c == nil || sessionID == "" {
		return agentexec.CompactionResult{}, nil
	}
	if _, err := resourceid.ParseSession(sessionID); err != nil {
		return agentexec.CompactionResult{}, fmt.Errorf("compactor: %w", err)
	}
	maxTokens, err := c.tokenTrigger(limits, options)
	if err != nil {
		return agentexec.CompactionResult{}, fmt.Errorf("compactor: resolve token trigger: %w", err)
	}
	msgs, err := c.store.Read(ctx, sessionID)
	if err != nil {
		return agentexec.CompactionResult{}, fmt.Errorf("compactor: read: %w", err)
	}
	budget := newModelContextBudget(c.policy.messageTrigger, maxTokens, nil, nil, nil, chat.Options{}, 0, nil)
	plan, err := c.planCompaction(ctx, msgs, budget)
	if err != nil {
		return agentexec.CompactionResult{}, err
	}
	if plan.action == noCompaction {
		return agentexec.CompactionResult{}, nil
	}
	if preCompact != nil && !preCompact(ctx) {
		return agentexec.CompactionResult{}, nil
	}

	if plan.action == trimCompaction {
		if rewriteForCompactionErr := c.store.RewriteForCompaction(ctx, sessionID, len(msgs), 0, 0, plan.trimmed...); rewriteForCompactionErr != nil {
			return agentexec.CompactionResult{}, fmt.Errorf("compactor: replace trimmed: %w", rewriteForCompactionErr)
		}
		return agentexec.CompactionResult{}, nil
	}

	summary, err := c.summarize(ctx, plan.older)
	if err != nil {
		return agentexec.CompactionResult{}, fmt.Errorf("compactor: summarize: %w", err)
	}

	rewritten := make([]chat.Message, 0, 2+len(plan.recent))
	rewritten = append(rewritten, summary.Message())
	// Right after the summary, carry over the live execution state the summary
	// dropped (running background shells) so the model does not forget a process
	// it started before the compacted Runs. Deterministic, no model
	// call; omitted entirely when nothing is active.
	if c.liveState != nil {
		if reminder, ok := liveStateReminder(c.liveState(ctx, sessionID)); ok {
			rewritten = append(rewritten, reminder)
		}
	}
	rewritten = append(rewritten, plan.recent...)
	// Atomically swap the history for [summary, ...recent]. The store rolls back
	// a failed rewrite, so a crash cannot
	// leave the conversation cleared-but-not-rewritten (losing `recent` too).
	prefixAfter := len(rewritten) - len(plan.recent)
	result, err := agentexec.NewCompactionResult(summary.Text(), plan.messagesBefore, len(rewritten))
	if err != nil {
		return agentexec.CompactionResult{}, fmt.Errorf("compactor: build result: %w", err)
	}
	if err := c.store.RewriteForCompaction(
		ctx, sessionID, plan.messagesBefore, plan.cutoff, prefixAfter, rewritten...,
	); err != nil {
		return agentexec.CompactionResult{}, fmt.Errorf("compactor: replace: %w", err)
	}
	return result, nil
}

func (c *Compactor) planCompaction(
	ctx context.Context,
	messages []chat.Message,
	budget modelContextBudget,
) (compactionPlan, error) {
	return c.planCompactionWithProtectedTail(ctx, messages, budget, 0)
}

func (c *Compactor) planCompactionWithProtectedTail(
	ctx context.Context,
	messages []chat.Message,
	budget modelContextBudget,
	protectedTail int,
) (compactionPlan, error) {
	overBudget, tokenTriggered, inputTokens, err := budget.triggered(ctx, messages)
	if err != nil {
		return compactionPlan{}, err
	}
	if !overBudget || len(messages) == 0 {
		return compactionPlan{inputTokens: inputTokens}, nil
	}
	if protectedTail < 0 || protectedTail > len(messages) {
		return compactionPlan{required: tokenTriggered, inputTokens: inputTokens}, nil
	}
	foldableLimit := len(messages) - protectedTail
	protectedOverBudget, _, err := budget.exceeded(ctx, messages[foldableLimit:])
	if err != nil {
		return compactionPlan{}, err
	}
	if protectedOverBudget {
		return compactionPlan{required: true, inputTokens: inputTokens}, nil
	}
	if foldableLimit == 0 {
		return compactionPlan{required: tokenTriggered, inputTokens: inputTokens}, nil
	}
	cutoff := c.summaryCutoffWithProtectedTail(messages, protectedTail)
	if cutoff == 0 {
		return compactionPlan{required: tokenTriggered, inputTokens: inputTokens}, nil
	}
	trimmed, changed := trimForBudgetBefore(messages, cutoff)
	trimmedOverBudget, trimmedTokens, err := budget.exceeded(ctx, trimmed)
	if err != nil {
		return compactionPlan{}, err
	}
	if tokenTriggered && changed && !trimmedOverBudget {
		return compactionPlan{
			action: trimCompaction, required: tokenTriggered,
			messagesBefore: len(messages), trimmed: trimmed,
			inputTokens: trimmedTokens,
		}, nil
	}
	// KeepRecent is a preference, not a license to preserve a suffix that is
	// already over budget by itself. In that case the preferred prefix summary
	// cannot converge: every later pass would keep the same oversized turn.
	// Widen the deterministic rung first; only summarize the complete, finished
	// history when cheap trimming still cannot make it executable.
	recentOverBudget, _, err := budget.exceeded(ctx, trimmed[cutoff:])
	if err != nil {
		return compactionPlan{}, err
	}
	if recentOverBudget {
		cutoff = foldableLimit
		trimmed, changed = trimForBudgetBefore(messages, cutoff)
		trimmedOverBudget, trimmedTokens, err = budget.exceeded(ctx, trimmed)
		if err != nil {
			return compactionPlan{}, err
		}
		if tokenTriggered && changed && !trimmedOverBudget {
			return compactionPlan{
				action: trimCompaction, required: tokenTriggered,
				messagesBefore: len(messages), trimmed: trimmed,
				inputTokens: trimmedTokens,
			}, nil
		}
	}
	return compactionPlan{
		action:         summarizeCompaction,
		required:       tokenTriggered,
		messagesBefore: len(messages),
		cutoff:         cutoff,
		older:          trimmed[:cutoff],
		recent:         trimmed[cutoff:],
	}, nil
}

// summaryCutoffWithProtectedTail returns a complete-turn boundary near the
// configured recent window without folding the caller-owned exact suffix. The
// preferred boundary is the first User message at or after the naive cutoff.
// If the cutoff landed inside the final foldable turn, it moves back to that
// turn's opening User message.
func (c *Compactor) summaryCutoffWithProtectedTail(
	messages []chat.Message,
	protectedTail int,
) int {
	if protectedTail < 0 || protectedTail > len(messages) {
		return 0
	}
	foldable := messages[:len(messages)-protectedTail]
	desired := max(0, len(messages)-c.policy.keepRecent)
	desired = min(desired, len(foldable))
	hasOpeningUser := false
	for index := desired; index < len(foldable); index++ {
		if foldable[index].Role == chat.RoleUser {
			if index > 0 {
				return index
			}
			hasOpeningUser = true
		}
	}
	for index := min(desired-1, len(foldable)-1); index >= 0; index-- {
		if foldable[index].Role == chat.RoleUser {
			if index > 0 {
				return index
			}
			hasOpeningUser = true
			break
		}
	}
	if hasOpeningUser {
		return len(foldable)
	}
	return 0
}

package maintenance

import (
	"context"
	"fmt"
	"math"

	modeltext "github.com/Tangerg/flame/runtime/model/text"

	"github.com/Tangerg/scope/core/chat"
)

// modelContextBudget owns the compaction trigger and the exact immutable
// request material that survives every candidate rewrite. Every main-model call
// pays only the local preflight cost. Provider counting is reserved for a local
// threshold hit or provider-priced media, and compaction begins only when that
// exact count reaches the threshold. Keeping the fixed prefix, mutable
// conversation, pending tail, Tool manifest, and Options in one value prevents
// independently rounded "fixed" and "history" token ledgers.
type modelContextBudget struct {
	maxTokens    int
	instructions []chat.Message
	tail         []chat.Message
	tools        []chat.ToolDefinition
	options      chat.Options
	adjustment   int
	counter      modelContextInputTokenCounter
}

// modelContextInputTokenCounter measures a candidate mutable suffix. The
// implementation owns the imminent call's frozen prefix — instructions, Tools
// and Options — so callers pass only the messages they are replacing.
type modelContextInputTokenCounter interface {
	CountInputTokens(context.Context, []chat.Message) (int64, error)
}

type modelContextFootprint struct {
	// tokens owns the capacity decision and may be provider-measured.
	// estimatedTokens keeps the provider-neutral baseline used to calibrate the
	// next request against eventual provider usage.
	tokens           int
	estimatedTokens  int
	providerMeasured bool
}

func (m modelContextFootprint) budgetTokens(adjustment int) int {
	if m.providerMeasured {
		return m.tokens
	}
	return calibratedTokenEstimate(m.tokens, adjustment)
}

func newModelContextBudget(
	maxTokens int,
	instructions []chat.Message,
	tail []chat.Message,
	tools []chat.ToolDefinition,
	options chat.Options,
	adjustment int,
	counter modelContextInputTokenCounter,
) modelContextBudget {
	return modelContextBudget{
		maxTokens:    maxTokens,
		instructions: cloneMessages(instructions),
		tail:         cloneMessages(tail),
		tools:        cloneToolDefinitions(tools),
		options:      options.Clone(),
		adjustment:   adjustment,
		counter:      counter,
	}
}

func (m modelContextBudget) triggered(ctx context.Context, messages []chat.Message) (bool, int, error) {
	footprint, err := m.triggerFootprint(ctx, messages)
	if err != nil {
		return false, 0, err
	}
	return footprint.budgetTokens(m.adjustment) >= m.maxTokens, footprint.estimatedTokens, nil
}

func (m modelContextBudget) exceeded(ctx context.Context, messages []chat.Message) (bool, int, error) {
	footprint, err := m.replacementFootprint(ctx, messages)
	if err != nil {
		return false, 0, err
	}
	return footprint.budgetTokens(m.adjustment) >= m.maxTokens, footprint.estimatedTokens, nil
}

func (m modelContextBudget) triggerFootprint(ctx context.Context, messages []chat.Message) (modelContextFootprint, error) {
	tokens, err := m.estimatedTokens(messages)
	if err != nil {
		return modelContextFootprint{}, err
	}
	calibrated := calibratedTokenEstimate(tokens, m.adjustment)
	if m.counter == nil || (calibrated < m.maxTokens && !m.hasProviderPricedMedia(messages)) {
		return modelContextFootprint{tokens: tokens, estimatedTokens: tokens}, nil
	}
	return m.countInputTokens(ctx, messages, tokens)
}

func (m modelContextBudget) replacementFootprint(ctx context.Context, messages []chat.Message) (modelContextFootprint, error) {
	tokens, err := m.estimatedTokens(messages)
	if err != nil {
		return modelContextFootprint{}, err
	}
	if m.counter != nil {
		return m.countInputTokens(ctx, messages, tokens)
	}
	return modelContextFootprint{tokens: tokens, estimatedTokens: tokens}, nil
}

func (m modelContextBudget) countInputTokens(
	ctx context.Context,
	messages []chat.Message,
	estimatedTokens int,
) (modelContextFootprint, error) {
	mutable := make([]chat.Message, 0, len(messages)+len(m.tail))
	mutable = append(mutable, cloneMessages(messages)...)
	mutable = append(mutable, cloneMessages(m.tail)...)
	count, err := m.counter.CountInputTokens(ctx, mutable)
	if err != nil {
		return modelContextFootprint{}, fmt.Errorf("maintenance: count provider input tokens: %w", err)
	}
	return modelContextFootprint{
		tokens:           tokenLimitInt(count),
		estimatedTokens:  estimatedTokens,
		providerMeasured: true,
	}, nil
}

func (m modelContextBudget) hasProviderPricedMedia(messages []chat.Message) bool {
	for _, group := range [][]chat.Message{m.instructions, messages, m.tail} {
		for messageIndex := range group {
			for partIndex := range group[messageIndex].Parts {
				part := group[messageIndex].Parts[partIndex]
				if part.Kind == chat.PartMedia {
					return true
				}
				if part.ToolResult != nil {
					for _, content := range part.ToolResult.Output.Content {
						if content.Kind == chat.PartMedia {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func (m modelContextBudget) estimatedTokens(messages []chat.Message) (int, error) {
	effective := make([]chat.Message, 0, len(m.instructions)+len(messages)+len(m.tail))
	effective = append(effective, m.instructions...)
	effective = append(effective, messages...)
	effective = append(effective, m.tail...)
	if len(effective) == 0 {
		return 0, nil
	}
	tokens, err := modeltext.EstimateRequestTokens(&chat.Request{Messages: effective, Tools: m.tools, Options: m.options})
	if err != nil {
		return 0, fmt.Errorf("maintenance: estimate model context: %w", err)
	}
	return tokens, nil
}

func calibratedTokenEstimate(estimate, adjustment int) int {
	if adjustment > 0 && estimate > math.MaxInt-adjustment {
		return math.MaxInt
	}
	if adjustment < 0 && estimate < -adjustment {
		return 0
	}
	return estimate + adjustment
}

func cloneToolDefinitions(tools []chat.ToolDefinition) []chat.ToolDefinition {
	cloned := make([]chat.ToolDefinition, len(tools))
	for index := range tools {
		cloned[index] = tools[index].Clone()
	}
	return cloned
}

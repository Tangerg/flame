// Package accounting holds token and cost accounting value objects for model
// execution and pricing.
package accounting

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

// Totals is the cumulative token and cost fact reported for one scope. CostUSD
// is absent when pricing was not available; an absent price is intentionally
// distinct from a reported zero price.
type Totals struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	CostUSD          *float64
}

// Clone returns an ownership-isolated value.
func (t Totals) Clone() Totals {
	if t.CostUSD != nil {
		cost := *t.CostUSD
		t.CostUSD = &cost
	}
	return t
}

// Validate reports whether the cumulative counters are internally consistent.
func (t Totals) Validate() error {
	if t.InputTokens < 0 || t.OutputTokens < 0 || t.CacheReadTokens < 0 ||
		t.CacheWriteTokens < 0 || t.ReasoningTokens < 0 {
		return errors.New("accounting: token counts must not be negative")
	}
	if t.CostUSD != nil && (*t.CostUSD < 0 || math.IsNaN(*t.CostUSD) || math.IsInf(*t.CostUSD, 0)) {
		return errors.New("accounting: cost must be finite and non-negative")
	}
	return nil
}

// ValidateAdvanceFrom proves that t has not erased previously committed
// cumulative accounting.
func (t Totals) ValidateAdvanceFrom(previous Totals) error {
	if err := previous.Validate(); err != nil {
		return fmt.Errorf("previous totals: %w", err)
	}
	if err := t.Validate(); err != nil {
		return fmt.Errorf("next totals: %w", err)
	}
	if t.InputTokens < previous.InputTokens || t.OutputTokens < previous.OutputTokens ||
		t.CacheReadTokens < previous.CacheReadTokens || t.CacheWriteTokens < previous.CacheWriteTokens ||
		t.ReasoningTokens < previous.ReasoningTokens {
		return errors.New("accounting: cumulative totals regressed")
	}
	nextCost, err := CostFromOptional(t.CostUSD)
	if err != nil {
		return fmt.Errorf("next totals: %w", err)
	}
	previousCost, err := CostFromOptional(previous.CostUSD)
	if err != nil {
		return fmt.Errorf("previous totals: %w", err)
	}
	return nextCost.ValidateAdvanceFrom(previousCost)
}

// Equal reports semantic equality, preserving the distinction between absent
// and reported-zero pricing.
func (t Totals) Equal(other Totals) bool {
	if t.InputTokens != other.InputTokens || t.OutputTokens != other.OutputTokens ||
		t.CacheReadTokens != other.CacheReadTokens || t.CacheWriteTokens != other.CacheWriteTokens ||
		t.ReasoningTokens != other.ReasoningTokens {
		return false
	}
	if t.CostUSD == nil || other.CostUSD == nil {
		return t.CostUSD == nil && other.CostUSD == nil
	}
	return *t.CostUSD == *other.CostUSD
}

// Usage is cumulative accounting for a Run. Total remains authoritative when
// a provider cannot attribute usage to individual models; ByModel is the
// optional breakdown and never replaces the total.
type Usage struct {
	Total   Totals
	ByModel map[string]Totals
}

// Clone returns an ownership-isolated usage value.
func (u Usage) Clone() Usage {
	u.Total = u.Total.Clone()
	if u.ByModel != nil {
		source := u.ByModel
		u.ByModel = make(map[string]Totals, len(source))
		for model, totals := range source {
			u.ByModel[model] = totals.Clone()
		}
	}
	return u
}

// Validate reports whether u is safe to persist and compare.
func (u Usage) Validate() error {
	if err := u.Total.Validate(); err != nil {
		return fmt.Errorf("accounting: total usage: %w", err)
	}
	models := make([]string, 0, len(u.ByModel))
	for model := range u.ByModel {
		models = append(models, model)
	}
	slices.Sort(models)
	for _, model := range models {
		if _, err := modelref.NewModelIdentity(model); err != nil {
			return fmt.Errorf("accounting: model identity: %w", err)
		}
		if err := u.ByModel[model].Validate(); err != nil {
			return fmt.Errorf("accounting: model %q: %w", model, err)
		}
	}
	return nil
}

// ValidateAdvanceFrom proves that u is a cumulative continuation of
// previous. Once a provider reports usage or a per-model key, it cannot vanish.
func (u Usage) ValidateAdvanceFrom(previous Usage) error {
	if err := u.Validate(); err != nil {
		return err
	}
	if err := u.Total.ValidateAdvanceFrom(previous.Total); err != nil {
		return err
	}
	for model, before := range previous.ByModel {
		after, found := u.ByModel[model]
		if !found {
			return fmt.Errorf("accounting: cumulative usage dropped model %q", model)
		}
		if err := after.ValidateAdvanceFrom(before); err != nil {
			return fmt.Errorf("accounting: model %q: %w", model, err)
		}
	}
	return nil
}

// Equal reports whether two snapshots contain the same cumulative fact. Nil
// and empty per-model maps are the same set.
func (u Usage) Equal(other Usage) bool {
	if !u.Total.Equal(other.Total) || len(u.ByModel) != len(other.ByModel) {
		return false
	}
	for model, totals := range u.ByModel {
		if otherTotals, found := other.ByModel[model]; !found || !totals.Equal(otherTotals) {
			return false
		}
	}
	return true
}

// TokenUsage is a token roll-up. ReasoningTokens is the chain-of-thought
// subset of CompletionTokens, so total counts only prompt + completion.
type TokenUsage struct {
	PromptTokens     int64
	CompletionTokens int64
	ReasoningTokens  int64
	CacheReadTokens  int64
	CacheWriteTokens int64
}

// Cost is one explicit pricing fact. Its zero value means pricing was
// unavailable; a priced zero is constructed explicitly and remains distinct.
// This prevents an unknown catalog entry from silently becoming a free model.
type Cost struct {
	usd       float64
	available bool
}

// NewCost constructs one available finite non-negative USD amount.
func NewCost(usd float64) (Cost, error) {
	if usd < 0 || math.IsNaN(usd) || math.IsInf(usd, 0) {
		return Cost{}, errors.New("accounting: cost must be finite and non-negative")
	}
	return Cost{usd: usd, available: true}, nil
}

// CostFromOptional restores the exact absent-versus-priced-zero distinction
// used by durable Run metrics and executor checkpoints.
func CostFromOptional(usd *float64) (Cost, error) {
	if usd == nil {
		return Cost{}, nil
	}
	return NewCost(*usd)
}

// USD returns the price only when the model call was priced.
func (c Cost) USD() (float64, bool) { return c.usd, c.available }

// OptionalUSD projects this value to the pointer vocabulary used by durable
// accounting records. The returned pointer owns an independent value.
func (c Cost) OptionalUSD() *float64 {
	if !c.available {
		return nil
	}
	value := c.usd
	return &value
}

// Add combines independently priced calls. One unavailable component makes
// the aggregate unavailable because a known partial sum is not a total cost.
func (c Cost) Add(other Cost) (Cost, error) {
	if err := c.Validate(); err != nil {
		return Cost{}, err
	}
	if err := other.Validate(); err != nil {
		return Cost{}, err
	}
	if !c.available || !other.available {
		return Cost{}, nil
	}
	if other.usd > math.MaxFloat64-c.usd {
		return Cost{}, errors.New("accounting: cost aggregate overflows")
	}
	return Cost{usd: c.usd + other.usd, available: true}, nil
}

// Subtract removes an available component from an available aggregate. An
// unavailable total cannot prove any remainder and therefore stays unavailable.
func (c Cost) Subtract(other Cost) (Cost, error) {
	if err := c.Validate(); err != nil {
		return Cost{}, err
	}
	if err := other.Validate(); err != nil {
		return Cost{}, err
	}
	if !c.available {
		return Cost{}, nil
	}
	if !other.available {
		return Cost{}, errors.New("accounting: unavailable cost cannot be subtracted from a priced total")
	}
	if c.usd+1e-9 < other.usd {
		return Cost{}, errors.New("accounting: cost subtraction exceeds total")
	}
	remaining := c.usd - other.usd
	if math.Abs(remaining) < 1e-9 {
		remaining = 0
	}
	return Cost{usd: remaining, available: true}, nil
}

// Validate reports corrupt private state. The unavailable zero value is valid.
func (c Cost) Validate() error {
	if !c.available {
		if c.usd != 0 {
			return errors.New("accounting: unavailable cost carries a value")
		}
		return nil
	}
	if c.usd < 0 || math.IsNaN(c.usd) || math.IsInf(c.usd, 0) {
		return errors.New("accounting: cost must be finite and non-negative")
	}
	return nil
}

// Equal preserves availability as part of the accounting fact.
func (c Cost) Equal(other Cost) bool {
	return c.available == other.available && (!c.available || c.usd == other.usd)
}

// ValidateAdvanceFrom proves that c can be the cumulative result after
// previous. Pricing may become unavailable when a later component is
// unpriced, but an unavailable aggregate can never become exact again.
func (c Cost) ValidateAdvanceFrom(previous Cost) error {
	if err := previous.Validate(); err != nil {
		return fmt.Errorf("previous cost: %w", err)
	}
	if err := c.Validate(); err != nil {
		return fmt.Errorf("next cost: %w", err)
	}
	if (!previous.available && c.available) ||
		(previous.available && c.available && c.usd < previous.usd) {
		return errors.New("accounting: cumulative cost regressed")
	}
	return nil
}

// Validate reports whether the token relationships can represent one model's
// cumulative usage.
func (t TokenUsage) Validate() error {
	if t.PromptTokens < 0 || t.CompletionTokens < 0 || t.ReasoningTokens < 0 ||
		t.CacheReadTokens < 0 || t.CacheWriteTokens < 0 {
		return errors.New("accounting: token counts must not be negative")
	}
	if t.ReasoningTokens > t.CompletionTokens {
		return errors.New("accounting: reasoning tokens exceed completion tokens")
	}
	if t.CacheReadTokens > t.PromptTokens || t.CacheWriteTokens > t.PromptTokens {
		return errors.New("accounting: cache tokens exceed prompt tokens")
	}
	return nil
}

// Total returns prompt + completion: the figure a token budget caps.
func (t TokenUsage) Total() (int64, error) {
	if err := t.Validate(); err != nil {
		return 0, err
	}
	total, ok := checkedAddInt64(t.PromptTokens, t.CompletionTokens)
	if !ok {
		return 0, errors.New("accounting: total token usage overflows")
	}
	return total, nil
}

// Add returns the checked sum of two independently valid token roll-ups.
func (t TokenUsage) Add(other TokenUsage) (TokenUsage, error) {
	if err := t.Validate(); err != nil {
		return TokenUsage{}, fmt.Errorf("left token usage: %w", err)
	}
	if err := other.Validate(); err != nil {
		return TokenUsage{}, fmt.Errorf("right token usage: %w", err)
	}
	next := TokenUsage{}
	fields := []struct {
		name        string
		left, right int64
		target      *int64
	}{
		{name: "prompt", left: t.PromptTokens, right: other.PromptTokens, target: &next.PromptTokens},
		{name: "completion", left: t.CompletionTokens, right: other.CompletionTokens, target: &next.CompletionTokens},
		{name: "reasoning", left: t.ReasoningTokens, right: other.ReasoningTokens, target: &next.ReasoningTokens},
		{name: "cache-read", left: t.CacheReadTokens, right: other.CacheReadTokens, target: &next.CacheReadTokens},
		{name: "cache-write", left: t.CacheWriteTokens, right: other.CacheWriteTokens, target: &next.CacheWriteTokens},
	}
	for _, field := range fields {
		value, ok := checkedAddInt64(field.left, field.right)
		if !ok {
			return TokenUsage{}, fmt.Errorf("accounting: %s token usage overflows", field.name)
		}
		*field.target = value
	}
	return next, nil
}

// Subtract removes one independently valid token roll-up from a cumulative
// total. The remainder must still satisfy the token subset relationships.
func (t TokenUsage) Subtract(other TokenUsage) (TokenUsage, error) {
	if err := t.Validate(); err != nil {
		return TokenUsage{}, fmt.Errorf("total token usage: %w", err)
	}
	if err := other.Validate(); err != nil {
		return TokenUsage{}, fmt.Errorf("subtracted token usage: %w", err)
	}
	next := TokenUsage{}
	fields := []struct {
		name        string
		total, used int64
		target      *int64
	}{
		{name: "prompt", total: t.PromptTokens, used: other.PromptTokens, target: &next.PromptTokens},
		{name: "completion", total: t.CompletionTokens, used: other.CompletionTokens, target: &next.CompletionTokens},
		{name: "reasoning", total: t.ReasoningTokens, used: other.ReasoningTokens, target: &next.ReasoningTokens},
		{name: "cache-read", total: t.CacheReadTokens, used: other.CacheReadTokens, target: &next.CacheReadTokens},
		{name: "cache-write", total: t.CacheWriteTokens, used: other.CacheWriteTokens, target: &next.CacheWriteTokens},
	}
	for _, field := range fields {
		if field.used > field.total {
			return TokenUsage{}, fmt.Errorf("accounting: %s token subtraction exceeds total", field.name)
		}
		*field.target = field.total - field.used
	}
	if err := next.Validate(); err != nil {
		return TokenUsage{}, fmt.Errorf("accounting: token remainder: %w", err)
	}
	return next, nil
}

// ModelUsage is one model's slice of an execution's tokens and cost.
type ModelUsage struct {
	Model string
	TokenUsage
	Cost  Cost
	Calls int
}

// Add returns the checked cumulative usage for the same served model.
func (m ModelUsage) Add(other ModelUsage) (ModelUsage, error) {
	if err := m.Validate(); err != nil {
		return ModelUsage{}, fmt.Errorf("left model usage: %w", err)
	}
	if err := other.Validate(); err != nil {
		return ModelUsage{}, fmt.Errorf("right model usage: %w", err)
	}
	if m.Model != other.Model {
		return ModelUsage{}, fmt.Errorf("accounting: cannot combine models %q and %q", m.Model, other.Model)
	}
	tokens, err := m.TokenUsage.Add(other.TokenUsage)
	if err != nil {
		return ModelUsage{}, err
	}
	cost, err := m.Cost.Add(other.Cost)
	if err != nil {
		return ModelUsage{}, err
	}
	if other.Calls > math.MaxInt-m.Calls {
		return ModelUsage{}, errors.New("accounting: model call count overflows")
	}
	return ModelUsage{Model: m.Model, TokenUsage: tokens, Cost: cost, Calls: m.Calls + other.Calls}, nil
}

// Subtract removes one model slice from a cumulative slice of the same model.
// The boolean reports whether a non-empty, valid ModelUsage remains; an exact
// subtraction returns the zero value and false.
func (m ModelUsage) Subtract(other ModelUsage) (ModelUsage, bool, error) {
	if err := validateModelUsageSubtraction(m, other); err != nil {
		return ModelUsage{}, false, err
	}
	tokens, err := m.TokenUsage.Subtract(other.TokenUsage)
	if err != nil {
		return ModelUsage{}, false, err
	}
	cost, err := m.Cost.Subtract(other.Cost)
	if err != nil {
		return ModelUsage{}, false, err
	}
	if other.Calls > m.Calls {
		return ModelUsage{}, false, errors.New("accounting: model call subtraction exceeds total")
	}
	return newModelUsageRemainder(m.Model, tokens, cost, m.Calls-other.Calls)
}

func validateModelUsageSubtraction(total, used ModelUsage) error {
	if err := total.Validate(); err != nil {
		return fmt.Errorf("total model usage: %w", err)
	}
	if err := used.Validate(); err != nil {
		return fmt.Errorf("subtracted model usage: %w", err)
	}
	if total.Model != used.Model {
		return fmt.Errorf("accounting: cannot subtract models %q and %q", total.Model, used.Model)
	}
	return nil
}

func newModelUsageRemainder(
	model string,
	tokens TokenUsage,
	cost Cost,
	calls int,
) (ModelUsage, bool, error) {
	if calls == 0 {
		usd, priced := cost.USD()
		if tokens != (TokenUsage{}) || priced && usd != 0 {
			return ModelUsage{}, false, errors.New("accounting: model usage remains without calls")
		}
		return ModelUsage{}, false, nil
	}
	remainder := ModelUsage{Model: model, TokenUsage: tokens, Cost: cost, Calls: calls}
	if err := remainder.Validate(); err != nil {
		return ModelUsage{}, false, fmt.Errorf("accounting: model usage remainder: %w", err)
	}
	return remainder, true, nil
}

// Snapshot is the durable usage projection for one complete execution tree.
// Models are unique and sorted by model ID so concurrent
// execution cannot make checkpoint bytes or output ordering nondeterministic.
type Snapshot struct {
	Models []ModelUsage
}

// Total returns the checked aggregate of every model in the snapshot. The
// result intentionally has an empty Model because it represents the whole
// execution subtree rather than another served model.
func (s Snapshot) Total() (ModelUsage, error) {
	if err := s.Validate(); err != nil {
		return ModelUsage{}, err
	}
	total := ModelUsage{}
	if len(s.Models) > 0 {
		// The additive identity is priced only when there are actual model facts
		// to aggregate. An empty snapshot has no pricing fact at all.
		total.Cost = Cost{available: true}
	}
	for index, model := range s.Models {
		tokens, err := total.TokenUsage.Add(model.TokenUsage)
		if err != nil {
			return ModelUsage{}, fmt.Errorf("accounting snapshot: models[%d] token aggregate: %w", index, err)
		}
		total.TokenUsage = tokens
		nextCost, err := total.Cost.Add(model.Cost)
		if err != nil {
			return ModelUsage{}, fmt.Errorf("accounting snapshot: models[%d] cost aggregate: %w", index, err)
		}
		total.Cost = nextCost
		if model.Calls > math.MaxInt-total.Calls {
			return ModelUsage{}, fmt.Errorf("accounting snapshot: models[%d] call aggregate overflows", index)
		}
		total.Calls += model.Calls
	}
	return total, nil
}

func checkedAddInt64(left, right int64) (int64, bool) {
	if right < 0 || left > math.MaxInt64-right {
		return 0, false
	}
	return left + right, true
}

// Validate checks that a persisted usage projection is canonical and safe to
// aggregate.
func (s Snapshot) Validate() error {
	var previous string
	for index, model := range s.Models {
		if _, err := modelref.NewModelIdentity(model.Model); err != nil {
			return fmt.Errorf("accounting snapshot: models[%d]: %w", index, err)
		}
		if index > 0 && model.Model <= previous {
			return errors.New("accounting snapshot: models must be unique and sorted by model ID")
		}
		previous = model.Model
		if err := model.Validate(); err != nil {
			return fmt.Errorf("accounting snapshot: models[%d]: %w", index, err)
		}
	}
	return nil
}

// ValidateAdvanceFrom proves that s is a cumulative continuation of previous.
// A checkpoint may add models or increase counters, but it cannot erase a model
// or rewind usage already committed at an earlier barrier.
func (s Snapshot) ValidateAdvanceFrom(previous Snapshot) error {
	if err := previous.Validate(); err != nil {
		return fmt.Errorf("previous usage: %w", err)
	}
	if err := s.Validate(); err != nil {
		return fmt.Errorf("next usage: %w", err)
	}
	nextByModel := make(map[string]ModelUsage, len(s.Models))
	for _, model := range s.Models {
		nextByModel[model.Model] = model
	}
	for _, before := range previous.Models {
		after, found := nextByModel[before.Model]
		if !found {
			return fmt.Errorf("accounting snapshot: model %q disappeared", before.Model)
		}
		if after.PromptTokens < before.PromptTokens ||
			after.CompletionTokens < before.CompletionTokens ||
			after.ReasoningTokens < before.ReasoningTokens ||
			after.CacheReadTokens < before.CacheReadTokens ||
			after.CacheWriteTokens < before.CacheWriteTokens ||
			after.Calls < before.Calls {
			return fmt.Errorf("accounting snapshot: model %q usage regressed", before.Model)
		}
		if err := after.Cost.ValidateAdvanceFrom(before.Cost); err != nil {
			return fmt.Errorf("accounting snapshot: model %q: %w", before.Model, err)
		}
	}
	return nil
}

// Validate checks one model's token and cost counters.
func (m ModelUsage) Validate() error {
	if _, err := modelref.NewModelIdentity(m.Model); err != nil {
		return fmt.Errorf("model usage: %w", err)
	}
	if err := m.TokenUsage.Validate(); err != nil {
		return fmt.Errorf("model usage: %w", err)
	}
	if err := m.Cost.Validate(); err != nil {
		return fmt.Errorf("model usage: %w", err)
	}
	if m.Calls <= 0 {
		return errors.New("model usage calls must be positive")
	}
	return nil
}

// Pricing computes the USD cost of one LLM round from the provider, served
// model, and full token usage.
type Pricing func(provider, model string, usage *chat.Usage) Cost

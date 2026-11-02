// Package accounting holds token and cost accounting value objects for model
// execution and pricing.
package accounting

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/scope/core/chat"
)

// Totals is the cumulative token and cost fact reported for one scope.
type Totals struct {
	Tokens
	Cost Cost
}

// Validate reports whether the cumulative counters are internally consistent.
func (t Totals) Validate() error {
	return t.Tokens.Validate()
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
	return t.Cost.ValidateAdvanceFrom(previous.Cost)
}

// Equal reports semantic equality, preserving the distinction between absent
// and reported-zero pricing.
func (t Totals) Equal(other Totals) bool {
	if t.InputTokens != other.InputTokens || t.OutputTokens != other.OutputTokens ||
		t.CacheReadTokens != other.CacheReadTokens || t.CacheWriteTokens != other.CacheWriteTokens ||
		t.ReasoningTokens != other.ReasoningTokens {
		return false
	}
	return t.Cost.Equal(other.Cost)
}

func (t Totals) Add(other Totals) (Totals, error) {
	if err := t.Validate(); err != nil {
		return Totals{}, err
	}
	if err := other.Validate(); err != nil {
		return Totals{}, err
	}
	tokens, err := t.Tokens.Add(other.Tokens)
	if err != nil {
		return Totals{}, err
	}
	cost, err := t.Cost.Add(other.Cost)
	if err != nil {
		return Totals{}, err
	}
	return Totals{Tokens: tokens, Cost: cost}, nil
}

// An unattributed report owns only its total. For attributed usage the total
// is a checked projection of all model subtotals, including unknown costs.
type Usage struct {
	Total   Totals
	ByModel map[string]Totals
}

// Clone returns an ownership-isolated usage value.
func (u Usage) Clone() Usage {
	u.ByModel = maps.Clone(u.ByModel)
	return u
}

// Validate reports whether u is safe to persist and compare.
func (u Usage) Validate() error {
	if err := u.Total.Validate(); err != nil {
		return fmt.Errorf("accounting: total usage: %w", err)
	}
	if u.ByModel == nil {
		return nil
	}
	total, err := modelTotal(u.ByModel)
	if err != nil {
		return err
	}
	if total != u.Total {
		return errors.New("accounting: attributed usage differs from model subtotals")
	}
	return nil
}

func modelTotal(parts map[string]Totals) (Totals, error) {
	if len(parts) == 0 {
		return Totals{}, errors.New("accounting: an attributed usage names no model")
	}
	total := Totals{Cost: Cost{available: true}}
	for _, model := range slices.Sorted(maps.Keys(parts)) {
		if _, err := modelchoice.NewModelIdentity(model); err != nil {
			return Totals{}, fmt.Errorf("accounting: model identity: %w", err)
		}
		next, err := total.Add(parts[model])
		if err != nil {
			return Totals{}, fmt.Errorf("accounting: model %q: %w", model, err)
		}
		total = next
	}
	return total, nil
}

func AttributedUsage(parts map[string]Totals) (Usage, error) {
	owned := maps.Clone(parts)
	total, err := modelTotal(owned)
	if err != nil {
		return Usage{}, err
	}
	return Usage{Total: total, ByModel: owned}, nil
}

func (u Usage) Accrue(model string, delta Totals) (Usage, error) {
	if u.ByModel == nil {
		total, err := u.Total.Add(delta)
		return Usage{Total: total}, err
	}
	parts := maps.Clone(u.ByModel)
	if previous, ok := parts[model]; ok {
		var err error
		delta, err = previous.Add(delta)
		if err != nil {
			return Usage{}, err
		}
	}
	parts[model] = delta
	total, err := modelTotal(parts)
	return Usage{Total: total, ByModel: parts}, err
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

// Equal reports whether two snapshots contain the same cumulative fact.
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

// Tokens is Runtime's cumulative token counter. It keeps [chat.Usage]'s
// vocabulary so one spelling survives from the provider report through storage
// to the protocol: ReasoningTokens is the subset of OutputTokens, and the two
// cache counters are subsets of InputTokens, so a total counts only input plus
// output.
type Tokens struct {
	InputTokens      int64
	OutputTokens     int64
	ReasoningTokens  int64
	CacheReadTokens  int64
	CacheWriteTokens int64
}

// Validate reports whether the token relationships can represent one model's
// cumulative usage.
func (t Tokens) Validate() error {
	if t.InputTokens < 0 || t.OutputTokens < 0 || t.ReasoningTokens < 0 ||
		t.CacheReadTokens < 0 || t.CacheWriteTokens < 0 {
		return errors.New("accounting: token counts must not be negative")
	}
	if t.ReasoningTokens > t.OutputTokens {
		return errors.New("accounting: reasoning tokens exceed output tokens")
	}
	if t.CacheReadTokens > t.InputTokens || t.CacheWriteTokens > t.InputTokens {
		return errors.New("accounting: cache tokens exceed input tokens")
	}
	return nil
}

// Total returns input plus output tokens.
func (t Tokens) Total() (int64, error) {
	if err := t.Validate(); err != nil {
		return 0, err
	}
	total, ok := checkedAddInt64(t.InputTokens, t.OutputTokens)
	if !ok {
		return 0, errors.New("accounting: total token usage overflows")
	}
	return total, nil
}

// Add returns the checked sum of two independently valid counters.
func (t Tokens) Add(other Tokens) (Tokens, error) {
	if err := t.Validate(); err != nil {
		return Tokens{}, fmt.Errorf("left token usage: %w", err)
	}
	if err := other.Validate(); err != nil {
		return Tokens{}, fmt.Errorf("right token usage: %w", err)
	}
	next := Tokens{}
	fields := []struct {
		name        string
		left, right int64
		target      *int64
	}{
		{name: "input", left: t.InputTokens, right: other.InputTokens, target: &next.InputTokens},
		{name: "output", left: t.OutputTokens, right: other.OutputTokens, target: &next.OutputTokens},
		{name: "reasoning", left: t.ReasoningTokens, right: other.ReasoningTokens, target: &next.ReasoningTokens},
		{name: "cache-read", left: t.CacheReadTokens, right: other.CacheReadTokens, target: &next.CacheReadTokens},
		{name: "cache-write", left: t.CacheWriteTokens, right: other.CacheWriteTokens, target: &next.CacheWriteTokens},
	}
	for _, field := range fields {
		value, ok := checkedAddInt64(field.left, field.right)
		if !ok {
			return Tokens{}, fmt.Errorf("accounting: %s token usage overflows", field.name)
		}
		*field.target = value
	}
	return next, nil
}

// Cost is one explicit pricing fact, held as an exact count of nano-dollars so
// aggregates sum without floating-point drift; a single call can cost less than
// a micro-dollar. Its zero value means pricing was unavailable; a priced zero is
// constructed explicitly and remains distinct. This prevents an unknown catalog
// entry from silently becoming a free model.
type Cost struct {
	nanoUSD   int64
	available bool
}

const nanoPerUSD = 1e9

// NewCost prices usd, the dollar amount a catalog or provider computed, to the
// nearest nano-dollar.
func NewCost(usd float64) (Cost, error) {
	if usd < 0 || math.IsNaN(usd) || math.IsInf(usd, 0) {
		return Cost{}, errors.New("accounting: cost must be finite and non-negative")
	}
	nano := math.Round(usd * nanoPerUSD)
	if nano >= math.MaxInt64 {
		return Cost{}, errors.New("accounting: cost overflows")
	}
	return Cost{nanoUSD: int64(nano), available: true}, nil
}

// CostFromNanoUSD restores an exact recorded amount.
func CostFromNanoUSD(nanoUSD int64) (Cost, error) {
	if nanoUSD < 0 {
		return Cost{}, errors.New("accounting: cost must be non-negative")
	}
	return Cost{nanoUSD: nanoUSD, available: true}, nil
}

// CostFromOptionalNanoUSD restores the exact absent-versus-priced-zero
// distinction durable records keep.
func CostFromOptionalNanoUSD(nanoUSD *int64) (Cost, error) {
	if nanoUSD == nil {
		return Cost{}, nil
	}
	return CostFromNanoUSD(*nanoUSD)
}

// CostFromOptional admits a dollar amount from the wire, where absence means
// unpriced.
func CostFromOptional(usd *float64) (Cost, error) {
	if usd == nil {
		return Cost{}, nil
	}
	return NewCost(*usd)
}

// NanoUSD returns the exact amount only when the model call was priced.
func (c Cost) NanoUSD() (int64, bool) { return c.nanoUSD, c.available }

// OptionalNanoUSD projects this value to the pointer vocabulary durable records
// use. The returned pointer owns an independent value.
func (c Cost) OptionalNanoUSD() *int64 {
	if !c.available {
		return nil
	}
	value := c.nanoUSD
	return &value
}

// USD presents the amount in dollars for display and the wire.
func (c Cost) USD() (float64, bool) { return float64(c.nanoUSD) / nanoPerUSD, c.available }

// OptionalUSD presents the amount in dollars, absent when unpriced.
func (c Cost) OptionalUSD() *float64 {
	if !c.available {
		return nil
	}
	value, _ := c.USD()
	return &value
}

// Add combines independently priced calls. One unavailable component makes
// the aggregate unavailable because a known partial sum is not a total cost.
func (c Cost) Add(other Cost) (Cost, error) {
	if !c.available || !other.available {
		return Cost{}, nil
	}
	if other.nanoUSD > math.MaxInt64-c.nanoUSD {
		return Cost{}, errors.New("accounting: cost aggregate overflows")
	}
	return Cost{nanoUSD: c.nanoUSD + other.nanoUSD, available: true}, nil
}

// Equal preserves availability as part of the accounting fact.
func (c Cost) Equal(other Cost) bool {
	return c.available == other.available && c.nanoUSD == other.nanoUSD
}

// ValidateAdvanceFrom proves that c can be the cumulative result after
// previous. Pricing may become unavailable when a later component is
// unpriced, but an unavailable aggregate can never become exact again.
func (c Cost) ValidateAdvanceFrom(previous Cost) error {
	if (!previous.available && c.available) ||
		(previous.available && c.available && c.nanoUSD < previous.nanoUSD) {
		return errors.New("accounting: cumulative cost regressed")
	}
	return nil
}

// ModelUsage is one model's slice of an execution's tokens and cost. Usage is
// the provider-reported fact itself, so an unsupported breakdown stays absent
// instead of becoming a reported zero.
type ModelUsage struct {
	Model string
	Tokens
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
	tokens, err := m.Tokens.Add(other.Tokens)
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
	return ModelUsage{Model: m.Model, Tokens: tokens, Cost: cost, Calls: m.Calls + other.Calls}, nil
}

// Snapshot is a per-model usage set. Models are unique and sorted by model ID
// so concurrent execution cannot make output ordering nondeterministic.
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
		tokens, err := total.Tokens.Add(model.Tokens)
		if err != nil {
			return ModelUsage{}, fmt.Errorf("accounting snapshot: models[%d] token aggregate: %w", index, err)
		}
		total.Tokens = tokens
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
		if _, err := modelchoice.NewModelIdentity(model.Model); err != nil {
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

// Validate checks one model's token and cost counters.
func (m ModelUsage) Validate() error {
	if _, err := modelchoice.NewModelIdentity(m.Model); err != nil {
		return fmt.Errorf("model usage: %w", err)
	}
	if err := m.Tokens.Validate(); err != nil {
		return fmt.Errorf("model usage: %w", err)
	}
	if m.Calls <= 0 {
		return errors.New("model usage calls must be positive")
	}
	return nil
}

// Pricing computes the USD cost of one LLM round from the provider, served
// model, and full token usage.
type Pricing func(provider, model string, usage *chat.Usage) (Cost, error)

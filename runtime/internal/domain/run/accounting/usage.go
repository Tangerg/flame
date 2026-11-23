package accounting

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/scope/core/chat"
)

type Totals struct {
	Tokens
	Cost Cost
}

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

type Usage struct {
	total   Totals
	byModel map[string]Totals
}

func NewUsage(total Totals, byModel map[string]Totals) (Usage, error) {
	if byModel == nil {
		if err := total.Validate(); err != nil {
			return Usage{}, fmt.Errorf("accounting: total usage: %w", err)
		}
		return Usage{total: total}, nil
	}
	usage, err := AttributedUsage(byModel)
	if err != nil {
		return Usage{}, err
	}
	if !usage.total.Equal(total) {
		return Usage{}, errors.New("accounting: attributed usage differs from model subtotals")
	}
	return usage, nil
}

func (u Usage) Total() Totals { return u.total }

func (u Usage) ByModel() map[string]Totals { return maps.Clone(u.byModel) }

func modelTotal(parts map[string]Totals) (Totals, error) {
	if len(parts) == 0 {
		return Totals{}, errors.New("accounting: an attributed usage names no model")
	}
	total := Totals{Cost: Cost{available: true}}
	for _, model := range slices.Sorted(maps.Keys(parts)) {
		if _, err := flamemodel.NewModelIdentity(model); err != nil {
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
	return newAttributedUsage(maps.Clone(parts))
}

func newAttributedUsage(owned map[string]Totals) (Usage, error) {
	total, err := modelTotal(owned)
	if err != nil {
		return Usage{}, err
	}
	return Usage{total: total, byModel: owned}, nil
}

func (u Usage) Accrue(model string, delta Totals) (Usage, error) {
	if u.byModel == nil {
		total, err := u.total.Add(delta)
		if err != nil {
			return Usage{}, err
		}
		return Usage{total: total}, nil
	}
	parts := maps.Clone(u.byModel)
	if previous, ok := parts[model]; ok {
		var err error
		delta, err = previous.Add(delta)
		if err != nil {
			return Usage{}, err
		}
	}
	parts[model] = delta
	return newAttributedUsage(parts)
}

func (u Usage) ValidateAdvanceFrom(previous Usage) error {
	if err := u.total.ValidateAdvanceFrom(previous.total); err != nil {
		return err
	}
	for model, before := range previous.byModel {
		after, found := u.byModel[model]
		if !found {
			return fmt.Errorf("accounting: cumulative usage dropped model %q", model)
		}
		if err := after.ValidateAdvanceFrom(before); err != nil {
			return fmt.Errorf("accounting: model %q: %w", model, err)
		}
	}
	return nil
}

func (u Usage) Equal(other Usage) bool {
	if !u.total.Equal(other.total) || len(u.byModel) != len(other.byModel) {
		return false
	}
	for model, totals := range u.byModel {
		if otherTotals, found := other.byModel[model]; !found || !totals.Equal(otherTotals) {
			return false
		}
	}
	return true
}

type Tokens struct {
	InputTokens      int64
	OutputTokens     int64
	ReasoningTokens  int64
	CacheReadTokens  int64
	CacheWriteTokens int64
}

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

type Cost struct {
	nanoUSD   int64
	available bool
}

const nanoPerUSD = 1e9

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

func CostFromNanoUSD(nanoUSD int64) (Cost, error) {
	if nanoUSD < 0 {
		return Cost{}, errors.New("accounting: cost must be non-negative")
	}
	return Cost{nanoUSD: nanoUSD, available: true}, nil
}

func CostFromOptionalNanoUSD(nanoUSD *int64) (Cost, error) {
	if nanoUSD == nil {
		return Cost{}, nil
	}
	return CostFromNanoUSD(*nanoUSD)
}

func CostFromOptional(usd *float64) (Cost, error) {
	if usd == nil {
		return Cost{}, nil
	}
	return NewCost(*usd)
}

func (c Cost) NanoUSD() (int64, bool) { return c.nanoUSD, c.available }

func (c Cost) OptionalNanoUSD() *int64 {
	if !c.available {
		return nil
	}
	value := c.nanoUSD
	return &value
}

func (c Cost) USD() (float64, bool) { return float64(c.nanoUSD) / nanoPerUSD, c.available }

func (c Cost) OptionalUSD() *float64 {
	if !c.available {
		return nil
	}
	value, _ := c.USD()
	return &value
}

// One unpriced component makes the aggregate unavailable because a partial sum is
// not a total cost.
func (c Cost) Add(other Cost) (Cost, error) {
	if !c.available || !other.available {
		return Cost{}, nil
	}
	if other.nanoUSD > math.MaxInt64-c.nanoUSD {
		return Cost{}, errors.New("accounting: cost aggregate overflows")
	}
	return Cost{nanoUSD: c.nanoUSD + other.nanoUSD, available: true}, nil
}

func (c Cost) Equal(other Cost) bool {
	return c.available == other.available && c.nanoUSD == other.nanoUSD
}

func (c Cost) ValidateAdvanceFrom(previous Cost) error {
	if (!previous.available && c.available) ||
		(previous.available && c.available && c.nanoUSD < previous.nanoUSD) {
		return errors.New("accounting: cumulative cost regressed")
	}
	return nil
}

func checkedAddInt64(left, right int64) (int64, bool) {
	if right < 0 || left > math.MaxInt64-right {
		return 0, false
	}
	return left + right, true
}

type Pricing func(provider, model string, usage *chat.Usage) (Cost, error)

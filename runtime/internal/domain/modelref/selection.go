// Package modelref defines an explicit provider/model/reasoning selection and
// the immutable token-limit envelope attached to that exact identity.
// Executions and specialized runtime roles share the same invariant: a
// selection is either unset (use the surrounding default) or names provider
// and model together; reasoning effort is optional and belongs to that exact
// model. Provider inference is deliberately unsupported.
package modelref

import (
	"errors"

	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// ErrIncomplete reports a provider/model pair where only one value was set.
var ErrIncomplete = runtimeidentity.ErrIncompleteModelSelection

// ErrReasoningEffortWithoutModel reports a reasoning choice that has no exact
// provider/model identity to interpret its model-owned vocabulary.
var ErrReasoningEffortWithoutModel = runtimeidentity.ErrReasoningEffortWithoutModel

// ErrUnsupported reports a syntactically valid exact selection the configured
// Runtime cannot admit.
var ErrUnsupported = errors.New("model selection: unsupported")

var errExactSelectionRequired = errors.New("model selection is required")

// IsInvalid reports whether err is a stable model-selection syntax failure.
func IsInvalid(err error) bool {
	return errors.Is(err, ErrIncomplete) ||
		errors.Is(err, ErrProviderIdentity) ||
		errors.Is(err, ErrModelIdentity) ||
		errors.Is(err, ErrReasoningEffortWithoutModel) ||
		errors.Is(err, ErrReasoningEffortIdentity)
}

// Selection is an immutable model choice. Its zero value asks the owning use
// case to use its configured default.
type Selection struct {
	provider        ProviderIdentity
	model           ModelIdentity
	reasoningEffort ReasoningEffortIdentity
}

// Patch describes an atomic edit of one model selection. Provider and model
// form one identity and must therefore be supplied together. Changing that
// identity without an explicit effort resets effort to the target model's
// provider default; changing effort alone retains the current identity.
type Patch struct {
	Provider        *string
	Model           *string
	ReasoningEffort *string
}

// Empty reports whether p requests no selection edit.
func (p Patch) Empty() bool {
	return p.Provider == nil && p.Model == nil && p.ReasoningEffort == nil
}

// Apply returns the exact selection produced by p.
func (p Patch) Apply(current Selection) (Selection, error) {
	if (p.Provider == nil) != (p.Model == nil) {
		return Selection{}, ErrIncomplete
	}
	provider, model, effort := current.Provider(), current.Model(), current.ReasoningEffort()
	if p.Provider != nil {
		provider, model = *p.Provider, *p.Model
		effort = ""
	}
	if p.ReasoningEffort != nil {
		effort = *p.ReasoningEffort
	}
	return NewWithReasoningEffort(provider, model, effort)
}

// New constructs a selection from its provider and model identities.
func New(provider, model string) (Selection, error) {
	return NewWithReasoningEffort(provider, model, "")
}

// NewWithReasoningEffort constructs one exact execution selection. An empty
// effort leaves intensity to the selected model's provider default; a non-empty
// value is interpreted only against that exact model's advertised vocabulary.
func NewWithReasoningEffort(provider, model, reasoningEffort string) (Selection, error) {
	if err := runtimeidentity.ValidateModelSelection(provider, model, reasoningEffort); err != nil {
		return Selection{}, err
	}
	return Selection{
		provider:        ProviderIdentity{value: provider},
		model:           ModelIdentity{value: model},
		reasoningEffort: ReasoningEffortIdentity{value: reasoningEffort},
	}, nil
}

// Validate documents the zero-or-complete invariant at aggregate boundaries.
// Selection is immutable, so values constructed by New already satisfy it.
func (s Selection) Validate() error {
	_, err := NewWithReasoningEffort(s.Provider(), s.Model(), s.ReasoningEffort())
	return err
}

// ValidateExact reports whether s names one provider/model pair instead of
// asking an owning use case to supply its default.
func (s Selection) ValidateExact() error {
	if err := s.Validate(); err != nil {
		return err
	}
	if !s.Configured() {
		return errExactSelectionRequired
	}
	return nil
}

// Configured reports whether s pins one provider and model.
func (s Selection) Configured() bool { return s.model.String() != "" }

// Provider returns the explicitly selected provider, or "" for the runtime default.
func (s Selection) Provider() string { return s.provider.String() }

// Model returns the explicitly selected model, or "" for the runtime default.
func (s Selection) Model() string { return s.model.String() }

// ReasoningEffort returns the selected model's explicit intensity, or "" to
// use that model's provider default.
func (s Selection) ReasoningEffort() string { return s.reasoningEffort.String() }

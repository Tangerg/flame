package model

import (
	"errors"
)

var ErrIncomplete = errors.New("model selection: provider and model must be set together")

var ErrReasoningEffortWithoutModel = errors.New("model selection: reasoning effort requires provider and model")

var ErrUnsupported = errors.New("model selection: unsupported")

var errExactSelectionRequired = errors.New("model selection is required")

func IsInvalid(err error) bool {
	return errors.Is(err, ErrIncomplete) ||
		errors.Is(err, ErrProviderIdentity) ||
		errors.Is(err, ErrModelIdentity) ||
		errors.Is(err, ErrReasoningEffortWithoutModel) ||
		errors.Is(err, ErrReasoningEffortIdentity)
}

type Selection struct {
	provider        ProviderIdentity
	model           ModelIdentity
	reasoningEffort ReasoningEffortIdentity
}

type Patch struct {
	Provider        *string
	Model           *string
	ReasoningEffort *string
}

func (p Patch) Empty() bool {
	return p.Provider == nil && p.Model == nil && p.ReasoningEffort == nil
}

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

func New(provider, model string) (Selection, error) {
	return NewWithReasoningEffort(provider, model, "")
}

func NewWithReasoningEffort(provider, model, reasoningEffort string) (Selection, error) {
	if err := ValidateSelection(provider, model, reasoningEffort); err != nil {
		return Selection{}, err
	}
	return Selection{
		provider:        ProviderIdentity{value: provider},
		model:           ModelIdentity{value: model},
		reasoningEffort: ReasoningEffortIdentity{value: reasoningEffort},
	}, nil
}

func (s Selection) ValidateExact() error {
	if !s.Configured() {
		return errExactSelectionRequired
	}
	return nil
}

func (s Selection) Equal(other Selection) bool { return s == other }

func (s Selection) String() string {
	if s.Provider() == "" && s.Model() == "" {
		return "<default>"
	}
	identity := s.Provider() + "/" + s.Model()
	if effort := s.ReasoningEffort(); effort != "" {
		return identity + " (reasoning effort " + effort + ")"
	}
	return identity
}

func (s Selection) Configured() bool { return s.model.String() != "" }

func (s Selection) Provider() string { return s.provider.String() }

func (s Selection) Model() string { return s.model.String() }

func (s Selection) ReasoningEffort() string { return s.reasoningEffort.String() }

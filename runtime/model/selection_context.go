package model

import "errors"

type Use string

const (
	Interaction Use = "interaction"
	Utility     Use = "utility"
	Independent Use = "independent"
)

func (Use) JSONSchemaEnum() []string {
	return []string{string(Interaction), string(Utility), string(Independent)}
}

func (u Use) Validate() error {
	switch u {
	case Interaction, Utility, Independent:
		return nil
	default:
		return errors.New("model selection: unknown use")
	}
}

type InputKind string

const (
	TextBytes    InputKind = "text"
	RequestBytes InputKind = "request"
)

func (InputKind) JSONSchemaEnum() []string { return []string{string(TextBytes), string(RequestBytes)} }

// SelectionLimits projects a caller-owned envelope; selection cannot change or
// enforce it. Text bytes and encoded request bytes measure different resources.
type SelectionLimits struct {
	InputKind       InputKind `json:"inputKind"`
	MaxInputBytes   int64     `json:"maxInputBytes" jsonschema:"minimum=1"`
	MaxOutputTokens int64     `json:"maxOutputTokens" jsonschema:"minimum=1"`
}

func (l SelectionLimits) Validate() error {
	if (l.InputKind != TextBytes && l.InputKind != RequestBytes) || l.MaxInputBytes <= 0 || l.MaxOutputTokens <= 0 {
		return errors.New("model selection: positive limits with an explicit input kind are required")
	}
	return nil
}

type SelectionContext struct {
	Use    Use              `json:"use"`
	Limits *SelectionLimits `json:"limits,omitzero"`
}

func (c SelectionContext) Validate() error {
	if err := c.Use.Validate(); err != nil {
		return err
	}
	if c.Use == Interaction {
		if c.Limits != nil {
			return errors.New("model selection: interaction has no captured caller limits")
		}
		return nil
	}
	if c.Limits == nil {
		return errors.New("model selection: captured caller limits are required")
	}
	if err := c.Limits.Validate(); err != nil {
		return err
	}
	if (c.Use == Utility && c.Limits.InputKind != TextBytes) || (c.Use == Independent && c.Limits.InputKind != RequestBytes) {
		return errors.New("model selection: input kind does not match the calling use")
	}
	return nil
}

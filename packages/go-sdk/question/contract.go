// Package question owns the Runtime's public ordered answer contract.
// It does not accept answers into a conversation or own question lifecycle.
package question

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

type Kind string

const (
	Text   Kind = "text"
	Choice Kind = "choice"
)

func (Kind) Values() []string { return []string{string(Text), string(Choice)} }

func (k Kind) Valid() bool { return slices.Contains(k.Values(), string(k)) }

// These bounds keep one complete interaction presentable. Custom input does not
// consume an advertised option.
const (
	MaximumFields  = 4
	MinimumOptions = 2
	MaximumOptions = 4
)

// Field describes only answer semantics; prompt, header and previews have their
// own presentation owners. New detaches each field's advertised option labels.
type Field struct {
	Kind        Kind
	Options     []string
	Multiple    bool
	AllowCustom bool
}

type Contract struct{ fields []Field }

// FieldError identifies the same zero-based field at every contract consumer.
type FieldError struct {
	Index int
	Cause error
}

func (e *FieldError) Error() string { return fmt.Sprintf("question field %d: %v", e.Index+1, e.Cause) }
func (e *FieldError) Unwrap() error { return e.Cause }

// ValidateFieldCount admits a definition before callers allocate its projections.
func ValidateFieldCount(count int) error {
	if count < 1 || count > MaximumFields {
		return fmt.Errorf("question field count must be between 1 and %d, got %d", MaximumFields, count)
	}
	return nil
}

func New(fields []Field) (Contract, error) {
	if err := ValidateFieldCount(len(fields)); err != nil {
		return Contract{}, err
	}
	owned := make([]Field, len(fields))
	for i, field := range fields {
		if err := validateField(field); err != nil {
			return Contract{}, &FieldError{Index: i, Cause: err}
		}
		field.Options = slices.Clone(field.Options)
		owned[i] = field
	}
	return Contract{fields: owned}, nil
}

func validateField(field Field) error {
	switch field.Kind {
	case Text:
		if len(field.Options) != 0 || field.Multiple || field.AllowCustom {
			return errors.New("text cannot carry choice settings")
		}
	case Choice:
		if len(field.Options) < MinimumOptions || len(field.Options) > MaximumOptions {
			return fmt.Errorf("choice option count must be between %d and %d, got %d", MinimumOptions, MaximumOptions, len(field.Options))
		}
	default:
		return fmt.Errorf("unknown kind %q", field.Kind)
	}
	seen := make(map[string]struct{}, len(field.Options))
	for _, label := range field.Options {
		if strings.TrimSpace(label) == "" {
			return errors.New("option label is required")
		}
		if label != strings.TrimSpace(label) {
			return errors.New("option label has surrounding whitespace")
		}
		if _, duplicate := seen[label]; duplicate {
			return fmt.Errorf("repeated option %q", label)
		}
		seen[label] = struct{}{}
	}
	return nil
}

// Validate requires one answer per field. An empty inner slice explicitly skips
// that field; an unanswered question is represented by its owner, not a candidate.
func (c Contract) Validate(answers [][]string) error {
	if err := ValidateFieldCount(len(c.fields)); err != nil {
		return err
	}
	if len(answers) != len(c.fields) {
		return fmt.Errorf("question answers contain %d entries for %d fields", len(answers), len(c.fields))
	}
	for i, values := range answers {
		if err := c.ValidateField(i, values); err != nil {
			return err
		}
	}
	return nil
}

func (c Contract) ValidateField(index int, values []string) error {
	if index < 0 || index >= len(c.fields) {
		return fmt.Errorf("question field index %d is out of range", index)
	}
	if err := validateValues(c.fields[index], values); err != nil {
		return &FieldError{Index: index, Cause: err}
	}
	return nil
}

func validateValues(field Field, values []string) error {
	if len(values) == 0 {
		return nil
	}
	if field.Kind == Text {
		if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
			return errors.New("one non-empty text value is required")
		}
		return nil
	}
	if !field.Multiple && len(values) != 1 {
		return errors.New("exactly one choice is required")
	}
	seen := make(map[string]struct{})
	customSeen := false
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return errors.New("choice values must not be empty")
		}
		if value != trimmed {
			return errors.New("choice values must not have surrounding whitespace")
		}
		if !slices.Contains(field.Options, value) {
			if !field.AllowCustom {
				return fmt.Errorf("unknown choice %q", value)
			}
			if customSeen {
				return errors.New("at most one custom choice is allowed")
			}
			customSeen = true
		}
		if _, duplicate := seen[value]; duplicate {
			return errors.New("duplicate choices are not allowed")
		}
		seen[value] = struct{}{}
	}
	return nil
}

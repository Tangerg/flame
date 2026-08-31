package runtimebinding

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// RunConcurrencyLimitKind is the complete process-wide Run admission policy
// published by one Runtime.
type RunConcurrencyLimitKind string

const (
	RunConcurrencyUnbounded RunConcurrencyLimitKind = "unbounded"
	RunConcurrencyBounded   RunConcurrencyLimitKind = "bounded"
)

// RunConcurrencyLimit is an immutable projection of the Runtime's enforced
// process-wide Run admission policy. Its zero value is intentionally invalid:
// callers must distinguish an unbounded Runtime from a bounded one.
type RunConcurrencyLimit struct {
	kind    RunConcurrencyLimitKind
	maximum int
}

func UnboundedRunConcurrencyLimit() RunConcurrencyLimit {
	return RunConcurrencyLimit{kind: RunConcurrencyUnbounded}
}

func NewBoundedRunConcurrencyLimit(maximum int) (RunConcurrencyLimit, error) {
	if maximum <= 0 {
		return RunConcurrencyLimit{}, errors.New("bounded run concurrency requires a positive maximum")
	}
	return RunConcurrencyLimit{kind: RunConcurrencyBounded, maximum: maximum}, nil
}

func (l RunConcurrencyLimit) Validate() error {
	switch l.kind {
	case RunConcurrencyUnbounded:
		if l.maximum != 0 {
			return errors.New("unbounded run concurrency carries a maximum")
		}
		return nil
	case RunConcurrencyBounded:
		if l.maximum <= 0 {
			return errors.New("bounded run concurrency requires a positive maximum")
		}
		return nil
	default:
		return fmt.Errorf("run concurrency has unknown kind %q", l.kind)
	}
}

func (l RunConcurrencyLimit) Kind() RunConcurrencyLimitKind { return l.kind }

// Maximum returns the enforced maximum and whether this policy is bounded.
func (l RunConcurrencyLimit) Maximum() (int, bool) {
	return l.maximum, l.kind == RunConcurrencyBounded
}

type runConcurrencyLimitJSON struct {
	Type    RunConcurrencyLimitKind `json:"type"`
	Maximum *int                    `json:"maximum,omitempty"`
}

func (l RunConcurrencyLimit) MarshalJSON() ([]byte, error) {
	if err := l.Validate(); err != nil {
		return nil, err
	}
	wire := runConcurrencyLimitJSON{Type: l.kind}
	if l.kind == RunConcurrencyBounded {
		maximum := l.maximum
		wire.Maximum = &maximum
	}
	return json.Marshal(wire)
}

func (l *RunConcurrencyLimit) UnmarshalJSON(data []byte) error {
	if l == nil {
		return errors.New("decode run concurrency into nil receiver")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire runConcurrencyLimitJSON
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("decode run concurrency: %w", err)
	}
	if err := rejectTrailingJSON(decoder); err != nil {
		return err
	}
	switch wire.Type {
	case RunConcurrencyUnbounded:
		if wire.Maximum != nil {
			return errors.New("unbounded run concurrency must not carry maximum")
		}
		*l = UnboundedRunConcurrencyLimit()
		return nil
	case RunConcurrencyBounded:
		if wire.Maximum == nil {
			return errors.New("bounded run concurrency requires maximum")
		}
		bounded, err := NewBoundedRunConcurrencyLimit(*wire.Maximum)
		if err != nil {
			return err
		}
		*l = bounded
		return nil
	default:
		return fmt.Errorf("run concurrency has unknown type %q", wire.Type)
	}
}

func rejectTrailingJSON(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("decode run concurrency: trailing JSON value")
		}
		return fmt.Errorf("decode run concurrency trailing data: %w", err)
	}
	return nil
}

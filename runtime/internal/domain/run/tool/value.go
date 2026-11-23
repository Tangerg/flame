package tool

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"

	framevalue "github.com/Tangerg/frame/value"
)

// The bytes are the identity resume and cache lookups compare, so the escape
// options reproduce what encoding/json wrote; a different spelling would stop
// matching.
var canonicalJSON = json.JoinOptions(
	json.Deterministic(true),
	jsontext.EscapeForHTML(true),
	jsontext.EscapeForJS(true),
)

var (
	ErrInvalidArguments = errors.New("tool: invalid arguments")
	ErrInvalidResult    = errors.New("tool: invalid result")
)

type Arguments struct {
	raw string
}

const emptyArguments = "{}"

func ParseArguments(raw string) (Arguments, error) {
	if len(bytes.TrimSpace([]byte(raw))) == 0 {
		return Arguments{}, nil
	}
	var value map[string]any
	if err := decodeValue([]byte(raw), &value); err != nil {
		return Arguments{}, fmt.Errorf("%w: %w", ErrInvalidArguments, err)
	}
	if value == nil {
		return Arguments{}, fmt.Errorf("%w: expected an object", ErrInvalidArguments)
	}
	return ArgumentsFromMap(value)
}

func ArgumentsFromMap(value map[string]any) (Arguments, error) {
	if value == nil {
		return Arguments{}, fmt.Errorf("%w: expected an object", ErrInvalidArguments)
	}
	encoded, err := json.Marshal(value, canonicalJSON)
	if err != nil {
		return Arguments{}, fmt.Errorf("%w: %w", ErrInvalidArguments, err)
	}
	if string(encoded) == emptyArguments {
		return Arguments{}, nil
	}
	return Arguments{raw: string(encoded)}, nil
}

func (a Arguments) Canonical() string {
	if a.raw == "" {
		return emptyArguments
	}
	return a.raw
}

func (a Arguments) Equal(other Arguments) bool { return a.Canonical() == other.Canonical() }

func (a Arguments) Map() map[string]any {
	var value map[string]any
	if err := decodeValue([]byte(a.Canonical()), &value); err != nil {
		panic(fmt.Sprintf("tool: corrupt Arguments invariant: %v", err))
	}
	return value
}

func (a Arguments) StringField(name string) (string, bool) {
	value, exists := a.Map()[name]
	if !exists {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
}

func (a Arguments) MarshalJSON() ([]byte, error) { return []byte(a.Canonical()), nil }

func (a *Arguments) UnmarshalJSON(data []byte) error {
	parsed, err := ParseArguments(string(data))
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}

type Result struct {
	raw string
}

func NewResult(value any) (Result, error) {
	encoded, err := json.Marshal(value, canonicalJSON)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrInvalidResult, err)
	}
	return ParseResult(encoded)
}

func ParseResult(data []byte) (Result, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return Result{}, fmt.Errorf("%w: empty JSON document", ErrInvalidResult)
	}
	var value any
	if err := decodeValue(data, &value); err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrInvalidResult, err)
	}
	encoded, err := json.Marshal(value, canonicalJSON)
	if err != nil {
		return Result{}, fmt.Errorf("%w: normalize: %w", ErrInvalidResult, err)
	}
	return Result{raw: string(encoded)}, nil
}

func (r Result) Any() any {
	var value any
	if err := decodeValue([]byte(r.Canonical()), &value); err != nil {
		panic(fmt.Sprintf("tool: corrupt Result invariant: %v", err))
	}
	return value
}

func (r Result) String() (string, bool) {
	if r.Canonical()[0] != '"' {
		return "", false
	}
	var value string
	if err := json.Unmarshal([]byte(r.Canonical()), &value); err != nil {
		return "", false
	}
	return value, true
}

func (r Result) Canonical() string {
	if r.raw == "" {
		return "null"
	}
	return r.raw
}

func (r Result) Equal(other Result) bool { return r.Canonical() == other.Canonical() }

func (r Result) MarshalJSON() ([]byte, error) { return []byte(r.Canonical()), nil }

func (r *Result) UnmarshalJSON(data []byte) error {
	parsed, err := ParseResult(data)
	if err != nil {
		return err
	}
	*r = parsed
	return nil
}

// framevalue.Numbers keeps identifiers past IEEE-754's exact range; repeated
// members are refused because a provider document must not mean two things by
// position.
func decodeValue(data []byte, destination any) error {
	return json.Unmarshal(data, destination, framevalue.Numbers())
}

package conversation

import (
	"bytes"
	json "encoding/json/v2"
	"errors"
	"fmt"

	framevalue "github.com/Tangerg/frame/value"
)

type ToolArgumentOverride struct {
	encoded []byte
}

// Duplicate keys are rejected because last-value-wins decoding would make the
// reviewed text and the executed arguments disagree.
func ParseToolArgumentOverride(encoded []byte) (*ToolArgumentOverride, error) {
	object, err := decodeToolArgumentObject(encoded)
	if err != nil {
		return nil, err
	}
	normalized, err := json.Marshal(object, json.Deterministic(true))
	if err != nil {
		return nil, fmt.Errorf("encode tool argument override: %w", err)
	}
	return &ToolArgumentOverride{encoded: normalized}, nil
}

func (t *ToolArgumentOverride) Validate() error {
	if t == nil || len(t.encoded) == 0 {
		return errors.New("tool argument override is not constructed")
	}
	return nil
}

func (t *ToolArgumentOverride) Clone() *ToolArgumentOverride {
	if t == nil {
		return nil
	}
	return &ToolArgumentOverride{encoded: bytes.Clone(t.encoded)}
}

func (t *ToolArgumentOverride) Equal(other *ToolArgumentOverride) bool {
	if t == nil || other == nil {
		return t == other
	}
	return bytes.Equal(t.encoded, other.encoded)
}

func (t *ToolArgumentOverride) JSON() []byte {
	if t == nil {
		return nil
	}
	return bytes.Clone(t.encoded)
}

func (t *ToolArgumentOverride) Object() (map[string]any, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return decodeToolArgumentObject(t.encoded)
}

func (t *ToolArgumentOverride) MarshalJSON() ([]byte, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return bytes.Clone(t.encoded), nil
}

func (t *ToolArgumentOverride) UnmarshalJSON(encoded []byte) error {
	parsed, err := ParseToolArgumentOverride(encoded)
	if err != nil {
		return err
	}
	*t = *parsed
	return nil
}

func decodeToolArgumentObject(encoded []byte) (map[string]any, error) {
	var value any
	if err := json.Unmarshal(encoded, &value, framevalue.Numbers()); err != nil {
		return nil, fmt.Errorf("tool argument override: %w", err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("tool argument override must be a JSON object")
	}
	return object, nil
}

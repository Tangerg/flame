// Package jsoninput owns immutable admitted JSON and its request fingerprint.
// Object order and spelling do not change identity; numeric lexemes do. Decoding
// and re-encoding a DTO must not replace the original request evidence.
package jsoninput

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
)

type Input struct{ value string }

func Parse(data []byte) (Input, error) {
	value := jsontext.Value(append([]byte(nil), data...))
	if err := value.Format(jsontext.ReorderRawObjects(true)); err != nil {
		return Input{}, err
	}
	return Input{value: string(value)}, nil
}

func Encode[T any](value T) (Input, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return Input{}, err
	}
	return Parse(data)
}

func (i Input) Validate() error {
	if i.value == "" {
		return errors.New("json input: admitted value is required")
	}
	return nil
}

func (i Input) String() string       { return i.value }
func (i Input) JSON() jsontext.Value { return []byte(i.value) }

func (i Input) Digest() string {
	hash := sha256.Sum256([]byte(i.value))
	return hex.EncodeToString(hash[:])
}

func (i Input) MarshalJSON() ([]byte, error) {
	if err := i.Validate(); err != nil {
		return nil, err
	}
	return []byte(i.value), nil
}

func (i *Input) UnmarshalJSON(data []byte) error {
	value, err := Parse(data)
	if err != nil {
		return err
	}
	*i = value
	return nil
}

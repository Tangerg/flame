package sqlite

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// decodeStoredJSON decodes exactly one strict JSON value from a stored column.
// A column carrying an unknown field or a second value is corrupt rather than
// merely unexpected, so both are refused here and each caller adds only the
// name of what it was reading.
func decodeStoredJSON(encoded []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("stored JSON has a trailing value")
		}
		return fmt.Errorf("stored JSON trailing value: %w", err)
	}
	return nil
}

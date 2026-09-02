// Package strictjson validates structural JSON invariants that encoding/json
// deliberately leaves to callers at trust boundaries.
package strictjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ValidateUniqueMembers requires exactly one JSON value and rejects duplicate
// object member names at every depth. Escaped and literal spellings of the same
// decoded member name are duplicates.
func ValidateUniqueMembers(encoded []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := validateValue(decoder, "$"); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON contains more than one value")
		}
		return err
	}
	return nil
}

func validateValue(decoder *json.Decoder, path string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, structured := token.(json.Delim)
	if !structured {
		return nil
	}
	var contentErr error
	switch delimiter {
	case '{':
		contentErr = validateObject(decoder, path)
	case '[':
		contentErr = validateArray(decoder, path)
	default:
		return fmt.Errorf("unexpected JSON delimiter %q at %s", delimiter, path)
	}
	if contentErr != nil {
		return contentErr
	}
	_, err = decoder.Token()
	return err
}

func validateObject(decoder *json.Decoder, path string) error {
	seen := make(map[string]struct{})
	for decoder.More() {
		nameToken, err := decoder.Token()
		if err != nil {
			return err
		}
		name, ok := nameToken.(string)
		if !ok {
			return fmt.Errorf("JSON object member at %s has a non-string name", path)
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("duplicate JSON member %q at %s", name, path)
		}
		seen[name] = struct{}{}
		if err := validateValue(decoder, path+"."+name); err != nil {
			return err
		}
	}
	return nil
}

func validateArray(decoder *json.Decoder, path string) error {
	for index := 0; decoder.More(); index++ {
		if err := validateValue(decoder, fmt.Sprintf("%s[%d]", path, index)); err != nil {
			return err
		}
	}
	return nil
}

package cmd

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"io"
)

func writeJSONLine(w io.Writer, value any) error {
	encoded, err := json.Marshal(value, json.Deterministic(true))
	if err != nil {
		return err
	}
	if _, err := w.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return nil
}

func writeIndentedJSON(w io.Writer, value any) error {
	encoded, err := json.Marshal(value, jsontext.WithIndent("  "), json.Deterministic(true))
	if err != nil {
		return err
	}
	if _, err := w.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return nil
}

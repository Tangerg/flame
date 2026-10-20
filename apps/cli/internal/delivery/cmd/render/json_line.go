package render

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"io"
)

// WriteJSONLine writes one machine-readable value as a single JSON line: the
// framing every CLI JSON mode shares, and the framing NDJSON consumers parse by
// splitting on newlines. Deterministic member order keeps a piped stream
// diffable between runs, which encoding/json/v2 does only on request.
func WriteJSONLine(w io.Writer, value any) error {
	encoded, err := json.Marshal(value, json.Deterministic(true))
	if err != nil {
		return err
	}
	if _, err := w.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return nil
}

// WriteIndentedJSON writes one value a person reads directly, with the same
// deterministic member order the machine-readable framing uses.
func WriteIndentedJSON(w io.Writer, value any) error {
	encoded, err := json.Marshal(value, jsontext.WithIndent("  "), json.Deterministic(true))
	if err != nil {
		return err
	}
	if _, err := w.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return nil
}

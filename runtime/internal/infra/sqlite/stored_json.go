package sqlite

import json "encoding/json/v2"

// Persisted columns require one unambiguous value with exact field names. The
// standard decoder rejects duplicate members at every depth in the same pass.
func decodeStoredJSON(encoded []byte, target any) error {
	return json.Unmarshal(encoded, target, json.RejectUnknownMembers(true))
}

// encodeStoredJSON writes every durable value. Deterministic ordering is the
// storage rule rather than a per-record choice: encoding/json/v2 leaves map
// members in iteration order, so the same value would otherwise reach the
// database as different bytes each write. An absent collection stays null so
// existing rows keep their exact shape and a decoded record still distinguishes
// "no value" from "empty".
func encodeStoredJSON(value any) ([]byte, error) {
	return json.Marshal(
		value,
		json.Deterministic(true),
		json.FormatNilSliceAsNull(true),
		json.FormatNilMapAsNull(true),
	)
}

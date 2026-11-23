package persistence

import json "encoding/json/v2"

func decodeStoredJSON(encoded []byte, target any) error {
	return json.Unmarshal(encoded, target, json.RejectUnknownMembers(true))
}

// encoding/json/v2 leaves map members in iteration order, so durable values are
// encoded deterministically; an absent collection stays null.
func encodeStoredJSON(value any) ([]byte, error) {
	return json.Marshal(
		value,
		json.Deterministic(true),
		json.FormatNilSliceAsNull(true),
		json.FormatNilMapAsNull(true),
	)
}

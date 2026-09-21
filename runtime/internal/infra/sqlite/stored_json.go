package sqlite

import json "encoding/json/v2"

// Persisted columns require one unambiguous value with exact field names. The
// standard decoder rejects duplicate members at every depth in the same pass.
func decodeStoredJSON(encoded []byte, target any) error {
	return json.Unmarshal(encoded, target, json.RejectUnknownMembers(true))
}

package execution

import (
	json "encoding/json/v2"
	"strings"

	scoperun "github.com/Tangerg/scope/agent/run"
)

// Both arrays come from Scope's public Entry codec. Their immutable encoding
// is retained with the acknowledged head, so extending the log need not parse
// and encode every earlier model request again.
func appendEntryJSON(previous string, added []byte) string {
	var entries strings.Builder
	entries.Grow(len(previous) + len(added))
	entries.WriteString(previous[:len(previous)-1])
	if len(previous) > 2 && len(added) > 2 {
		entries.WriteByte(',')
	}
	entries.Write(added[1:])
	return entries.String()
}

// Flame owns this storage envelope; Scope continues to encode every Entry and
// decode the complete public Log. Only fixed JSON delimiters are written here.
func encodeExecutionLog(root scoperun.ProcessID, entries string, prepared []preparedRequestWire) ([]byte, error) {
	rootJSON, err := json.Marshal(root)
	if err != nil {
		return nil, err
	}
	var material []byte
	if len(prepared) > 0 {
		material, err = json.Marshal(prepared, json.Deterministic(true))
		if err != nil {
			return nil, err
		}
	}
	payload := make([]byte, 0, len(rootJSON)+len(entries)+len(material)+64)
	payload = append(payload, `{"log":{"root":`...)
	payload = append(payload, rootJSON...)
	payload = append(payload, `,"entries":`...)
	payload = append(payload, entries...)
	payload = append(payload, '}')
	if len(material) > 0 {
		payload = append(payload, `,"preparedRequests":`...)
		payload = append(payload, material...)
	}
	return append(payload, '}'), nil
}

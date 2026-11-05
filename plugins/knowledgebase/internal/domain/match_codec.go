package knowledge

import (
	json "encoding/json/v2"

	"github.com/Tangerg/frame/value"
)

type matchProjection struct {
	Document Header       `json:"document"`
	Chunk    int          `json:"chunk"`
	Text     string       `json:"text"`
	Score    value.Number `json:"score"`
}

// Search owns a binary64 cosine score. The wire exposes its exact JSON number;
// decoding into the native calculation type happens only at this boundary.
func (Match) JSONSchemaModel() any { return matchProjection{} }

func (m Match) MarshalJSON() ([]byte, error) {
	type native Match
	return json.Marshal(native(m))
}

func (m *Match) UnmarshalJSON(data []byte) error {
	var projection matchProjection
	if err := value.DecodeValue(data, &projection, "knowledge search match"); err != nil {
		return err
	}
	type native Match
	var decoded native
	if err := json.Unmarshal(data, &decoded, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	*m = Match(decoded)
	return nil
}

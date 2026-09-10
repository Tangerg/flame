package sqlite

import (
	"bytes"
	"encoding/json"

	"github.com/Tangerg/flame/runtime/internal/strictjson"
)

// decodeStoredJSON decodes one stored column under the same rule the executor
// checkpoint and the wire already use: exactly one JSON value, no member name
// repeated at any depth, and no field this build does not know. A column that
// decodes two ways is corrupt however plausible each reading looks, so it costs
// one validating pass before the decode rather than a silent last-wins result.
func decodeStoredJSON(encoded []byte, target any) error {
	if err := strictjson.ValidateUniqueMembers(encoded); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

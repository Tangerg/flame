package runtimebinding

import (
	json "encoding/json/v2"
)

// The bytes are compared, shown and persisted, so member order must be a property
// of the value rather than of map iteration.
func encodeProjection(value any) ([]byte, error) {
	return json.Marshal(value, json.Deterministic(true))
}

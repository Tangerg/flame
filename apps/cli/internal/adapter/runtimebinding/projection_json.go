package runtimebinding

import (
	json "encoding/json/v2"
)

// encodeProjection re-encodes a value Runtime already decoded. The bytes are
// compared against a second projection of the same fact, shown to the operator,
// and carried into durable CLI state, so member order has to be a property of
// the value rather than of the map iteration that produced it.
func encodeProjection(value any) ([]byte, error) {
	return json.Marshal(value, json.Deterministic(true))
}

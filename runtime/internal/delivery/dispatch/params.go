package dispatch

import (
	"encoding/json/jsontext"

	"github.com/Tangerg/flame/runtime/internal/delivery/transport"
)

func decodeParams(raw jsontext.Value, dst any) error {
	if len(raw) == 0 {
		return nil
	}
	return transport.DecodeValue(raw, dst, "params")
}

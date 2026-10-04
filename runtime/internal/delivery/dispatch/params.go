package dispatch

import (
	"encoding/json/jsontext"

	"github.com/Tangerg/flame/runtime/protocol"
)

func decodeParams(raw jsontext.Value, dst any) error {
	if len(raw) == 0 {
		raw = jsontext.Value(`{}`)
	}
	return protocol.DecodeRequest(raw, dst)
}

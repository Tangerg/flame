package maintenance

import (
	toolcontract "github.com/Tangerg/scope/core/tool"

	json "encoding/json/v2"
	"math"
)

func encodedVisibleToolOutputBytes(output toolcontract.Output) int {
	if len(output.Content) > 0 {
		output.Details = nil
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return math.MaxInt
	}
	return len(encoded)
}

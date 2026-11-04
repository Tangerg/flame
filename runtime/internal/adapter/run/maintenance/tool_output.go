package maintenance

import (
	toolcontract "github.com/Tangerg/scope/core/tool"

	json "encoding/json/v2"
	"fmt"
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

func trimToolOutput(output toolcontract.Output) (toolcontract.Output, bool) {
	if text, textual := output.Text(); textual {
		if len(text) <= ladderResultCap {
			return output, false
		}
		return toolcontract.NewTextOutput(clipResult(text)), true
	}
	encodedBytes := encodedVisibleToolOutputBytes(output)
	if encodedBytes <= ladderResultCap {
		return output, false
	}
	return toolcontract.NewTextOutput(fmt.Sprintf(
		"[%d bytes of media tool output trimmed on compaction; not retrievable]",
		encodedBytes,
	)), true
}

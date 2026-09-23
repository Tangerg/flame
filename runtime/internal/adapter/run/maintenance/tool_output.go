package maintenance

import (
	json "encoding/json/v2"
	"fmt"
	"math"
	"strings"

	"github.com/Tangerg/scope/core/chat"
)

func renderToolOutput(output chat.ToolOutput) string {
	if text, textual := output.Text(); textual {
		return text
	}
	var rendered strings.Builder
	for _, part := range output.Content {
		if rendered.Len() > 0 {
			rendered.WriteByte(' ')
		}
		switch part.Kind {
		case chat.PartText:
			rendered.WriteString(part.Text)
		case chat.PartMedia:
			rendered.WriteString(transcriptMediaMarker)
		}
	}
	return rendered.String()
}

func encodedVisibleToolOutputBytes(output chat.ToolOutput) int {
	if len(output.Content) > 0 {
		output.Details = nil
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return math.MaxInt
	}
	return len(encoded)
}

func trimToolOutput(output chat.ToolOutput) (chat.ToolOutput, bool) {
	if text, textual := output.Text(); textual {
		if len(text) <= ladderResultCap {
			return output, false
		}
		return chat.NewTextToolOutput(clipResult(text)), true
	}
	encodedBytes := encodedVisibleToolOutputBytes(output)
	if encodedBytes <= ladderResultCap {
		return output, false
	}
	return chat.NewTextToolOutput(fmt.Sprintf(
		"[%d bytes of media tool output trimmed on compaction; not retrievable]",
		encodedBytes,
	)), true
}

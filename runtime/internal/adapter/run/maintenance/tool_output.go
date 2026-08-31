package maintenance

import (
	"encoding/json"
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
			rendered.WriteString("[media]")
		}
	}
	if len(output.Details) > 0 {
		if rendered.Len() > 0 {
			rendered.WriteByte(' ')
		}
		rendered.Write(output.Details)
	}
	return rendered.String()
}

func encodedToolOutputBytes(output chat.ToolOutput) int {
	encoded, err := json.Marshal(output)
	if err != nil {
		return math.MaxInt
	}
	return len(encoded)
}

func trimToolOutput(output chat.ToolOutput, limit int) (chat.ToolOutput, bool) {
	encodedBytes := encodedToolOutputBytes(output)
	if encodedBytes <= limit {
		return output, false
	}
	if text, textual := output.Text(); textual {
		return chat.NewTextToolOutput(clipResult(text)), true
	}
	return chat.NewTextToolOutput(fmt.Sprintf(
		"[%d bytes of media tool output trimmed on compaction; not retrievable]",
		encodedBytes,
	)), true
}

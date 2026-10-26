package model

import (
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
			rendered.WriteString(mediaMarker)
		}
	}
	return rendered.String()
}

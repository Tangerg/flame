package model

import (
	toolcontract "github.com/Tangerg/scope/core/tool"

	"strings"

	"github.com/Tangerg/scope/core/content"
)

func renderToolOutput(output toolcontract.Output) string {
	if text, textual := output.Text(); textual {
		return text
	}
	var rendered strings.Builder
	for _, part := range output.Content {
		if rendered.Len() > 0 {
			rendered.WriteByte(' ')
		}
		switch part.Kind {
		case content.Text:
			rendered.WriteString(part.Text)
		case content.Media:
			rendered.WriteString(mediaMarker)
		}
	}
	return rendered.String()
}

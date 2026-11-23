package plugin

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
)

const MaxRendererSelectorNameBytes = 256

var fileExtensionPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_+-]*$`)

type RendererSelector struct {
	Kind  extension.RendererSelectorKind
	Name  string
	Shape extension.ResultShape
}

func (s RendererSelector) exact() bool {
	switch s.Kind {
	case extension.RendererFileExtension:
		return s.Shape == "" && len(s.Name) <= MaxRendererSelectorNameBytes && fileExtensionPattern.MatchString(s.Name)
	case extension.RendererToolName:
		return s.Shape == "" && s.Name != "" && len(s.Name) <= MaxRendererSelectorNameBytes &&
			utf8.ValidString(s.Name) && !strings.ContainsAny(s.Name, " \t\r\n")
	case extension.RendererResultShape:
		return s.Name == "" && s.Shape.Valid()
	default:
		return false
	}
}

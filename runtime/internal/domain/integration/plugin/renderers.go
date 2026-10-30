package plugin

import (
	"regexp"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
)

const MaxRendererSelectorNameBytes = 256

var fileExtensionPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_+-]*$`)

// A selector chooses presentation, never an executable tool or authority.
// Exact tool names take precedence over result shapes; equal candidates are
// ambiguous and the client retains its generic recorded-result presentation.
type RendererSelector struct {
	Kind  extension.RendererSelectorKind
	Name  string
	Shape extension.ResultShape
}

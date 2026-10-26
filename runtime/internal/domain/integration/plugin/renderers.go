package plugin

import "regexp"

const MaxRendererSelectorNameBytes = 256

var fileExtensionPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_+-]*$`)

type RendererSelectorKind string

const (
	rendererToolName      RendererSelectorKind = "toolName"
	rendererResultShape   RendererSelectorKind = "resultShape"
	rendererFileExtension RendererSelectorKind = "fileExtension"
)

// A selector chooses presentation, never an executable tool or authority.
// Exact tool names take precedence over result shapes; equal candidates are
// ambiguous and the client retains its generic recorded-result presentation.
type RendererSelector struct {
	Kind RendererSelectorKind
	Name string
}

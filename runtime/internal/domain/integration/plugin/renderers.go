package plugin

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

const (
	MaxRenderers                 = 32
	MaxRendererSelectorNameBytes = 256
)

var fileExtensionPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_+-]*$`)

type RendererSelectorKind string

const (
	RendererToolName      RendererSelectorKind = "toolName"
	RendererResultShape   RendererSelectorKind = "resultShape"
	RendererFileExtension RendererSelectorKind = "fileExtension"
)

// A selector chooses presentation, never an executable tool or authority.
// Exact tool names take precedence over result shapes; equal candidates are
// ambiguous and the client retains its generic recorded-result presentation.
type RendererSelector struct {
	Kind RendererSelectorKind
	Name string
}

type Renderer struct {
	ID       string
	Title    string
	Selector RendererSelector
	Entry    string
}

func (b *Builder) AdmitRenderer(renderer Renderer) error {
	if len(b.declaration.Renderers) >= MaxRenderers {
		return refuse(DiagnosticComponentLimit, "renderer capacity")
	}
	duplicate := slices.ContainsFunc(b.declaration.Renderers, func(existing Renderer) bool { return existing.ID == renderer.ID })
	if duplicate || !idPattern.MatchString(renderer.ID) || renderer.Title == "" || len(renderer.Title) > MaxPresentationTitleBytes || !utf8.ValidString(renderer.Title) {
		return refuse(DiagnosticInvalidDeclaration, "renderer identity or title %q", renderer.ID)
	}
	selector := renderer.Selector
	if selector.Name == "" || len(selector.Name) > MaxRendererSelectorNameBytes || !utf8.ValidString(selector.Name) {
		return refuse(DiagnosticInvalidDeclaration, "renderer %q selector name", renderer.ID)
	}
	switch selector.Kind {
	case RendererToolName:
		if strings.ContainsAny(selector.Name, " \t\r\n") {
			return refuse(DiagnosticInvalidDeclaration, "renderer %q tool name", renderer.ID)
		}
	case RendererResultShape:
		if !slices.Contains([]string{"command", "search", "patch", "webSearch"}, selector.Name) {
			return refuse(DiagnosticInvalidDeclaration, "renderer %q result shape", renderer.ID)
		}
	case RendererFileExtension:
		if !fileExtensionPattern.MatchString(selector.Name) {
			return refuse(DiagnosticInvalidDeclaration, "renderer %q file extension", renderer.ID)
		}
	default:
		return refuse(DiagnosticInvalidDeclaration, "renderer %q selector", renderer.ID)
	}
	if !ValidResourcePath(renderer.Entry) || !strings.HasSuffix(renderer.Entry, ".html") {
		return refuse(DiagnosticInvalidDeclaration, "renderer %q HTML entry", renderer.ID)
	}
	b.declaration.Renderers = append(b.declaration.Renderers, renderer)
	return nil
}

func (i *Installation) AuthorizeRenderer(release Release, digest fingerprint.Digest, id string) (Renderer, error) {
	if err := i.authorizePresentation(release, digest); err != nil {
		return Renderer{}, err
	}
	for _, renderer := range release.declaration.Renderers {
		if renderer.ID == id {
			return renderer, nil
		}
	}
	return Renderer{}, fmt.Errorf("%w: renderer %q", ErrNotFound, id)
}

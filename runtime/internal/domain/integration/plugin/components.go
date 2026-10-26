package plugin

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

// ComponentScope is the host context a client component renders in.
type ComponentScope string

const (
	componentScopeInstallation ComponentScope = "installation"
	componentScopeWorkspace    ComponentScope = "workspace"
	componentScopeSession      ComponentScope = "session"
	componentScopeClient       ComponentScope = "client"
	componentScopeFile         ComponentScope = "file"
	componentScopeToolResult   ComponentScope = "toolResult"
)

func (s ComponentScope) Valid() bool {
	switch s {
	case componentScopeInstallation, componentScopeWorkspace, componentScopeSession,
		componentScopeClient, componentScopeFile, componentScopeToolResult:
		return true
	default:
		return false
	}
}

// ComponentPlacement is the host surface a client component attaches to.
type ComponentPlacement string

const (
	componentPlacementExport             ComponentPlacement = "export"
	componentPlacementWorkspaceView      ComponentPlacement = "workspaceView"
	componentPlacementSessionAction      ComponentPlacement = "sessionAction"
	componentPlacementFileRenderer       ComponentPlacement = "fileRenderer"
	componentPlacementToolResultRenderer ComponentPlacement = "toolResultRenderer"
)

var componentExport = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]{0,63}$`)

// ClientComponent describes a public rendering entry with an optional own
// callable contribution. It owns no business state or backend activation policy.
type ClientComponent struct {
	Selector     *RendererSelector
	ID           ContributionID
	Title        string
	Scope        ComponentScope
	Placement    ComponentPlacement
	Entry        string
	Export       string
	Contribution ContributionID
	Material     string
}

func (b *Builder) AdmitComponent(value ClientComponent) error {
	if err := value.ID.Validate(); err != nil {
		return err
	}
	if value.Contribution != "" {
		if err := value.Contribution.Validate(); err != nil {
			return err
		}
	}
	if value.Title == "" || len(value.Title) > maxPresentationTitleBytes || !utf8.ValidString(value.Title) || !value.Scope.Valid() || !ValidResourcePath(value.Entry) || !strings.HasPrefix(value.Entry, Namespace+"/") || !strings.HasSuffix(value.Entry, ".js") || !componentExport.MatchString(value.Export) {
		return refuse(DiagnosticInvalidDeclaration, "invalid component %q", value.ID)
	}
	switch value.Placement {
	case componentPlacementExport:
		if value.Selector != nil || value.Material != "" {
			return refuse(DiagnosticInvalidDeclaration, "exported component cannot declare host placement metadata")
		}
	case componentPlacementWorkspaceView:
		if (value.Scope == componentScopeFile || value.Scope == componentScopeToolResult) || value.Selector != nil {
			return refuse(DiagnosticInvalidDeclaration, "workspace component has a renderer target")
		}
	case componentPlacementSessionAction:
		if value.Scope != componentScopeSession || value.Material != "" || value.Selector != nil {
			return refuse(DiagnosticInvalidDeclaration, "session action requires session context")
		}
	case componentPlacementFileRenderer:
		if value.Scope != componentScopeFile || value.Material != "" || value.Selector == nil || value.Selector.Kind != rendererFileExtension || len(value.Selector.Name) > MaxRendererSelectorNameBytes || !fileExtensionPattern.MatchString(value.Selector.Name) {
			return refuse(DiagnosticInvalidDeclaration, "file component requires an exact extension selector")
		}
	case componentPlacementToolResultRenderer:
		if value.Scope != componentScopeToolResult || value.Material != "" || value.Selector == nil || value.Selector.Name == "" || len(value.Selector.Name) > MaxRendererSelectorNameBytes || !utf8.ValidString(value.Selector.Name) {
			return refuse(DiagnosticInvalidDeclaration, "tool result component requires an exact selector")
		}
		switch value.Selector.Kind {
		case rendererToolName:
			if strings.ContainsAny(value.Selector.Name, " \t\r\n") {
				return refuse(DiagnosticInvalidDeclaration, "invalid tool result name")
			}
		case rendererResultShape:
			if !slices.Contains([]string{"command", "search", "patch", "webSearch"}, value.Selector.Name) {
				return refuse(DiagnosticInvalidDeclaration, "invalid tool result shape")
			}
		default:
			return refuse(DiagnosticInvalidDeclaration, "invalid tool result selector")
		}
	default:
		return refuse(DiagnosticInvalidDeclaration, "unsupported component placement")
	}

	if value.Material != "" && !identity.ValidDeclarationID(value.Material) {
		return refuse(DiagnosticInvalidDeclaration, "invalid component material %q", value.Material)
	}
	if slices.ContainsFunc(b.declaration.Components, func(current ClientComponent) bool { return current.ID == value.ID }) {
		return refuse(DiagnosticInvalidDeclaration, "duplicate component %q", value.ID)
	}
	if value.Contribution != "" && !slices.ContainsFunc(b.declaration.Contributions, func(current Contribution) bool { return current.ID == value.Contribution }) {
		return refuse(DiagnosticInvalidDependencies, "component %q has no declared callable contribution", value.ID)
	}
	if value.Selector != nil {
		copy := *value.Selector
		value.Selector = &copy
	}
	b.declaration.Components = append(b.declaration.Components, value)
	return nil
}

func (i *Installation) AuthorizeComponent(release Release, digest identity.Digest, id ContributionID) (ClientComponent, error) {
	if err := i.authorizePresentation(release, digest); err != nil {
		return ClientComponent{}, err
	}
	for _, value := range release.declaration.Components {
		if value.ID == id {
			if value.Selector != nil {
				copy := *value.Selector
				value.Selector = &copy
			}
			return value, nil
		}
	}
	return ClientComponent{}, fmt.Errorf("%w: component %q", ErrNotFound, id)
}

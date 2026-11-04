package plugin

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

var componentExport = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]{0,63}$`)

// ClientComponent describes a public rendering entry with an optional own
// callable contribution. It owns no business state or backend activation policy.
type ClientComponent struct {
	ToolCategory builtin.ToolCategory
	TargetView   string

	Selector        *RendererSelector
	ID              ContributionID
	Title           string
	LocalizedTitles map[string]string
	Scope           extension.ComponentScope
	Placement       extension.ComponentPlacement
	Entry           string
	Export          string
	Contribution    ContributionID
	Material        string
	SettingsIntent  string
}

// componentPlacement is what one host placement admits: the contexts it renders
// in (nil admits any), the renderer selector kinds it targets (none forbids a
// selector), and whether it may carry view material.
type componentPlacement struct {
	scopes    []extension.ComponentScope
	selectors []extension.RendererSelectorKind
	material  bool
	refusal   string
}

var componentPlacements = map[extension.ComponentPlacement]componentPlacement{
	extension.ComponentPlacementExport: {
		refusal: "exported component cannot declare host placement metadata",
	},
	extension.ComponentPlacementWorkspaceView: {
		scopes: []extension.ComponentScope{
			extension.ComponentScopeInstallation, extension.ComponentScopeWorkspace,
			extension.ComponentScopeSession, extension.ComponentScopeClient,
		},
		material: true,
		refusal:  "workspace component has a renderer target",
	},
	extension.ComponentPlacementSettingsPane: {
		scopes: []extension.ComponentScope{
			extension.ComponentScopeClient, extension.ComponentScopeWorkspace, extension.ComponentScopeInstallation,
		},
		material: true,
		refusal:  "settings component requires client, workspace or installation context",
	},
	extension.ComponentPlacementWorkspaceChangesAction: {scopes: []extension.ComponentScope{extension.ComponentScopeWorkspace}, refusal: "workspace changes action requires workspace context"},
	extension.ComponentPlacementSharedClientEffect:     {scopes: []extension.ComponentScope{extension.ComponentScopeClient}, refusal: "shared effect requires client context"},
	extension.ComponentPlacementClientEffect: {
		scopes:  []extension.ComponentScope{extension.ComponentScopeClient},
		refusal: "client effect requires client context without view metadata",
	},
	extension.ComponentPlacementMessageAction: {scopes: []extension.ComponentScope{extension.ComponentScopeSession}, refusal: "message action requires captured session context"},
	extension.ComponentPlacementSessionAction: {
		scopes:  []extension.ComponentScope{extension.ComponentScopeSession},
		refusal: "session action requires session context",
	},
	extension.ComponentPlacementFileRenderer: {
		scopes:    []extension.ComponentScope{extension.ComponentScopeFile},
		selectors: []extension.RendererSelectorKind{extension.RendererFileExtension},
		refusal:   "file component requires an exact extension selector",
	},
	extension.ComponentPlacementToolResultRenderer: {
		scopes:    []extension.ComponentScope{extension.ComponentScopeToolResult},
		selectors: []extension.RendererSelectorKind{extension.RendererToolName, extension.RendererResultShape},
		refusal:   "tool result component requires an exact selector",
	},
}

func (p componentPlacement) admits(value ClientComponent) bool {
	if p.scopes != nil && !slices.Contains(p.scopes, value.Scope) {
		return false
	}
	if value.Material != "" && !p.material {
		return false
	}
	if value.Selector == nil {
		return len(p.selectors) == 0
	}
	return slices.Contains(p.selectors, value.Selector.Kind) && value.Selector.exact()
}

// validate checks what the component declares about itself; the builder checks
// it against the rest of the declaration.
func (c ClientComponent) validate() error {
	if err := c.ID.Validate(); err != nil {
		return err
	}
	if c.Contribution != "" {
		if err := c.Contribution.Validate(); err != nil {
			return err
		}
	}
	if err := c.validatePresentation(); err != nil {
		return err
	}
	if !ValidResourcePath(c.Entry) || !strings.HasPrefix(c.Entry, Namespace+"/") || !strings.HasSuffix(c.Entry, ".js") || !componentExport.MatchString(c.Export) {
		return refuse(DiagnosticInvalidDeclaration, "invalid component %q entry", c.ID)
	}
	placement, known := componentPlacements[c.Placement]
	if !known {
		return refuse(DiagnosticInvalidDeclaration, "unsupported component placement")
	}
	if !placement.admits(c) {
		return refuse(DiagnosticInvalidDeclaration, "%s", placement.refusal)
	}
	if c.Material != "" && !identity.ValidDeclarationID(c.Material) {
		return refuse(DiagnosticInvalidDeclaration, "invalid component material %q", c.Material)
	}
	return nil
}

func (b *Builder) AdmitComponent(value ClientComponent) error {
	if err := value.validate(); err != nil {
		return err
	}
	if slices.ContainsFunc(b.declaration.Components, func(current ClientComponent) bool { return current.ID == value.ID }) {
		return refuse(DiagnosticInvalidDeclaration, "duplicate component %q", value.ID)
	}
	if value.Contribution != "" && !slices.ContainsFunc(b.declaration.Contributions, func(current Contribution) bool { return current.ID == value.Contribution }) {
		return refuse(DiagnosticInvalidDependencies, "component %q has no declared callable contribution", value.ID)
	}
	b.declaration.Components = append(b.declaration.Components, value.clone())
	return nil
}

func (i Installation) AuthorizeComponent(release Release, digest framevalue.Digest, id ContributionID) (ClientComponent, error) {
	if err := i.authorizePresentation(release, digest); err != nil {
		return ClientComponent{}, err
	}
	for _, value := range release.declaration.Components {
		if value.ID == id {
			return value.clone(), nil
		}
	}
	return ClientComponent{}, fmt.Errorf("%w: component %q", ErrNotFound, id)
}

func validComponentTitle(title string) bool {
	return title != "" && len(title) <= MaxPresentationTitleBytes && utf8.ValidString(title)
}

func (c ClientComponent) clone() ClientComponent {
	c.LocalizedTitles = maps.Clone(c.LocalizedTitles)
	if c.Selector != nil {
		selector := *c.Selector
		c.Selector = &selector
	}
	return c
}

func (c ClientComponent) validatePresentation() error {
	if c.SettingsIntent != "" && (c.Placement != extension.ComponentPlacementSettingsPane || !identity.ValidDeclarationID(c.SettingsIntent)) {
		return refuse(DiagnosticInvalidDeclaration, "invalid settings intent for component %q", c.ID)
	}
	if !validComponentTitle(c.Title) || !c.Scope.Valid() || len(c.LocalizedTitles) > maxLocales {
		return refuse(DiagnosticInvalidDeclaration, "invalid component %q", c.ID)
	}
	for tag, title := range c.LocalizedTitles {
		if !validTranslationTag(tag) || !validComponentTitle(title) {
			return refuse(DiagnosticInvalidDeclaration, "invalid localized title for component %q", c.ID)
		}
	}
	if c.ToolCategory != "" && (c.Placement != extension.ComponentPlacementWorkspaceView || !slices.Contains(c.ToolCategory.Values(), string(c.ToolCategory))) {
		return refuse(DiagnosticInvalidDeclaration, "invalid tool category for component %q", c.ID)
	}
	if (c.Placement == extension.ComponentPlacementWorkspaceChangesAction) != (c.TargetView != "") || (c.TargetView != "" && !identity.ValidDeclarationID(c.TargetView)) {
		return refuse(DiagnosticInvalidDeclaration, "invalid target view for component %q", c.ID)
	}
	return nil
}

package plugin

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/qimu/integrity"
)

var componentExport = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]{0,63}$`)

// ClientComponent describes a public rendering entry with an optional own
// callable contribution. It owns no business state or backend activation policy.
type ClientComponent struct {
	Selector       *RendererSelector
	ID             ContributionID
	Title          string
	Scope          extension.ComponentScope
	Placement      extension.ComponentPlacement
	Entry          string
	Export         string
	Contribution   ContributionID
	Material       string
	SettingsIntent extension.SettingsIntent
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
	extension.ComponentPlacementClientEffect: {
		scopes:  []extension.ComponentScope{extension.ComponentScopeClient},
		refusal: "client effect requires client context without view metadata",
	},
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
	if c.SettingsIntent != "" && (c.Placement != extension.ComponentPlacementSettingsPane || !c.SettingsIntent.Valid()) {
		return refuse(DiagnosticInvalidDeclaration, "invalid settings intent for component %q", c.ID)
	}
	if c.Title == "" || len(c.Title) > MaxPresentationTitleBytes || !utf8.ValidString(c.Title) || !c.Scope.Valid() {
		return refuse(DiagnosticInvalidDeclaration, "invalid component %q", c.ID)
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
	if value.Selector != nil {
		copy := *value.Selector
		value.Selector = &copy
	}
	b.declaration.Components = append(b.declaration.Components, value)
	return nil
}

func (i *Installation) AuthorizeComponent(release Release, digest integrity.Digest, id ContributionID) (ClientComponent, error) {
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

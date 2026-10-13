package plugin

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

const ComponentScopePattern = `^(installation|workspace|session|client|file)$`

var componentScope = regexp.MustCompile(ComponentScopePattern)
var componentExport = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]{0,63}$`)

// ClientComponent describes a public rendering entry with an optional own
// callable contribution. It owns no business state or backend activation policy.
type ClientComponent struct {
	Selector     *RendererSelector
	ID           ContributionID
	Title        string
	Scope        string
	Placement    string
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
	if value.Title == "" || len(value.Title) > MaxPresentationTitleBytes || !utf8.ValidString(value.Title) || !componentScope.MatchString(value.Scope) || !ValidResourcePath(value.Entry) || !strings.HasPrefix(value.Entry, Namespace+"/") || !strings.HasSuffix(value.Entry, ".js") || !componentExport.MatchString(value.Export) {
		return refuse(DiagnosticInvalidDeclaration, "invalid component %q", value.ID)
	}
	switch value.Placement {
	case "workspaceView":
		if value.Scope == "file" || value.Selector != nil {
			return refuse(DiagnosticInvalidDeclaration, "workspace component has a renderer target")
		}
	case "fileRenderer":
		if value.Scope != "file" || value.Material != "" || value.Selector == nil || value.Selector.Kind != RendererFileExtension || len(value.Selector.Name) > MaxRendererSelectorNameBytes || !fileExtensionPattern.MatchString(value.Selector.Name) {
			return refuse(DiagnosticInvalidDeclaration, "file component requires an exact extension selector")
		}
	default:
		return refuse(DiagnosticInvalidDeclaration, "unsupported component placement")
	}

	if value.Material != "" && !idPattern.MatchString(value.Material) {
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

func (i *Installation) AuthorizeComponent(release Release, digest fingerprint.Digest, id ContributionID) (ClientComponent, error) {
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

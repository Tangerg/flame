package plugin

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

var componentExport = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]{0,63}$`)

// ClientComponent describes a public rendering entry and its own callable
// contribution. It owns no business state or backend activation policy.
type ClientComponent struct {
	ID           ContributionID
	Title        string
	Scope        string
	Placement    string
	Entry        string
	Export       string
	Contribution ContributionID
}

func (b *Builder) AdmitComponent(value ClientComponent) error {
	if err := value.ID.Validate(); err != nil {
		return err
	}
	if err := value.Contribution.Validate(); err != nil {
		return err
	}
	if value.Title == "" || len(value.Title) > MaxViewTitleBytes || !utf8.ValidString(value.Title) || value.Scope != "installation" || value.Placement != "workspaceView" || !ValidResourcePath(value.Entry) || !strings.HasPrefix(value.Entry, Namespace+"/") || !strings.HasSuffix(value.Entry, ".js") || !componentExport.MatchString(value.Export) {
		return refuse(DiagnosticInvalidDeclaration, "invalid component %q", value.ID)
	}
	if slices.ContainsFunc(b.declaration.Components, func(current ClientComponent) bool { return current.ID == value.ID }) {
		return refuse(DiagnosticInvalidDeclaration, "duplicate component %q", value.ID)
	}
	if !slices.ContainsFunc(b.declaration.Contributions, func(current Contribution) bool { return current.ID == value.Contribution }) {
		return refuse(DiagnosticInvalidDependencies, "component %q has no declared callable contribution", value.ID)
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
			return value, nil
		}
	}
	return ClientComponent{}, fmt.Errorf("%w: component %q", ErrNotFound, id)
}

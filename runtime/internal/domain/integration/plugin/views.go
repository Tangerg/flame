package plugin

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

type ViewKind string

const (
	SessionTrajectory ViewKind = "sessionTrajectory"
	AgentMemory       ViewKind = "agentMemory"
)

const (
	MaxViewBytes      = 512 << 10
	MaxViewTitleBytes = 128
)

// ViewDeclaration admits one self-contained HTML page and its only read scope.
// Entry stays within the release; clients address the view by ID, never by path.
type ViewDeclaration struct {
	ID    string
	Title string
	Kind  ViewKind
	Entry string
}

func (b *Builder) AdmitView(view ViewDeclaration) error {
	if len(b.declaration.Views) >= MaxViews {
		return refuse(DiagnosticComponentLimit, "view capacity")
	}
	duplicate := slices.ContainsFunc(b.declaration.Views, func(existing ViewDeclaration) bool { return existing.ID == view.ID })
	if duplicate || !idPattern.MatchString(view.ID) || view.Title == "" || len(view.Title) > MaxViewTitleBytes {
		return refuse(DiagnosticInvalidDeclaration, "view identity or title %q", view.ID)
	}
	if (view.Kind != SessionTrajectory && view.Kind != AgentMemory) || !ValidResourcePath(view.Entry) || !strings.HasSuffix(view.Entry, ".html") {
		return refuse(DiagnosticInvalidDeclaration, "view %q kind or HTML entry", view.ID)
	}
	b.declaration.Views = append(b.declaration.Views, view)
	return nil
}

// AuthorizeView binds authority to the active installation's exact release.
func (i *Installation) AuthorizeView(release Release, digest fingerprint.Digest, id string) (ViewDeclaration, error) {
	if i.Selected() != digest || release.Digest() != digest {
		return ViewDeclaration{}, fmt.Errorf("%w: view release", ErrStale)
	}
	if !i.Active() {
		return ViewDeclaration{}, fmt.Errorf("%w: view requires an enabled approved release", ErrUnapproved)
	}
	for _, view := range release.declaration.Views {
		if view.ID == id {
			return view, nil
		}
	}
	return ViewDeclaration{}, fmt.Errorf("%w: view %q", ErrNotFound, id)
}

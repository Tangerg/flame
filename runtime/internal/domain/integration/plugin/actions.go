package plugin

import (
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

type ActionOperation string

const RenameSession ActionOperation = "renameSession"

const MaxActionTitleBytes = 128

// Action binds a human intent to an existing product operation. It owns no
// parameter schema, executable code, or authority to invoke itself.
type Action struct {
	ID        string
	Title     string
	Operation ActionOperation
}

func (b *Builder) AdmitAction(action Action) error {
	if len(b.declaration.Actions) >= MaxActions {
		return refuse(DiagnosticComponentLimit, "action capacity")
	}
	duplicate := slices.ContainsFunc(b.declaration.Actions, func(existing Action) bool { return existing.ID == action.ID })
	if duplicate || !idPattern.MatchString(action.ID) || action.Title == "" || len(action.Title) > MaxActionTitleBytes || !utf8.ValidString(action.Title) {
		return refuse(DiagnosticInvalidDeclaration, "action identity or title %q", action.ID)
	}
	if action.Operation != RenameSession {
		return refuse(DiagnosticUnsupportedContribution, "action operation %q", action.Operation)
	}
	b.declaration.Actions = append(b.declaration.Actions, action)
	return nil
}

func (i *Installation) AuthorizeAction(release Release, digest fingerprint.Digest, id string, operation ActionOperation) error {
	if i.Selected() != digest || release.Digest() != digest {
		return fmt.Errorf("%w: action release", ErrStale)
	}
	if !i.Active() {
		return fmt.Errorf("%w: action requires an enabled approved release", ErrUnapproved)
	}
	for _, action := range release.declaration.Actions {
		if action.ID == id && action.Operation == operation {
			return nil
		}
	}
	return fmt.Errorf("%w: action %q", ErrNotFound, id)
}

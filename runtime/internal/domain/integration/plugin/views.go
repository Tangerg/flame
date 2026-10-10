package plugin

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"

	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

type ViewKind string

const (
	SessionTrajectory ViewKind = "sessionTrajectory"
	AgentMemory       ViewKind = "agentMemory"
	Schedules         ViewKind = "schedules"
)

const (
	MaxViewBytes      = 512 << 10
	MaxViewTitleBytes = 128
)

// ViewDeclaration admits one self-contained HTML page and its only read scope.
// Entry stays within the release; clients address the view by ID, never by path.
type ViewDeclaration struct {
	ID                string
	Title             string
	Kind              ViewKind
	Entry             string
	ScheduleTemplates []ScheduleTemplate
}

// ScheduleTemplate is authoring content, never a reference retained by a Schedule.
type ScheduleTemplate struct {
	ID           string
	Title        string
	Instructions string
	Cron         string
}

const MaxScheduleTemplates = 16

func (v ViewDeclaration) clone() ViewDeclaration {
	v.ScheduleTemplates = slices.Clone(v.ScheduleTemplates)
	return v
}

func (b *Builder) AdmitView(view ViewDeclaration) error {
	if len(b.declaration.Views) >= MaxViews {
		return refuse(DiagnosticComponentLimit, "view capacity")
	}
	duplicate := slices.ContainsFunc(b.declaration.Views, func(existing ViewDeclaration) bool { return existing.ID == view.ID })
	if duplicate || !idPattern.MatchString(view.ID) || view.Title == "" || len(view.Title) > MaxViewTitleBytes {
		return refuse(DiagnosticInvalidDeclaration, "view identity or title %q", view.ID)
	}
	if (view.Kind != SessionTrajectory && view.Kind != AgentMemory && view.Kind != Schedules) || !ValidResourcePath(view.Entry) || !strings.HasSuffix(view.Entry, ".html") {
		return refuse(DiagnosticInvalidDeclaration, "view %q kind or HTML entry", view.ID)
	}
	if len(view.ScheduleTemplates) > MaxScheduleTemplates || (view.Kind != Schedules && view.ScheduleTemplates != nil) {
		return refuse(DiagnosticInvalidDeclaration, "view %q schedule templates", view.ID)
	}
	ids := make(map[string]struct{}, len(view.ScheduleTemplates))
	for _, template := range view.ScheduleTemplates {
		_, duplicate := ids[template.ID]
		if duplicate || !idPattern.MatchString(template.ID) || template.Title == "" || len(template.Title) > MaxViewTitleBytes || !utf8.ValidString(template.Title) || !utf8.ValidString(template.Instructions) || len(template.Instructions) > MaxViewBytes {
			return refuse(DiagnosticInvalidDeclaration, "view %q schedule template %q", view.ID, template.ID)
		}
		if err := (schedule.Draft{Instructions: template.Instructions, Cron: template.Cron}).Validate(); err != nil {
			return refuse(DiagnosticInvalidDeclaration, "view %q schedule template %q: %v", view.ID, template.ID, err)
		}
		ids[template.ID] = struct{}{}
	}
	b.declaration.Views = append(b.declaration.Views, view.clone())
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
			return view.clone(), nil
		}
	}
	return ViewDeclaration{}, fmt.Errorf("%w: view %q", ErrNotFound, id)
}

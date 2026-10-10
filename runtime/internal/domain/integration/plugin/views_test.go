package plugin

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestViewAuthorityFollowsTheActiveExactRelease(t *testing.T) {
	view := ViewDeclaration{ID: "trajectory", Title: "Trajectory", Kind: SessionTrajectory, Entry: "views/index.html"}
	release := testRelease(t, "1", Declaration{Name: "review", Views: []ViewDeclaration{view}})
	installation := approvedInstallation(t, release, nil)
	if _, err := installation.AuthorizeView(release, release.Digest(), view.ID); !errors.Is(err, ErrUnapproved) {
		t.Fatalf("disabled view admitted: %v", err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	if got, err := installation.AuthorizeView(release, release.Digest(), view.ID); err != nil || !reflect.DeepEqual(got, view) {
		t.Fatalf("active view: %v, %v", got, err)
	}
	copy := release.Declaration()
	copy.Views[0].Entry = "other.html"
	if release.Declaration().Views[0].Entry != view.Entry {
		t.Fatal("projection advanced release content")
	}
	if _, err := installation.AuthorizeView(release, testDigest("2"), view.ID); !errors.Is(err, ErrStale) {
		t.Fatalf("stale digest admitted: %v", err)
	}
	if _, err := installation.AuthorizeView(release, release.Digest(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("undeclared view admitted: %v", err)
	}
	installation.Revoke()
	if _, err := installation.AuthorizeView(release, release.Digest(), view.ID); !errors.Is(err, ErrUnapproved) {
		t.Fatalf("revoked view admitted: %v", err)
	}
}

func TestViewDeclarationCannotGrantOtherOperationsOrHostPaths(t *testing.T) {
	for _, view := range []ViewDeclaration{
		{ID: "trajectory", Title: "Trajectory", Kind: "tools.invoke", Entry: "view.html"},
		{ID: "trajectory", Title: "Trajectory", Kind: SessionTrajectory, Entry: "../view.html"},
		{ID: "trajectory", Title: "Trajectory", Kind: SessionTrajectory, Entry: "/view.html"},
		{ID: "trajectory", Title: "Trajectory", Kind: SessionTrajectory, Entry: "https://remote/view.html"},
		{ID: "trajectory", Title: "Trajectory", Kind: SessionTrajectory, Entry: "view.js"},
	} {
		if _, err := NewRelease(testDigest("1"), Declaration{Name: "review", Views: []ViewDeclaration{view}}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid view admitted: %+v: %v", view, err)
		}
	}
}

func TestScheduleTemplatesRemainImmutableAuthoringContent(t *testing.T) {
	template := ScheduleTemplate{ID: "weekly", Title: "Weekly review", Instructions: "Review changes", Cron: "0 9 * * 1"}
	view := ViewDeclaration{ID: "schedules", Title: "Schedules", Kind: Schedules, Entry: "views/schedules.html", ScheduleTemplates: []ScheduleTemplate{template}}
	builder, err := NewBuilder("schedules", "1.0.0", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.AdmitView(view); err != nil {
		t.Fatal(err)
	}
	view.ScheduleTemplates[0].Cron = "invalid"
	release, err := builder.Release(testDigest("1"))
	if err != nil {
		t.Fatal(err)
	}
	installation := approvedInstallation(t, release, nil)
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	projected := release.Declaration()
	projected.Views[0].ScheduleTemplates[0].Instructions = "Changed by a projection"
	authorized, err := installation.AuthorizeView(release, release.Digest(), "schedules")
	if err != nil || authorized.ScheduleTemplates[0] != template {
		t.Fatalf("template ownership: %+v, %v", authorized, err)
	}
	authorized.ScheduleTemplates[0].Cron = "0 * * * *"
	if release.Declaration().Views[0].ScheduleTemplates[0] != template {
		t.Fatal("authorization leaked mutable template content")
	}
}

func TestScheduleTemplatesUseScheduleValidationAndOnlyTheirDeclaredKind(t *testing.T) {
	template := ScheduleTemplate{ID: "daily", Title: "Daily", Instructions: "Review", Cron: "0 9 * * *"}
	for _, view := range []ViewDeclaration{
		{ID: "memory", Title: "Memory", Kind: AgentMemory, Entry: "view.html", ScheduleTemplates: []ScheduleTemplate{template}},
		{ID: "memory", Title: "Memory", Kind: AgentMemory, Entry: "view.html", ScheduleTemplates: []ScheduleTemplate{}},
		{ID: "schedules", Title: "Schedules", Kind: Schedules, Entry: "view.html", ScheduleTemplates: []ScheduleTemplate{template, template}},
		{ID: "schedules", Title: "Schedules", Kind: Schedules, Entry: "view.html", ScheduleTemplates: []ScheduleTemplate{{ID: "daily", Title: "Daily", Instructions: "Review", Cron: "broken"}}},
		{ID: "schedules", Title: "Schedules", Kind: Schedules, Entry: "view.html", ScheduleTemplates: []ScheduleTemplate{{ID: "daily", Title: "Daily", Cron: template.Cron}}},
	} {
		if _, err := NewRelease(testDigest("1"), Declaration{Name: "schedules", Views: []ViewDeclaration{view}}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid template admitted: %+v, %v", view, err)
		}
	}
}

func TestScheduleTemplateCapacityIsAnAdmissionBound(t *testing.T) {
	view := ViewDeclaration{ID: "schedules", Title: "Schedules", Kind: Schedules, Entry: "view.html"}
	for index := range MaxScheduleTemplates {
		view.ScheduleTemplates = append(view.ScheduleTemplates, ScheduleTemplate{ID: fmt.Sprintf("template-%d", index), Title: "Review", Instructions: "Review", Cron: "0 9 * * 1"})
	}
	if _, err := NewRelease(testDigest("1"), Declaration{Name: "schedules", Views: []ViewDeclaration{view}}); err != nil {
		t.Fatal(err)
	}
	view.ScheduleTemplates = append(view.ScheduleTemplates, ScheduleTemplate{ID: "excess", Title: "Review", Instructions: "Review", Cron: "0 9 * * 1"})
	if _, err := NewRelease(testDigest("1"), Declaration{Name: "schedules", Views: []ViewDeclaration{view}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unbounded templates admitted: %v", err)
	}
}

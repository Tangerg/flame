package plugin

import (
	"errors"
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
	if got, err := installation.AuthorizeView(release, release.Digest(), view.ID); err != nil || got != view {
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

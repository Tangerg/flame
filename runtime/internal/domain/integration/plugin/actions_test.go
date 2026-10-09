package plugin

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestActionAdmissionIsClosedBoundedAndImmutable(t *testing.T) {
	builder, err := NewBuilder("actions", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []Action{
		{ID: "rename", Title: "Rename", Operation: "tools.invoke"},
		{ID: "invalid id", Title: "Rename", Operation: RenameSession},
		{ID: "rename", Title: strings.Repeat("x", MaxActionTitleBytes+1), Operation: RenameSession},
	} {
		if err := builder.AdmitAction(action); err == nil {
			t.Fatalf("admitted %+v", action)
		}
	}
	for n := range MaxActions {
		if err := builder.AdmitAction(Action{ID: fmt.Sprintf("rename-%d", n), Title: "Rename", Operation: RenameSession}); err != nil {
			t.Fatal(err)
		}
	}
	if err := builder.AdmitAction(Action{ID: "extra", Title: "Rename", Operation: RenameSession}); err == nil {
		t.Fatal("action bound was ignored")
	}
	release, err := builder.Release(testDigest("1"))
	if err != nil {
		t.Fatal(err)
	}
	projected := release.Declaration()
	projected.Actions[0].Operation = "tools.invoke"
	if release.Declaration().Actions[0].Operation != RenameSession {
		t.Fatal("projection changed the admitted action")
	}
	duplicate, err := NewBuilder("actions", "", "")
	if err != nil {
		t.Fatal(err)
	}
	action := Action{ID: "rename", Title: "Rename", Operation: RenameSession}
	if err := duplicate.AdmitAction(action); err != nil {
		t.Fatal(err)
	}
	if err := duplicate.AdmitAction(action); err == nil {
		t.Fatal("duplicate action admitted")
	}
}

func TestActionAuthorityRequiresAnExactEnabledReleaseAndOperation(t *testing.T) {
	release, err := NewRelease(testDigest("1"), Declaration{Name: "actions", Actions: []Action{{ID: "rename", Title: "Rename", Operation: RenameSession}}})
	if err != nil {
		t.Fatal(err)
	}
	installation, err := New(testInstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.AuthorizeAction(release, release.Digest(), "rename", RenameSession); !errors.Is(err, ErrUnapproved) {
		t.Fatalf("unapproved: %v", err)
	}
	if err := installation.Approve(release); err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	if err := installation.AuthorizeAction(release, release.Digest(), "rename", RenameSession); err != nil {
		t.Fatal(err)
	}
	if err := installation.AuthorizeAction(release, testDigest("2"), "rename", RenameSession); !errors.Is(err, ErrStale) {
		t.Fatalf("stale: %v", err)
	}
	for _, action := range []Action{{ID: "missing", Operation: RenameSession}, {ID: "rename", Operation: "tools.invoke"}} {
		if err := installation.AuthorizeAction(release, release.Digest(), action.ID, action.Operation); !errors.Is(err, ErrNotFound) {
			t.Fatalf("undeclared operation: %v", err)
		}
	}
	installation.Revoke()
	if err := installation.AuthorizeAction(release, release.Digest(), "rename", RenameSession); !errors.Is(err, ErrUnapproved) {
		t.Fatalf("withdrawn: %v", err)
	}
}

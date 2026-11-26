package skillauthoring

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	skillspec "github.com/Tangerg/scope/skills"
)

func TestPublicationResumesItsPreparedFile(t *testing.T) {
	for _, test := range []struct {
		name  string
		place bool
		alter func(*testing.T, string)
		want  error
	}{
		{name: "before placement"},
		{name: "placed and edited", place: true, alter: editPublishedFile},
		{name: "same bytes in a replacement", place: true, alter: replacePublishedFile, want: skills.ErrPublicationUnconfirmed},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			repository := t.TempDir()
			library := filepath.Join(repository, "skills")
			store, err := NewRepositoryStore(repository, library)
			if err != nil {
				t.Fatal(err)
			}
			proposal := skills.Proposal{
				Scope: skills.ScopeRepository, Name: "revise", Description: "Read the deployment manifest.",
				Instructions: "Read the current deployment manifest.", Origin: skills.ProposalOriginRequested, Revises: true,
			}
			path := filepath.Join(library, proposal.Name, skillspec.SkillFile)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			before := []byte("---\nname: revise\ndescription: Read the manifest.\n---\nRead the original manifest.\n")
			if err := os.WriteFile(path, before, 0600); err != nil {
				t.Fatal(err)
			}
			originalMode, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			previous := digestDocument(before, true)
			permit := fmt.Sprintf("%x", sha256.Sum256([]byte("original publication")))
			journal := t.TempDir()
			leavePreparedPublication(t, store, proposal, previous, permit, journal, test.place)
			if test.alter != nil {
				test.alter(t, path)
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.Publish(ctx, proposal, previous, permit, journal, func(ctx context.Context, write func(context.Context) error) error {
				return write(ctx)
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("publication recovery = %v, want %v", err, test.want)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if test.place && string(got) != string(want) {
				t.Fatalf("recovery overwrote a later document: %q", got)
			}
			if !test.place && !strings.Contains(string(got), proposal.Instructions) {
				t.Fatalf("prepared publication did not place its document: %q", got)
			}
			if info, err := os.Stat(path); err != nil || info.Mode().Perm() != originalMode.Mode().Perm() {
				t.Fatalf("published document permissions = %v, %v", info, err)
			}
		})
	}
}

func leavePreparedPublication(t *testing.T, store *Store, proposal skills.Proposal, previous, permit, journal string, place bool) {
	t.Helper()
	root, release, err := store.openWritableRoot(t.Context(), "prepare review fixture")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	private, err := os.OpenRoot(journal)
	if err != nil {
		t.Fatal(err)
	}
	defer private.Close()
	p := publication{store: store, root: root, proposal: proposal, previous: previous, permit: permit}
	binding, err := p.binding()
	if err != nil {
		t.Fatal(err)
	}
	proof, err := p.prepare(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	receipt := publicationReceipt{root: private, permit: permit}
	if err := receipt.save(publicationRecord{Binding: binding, State: publicationPrepared, Proof: &proof}); err != nil {
		t.Fatal(err)
	}
	if place {
		if _, err := p.place(t.Context(), proof); err != nil {
			t.Fatal(err)
		}
	}
}

func editPublishedFile(t *testing.T, path string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString("Use the new user-authored shortcut.\n")
	if err := errors.Join(err, file.Close()); err != nil {
		t.Fatal(err)
	}
}

func replacePublishedFile(t *testing.T, path string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(filepath.Dir(path), ".retained-skill")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0600); err != nil {
		t.Fatal(err)
	}
}

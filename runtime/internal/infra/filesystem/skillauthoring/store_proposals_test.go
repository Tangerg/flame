package skillauthoring_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skillauthoring"
)

func candidate(name, body string, revision bool) skills.Proposal {
	return skills.Proposal{Origin: skills.ProposalOriginRequested, Name: name, Scope: skills.ScopeUser, Description: "A reusable workflow for verifying the workspace.", Instructions: body, Revises: revision}
}
func installActive(t *testing.T, s *skillauthoring.Store, name, body string) {
	t.Helper()
	if _, err := publish(t, s, candidate(name, body, false)); err != nil {
		t.Fatal(err)
	}
}
func approveRevision(t *testing.T, s *skillauthoring.Store, name, body string) {
	t.Helper()
	if _, err := publish(t, s, candidate(name, body, true)); err != nil {
		t.Fatal(err)
	}
}
func TestApproveProposalRevisionReplacesActiveAndArchivesOld(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root, skills.ScopeUser)
	installActive(t, store, "run-tests", "old instructions: use make test")
	approveRevision(t, store, "run-tests", "new instructions: use go test ./...")

	active, err := os.ReadFile(filepath.Join(root, "run-tests", "SKILL.md"))
	if err != nil {
		t.Fatalf("read active: %v", err)
	}
	if !strings.Contains(string(active), "go test ./...") {
		t.Fatalf("active not replaced with the revision:\n%s", active)
	}
	archived, err := os.ReadFile(filepath.Join(root, "_archive", "run-tests", "SKILL.md"))
	if err != nil {
		t.Fatalf("superseded version not archived: %v", err)
	}
	if !strings.Contains(string(archived), "make test") {
		t.Fatalf("archived version is not the superseded one:\n%s", archived)
	}
}

func TestApproveProposalRevisionOverwritesStaleArchiveSlot(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root, skills.ScopeUser)
	installActive(t, store, "note", "instructions v1")
	approveRevision(t, store, "note", "instructions v2") // archives v1
	approveRevision(t, store, "note", "instructions v3") // archives v2, overwriting v1

	active, err := os.ReadFile(filepath.Join(root, "note", "SKILL.md"))
	if err != nil || !strings.Contains(string(active), "instructions v3") {
		t.Fatalf("active = %q, err=%v; want instructions v3", active, err)
	}
	archived, err := os.ReadFile(filepath.Join(root, "_archive", "note", "SKILL.md"))
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if !strings.Contains(string(archived), "instructions v2") || strings.Contains(string(archived), "instructions v1") {
		t.Fatalf("archive should hold only the immediately-superseded v2:\n%s", archived)
	}
}

func TestPublicationReceiptSurvivesExternalDeletionAndRestart(t *testing.T) {
	root := t.TempDir()
	p := candidate("verify", "Original instructions", false)
	s := newStore(t, root, skills.ScopeUser)
	permit := strings.Repeat("a", 64)
	journal := t.TempDir()
	if _, err := s.Publish(t.Context(), p, "", permit, journal); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, p.Name)); err != nil {
		t.Fatal(err)
	}
	reopened := newStore(t, root, skills.ScopeUser)
	changed, err := reopened.Publish(t.Context(), p, "", permit, journal)
	if err != nil || len(changed) != 0 {
		t.Fatalf("replay changed files: %v, %v", changed, err)
	}
	if _, err := os.Stat(filepath.Join(root, p.Name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted skill republished: %v", err)
	}
}
func TestPublicationRequiresReviewedPreimage(t *testing.T) {
	root := t.TempDir()
	s := newStore(t, root, skills.ScopeUser)
	installActive(t, s, "verify", "original instructions")
	p := candidate("verify", "replacement instructions", true)
	original, err := s.Preimage(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, p.Name, "SKILL.md")
	outside := []byte("intervening user edit")
	if err := os.WriteFile(path, outside, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(t.Context(), p, original, strings.Repeat("b", 64), t.TempDir()); !errors.Is(err, skills.ErrProposalChanged) {
		t.Fatalf("changed preimage: %v", err)
	}
	if body, err := os.ReadFile(path); err != nil || string(body) != string(outside) {
		t.Fatalf("intervening edit lost: %s %v", body, err)
	}
}

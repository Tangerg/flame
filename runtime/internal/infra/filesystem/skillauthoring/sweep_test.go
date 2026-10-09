package skillauthoring_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skillauthoring"
)

const (
	sweepArchive = 30 * 24 * time.Hour
)

var sweepBase = time.Unix(1_700_000_000, 0)

// installActiveAgentSkill approves an agent-authored skill (origin=agent) so the
// provenance-gated curator will consider it.
func installActiveAgentSkill(t *testing.T, store *skillauthoring.Store, name string) {
	t.Helper()
	ref, _, err := store.SubmitProposal(t.Context(), skills.Proposal{Scope: skills.ScopeUser,
		Name:         name,
		Description:  "An agent-authored skill with a long enough description.",
		Instructions: "instructions",
		Origin:       skills.ProposalOriginMined,
	})
	if err != nil {
		t.Fatalf("SubmitProposal(%s): %v", name, err)
	}
	if _, err := store.ApproveProposal(t.Context(), ref); err != nil {
		t.Fatalf("ApproveProposal(%s): %v", name, err)
	}
}

func TestSweepIdleArchivesOnlyIdleAgentSkills(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root, skills.ScopeUser)
	installActiveAgentSkill(t, store, "agent-skill")
	humanSkill := "---\nname: human-skill\ndescription: A skill the user wrote directly into the library.\n---\n\ninstructions\n"
	if err := os.MkdirAll(filepath.Join(root, "human-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "human-skill", "SKILL.md"), []byte(humanSkill), 0o644); err != nil {
		t.Fatal(err)
	}

	// First sweep seeds FirstSeen for both; nothing is idle yet.
	archived, _, err := store.SweepIdle(t.Context(), sweepBase, sweepArchive)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 0 {
		t.Fatalf("first sweep archived %v", archived)
	}

	// Far past the archive threshold: the agent skill is idle, the human one is exempt.
	later := sweepBase.Add(sweepArchive + time.Hour)
	archived, _, err = store.SweepIdle(t.Context(), later, sweepArchive)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 || archived[0] != "agent-skill" {
		t.Fatalf("archived = %v, want [agent-skill]", archived)
	}
	if _, err := os.Stat(filepath.Join(root, "_archive", "agent-skill", "SKILL.md")); err != nil {
		t.Fatalf("agent-skill not archived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "human-skill", "SKILL.md")); err != nil {
		t.Fatalf("human-skill must stay active (provenance gate): %v", err)
	}
}

func TestSweepIdleGivesNeverSweptSkillGrace(t *testing.T) {
	store := newStore(t, t.TempDir(), skills.ScopeUser)
	installActiveAgentSkill(t, store, "fresh")
	// A skill first seen at this sweep gets FirstSeen=now, so it can't be idle yet.
	archived, _, err := store.SweepIdle(t.Context(), sweepBase, sweepArchive)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 0 {
		t.Fatalf("archived a skill within its grace floor: %v", archived)
	}
}

func TestSweepIdleRetainsLatestUseAfterDelayedObservation(t *testing.T) {
	store := newStore(t, t.TempDir(), skills.ScopeUser)
	installActiveAgentSkill(t, store, "agent-skill")
	latest := sweepBase.Add(20 * 24 * time.Hour)
	for _, observed := range []time.Time{sweepBase, latest, sweepBase.Add(time.Hour)} {
		if err := store.RecordUse(t.Context(), "agent-skill", observed); err != nil {
			t.Fatal(err)
		}
	}
	archived, _, err := store.SweepIdle(t.Context(), sweepBase.Add(sweepArchive+time.Hour), sweepArchive)
	if err != nil || len(archived) != 0 {
		t.Fatalf("delayed observation erased recent activity: archived=%v, err=%v", archived, err)
	}
	archived, _, err = store.SweepIdle(t.Context(), latest.Add(sweepArchive), sweepArchive)
	if err != nil || len(archived) != 1 || archived[0] != "agent-skill" {
		t.Fatalf("skill did not become idle after its latest use: archived=%v, err=%v", archived, err)
	}
}

func TestSweepIdleRestoredSkillGetsFreshGrace(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root, skills.ScopeUser)
	installActiveAgentSkill(t, store, "agent-skill")
	if _, _, err := store.SweepIdle(t.Context(), sweepBase, sweepArchive); err != nil {
		t.Fatal(err)
	}
	later := sweepBase.Add(sweepArchive + time.Hour)
	if _, _, err := store.SweepIdle(t.Context(), later, sweepArchive); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Restore(t.Context(), "agent-skill"); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	// An immediate re-sweep at the same instant must NOT re-archive the just-restored
	// skill: archiving dropped its usage record, so it starts a fresh grace floor.
	archived, _, err := store.SweepIdle(t.Context(), later, sweepArchive)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 0 {
		t.Fatalf("re-archived a just-restored skill: %v", archived)
	}
	if _, err := os.Stat(filepath.Join(root, "agent-skill", "SKILL.md")); err != nil {
		t.Fatalf("restored skill should be active: %v", err)
	}
}

func TestManualArchiveThenRestoreGetsFreshGrace(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root, skills.ScopeUser)
	installActiveAgentSkill(t, store, "agent-skill")
	// Seed a usage record with an old activity time.
	if _, _, err := store.SweepIdle(t.Context(), sweepBase, sweepArchive); err != nil {
		t.Fatal(err)
	}
	// A human archives then restores it much later. Archiving drops the usage
	// record, so the restored skill starts a fresh grace floor.
	if _, err := store.Archive(t.Context(), "agent-skill"); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if _, err := store.Restore(t.Context(), "agent-skill"); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	later := sweepBase.Add(sweepArchive + time.Hour)
	archived, _, err := store.SweepIdle(t.Context(), later, sweepArchive)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 0 {
		t.Fatalf("re-archived a manually archived-then-restored skill: %v", archived)
	}
	if _, err := os.Stat(filepath.Join(root, "agent-skill", "SKILL.md")); err != nil {
		t.Fatalf("restored skill should be active: %v", err)
	}
}

func TestSweepIdleRejectsOverCapacityManagedLibrary(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root, skills.ScopeUser)
	for index := range skills.MaxSkillsPerSource + 1 {
		writeActiveSkillFixture(t, root, fmt.Sprintf("skill-%03d", index))
	}

	if _, _, err := store.SweepIdle(t.Context(), sweepBase, sweepArchive); !errors.Is(err, skills.ErrLibraryCapacity) {
		t.Fatalf("SweepIdle error = %v, want ErrLibraryCapacity beyond %d active Skills", err, skills.MaxSkillsPerSource)
	}
}

func TestSweepIdlePreservesSkillsWithInvalidDocumentIdentity(t *testing.T) {
	for _, invalid := range []string{"other-skill", "ａｇｅｎｔ-skill"} {
		t.Run(invalid, func(t *testing.T) {
			root := t.TempDir()
			store := newStore(t, root, skills.ScopeUser)
			installActiveAgentSkill(t, store, "agent-skill")
			if err := store.RecordUse(t.Context(), "agent-skill", sweepBase); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "agent-skill", "SKILL.md")
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			content = []byte(strings.Replace(string(content), "name: agent-skill", "name: "+invalid, 1))
			if err := os.WriteFile(path, content, 0o644); err != nil {
				t.Fatal(err)
			}
			archived, changed, err := store.SweepIdle(t.Context(), sweepBase.Add(sweepArchive+time.Hour), sweepArchive)
			if err != nil || len(archived) != 0 || len(changed) != 0 {
				t.Fatalf("invalid document was curated: archived=%v, changed=%v, err=%v", archived, changed, err)
			}
			if remaining, err := os.ReadFile(path); err != nil || string(remaining) != string(content) {
				t.Fatalf("invalid document changed: %q, %v", remaining, err)
			}
		})
	}
}

func TestRestoreReplayPreservesRecordedUse(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root, skills.ScopeUser)
	installActiveAgentSkill(t, store, "agent-skill")
	if _, err := store.Archive(t.Context(), "agent-skill"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Restore(t.Context(), "agent-skill"); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordUse(t.Context(), "agent-skill", sweepBase); err != nil {
		t.Fatal(err)
	}
	changed, err := store.Restore(t.Context(), "agent-skill")
	if err != nil || len(changed) != 0 {
		t.Fatalf("restore replay = %v, %v", changed, err)
	}
	archived, _, err := store.SweepIdle(t.Context(), sweepBase.Add(sweepArchive+time.Hour), sweepArchive)
	if err != nil || len(archived) != 1 || archived[0] != "agent-skill" {
		t.Fatalf("restore replay erased the recorded activity: archived=%v, err=%v", archived, err)
	}
}

func TestFailedRestorePreservesRecordedUse(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root, skills.ScopeUser)
	if err := store.RecordUse(t.Context(), "missing-skill", sweepBase); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".usage.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Restore(t.Context(), "missing-skill"); !errors.Is(err, skills.ErrNotFound) {
		t.Fatalf("restore missing skill = %v, want not found", err)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != string(before) {
		t.Fatalf("failed restore changed recorded activity: %q, %v", after, err)
	}
}

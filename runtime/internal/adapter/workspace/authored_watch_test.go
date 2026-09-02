package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
)

func TestAuthoredWatcherMapsGlobalAndWorkspaceCascades(t *testing.T) {
	home := t.TempDir()
	knowledgeHome := t.TempDir()
	skillsHome := t.TempDir()
	project := t.TempDir()
	workspace := filepath.Join(project, "packages", "desktop")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	watcher, err := NewAuthoredWatcher(knowledgeHome, home, skillsHome)
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan workspaceapp.AuthoredResource, 8)
	closer, err := watcher.Watch(
		[]workspaceapp.AuthoredScope{{Workspace: workspace, ProjectRoot: project}},
		[]workspaceapp.AuthoredResource{workspaceapp.AuthoredKnowledge, workspaceapp.AuthoredHooks, workspaceapp.AuthoredSkills},
		func(resource workspaceapp.AuthoredResource) { events <- resource },
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closer.Close() }()

	for _, change := range []struct {
		path     string
		resource workspaceapp.AuthoredResource
	}{
		{filepath.Join(knowledgeHome, "FLAME.md"), workspaceapp.AuthoredKnowledge},
		{filepath.Join(project, "FLAME.md"), workspaceapp.AuthoredKnowledge},
		{filepath.Join(workspace, "FLAME.md"), workspaceapp.AuthoredKnowledge},
		{filepath.Join(home, ".flame", "hooks.json"), workspaceapp.AuthoredHooks},
		{filepath.Join(project, ".flame", "hooks.json"), workspaceapp.AuthoredHooks},
		{filepath.Join(workspace, ".flame", "hooks.json"), workspaceapp.AuthoredHooks},
		{filepath.Join(skillsHome, "global-skill", "SKILL.md"), workspaceapp.AuthoredSkills},
		{filepath.Join(workspace, ".flame", "skills", "project-skill", "SKILL.md"), workspaceapp.AuthoredSkills},
	} {
		if err := os.MkdirAll(filepath.Dir(change.path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(change.path, []byte(time.Now().String()), 0o644); err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-events:
			if got != change.resource {
				t.Fatalf("change %s produced %v, want %v", change.path, got, change.resource)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("no resource event for %s", change.path)
		}
	}
}

func TestAuthoredWatcherScopesSkillsToSelectedWorkspace(t *testing.T) {
	project := t.TempDir()
	workspace := filepath.Join(project, "packages", "desktop")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	watcher, err := NewAuthoredWatcher(t.TempDir(), t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan workspaceapp.AuthoredResource, 2)
	observation, err := watcher.Watch(
		[]workspaceapp.AuthoredScope{{Workspace: workspace, ProjectRoot: project}},
		[]workspaceapp.AuthoredResource{workspaceapp.AuthoredSkills},
		func(resource workspaceapp.AuthoredResource) { events <- resource },
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = observation.Close() }()

	projectOnly := filepath.Join(project, ".flame", "skills", "project-only", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(projectOnly), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectOnly, []byte("outside selected workspace"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case resource := <-events:
		t.Fatalf("project-root-only Skill published %v", resource)
	case <-time.After(300 * time.Millisecond):
	}

	visible := filepath.Join(workspace, ".flame", "skills", "visible", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(visible), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(visible, []byte("inside selected workspace"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case resource := <-events:
		if resource != workspaceapp.AuthoredSkills {
			t.Fatalf("workspace Skill published %v", resource)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("workspace Skill change was not observed")
	}
}

func TestDirectoriesRootToLeafRejectsWorkspaceOutsideProject(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if _, err := directoriesRootToLeaf(root, outside); err == nil {
		t.Fatal("outside workspace was accepted")
	}
}

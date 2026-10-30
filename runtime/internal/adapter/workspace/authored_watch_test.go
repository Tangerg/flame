package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
)

func TestAuthoredWatcherTracksSkillDocumentSymlinkContent(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "review")
	materials := filepath.Join(bundle, "materials")
	if err := os.MkdirAll(materials, 0o755); err != nil {
		t.Fatal(err)
	}
	first, second := filepath.Join(materials, "first.md"), filepath.Join(materials, "second.md")
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, fmt.Appendf(nil, "---\nname: review\ndescription: %s\n---\n%s", body, body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(first, "before")
	write(second, "retargeted")
	link := filepath.Join(bundle, "SKILL.md")
	if err := os.Symlink(filepath.Join("materials", "first.md"), link); err != nil {
		t.Fatal(err)
	}
	source, err := promptsource.NewSkillSource(root, root)
	if err != nil {
		t.Fatal(err)
	}
	assertContent := func(body string) {
		t.Helper()
		listed, err := source.List(t.Context())
		if err != nil || len(listed) != 1 || listed[0].Name != "review" || listed[0].Description != body {
			t.Fatalf("catalog = %+v, %v; want %q", listed, err, body)
		}
		loaded, err := source.Load(t.Context(), "review")
		if err != nil || loaded.Instructions != body {
			t.Fatalf("loaded = %+v, %v; want %q", loaded, err, body)
		}
	}
	assertContent("before")
	watcher, err := NewAuthoredWatcher(t.TempDir(), root)
	if err != nil {
		t.Fatal(err)
	}
	events, reports := make(chan workspaceapp.AuthoredResource, 16), make(chan error, 4)
	observation, err := watcher.Watch(nil, []workspaceapp.AuthoredResource{workspaceapp.AuthoredSkills}, func(resource workspaceapp.AuthoredResource) {
		events <- resource
	}, func(err error) { reports <- err })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := observation.Close(); err != nil {
			t.Error(err)
		}
	})
	assertQuiet := func() {
		t.Helper()
		select {
		case resource := <-events:
			t.Fatalf("unchanged Skill document published %v", resource)
		case err := <-reports:
			t.Fatal(err)
		case <-time.After(200 * time.Millisecond):
		}
	}
	assertChanged := func() {
		t.Helper()
		select {
		case resource := <-events:
			if resource != workspaceapp.AuthoredSkills {
				t.Fatalf("published %v, want skills", resource)
			}
		case err := <-reports:
			t.Fatal(err)
		case <-time.After(3 * time.Second):
			t.Fatal("Skill source changed without a skills.changed invalidation")
		}
		assertQuiet()
	}
	write(first, "inplace")
	assertContent("inplace")
	assertChanged()
	staged := filepath.Join(materials, "staged.md")
	write(staged, "atomic")
	if err := os.Rename(staged, first); err != nil {
		t.Fatal(err)
	}
	assertContent("atomic")
	assertChanged()
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("materials", "second.md"), link); err != nil {
		t.Fatal(err)
	}
	assertContent("retargeted")
	assertChanged()
	write(first, "retired target")
	assertQuiet()
	if err := os.Remove(second); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Load(t.Context(), "review"); err == nil {
		t.Fatal("deleted document remained readable")
	}
	assertChanged()
	write(second, "recreated")
	assertContent("recreated")
	assertChanged()
	resources := filepath.Join(bundle, "resources")
	if err := os.Mkdir(resources, 0o755); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(resources, "unrelated.md"), "unrelated")
	assertQuiet()
}

func TestAuthoredWatcherReportsOutagesAndRecovers(t *testing.T) {
	for _, test := range []struct {
		name     string
		resource workspaceapp.AuthoredResource
		file     string
	}{
		{name: "hooks", resource: workspaceapp.AuthoredHooks, file: "hooks.json"},
		{name: "skills", resource: workspaceapp.AuthoredSkills, file: filepath.Join("skills", "lint", "SKILL.md")},
	} {
		t.Run(test.name, func(t *testing.T) {
			project := t.TempDir()
			reports := make(chan error, 8)
			watcher, err := NewAuthoredWatcher(t.TempDir(), "")
			if err != nil {
				t.Fatal(err)
			}
			events := make(chan workspaceapp.AuthoredResource, 8)
			observation, err := watcher.Watch(
				[]workspaceapp.AuthoredScope{{Workspace: project, ProjectRoot: project}},
				[]workspaceapp.AuthoredResource{test.resource},
				func(resource workspaceapp.AuthoredResource) { events <- resource },
				func(err error) { reports <- err },
			)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = observation.Close() }()

			parent := filepath.Join(project, ".flame")
			for outage := range 2 {
				if err := os.RemoveAll(parent); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(parent, []byte("parent is not a directory"), 0o644); err != nil {
					t.Fatal(err)
				}
				select {
				case err := <-reports:
					var pathError *os.PathError
					if !errors.As(err, &pathError) {
						t.Fatalf("outage %d lost filesystem cause: %v", outage, err)
					}
				case <-time.After(3 * time.Second):
					t.Fatalf("outage %d was not reported", outage)
				}
				// Repeated background retries must not flood the diagnostic sink.
				select {
				case err := <-reports:
					t.Fatalf("outage %d was reported again: %v", outage, err)
				case resource := <-events:
					t.Fatalf("failed observation published %v", resource)
				case <-time.After(1100 * time.Millisecond):
				}
				if err := os.Remove(parent); err != nil {
					t.Fatal(err)
				}
				file := filepath.Join(parent, test.file)
				if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, fmt.Appendf(nil, "recovered after outage %d", outage), 0o644); err != nil {
					t.Fatal(err)
				}
				select {
				case resource := <-events:
					if resource != test.resource {
						t.Fatalf("recovery published %v, want %v", resource, test.resource)
					}
				case <-time.After(3 * time.Second):
					t.Fatalf("outage %d did not recover", outage)
				}
			}
			if err := os.WriteFile(filepath.Join(parent, test.file), []byte("later external edit"), 0o644); err != nil {
				t.Fatal(err)
			}
			select {
			case resource := <-events:
				if resource != test.resource {
					t.Fatalf("later edit published %v, want %v", resource, test.resource)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("recovered observer lost subsequent changes")
			}
		})
	}
}

func TestAuthoredWatcherMapsGlobalAndWorkspaceCascades(t *testing.T) {
	home := t.TempDir()
	skillsHome := t.TempDir()
	project := t.TempDir()
	workspace := filepath.Join(project, "packages", "desktop")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	watcher, err := NewAuthoredWatcher(home, skillsHome)
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan workspaceapp.AuthoredResource, 8)
	closer, err := watcher.Watch(
		[]workspaceapp.AuthoredScope{{Workspace: workspace, ProjectRoot: project}},
		[]workspaceapp.AuthoredResource{workspaceapp.AuthoredHooks, workspaceapp.AuthoredSkills},
		func(resource workspaceapp.AuthoredResource) { events <- resource },
		func(err error) { t.Errorf("observation outage: %v", err) },
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closer.Close() }()

	for _, change := range []struct {
		path     string
		resource workspaceapp.AuthoredResource
	}{
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
	watcher, err := NewAuthoredWatcher(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan workspaceapp.AuthoredResource, 2)
	observation, err := watcher.Watch(
		[]workspaceapp.AuthoredScope{{Workspace: workspace, ProjectRoot: project}},
		[]workspaceapp.AuthoredResource{workspaceapp.AuthoredSkills},
		func(resource workspaceapp.AuthoredResource) { events <- resource },
		func(err error) { t.Errorf("observation outage: %v", err) },
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

package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
)

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
			previousLogger := slog.Default()
			slog.SetDefault(slog.New(observationDiagnostics{Handler: slog.NewTextHandler(io.Discard, nil), failures: reports}))
			t.Cleanup(func() { slog.SetDefault(previousLogger) })
			watcher, err := NewAuthoredWatcher(t.TempDir(), t.TempDir(), "")
			if err != nil {
				t.Fatal(err)
			}
			events := make(chan workspaceapp.AuthoredResource, 8)
			observation, err := watcher.Watch(
				[]workspaceapp.AuthoredScope{{Workspace: project, ProjectRoot: project}},
				[]workspaceapp.AuthoredResource{test.resource},
				func(resource workspaceapp.AuthoredResource) { events <- resource },
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

type observationDiagnostics struct {
	slog.Handler
	failures chan error
}

func (d observationDiagnostics) Handle(_ context.Context, record slog.Record) error {
	record.Attrs(func(attr slog.Attr) bool {
		if err, ok := attr.Value.Any().(error); attr.Key == "error" && ok {
			d.failures <- err
		}
		return true
	})
	return nil
}

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

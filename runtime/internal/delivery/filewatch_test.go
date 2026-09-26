package delivery

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/run/segment"
	workspaceadapter "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skillauthoring"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestWorkspaceSubscribe_ExternalContentTargets(t *testing.T) {
	for _, repository := range []bool{false, true} {
		t.Run(fmt.Sprintf("git=%t", repository), func(t *testing.T) {
			root := t.TempDir()
			canonicalRoot := canonicalWorkspacePath(t, root)
			if repository {
				fileWatchGitCommand(t, root, "init", "-q")
			}
			path := filepath.Join(root, "source.txt")
			if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
				t.Fatal(err)
			}
			if repository {
				fileWatchGitCommand(t, root, "add", "source.txt")
			}
			s := newWorkspaceHandler(root)
			s.workspaceHub = newWorkspaceHub()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			_, stream, err := s.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
				Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged},
				Watches: []protocol.WatchSpec{{WatchID: "content", Workspace: protocol.WorkspaceRef{Path: root}, Paths: []string{"source.txt", "."}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			events := drainSeq(ctx, stream)
			check := func(path string) {
				t.Helper()
				deadline := time.After(3 * time.Second)
				for {
					select {
					case event := <-events:
						if event.Type != protocol.RuntimeFilesChanged || event.WatchID != "content" || event.Workspace == nil || event.Workspace.Path != canonicalRoot {
							t.Fatalf("unexpected scoped event: %+v", event)
						}
						if slices.Contains(event.Paths, path) {
							return
						}
					case <-deadline:
						t.Fatalf("no external invalidation for %q", path)
					}
				}
			}
			if err := os.WriteFile(path, []byte("external write without git"), 0o600); err != nil {
				t.Fatal(err)
			}
			check("source.txt")
			if err := os.WriteFile(filepath.Join(root, "atomic.tmp"), []byte("atomic save"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(filepath.Join(root, "atomic.tmp"), path); err != nil {
				t.Fatal(err)
			}
			check("source.txt")
			if err := os.Rename(path, filepath.Join(root, "renamed.txt")); err != nil {
				t.Fatal(err)
			}
			check("source.txt")
			if err := os.WriteFile(path, []byte("recreated"), 0o600); err != nil {
				t.Fatal(err)
			}
			check("source.txt")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			check("source.txt")
			if err := os.WriteFile(filepath.Join(root, "new-child.txt"), []byte("child"), 0o600); err != nil {
				t.Fatal(err)
			}
			check(".")
		})
	}
}

func TestWorkspaceSubscribe_ContentScopeIsolationAndDirectoryRecreation(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	canonicalSecond := canonicalWorkspacePath(t, second)
	for _, root := range []string{first, second} {
		if err := os.Mkdir(filepath.Join(root, "src"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	s := newWorkspaceHandler(first)
	s.workspaceHub = newWorkspaceHub()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, stream, err := s.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicFilesChanged},
		Watches: []protocol.WatchSpec{
			{WatchID: "first", Workspace: protocol.WorkspaceRef{Path: first}, Paths: []string{"src"}},
			{WatchID: "second", Workspace: protocol.WorkspaceRef{Path: second}, Paths: []string{"src"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := drainSeq(ctx, stream)
	check := func() {
		t.Helper()
		select {
		case event := <-events:
			if event.WatchID != "second" || event.Workspace == nil || event.Workspace.Path != canonicalSecond || !slices.Contains(event.Paths, "src") {
				t.Fatalf("event crossed workspace scope: %+v", event)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("no directory event")
		}
	}
	path := filepath.Join(second, "src", "same.txt")
	if err := os.WriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	check()
	if err := os.WriteFile(path, []byte("longer direct entry"), 0o600); err != nil {
		t.Fatal(err)
	}
	check()
	if err := os.RemoveAll(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	check()
	if err := os.Mkdir(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	check()
	if err := os.WriteFile(path, []byte("after recreation"), 0o600); err != nil {
		t.Fatal(err)
	}
	check()
}

func TestWorkspaceSubscribe_ContentObservationFailureEndsStream(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "src")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	s := newWorkspaceHandler(root)
	s.workspaceHub = newWorkspaceHub()
	_, stream, err := s.SubscribeRuntime(t.Context(), protocol.RuntimeSubscribeRequest{
		Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged},
		Watches: []protocol.WatchSpec{{WatchID: "open-file", Workspace: protocol.WorkspaceRef{Path: root}, Paths: []string{"src/file.txt"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ended := make(chan error, 1)
	go func() {
		for _, err := range stream {
			if err != nil {
				ended <- err
				return
			}
		}
		ended <- nil
	}()
	if err := os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(parent, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-ended:
		if err == nil {
			t.Fatal("observation failure became a clean end")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("unavailable observation left a silently live subscription")
	}
}

func TestWorkspaceSubscribe_ContentRegistrationRejectsOutsideAlias(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	s := newWorkspaceHandler(root)
	s.workspaceHub = newWorkspaceHub()
	ack, stream, err := s.SubscribeRuntime(t.Context(), protocol.RuntimeSubscribeRequest{
		Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged},
		Watches: []protocol.WatchSpec{{WatchID: "outside", Workspace: protocol.WorkspaceRef{Path: root}, Paths: []string{"alias/file.txt"}}},
	})
	if err == nil || ack != nil || stream != nil {
		t.Fatalf("outside alias registered: (%v, %v)", ack, err)
	}
}

func TestWorkspaceSubscribe_RebuildsReplacedWorkspaceRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "src", "file.txt")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := newWorkspaceHandler(root)
	s.workspaceHub = newWorkspaceHub()
	request := protocol.RuntimeSubscribeRequest{
		Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged},
		Watches: []protocol.WatchSpec{{WatchID: "file", Workspace: protocol.WorkspaceRef{Path: root}, Paths: []string{"src/file.txt"}}},
	}
	_, stream, err := s.SubscribeRuntime(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	ended := make(chan error, 1)
	go func() {
		for _, err := range stream {
			if err != nil {
				ended <- err
				return
			}
		}
		ended <- nil
	}()
	if err := os.Rename(root, root+"-retired"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replaced root"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-ended:
		if err == nil {
			t.Fatal("lost root identity was not reported")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("retired root remained silently observed")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, stream, err = s.SubscribeRuntime(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	events := drainSeq(ctx, stream)
	if err := os.WriteFile(path, []byte("edited new root"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertRuntimeEventType(t, events, protocol.RuntimeFilesChanged)
}

// TestWorkspaceSubscribe_GitWatch verifies that a real staged index transition
// surfaces a debounced resync without recursively watching the working tree.
func TestWorkspaceSubscribe_GitWatch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	fileWatchGitCommand(t, dir, "init", "-q")
	tracked := filepath.Join(dir, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("v0\n"), 0o644); err != nil {
		t.Fatalf("seed index: %v", err)
	}
	fileWatchGitCommand(t, dir, "add", "tracked.txt")
	s := newWorkspaceHandler(dir)
	s.workspaceHub = newWorkspaceHub()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, seq, err := s.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged},
		Watches: []protocol.WatchSpec{{WatchID: "w1", Workspace: protocol.WorkspaceRef{Path: dir}}},
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	events := drainSeq(ctx, seq)

	// A git operation semantically changes the staged index → expect a debounced resync.
	if err := os.WriteFile(tracked, []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("modify tracked file: %v", err)
	}
	fileWatchGitCommand(t, dir, "add", "tracked.txt")
	select {
	case ev := <-events:
		if ev.Type != "resync" {
			t.Fatalf("event = %+v, want resync", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no resync within 3s of a staged index change")
	}
}

// TestWorkspaceSubscribe_NonRepoInert: a watch on a cwd that isn't a git repo
// contributes no watcher (and doesn't error) — the broadcast stream still works.
func TestWorkspaceSubscribe_NonRepoInert(t *testing.T) {
	dir := t.TempDir() // no .git
	s := newWorkspaceHandler(dir)
	s.workspaceHub = newWorkspaceHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, seq, err := s.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged, protocol.TopicSkillsChanged},
		Watches: []protocol.WatchSpec{{WatchID: "w1", Workspace: protocol.WorkspaceRef{Path: dir}}},
	})
	if err != nil {
		t.Fatalf("subscribe (non-repo) must not error: %v", err)
	}
	events := drainSeq(ctx, seq)
	// Broadcast events still flow on the subscription.
	s.workspaceHub.publish(protocol.RuntimeEvent{Type: "skills.changed"})
	select {
	case ev := <-events:
		if ev.Type != "skills.changed" {
			t.Fatalf("event = %+v, want skills.changed", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("broadcast event not delivered on a non-repo subscription")
	}
}

// TestWorkspaceSubscribe_ExternalAuthoredFiles verifies that the file-backed,
// user-authored resources converge when another process edits them. Git
// observation is deliberately irrelevant here: neither hooks.json nor a SKILL.md
// needs to be staged before its query projection becomes stale.
func TestWorkspaceSubscribe_ExternalAuthoredFiles(t *testing.T) {
	dir := t.TempDir()
	s := newWorkspaceHandler(dir)
	s.workspaceHub = newWorkspaceHub()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, seq, err := s.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{
			protocol.TopicFilesChanged,
			protocol.TopicHooksChanged,
			protocol.TopicSkillsChanged,
		},
		Watches: []protocol.WatchSpec{{WatchID: "authored", Workspace: protocol.WorkspaceRef{Path: dir}}},
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	events := drainSeq(ctx, seq)

	if err := os.MkdirAll(filepath.Join(dir, ".flame"), 0o755); err != nil {
		t.Fatalf("create hook directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".flame", "hooks.json"), []byte(`{"hooks":[]}`), 0o644); err != nil {
		t.Fatalf("write hooks: %v", err)
	}
	assertRuntimeEventType(t, events, protocol.RuntimeHooksChanged)

	skillPath := filepath.Join(dir, ".flame", "skills", "external-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatalf("create skill directory: %v", err)
	}
	if err := os.WriteFile(skillPath, []byte("external skill"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	assertRuntimeEventType(t, events, protocol.RuntimeSkillsChanged)
}

func TestWorkspaceSubscribe_GlobalAuthoredFilesDoNotRequireWorkspaceWatch(t *testing.T) {
	workspaceRoot := t.TempDir()
	hooksHome := t.TempDir()
	skillsHome := t.TempDir()
	authored, err := workspaceadapter.NewAuthoredWatcher(hooksHome, skillsHome)
	if err != nil {
		t.Fatal(err)
	}
	s := newWorkspaceHandlerWithConfig(workspaceRoot, workspaceTestConfig{AuthoredWatcher: authored})
	s.workspaceHub = newWorkspaceHub()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, seq, err := s.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{Topics: []protocol.RuntimeTopic{
		protocol.TopicHooksChanged, protocol.TopicSkillsChanged,
	}})
	if err != nil {
		t.Fatal(err)
	}
	events := drainSeq(ctx, seq)
	if err := os.MkdirAll(filepath.Join(hooksHome, ".flame"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooksHome, ".flame", "hooks.json"), []byte(`{"hooks":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	assertRuntimeEventType(t, events, protocol.RuntimeHooksChanged)
	globalSkill := filepath.Join(skillsHome, "global-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(globalSkill), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(globalSkill, []byte("global skill"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertRuntimeEventType(t, events, protocol.RuntimeSkillsChanged)

	if err := os.WriteFile(filepath.Join(workspaceRoot, "AGENTS.md"), []byte("unwatched workspace"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-events:
		t.Fatalf("workspace path without a watch produced %+v", event)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestWorkspaceSubscribe_SkillArchiveDoesNotDoublePublishFromTreeObservation(t *testing.T) {
	workspaceRoot := t.TempDir()
	skillsHome := t.TempDir()
	skillPath := filepath.Join(skillsHome, "lint", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte("---\nname: lint\ndescription: Run the project linter before completing a change.\n---\nRun the linter.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	curator, err := skillauthoring.NewStore(skillsHome, skills.ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	authored, err := workspaceadapter.NewAuthoredWatcher(t.TempDir(), skillsHome)
	if err != nil {
		t.Fatal(err)
	}
	surfaces := newWorkspaceSurfaces(workspaceRoot, workspaceTestConfig{
		Curator: curator, AuthoredWatcher: authored,
	})
	s := &Handler{}
	applyWorkspaceSurfaces(s, surfaces)
	s.workspaceHub = newWorkspaceHub()
	s.workspaceSkills, err = workspaceapp.NewSkills(
		surfaces.roots, fakeSkillCatalog{}, curator, &stubSkillProposals{}, surfaces.authoredWatch,
		func(notice invalidation.Notice) {
			if event, ok := runtimeEventFor(notice); ok {
				s.workspaceHub.publish(event)
			}
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, seq, err := s.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicSkillsChanged},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := drainSeq(ctx, seq)
	if err := s.ArchiveSkill(ctx, protocol.SkillNameRequest{Name: "lint"}); err != nil {
		t.Fatal(err)
	}
	assertRuntimeEventType(t, events, protocol.RuntimeSkillsChanged)
	select {
	case event := <-events:
		t.Fatalf("skills.library.archive published a duplicate observation event: %+v", event)
	case <-time.After(500 * time.Millisecond):
	}
}

func assertRuntimeEventType(t *testing.T, events <-chan protocol.RuntimeEvent, want protocol.RuntimeEventType) {
	t.Helper()
	select {
	case event := <-events:
		if event.Type != want {
			t.Fatalf("event = %+v, want %s", event, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("no %s event within 3s of an external file change", want)
	}
}

func fileWatchGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

// TestRunEffectsNudgePublishesFileChange verifies the application nudge adapter
// reaches the workspace event hub. Tool-item-to-nudge decisions belong to and
// are tested in application/agent/runs.
func TestRunEffectsNudgePublishesFileChange(t *testing.T) {
	s := &Handler{workspaceHub: newWorkspaceHub()}
	events, unsub := s.workspaceHub.subscribe()
	defer unsub()

	// Wire the production seam: the run effects publish nudges through the
	// notifier, and the hub observes it (mapping to the wire files.changed).
	fc := &testNotification[workspaceapp.FileChangeNotice]{}
	s.observeFileChanges(fc.Observe)
	effects := segment.NewWorkspaceNotifier(fc.Publish)

	effects.Nudge("/proj", []string{"src/a.go"})
	select {
	case ev := <-events:
		if ev.Type != "files.changed" || ev.Workspace == nil || ev.Workspace.Path != "/proj" || len(ev.Paths) != 1 || ev.Paths[0] != "src/a.go" {
			t.Fatalf("event = %+v, want files.changed cwd=/proj [src/a.go]", ev)
		}
	default:
		t.Fatal("write tool call must publish files.changed")
	}

}

// TestWorkspaceSubscribe_MissingWatchID rejects a watch with no id.
func TestWorkspaceSubscribe_MissingWatchID(t *testing.T) {
	s := newWorkspaceHandler(t.TempDir())
	s.workspaceHub = newWorkspaceHub()
	if _, _, err := s.SubscribeRuntime(context.Background(), protocol.RuntimeSubscribeRequest{
		Watches: []protocol.WatchSpec{{}},
	}); err == nil {
		t.Fatal("watch missing watchId must be invalid_params")
	}
}

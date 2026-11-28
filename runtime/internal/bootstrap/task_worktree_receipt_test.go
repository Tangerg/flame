package bootstrap

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
)

type taskGitBoundary string

const (
	beforeCheckoutCreation taskGitBoundary = "before-create"
	afterCheckoutCreation  taskGitBoundary = "after-create"
	beforeHandoffPreview   taskGitBoundary = "before-preview"
	afterHandoffApply      taskGitBoundary = "after-apply"
	beforeCheckoutRemoval  taskGitBoundary = "before-removal"
	afterCheckoutRemoval   taskGitBoundary = "after-removal"
)

func TestTaskWorktreeReceiptsSurviveRequestCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Git timing fixture uses a POSIX shell")
	}
	for _, boundary := range []taskGitBoundary{beforeHandoffPreview, afterHandoffApply, beforeCheckoutRemoval, afterCheckoutRemoval} {
		t.Run(string(boundary), func(t *testing.T) {
			source, git := taskWorktreeTestRepository(t)
			const name = "material.txt"
			const original, proposed = "original\n", "handed off \n"
			if err := os.WriteFile(filepath.Join(source, name), []byte(original), 0o600); err != nil {
				t.Fatal(err)
			}
			git("add", name)
			git("commit", "--quiet", "-m", "initial checkout")
			git("config", "apply.whitespace", "fix")
			requireStaged := func(want string) {
				t.Helper()
				body, err := exec.CommandContext(t.Context(), "git", "-C", source, "show", ":"+name).CombinedOutput()
				if err != nil || string(body) != want {
					t.Fatalf("staged material = %q, %v; want %q", body, err, want)
				}
			}
			cfg := poMCPConfig(t)
			cfg.defaultWorkspacePath = source
			r := poMCPOpen(t, cfg)
			task := r.must(delivery.TaskWorktreesCreate, protocol.CreateTaskWorktreeRequest{
				Source: protocol.WorkspaceRef{Path: source}, Base: "target",
			}, "receipt-task").(*protocol.TaskWorktree)

			var method delivery.Name
			var request any
			var preview *protocol.WorktreeHandoff
			releasing := boundary == beforeCheckoutRemoval || boundary == afterCheckoutRemoval
			if releasing {
				method, request = delivery.TaskWorktreesRelease, protocol.TaskWorktreeRequest{ID: task.ID}
			} else {
				if err := os.WriteFile(filepath.Join(task.Workspace.Path, name), []byte(proposed), 0o600); err != nil {
					t.Fatal(err)
				}
				preview = r.must(delivery.TaskWorktreesPreview, protocol.TaskWorktreeRequest{ID: task.ID}, "").(*protocol.WorktreeHandoff)
				method, request = delivery.TaskWorktreesApply, protocol.ApplyWorktreeHandoffRequest{ID: task.ID, Digest: preview.Digest}
			}

			entered, proceed := holdTaskGitBoundary(t, boundary)
			ctx, cancel := context.WithTimeout(protocolLifecycleContext(t.Context()), lifecycleWaitBudget(t))
			var caller sync.WaitGroup
			defer func() {
				cancel()
				if err := os.WriteFile(proceed, nil, 0o600); err != nil {
					t.Error(err)
				}
				caller.Wait()
			}()
			first := make(chan protocol.OperationResult, 1)
			options := protocol.CallMetadata{IdempotencyKey: "cancelled-git-command"}
			caller.Go(func() { first <- r.endpoint.Invoke(ctx, method, request, options) })
			awaitTaskGitBoundary(t, ctx, entered, first)
			if boundary == afterCheckoutRemoval {
				if _, err := os.Stat(task.Workspace.Path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("Git had not removed the checkout at cancellation: %v", err)
				}
			} else if boundary == afterHandoffApply {
				requireStaged(proposed)
			}
			cancel()
			if err := os.WriteFile(proceed, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			caller.Wait()
			initial := <-first
			switch boundary {
			case beforeHandoffPreview:
				requireStaged(original)
			case beforeCheckoutRemoval:
				if _, err := os.Stat(task.Workspace.Path); err != nil {
					t.Fatalf("release changed the checkout during cancelled preflight: %v", err)
				}
			}
			if boundary == beforeHandoffPreview || boundary == beforeCheckoutRemoval {
				if initial.Failure == nil {
					t.Fatalf("cancelled preflight returned a success: %+v", initial.Value)
				}
			} else if initial.Failure != nil {
				t.Fatalf("the accepted operation did not settle: %v", initial.Failure)
			}

			r = restartTaskWorktreeRuntime(t, r, cfg)

			retry, stop := context.WithTimeout(protocolLifecycleContext(t.Context()), lifecycleWaitBudget(t))
			defer stop()
			result := r.endpoint.Invoke(retry, method, request, options)
			if result.Failure != nil {
				t.Fatalf("retry with the original key after %s = %v", boundary, result.Failure)
			}
			if initial.Failure == nil && !reflect.DeepEqual(result.Value, initial.Value) {
				t.Fatalf("restart changed the accepted receipt: %+v; want %+v", result.Value, initial.Value)
			}
			if releasing {
				tasks := r.must(delivery.TaskWorktreesList, struct{}{}, "").(*protocol.Page[protocol.TaskWorktree])
				if len(tasks.Data) != 0 {
					t.Fatalf("release receipt left registered worktrees: %+v", tasks.Data)
				}
				return
			}
			handoff := result.Value.(*protocol.WorktreeHandoff)
			if handoff.Digest != preview.Digest || handoff.Patch != preview.Patch {
				t.Fatalf("retry returned a different handoff: %+v, want %+v", handoff, preview)
			}
			requireStaged(proposed)
			if changed := git("diff", "--name-only"); changed != "" {
				t.Fatalf("handoff left unstaged material: %q", changed)
			}
		})
	}
}

func TestTaskWorktreeCreationSettlesAcrossRequestCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Git timing fixture uses a POSIX shell")
	}
	for _, boundary := range []taskGitBoundary{beforeCheckoutCreation, afterCheckoutCreation} {
		t.Run(string(boundary), func(t *testing.T) {
			source, git := taskWorktreeTestRepository(t)
			git("commit", "--quiet", "--allow-empty", "-m", "initial checkout")
			cfg := poMCPConfig(t)
			cfg.defaultWorkspacePath = source
			r := poMCPOpen(t, cfg)
			registered := func() []string {
				var paths []string
				for line := range strings.SplitSeq(git("worktree", "list", "--porcelain"), "\n") {
					if path, ok := strings.CutPrefix(line, "worktree "); ok {
						paths = append(paths, path)
					}
				}
				return paths
			}
			request := protocol.CreateTaskWorktreeRequest{Source: protocol.WorkspaceRef{Path: source}, Base: "target"}
			options := protocol.CallMetadata{IdempotencyKey: "cancelled-worktree-creation"}
			entered, proceed := holdTaskGitBoundary(t, boundary)
			ctx, cancel := context.WithTimeout(protocolLifecycleContext(t.Context()), lifecycleWaitBudget(t))
			var caller sync.WaitGroup
			defer func() {
				cancel()
				if err := os.WriteFile(proceed, nil, 0o600); err != nil {
					t.Error(err)
				}
				caller.Wait()
			}()
			first := make(chan protocol.OperationResult, 1)
			caller.Go(func() { first <- r.endpoint.Invoke(ctx, delivery.TaskWorktreesCreate, request, options) })
			awaitTaskGitBoundary(t, ctx, entered, first)
			paths := registered()
			expectedRegistrations := 1
			if boundary == afterCheckoutCreation {
				expectedRegistrations = 2
			}
			if len(paths) != expectedRegistrations {
				t.Fatalf("Git registrations at %s = %q", boundary, paths)
			}
			cancel()
			if err := os.WriteFile(proceed, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			caller.Wait()
			initial := <-first
			if boundary == beforeCheckoutCreation {
				if paths := registered(); initial.Failure == nil || len(paths) != 1 {
					t.Fatalf("cancelled preflight = %+v, %v; Git registrations = %q", initial.Value, initial.Failure, paths)
				}
			} else if initial.Failure != nil {
				t.Errorf("the registered checkout did not produce a receipt: %v", initial.Failure)
			}

			r = restartTaskWorktreeRuntime(t, r, cfg)
			retry, stop := context.WithTimeout(protocolLifecycleContext(t.Context()), lifecycleWaitBudget(t))
			defer stop()
			result := r.endpoint.Invoke(retry, delivery.TaskWorktreesCreate, request, options)
			if result.Failure != nil {
				t.Fatalf("retry with the original creation key after %s = %v", boundary, result.Failure)
			}
			task := result.Value.(*protocol.TaskWorktree)
			published := r.must(delivery.TaskWorktreesList, struct{}{}, "").(*protocol.Page[protocol.TaskWorktree])
			if paths := registered(); !task.Available || len(paths) != 2 || paths[1] != task.Workspace.Path ||
				len(published.Data) != 1 || !reflect.DeepEqual(published.Data[0], *task) {
				t.Fatalf("creation receipt = %+v; registrations = %q; catalog = %+v", task, paths, published.Data)
			}
			if boundary == afterCheckoutCreation && (paths[1] != task.Workspace.Path || !reflect.DeepEqual(initial.Value, result.Value)) {
				t.Fatalf("restart changed the registered checkout: before = %+v (%q), after = %+v", initial.Value, paths[1], task)
			}
		})
	}
}

func restartTaskWorktreeRuntime(t *testing.T, r *poMCPRuntime, cfg assemblyConfig) *poMCPRuntime {
	t.Helper()
	r.close()
	if err := cfg.stores.Close(); err != nil {
		t.Fatal(err)
	}
	stores, err := persistence.Open(t.Context(), persistence.Config{
		DataDirectory: cfg.stores.DataDirectory, ValidateDirectory: persistencefixture.UnleasedDirectory,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stores.Close() })
	cfg.stores, cfg.providerRegistry = stores, stores.Providers
	return poMCPOpen(t, cfg)
}

func holdTaskGitBoundary(t *testing.T, boundary taskGitBoundary) (entered, proceed string) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	control := t.TempDir()
	entered, proceed = filepath.Join(control, "entered"), filepath.Join(control, "proceed")
	const wrapper = `#!/bin/sh
operation=
previous=
for argument do
  case "$argument" in
    list) if [ "$previous" = worktree ]; then operation=before-preview; fi ;;
    apply) operation=after-apply ;;
    *'^{commit}') operation=before-create ;;
    add) if [ "$previous" = worktree ]; then operation=after-create; fi ;;
    HEAD) if [ "$previous" = --verify ]; then operation=before-removal; fi ;;
    remove) if [ "$previous" = worktree ]; then operation=after-removal; fi ;;
  esac
  previous=$argument
done
if [ "$operation" != "$FLAME_TEST_GIT_BOUNDARY" ]; then
  exec "$FLAME_TEST_REAL_GIT" "$@"
fi
case "$operation" in
after-*)
  "$FLAME_TEST_REAL_GIT" "$@" || exit
  ;;
esac
: > "$FLAME_TEST_GIT_ENTERED" || exit
while [ ! -f "$FLAME_TEST_GIT_PROCEED" ]; do sleep 0.02 || exit; done
case "$operation" in
before-*)
  exec "$FLAME_TEST_REAL_GIT" "$@"
  ;;
esac
`
	if err := os.WriteFile(filepath.Join(control, "git"), []byte(wrapper), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", control+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FLAME_TEST_REAL_GIT", realGit)
	t.Setenv("FLAME_TEST_GIT_BOUNDARY", string(boundary))
	t.Setenv("FLAME_TEST_GIT_ENTERED", entered)
	t.Setenv("FLAME_TEST_GIT_PROCEED", proceed)
	return entered, proceed
}

func awaitTaskGitBoundary(t *testing.T, ctx context.Context, entered string, result <-chan protocol.OperationResult) {
	t.Helper()
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	for {
		_, err := os.Stat(entered)
		if err == nil {
			return
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		select {
		case returned := <-result:
			t.Fatalf("command settled before the Git boundary: %+v, %v", returned.Value, returned.Failure)
		case <-ctx.Done():
			t.Fatalf("command did not reach the Git boundary: %v", ctx.Err())
		case <-poll.C:
		}
	}
}

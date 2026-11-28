package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/packages/go-sdk/sandbox"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

type heldTaskSetup struct {
	request     protocol.CreateTaskWorktreeRequest
	key         string
	ctx         context.Context
	cancel      context.CancelFunc
	result      chan protocol.OperationResult
	releasePath string
	requests    sync.WaitGroup
}

func holdTaskSetup(t *testing.T, r *poMCPRuntime, request protocol.CreateTaskWorktreeRequest, git func(...string) string, key string) *heldTaskSetup {
	t.Helper()
	const readyFile = "setup-ready"
	const releaseFile = "release-setup"
	request.Setup = fmt.Sprintf("printf ready > %[1]s; while [ ! -f %[2]s ]; do sleep 0.05 || exit; done", readyFile, releaseFile)
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	setup := &heldTaskSetup{request: request, key: key, ctx: ctx, cancel: cancel, result: make(chan protocol.OperationResult, 1)}
	t.Cleanup(func() {
		setup.cancel()
		setup.requests.Wait()
	})
	setup.requests.Go(func() {
		setup.result <- r.endpoint.Invoke(ctx, delivery.TaskWorktreesCreate, request, protocol.CallMetadata{IdempotencyKey: key})
	})

	ready, cancelReady := context.WithTimeout(ctx, 20*time.Second)
	defer cancelReady()
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	for {
		for field := range strings.SplitSeq(git("worktree", "list", "--porcelain", "-z"), "\x00") {
			checkout, found := strings.CutPrefix(field, "worktree ")
			if !found {
				continue
			}
			if _, err := os.Stat(filepath.Join(checkout, readyFile)); err == nil {
				setup.releasePath = filepath.Join(checkout, releaseFile)
				return setup
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
		}
		select {
		case reply := <-setup.result:
			t.Fatalf("setup settled before it reached the fixture gate: %v", reply.Failure)
		case <-ready.Done():
			t.Fatal("the new checkout never started its setup")
		case <-poll.C:
		}
	}
}

func TestTaskWorktreeSetupKeepsOtherTasksAvailable(t *testing.T) {
	if !sandbox.Available() {
		t.Skip("workspace setup requires the native sandbox")
	}
	source, git := taskWorktreeTestRepository(t)
	git("commit", "--quiet", "--allow-empty", "-m", "initial snapshot")
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)

	request := protocol.CreateTaskWorktreeRequest{Source: protocol.WorkspaceRef{Path: source}, Base: "target"}
	existing := r.must(delivery.TaskWorktreesCreate, request, "existing-task").(*protocol.TaskWorktree)

	setup := holdTaskSetup(t, r, request, git, "task-with-held-setup")

	listCtx, cancelList := context.WithTimeout(setup.ctx, 5*time.Second)
	defer cancelList()
	listed := make(chan protocol.OperationResult, 1)
	setup.requests.Go(func() {
		listed <- r.endpoint.Invoke(listCtx, delivery.TaskWorktreesList, struct{}{}, protocol.CallMetadata{})
	})
	select {
	case result := <-listed:
		if result.Failure != nil {
			t.Fatalf("the existing task catalog failed during setup: %v", result.Failure)
		}
		list := result.Value.(*protocol.Page[protocol.TaskWorktree])
		if len(list.Data) != 1 || list.Data[0].ID != existing.ID || !list.Data[0].Available {
			t.Fatalf("the catalog published an unfinished checkout or hid the existing task: %+v", list.Data)
		}
	case <-listCtx.Done():
		t.Fatal("one checkout's unfinished setup blocked the task catalog")
	}

	parallel := r.endpoint.Invoke(setup.ctx, delivery.TaskWorktreesCreate, request, protocol.CallMetadata{IdempotencyKey: "parallel-task"})
	if parallel.Failure != nil {
		t.Fatalf("the same repository could not create another task while setup was running: %v", parallel.Failure)
	}
	parallelTask := parallel.Value.(*protocol.TaskWorktree)
	if !parallelTask.Available || parallelTask.Workspace.Path == existing.Workspace.Path ||
		parallelTask.Workspace.Path == filepath.Dir(setup.releasePath) || parallelTask.Base != existing.Base {
		t.Fatalf("parallel creation did not produce an independent checkout at the chosen commit: %+v", parallelTask)
	}
	if status := git("status", "--porcelain"); status != "" {
		t.Fatalf("task preparation changed the source checkout: %q", status)
	}

	if err := os.WriteFile(setup.releasePath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	result := <-setup.result
	if result.Failure != nil {
		t.Fatal(result.Failure)
	}
	finished := result.Value.(*protocol.TaskWorktree)
	if !finished.Available || finished.Setup == nil || finished.Setup.ExitCode == nil || *finished.Setup.ExitCode != 0 {
		t.Fatalf("the finished checkout was not published with its setup outcome: %+v", finished)
	}
	list := r.must(delivery.TaskWorktreesList, struct{}{}, "").(*protocol.Page[protocol.TaskWorktree])
	if len(list.Data) != 3 || !slices.ContainsFunc(list.Data, func(task protocol.TaskWorktree) bool {
		return task.ID == finished.ID && task.Available && task.Setup != nil && task.Setup.ExitCode != nil && *task.Setup.ExitCode == 0
	}) {
		t.Fatalf("the finished checkout was missing from the catalog: %+v", list.Data)
	}
}

func TestTaskWorktreeCreateRecoversReceiptAfterSetupCancellation(t *testing.T) {
	if !sandbox.Available() {
		t.Skip("workspace setup requires the native sandbox")
	}
	source, git := taskWorktreeTestRepository(t)
	git("commit", "--quiet", "--allow-empty", "-m", "initial snapshot")
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)

	request := protocol.CreateTaskWorktreeRequest{Source: protocol.WorkspaceRef{Path: source}, Base: "target"}
	existing := r.must(delivery.TaskWorktreesCreate, request, "existing-task").(*protocol.TaskWorktree)
	setup := holdTaskSetup(t, r, request, git, "task-with-canceled-setup")
	setup.cancel()
	<-setup.result

	list := r.must(delivery.TaskWorktreesList, struct{}{}, "").(*protocol.Page[protocol.TaskWorktree])
	if len(list.Data) != 2 {
		t.Fatalf("canceled observation lost or duplicated the checkout: %+v", list.Data)
	}
	var published protocol.TaskWorktree
	for _, task := range list.Data {
		if task.ID != existing.ID {
			published = task
		}
	}
	if !published.Available || published.Setup == nil || published.Setup.ExitCode != nil ||
		published.Workspace.Path != filepath.Dir(setup.releasePath) {
		t.Fatalf("the checkout lacked its actual interrupted setup result: %+v", published)
	}

	replayCtx, cancelReplay := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelReplay()
	replay := r.endpoint.Invoke(replayCtx, delivery.TaskWorktreesCreate, setup.request, protocol.CallMetadata{IdempotencyKey: setup.key})
	if replay.Failure != nil {
		t.Fatalf("the original command could not recover its published checkout: %v", replay.Failure)
	}
	recovered := replay.Value.(*protocol.TaskWorktree)
	if recovered.ID != published.ID || recovered.Workspace != published.Workspace ||
		recovered.Setup == nil || recovered.Setup.ExitCode != nil {
		t.Fatalf("the original command returned a different checkout or setup outcome: %+v", recovered)
	}
	list = r.must(delivery.TaskWorktreesList, struct{}{}, "").(*protocol.Page[protocol.TaskWorktree])
	if len(list.Data) != 2 {
		t.Fatalf("checking the original receipt created another checkout: %+v", list.Data)
	}
}

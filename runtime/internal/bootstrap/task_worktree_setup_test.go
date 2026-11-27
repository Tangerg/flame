package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/packages/go-sdk/sandbox"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestTaskWorktreeSetupDoesNotBlockTheCatalog(t *testing.T) {
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

	const readyFile = "setup-ready"
	const releaseFile = "release-setup"
	request.Setup = fmt.Sprintf("printf ready > %[1]s; while [ ! -f %[2]s ]; do sleep 0.05 || exit; done", readyFile, releaseFile)
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	var requests sync.WaitGroup
	defer func() {
		cancel()
		requests.Wait()
	}()
	created := make(chan protocol.OperationResult, 1)
	requests.Go(func() {
		created <- r.endpoint.Invoke(ctx, delivery.TaskWorktreesCreate, request, protocol.CallMetadata{IdempotencyKey: "task-with-held-setup"})
	})

	checkout := existing.Workspace.Path
	markerPattern := filepath.Join(filepath.Dir(filepath.Dir(checkout)), "*", filepath.Base(checkout), readyFile)
	ready, cancelReady := context.WithTimeout(ctx, 20*time.Second)
	defer cancelReady()
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	var releasePath string
	for releasePath == "" {
		matches, err := filepath.Glob(markerPattern)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) == 1 {
			releasePath = filepath.Join(filepath.Dir(matches[0]), releaseFile)
			break
		}
		select {
		case reply := <-created:
			t.Fatalf("setup settled before it reached the fixture gate: %v", reply.Failure)
		case <-ready.Done():
			t.Fatal("the new checkout never started its setup")
		case <-poll.C:
		}
	}

	listCtx, cancelList := context.WithTimeout(ctx, 5*time.Second)
	defer cancelList()
	listed := make(chan protocol.OperationResult, 1)
	requests.Go(func() {
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
	if err := os.WriteFile(releasePath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	result := <-created
	if result.Failure != nil {
		t.Fatal(result.Failure)
	}
	finished := result.Value.(*protocol.TaskWorktree)
	if !finished.Available || finished.Setup == nil || finished.Setup.ExitCode == nil || *finished.Setup.ExitCode != 0 {
		t.Fatalf("the finished checkout was not published with its setup outcome: %+v", finished)
	}
	list := r.must(delivery.TaskWorktreesList, struct{}{}, "").(*protocol.Page[protocol.TaskWorktree])
	if len(list.Data) != 2 || !slices.ContainsFunc(list.Data, func(task protocol.TaskWorktree) bool {
		return task.ID == finished.ID && task.Available && task.Setup != nil && task.Setup.ExitCode != nil && *task.Setup.ExitCode == 0
	}) {
		t.Fatalf("the finished checkout was missing from the catalog: %+v", list.Data)
	}
}

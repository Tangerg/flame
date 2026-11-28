package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/packages/go-sdk/sandbox"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestTaskWorktreeCatalogExcludesRefusedGitRegistrations(t *testing.T) {
	source, git := taskWorktreeTestRepository(t)
	const file = "material.txt"
	const material = "material in the original checkout\n"
	const filter = "checkout-fixture"
	if err := os.WriteFile(filepath.Join(source, file), []byte(material), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".gitattributes"), []byte(file+" filter="+filter+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "--", file, ".gitattributes")
	git("commit", "--quiet", "-m", "material with a checkout filter")
	section := "filter." + filter
	git("config", section+".smudge", "false")
	git("config", section+".required", "true")
	before := git("worktree", "list", "--porcelain")
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)
	ctx := protocolLifecycleContext(t.Context())
	request := protocol.CreateTaskWorktreeRequest{Source: protocol.WorkspaceRef{Path: source}, Base: "HEAD"}
	options := protocol.CallMetadata{IdempotencyKey: "task-refused-by-checkout-filter"}
	refused := r.endpoint.Invoke(ctx, delivery.TaskWorktreesCreate, request, options)
	if refused.Failure == nil || !errors.Is(refused.Failure, protocol.ErrVcsUnavailable) || !refused.Failure.RPCData().RecordedOutcome {
		t.Errorf("native checkout refusal = %+v, %v; want a recorded Git failure", refused.Value, refused.Failure)
	}
	if after := git("worktree", "list", "--porcelain"); after != before {
		t.Fatalf("failed checkout left a Git registration: %s", after)
	}
	catalog := r.must(delivery.TaskWorktreesList, struct{}{}, "").(*protocol.Page[protocol.TaskWorktree])
	if len(catalog.Data) != 0 {
		t.Errorf("catalog published a task Git refused to register: %+v", catalog.Data)
	}

	git("config", "--remove-section", section)
	replayed := r.endpoint.Invoke(ctx, delivery.TaskWorktreesCreate, request, options)
	if replayed.Failure == nil || refused.Failure == nil || !reflect.DeepEqual(replayed.Failure.RPCData(), refused.Failure.RPCData()) {
		t.Errorf("original key after repairing the filter = %+v, %v; want the original refusal", replayed.Value, replayed.Failure)
	}
	created := r.must(delivery.TaskWorktreesCreate, request, "task-after-checkout-filter-repair").(*protocol.TaskWorktree)
	catalog = r.must(delivery.TaskWorktreesList, struct{}{}, "").(*protocol.Page[protocol.TaskWorktree])
	if len(catalog.Data) != 1 || !reflect.DeepEqual(catalog.Data[0], *created) {
		t.Fatalf("catalog after successful registration = %+v; want only %+v", catalog.Data, created)
	}
}

func TestTaskWorktreeCatalogPersistsEscapedSetupOutput(t *testing.T) {
	if !sandbox.Available() {
		t.Skip("workspace setup requires the native sandbox")
	}
	source, git := taskWorktreeTestRepository(t)
	git("commit", "--quiet", "--allow-empty", "-m", "initial checkout")
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)
	const outputBytes = 3000
	command := fmt.Sprintf(`i=0; while [ "$i" -lt %d ]; do printf '\001'; i=$((i + 1)); done`, outputBytes)
	request := protocol.CreateTaskWorktreeRequest{Source: protocol.WorkspaceRef{Path: source}, Base: "HEAD", Setup: command}
	const key = "task-with-escaped-setup"
	created := r.must(delivery.TaskWorktreesCreate, request, key).(*protocol.TaskWorktree)
	if created.Setup == nil {
		t.Fatal("the creation receipt omitted setup")
	}
	if created.Setup.ExitCode == nil || *created.Setup.ExitCode != 0 || !strings.HasPrefix(created.Setup.Output, strings.Repeat("\x01", outputBytes)) {
		prefix := created.Setup.Output[:min(80, len(created.Setup.Output))]
		t.Fatalf("native setup returned %d bytes starting with %q; want %d control bytes", len(created.Setup.Output), prefix, outputBytes)
	}

	r = restartTaskWorktreeRuntime(t, r, cfg)
	catalog := r.must(delivery.TaskWorktreesList, struct{}{}, "").(*protocol.Page[protocol.TaskWorktree])
	if len(catalog.Data) != 1 || !reflect.DeepEqual(catalog.Data[0], *created) {
		t.Fatal("cold catalog did not preserve the original setup result")
	}
	replayed := r.must(delivery.TaskWorktreesCreate, request, key).(*protocol.TaskWorktree)
	if !reflect.DeepEqual(replayed, created) {
		t.Fatal("cold replay changed the original setup result")
	}
	r.must(delivery.TaskWorktreesRelease, protocol.TaskWorktreeRequest{ID: created.ID}, "release-task-with-escaped-setup")
}

func TestTaskWorktreeCatalogReportsOnlyCommittedChanges(t *testing.T) {
	source, git := taskWorktreeTestRepository(t)
	git("commit", "--quiet", "--allow-empty", "-m", "initial checkout")
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)
	ctx, cancel := context.WithTimeout(protocolLifecycleContext(t.Context()), lifecycleWaitBudget(t))
	defer cancel()
	_, changes, err := r.api.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicTaskWorktreesChanged, protocol.TopicSessionsChanged},
	})
	if err != nil {
		t.Fatal(err)
	}
	next, stop := iter.Pull2(changes)
	defer stop()
	expect := func(want protocol.RuntimeEventType) protocol.RuntimeEvent {
		t.Helper()
		event, err, ok := next()
		if !ok || err != nil || event.Type != want {
			t.Fatalf("next notification = %+v, %v, observed=%t; want %s", event, err, ok, want)
		}
		return event
	}
	expectSessionBarrier := func(key string) {
		t.Helper()
		session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: key}, key).(*protocol.Session)
		event := expect(protocol.RuntimeSessionsChanged)
		if len(event.SessionIDs) != 1 || event.SessionIDs[0] != session.ID {
			t.Fatalf("session notification = %+v; want %s", event, session.ID)
		}
	}

	task := r.must(delivery.TaskWorktreesCreate, protocol.CreateTaskWorktreeRequest{
		Source: protocol.WorkspaceRef{Path: source}, Base: "HEAD",
	}, "create-observed-worktree").(*protocol.TaskWorktree)
	expect(protocol.RuntimeTaskWorktreesChanged)
	draft := filepath.Join(task.Workspace.Path, "draft.txt")
	if err := os.WriteFile(draft, []byte("unfinished work\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := protocol.TaskWorktreeRequest{ID: task.ID}
	refusal := r.endpoint.Invoke(ctx, delivery.TaskWorktreesRelease, request, protocol.CallMetadata{
		IdempotencyKey: "release-dirty-observed-worktree",
	})
	if refusal.Failure == nil || !errors.Is(refusal.Failure, protocol.ErrRevisionConflict) {
		t.Fatalf("dirty release = %+v, %v; want revision conflict", refusal.Value, refusal.Failure)
	}
	expectSessionBarrier("after-refused-worktree-release")

	if err := os.Remove(draft); err != nil {
		t.Fatal(err)
	}
	const key = "release-clean-observed-worktree"
	r.must(delivery.TaskWorktreesRelease, request, key)
	expect(protocol.RuntimeTaskWorktreesChanged)
	r.must(delivery.TaskWorktreesRelease, request, key)
	expectSessionBarrier("after-replayed-worktree-release")
}

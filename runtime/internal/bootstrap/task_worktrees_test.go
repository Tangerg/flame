package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/packages/go-sdk/sandbox"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func taskWorktreeTestRepository(t *testing.T) (string, func(...string) string) {
	t.Helper()
	source := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", append([]string{
			"-c", "user.name=Worktree Fixture", "-c", "user.email=fixture@example.test",
			"-c", "core.hooksPath=" + os.DevNull,
		}, args...)...)
		cmd.Dir = source
		raw, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, raw, err)
		}
		return strings.TrimSpace(string(raw))
	}
	git("init", "--quiet", "--template=", "--initial-branch=target")
	return source, git
}

func TestTaskWorktreesKeepSeparateChangesAndHandoffAfterRestart(t *testing.T) {
	source, git := taskWorktreeTestRepository(t)
	write := func(root, name string, body []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	requireContents := func(root, name string, want []byte) {
		t.Helper()
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s in %s = %q, %v", name, root, got, err)
		}
	}
	write(source, "shared.txt", []byte("base\n"))
	write(source, ".gitignore", []byte("setup.marker\n"))
	git("add", ".")
	git("commit", "--quiet", "-m", "initial snapshot")
	base := git("rev-parse", "HEAD")

	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("Worktree verification"), nil
		}
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult != nil && part.ToolResult.ID == "verify-worktree" {
					return completedTextResponse("Verified the task workspace"), nil
				}
			}
		}
		return filewatchToolCall(chat.ToolCall{ID: "verify-worktree", Name: "ask_user", Arguments: `{"questions":[{"question":"Continue the verification in this task workspace?"}]}`})
	})}
	chatClient, err := chatclient.New(model)
	if err != nil {
		t.Fatal(err)
	}
	cfg.modelProviders = testChatResolver(chatClient)
	r := poMCPOpen(t, cfg)
	create := func(key, setup string) *protocol.TaskWorktree {
		t.Helper()
		return r.must(delivery.TaskWorktreesCreate, protocol.CreateTaskWorktreeRequest{
			Source: protocol.WorkspaceRef{Path: source}, Base: "target", Setup: setup,
		}, key).(*protocol.TaskWorktree)
	}
	a := create("worktree-create-a", "printf ready > setup.marker")
	b := create("worktree-create-b", "")
	unused := create("worktree-create-unused", "")
	if !a.Available || !b.Available || a.Base != base || b.Base != base || a.Workspace == b.Workspace {
		t.Fatalf("task worktrees did not reserve separate directories at %s: %+v %+v", base, a, b)
	}
	if sandbox.Available() {
		if a.Setup == nil || a.Setup.ExitCode == nil || *a.Setup.ExitCode != 0 {
			t.Fatalf("task setup did not finish in the managed workspace: %+v", a.Setup)
		}
		requireContents(a.Workspace.Path, "setup.marker", []byte("ready"))
	}
	open := func(task *protocol.TaskWorktree, key string) *protocol.Session {
		t.Helper()
		return r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{
			Title: key, Workspace: &task.Workspace,
		}, key).(*protocol.Session)
	}
	first, second := open(a, "worktree-session-a"), open(b, "worktree-session-b")
	write(a.Workspace.Path, "shared.txt", []byte("first task\n"))
	write(b.Workspace.Path, "shared.txt", []byte("second task\n"))
	binary := []byte{0, 15, 255, 89}
	write(b.Workspace.Path, "attachment.bin", binary)
	requireContents(source, "shared.txt", []byte("base\n"))

	meta := protocol.RequestMeta{ProtocolVersion: new(protocol.Version), ClientCapabilities: &protocol.ClientCapabilities{
		InterruptTypes: []protocol.InterruptType{protocol.InterruptQuestion},
	}}
	ctx := delivery.WithRequestMeta(t.Context(), meta)
	firstRun, firstEvents, err := r.api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: first.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Verify the first task"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondRun, secondEvents, err := r.api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: second.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Verify the second task"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, err := range firstEvents {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, err := range secondEvents {
		if err != nil {
			t.Fatal(err)
		}
	}
	question := func(runID string) string {
		t.Helper()
		pending, err := r.api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: runID})
		if err != nil || len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
			t.Fatalf("original worktree verification did not keep its interruption: %+v, %v", pending, err)
		}
		return pending.Data[0].Interrupts[0].ItemID
	}
	questions := map[string]string{firstRun.RunID: question(firstRun.RunID), secondRun.RunID: question(secondRun.RunID)}
	if firstRun.RunID == secondRun.RunID || questions[firstRun.RunID] == questions[secondRun.RunID] {
		t.Fatal("parallel worktrees reused an independent verification")
	}

	r = restartTaskWorktreeRuntime(t, r, cfg)
	for _, want := range []*protocol.Session{first, second} {
		found := r.must(delivery.SessionsGet, protocol.GetSessionRequest{SessionID: want.ID}, "").(*protocol.Session)
		if found.Workspace != want.Workspace {
			t.Fatalf("restart changed the session workspace: %+v", found.Workspace)
		}
	}
	for runID, original := range questions {
		if question(runID) != original {
			t.Fatal("restart replaced the original verification permission")
		}
		result := r.endpoint.Invoke(ctx, delivery.RunsResume, protocol.ResumeRunRequest{RunID: runID, Responses: []protocol.InterruptResponse{{
			ItemID: original, Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseAnswer, Answers: [][]string{{"Continue"}}},
		}}}, protocol.CallMetadata{IdempotencyKey: "verify-" + runID, RequestMeta: meta})
		if result.Failure != nil {
			t.Fatal(result.Failure)
		}
		for _, err := range result.Events {
			if err != nil {
				t.Fatal(err)
			}
		}
		verified := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: runID}, "").(*protocol.RunRef)
		if verified.Outcome == nil || verified.Outcome.Type != protocol.OutcomeCompleted {
			t.Fatalf("the original verification did not complete: %+v", verified)
		}
	}
	list := r.must(delivery.TaskWorktreesList, struct{}{}, "").(*protocol.Page[protocol.TaskWorktree])
	if len(list.Data) != 3 {
		t.Fatalf("restart lost worktree custody: %+v", list.Data)
	}
	preview := func(task *protocol.TaskWorktree) *protocol.WorktreeHandoff {
		t.Helper()
		return r.must(delivery.TaskWorktreesPreview, protocol.TaskWorktreeRequest{ID: task.ID}, "").(*protocol.WorktreeHandoff)
	}
	if patch := preview(a).Patch; !strings.Contains(patch, "+first task") || strings.Contains(patch, "+second task") {
		t.Fatalf("the first task preview mixed worktrees: %q", patch)
	}
	chosen := preview(b)
	if chosen.Branch != "target" || chosen.Head != base || !strings.Contains(chosen.Patch, "+second task") || !strings.Contains(chosen.Patch, "attachment.bin") {
		t.Fatalf("the handoff preview lacks its exact destination and changes: %+v", chosen)
	}
	write(b.Workspace.Path, "shared.txt", []byte("second task updated\n"))
	stale := r.endpoint.Invoke(t.Context(), delivery.TaskWorktreesApply, protocol.ApplyWorktreeHandoffRequest{ID: b.ID, Digest: chosen.Digest},
		protocol.CallMetadata{IdempotencyKey: "stale-worktree-handoff"})
	if !errors.Is(stale.Failure, protocol.ErrRevisionConflict) {
		t.Fatalf("stale worktree handoff was not fenced: %v", stale.Failure)
	}
	requireContents(source, "shared.txt", []byte("base\n"))
	chosen = preview(b)
	r.must(delivery.TaskWorktreesApply, protocol.ApplyWorktreeHandoffRequest{ID: b.ID, Digest: chosen.Digest}, "apply-worktree-handoff")
	r.must(delivery.TaskWorktreesApply, protocol.ApplyWorktreeHandoffRequest{ID: b.ID, Digest: chosen.Digest}, "apply-worktree-handoff")
	requireContents(source, "shared.txt", []byte("second task updated\n"))
	requireContents(source, "attachment.bin", binary)
	requireContents(a.Workspace.Path, "shared.txt", []byte("first task\n"))
	requireContents(b.Workspace.Path, "shared.txt", []byte("second task updated\n"))

	busy := r.endpoint.Invoke(t.Context(), delivery.TaskWorktreesRelease, protocol.TaskWorktreeRequest{ID: a.ID},
		protocol.CallMetadata{IdempotencyKey: "release-worktree-in-use"})
	if !errors.Is(busy.Failure, protocol.ErrSessionBusy) {
		t.Fatalf("released a worktree with a bound session: %v", busy.Failure)
	}
	r.must(delivery.TaskWorktreesRelease, protocol.TaskWorktreeRequest{ID: unused.ID}, "release-unused-worktree")
	if _, err := os.Stat(unused.Workspace.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("released workspace still exists: %v", err)
	}
	requireContents(source, "shared.txt", []byte("second task updated\n"))
}

func TestTaskWorktreesKeepListingWhenAMovedCheckoutPathIsReused(t *testing.T) {
	source, git := taskWorktreeTestRepository(t)
	git("commit", "--quiet", "--allow-empty", "-m", "initial snapshot")
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)

	request := protocol.CreateTaskWorktreeRequest{Source: protocol.WorkspaceRef{Path: source}, Base: "target"}
	moved := r.must(delivery.TaskWorktreesCreate, request, "move-task-worktree").(*protocol.TaskWorktree)
	retained := r.must(delivery.TaskWorktreesCreate, request, "retain-task-worktree").(*protocol.TaskWorktree)
	newPath := filepath.Join(t.TempDir(), "moved")
	git("worktree", "move", "--", moved.Workspace.Path, newPath)
	if err := os.Mkdir(moved.Workspace.Path, 0o700); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(moved.Workspace.Path, "notes.txt")
	const notes = "keep these notes"
	if err := os.WriteFile(replacement, []byte(notes), 0o600); err != nil {
		t.Fatal(err)
	}

	requireUnavailable := func(releaseKey string) {
		t.Helper()
		list := r.must(delivery.TaskWorktreesList, struct{}{}, "").(*protocol.Page[protocol.TaskWorktree])
		if len(list.Data) != 2 ||
			!slices.ContainsFunc(list.Data, func(task protocol.TaskWorktree) bool { return task.ID == moved.ID && !task.Available }) ||
			!slices.ContainsFunc(list.Data, func(task protocol.TaskWorktree) bool { return task.ID == retained.ID && task.Available }) {
			t.Fatalf("worktree catalog lost the retained and moved entries: %+v", list.Data)
		}
		release := r.endpoint.Invoke(t.Context(), delivery.TaskWorktreesRelease, protocol.TaskWorktreeRequest{ID: moved.ID},
			protocol.CallMetadata{IdempotencyKey: releaseKey})
		if release.Failure == nil || !errors.Is(release.Failure, protocol.ErrRevisionConflict) {
			t.Fatalf("release did not report the replaced checkout: %v", release.Failure)
		}
	}

	requireUnavailable("release-plain-replacement")
	contents, err := os.ReadFile(replacement)
	if err != nil || string(contents) != notes {
		t.Fatalf("replacement directory lost its contents: %q, %v", contents, err)
	}
	if err := os.Rename(moved.Workspace.Path, filepath.Join(t.TempDir(), "notes")); err != nil {
		t.Fatal(err)
	}
	git("worktree", "add", "--quiet", "--detach", "--", moved.Workspace.Path, moved.Base)
	requireUnavailable("release-unowned-git-replacement")
	if _, err := os.Stat(filepath.Join(moved.Workspace.Path, ".git")); err != nil {
		t.Fatalf("replacement worktree lost its Git checkout: %v", err)
	}
	if _, err := os.Stat(filepath.Join(newPath, ".git")); err != nil {
		t.Fatalf("moved worktree lost its Git checkout: %v", err)
	}
}

func TestTaskWorktreePreviewDoesNotNeedMutationAdmission(t *testing.T) {
	source, git := taskWorktreeTestRepository(t)
	const name = "handoff.txt"
	if err := os.WriteFile(filepath.Join(source, name), []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", name)
	git("commit", "--quiet", "-m", "initial checkout")
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)
	task := r.must(delivery.TaskWorktreesCreate, protocol.CreateTaskWorktreeRequest{
		Source: protocol.WorkspaceRef{Path: source}, Base: "target",
	}, "create-preview-task").(*protocol.TaskWorktree)
	if err := os.WriteFile(filepath.Join(task.Workspace.Path, name), []byte("handoff\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	query := protocol.TaskWorktreeRequest{ID: task.ID}
	expected := r.must(delivery.TaskWorktreesPreview, query, "").(*protocol.WorktreeHandoff)
	apply := protocol.ApplyWorktreeHandoffRequest{ID: task.ID, Digest: expected.Digest}

	for _, target := range []struct{ name, path string }{
		{"source", task.Source.Path}, {"task", task.Workspace.Path},
	} {
		t.Run(target.name, func(t *testing.T) {
			release, acquired, err := r.host.application.admissions.AcquireWorkingTreeMutation(target.path)
			if err != nil || !acquired {
				t.Fatalf("reserve working tree %q: acquired=%t, %v", target.path, acquired, err)
			}
			defer release()
			meta := protocol.CallMetadata{RequestMeta: protocol.RequestMeta{ProtocolVersion: new(protocol.Version)}}
			observed := r.endpoint.Invoke(t.Context(), delivery.TaskWorktreesPreview, query, meta)
			if observed.Failure != nil {
				t.Fatalf("read handoff while %s is reserved: %v", target.name, observed.Failure)
			}
			got := observed.Value.(*protocol.WorktreeHandoff)
			if got.Digest != expected.Digest || got.Patch != expected.Patch {
				t.Fatalf("reservation changed the handoff preview: %+v", got)
			}
			meta.IdempotencyKey = "apply-with-reserved-" + target.name
			blocked := r.endpoint.Invoke(t.Context(), delivery.TaskWorktreesApply, apply, meta)
			if !errors.Is(blocked.Failure, protocol.ErrSessionBusy) {
				t.Fatalf("handoff could write while %s is reserved: %v", target.name, blocked.Failure)
			}
			if got := git("diff", "--cached", "--", name); got != "" {
				t.Fatalf("preview or refused handoff modified the index: %q", got)
			}
		})
	}
	r.must(delivery.TaskWorktreesApply, apply, "apply-without-reservation")
	if got := git("show", ":"+name); got != "handoff" {
		t.Fatalf("confirmed handoff staged %q, want handoff", got)
	}
}

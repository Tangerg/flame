package bootstrap

import (
	"errors"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestTaskWorktreePreviewReportsPatchCapacity(t *testing.T) {
	source, git := taskWorktreeTestRepository(t)
	git("commit", "--quiet", "--allow-empty", "-m", "initial checkout")
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)
	task := r.must(delivery.TaskWorktreesCreate, protocol.CreateTaskWorktreeRequest{
		Source: protocol.WorkspaceRef{Path: source}, Base: "HEAD",
	}, "create-large-patch-worktree").(*protocol.TaskWorktree)
	asset, err := os.Create(filepath.Join(task.Workspace.Path, "asset.bin"))
	if err != nil {
		t.Fatal(err)
	}
	// Git's binary encoding expands incompressible data past its stdout ceiling.
	const assetBytes = 56 << 20
	_, writeErr := io.CopyN(asset, rand.NewChaCha8([32]byte{}), assetBytes)
	if err := errors.Join(writeErr, asset.Close()); err != nil {
		t.Fatal(err)
	}
	result := r.endpoint.Invoke(protocolLifecycleContext(t.Context()), delivery.TaskWorktreesPreview,
		protocol.TaskWorktreeRequest{ID: task.ID}, protocol.CallMetadata{})
	if result.Failure == nil || !errors.Is(result.Failure, protocol.ErrInvalidParams) {
		t.Fatalf("oversized binary handoff preview = %+v, %v; want invalid params", result.Value, result.Failure)
	}
}

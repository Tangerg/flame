package skillauthoring

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestStageProposalRemovesUnpublishedSlot(t *testing.T) {
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := root.Mkdir(proposalsSubdir, 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := stageProposal(ctx, root, proposalsSubdir+"/canceled", []byte("proposal")); !errors.Is(err, context.Canceled) {
		t.Fatalf("stageProposal error = %v, want context.Canceled", err)
	}
	if _, err := root.Lstat(proposalsSubdir + "/canceled"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unpublished proposal slot remains: %v", err)
	}
}

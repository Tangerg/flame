package skillauthoring

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestCanceledPublicationDoesNotExposeStagedSkill(t *testing.T) {
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := stageSkill(ctx, root, "canceled", []byte("proposal")); !errors.Is(err, context.Canceled) {
		t.Fatalf("stageProposal error = %v, want context.Canceled", err)
	}
	if _, err := root.Lstat("canceled"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unpublished proposal slot remains: %v", err)
	}
}

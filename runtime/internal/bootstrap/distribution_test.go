package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func TestDistributionSourcesBoundsItsCatalog(t *testing.T) {
	root := t.TempDir()
	if _, err := distributionSources(t.Context(), root); err == nil {
		t.Fatal("empty distribution was admitted")
	}
	for index := range plugin.MaxInstallations {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("%03d", index)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	sources, err := distributionSources(t.Context(), root)
	if err != nil || len(sources) != plugin.MaxInstallations {
		t.Fatalf("bounded distribution = %d sources, %v", len(sources), err)
	}
	for index, source := range sources {
		if source != filepath.Join(root, fmt.Sprintf("%03d", index)) {
			t.Fatalf("source %d = %s", index, source)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "excess"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := distributionSources(t.Context(), root); err == nil {
		t.Fatal("oversized distribution was admitted")
	}
}

func TestDistributionSourcesRejectsFilesAndCancellation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := distributionSources(t.Context(), root); err == nil {
		t.Fatal("a file became a package source")
	}
	cause := errors.New("distribution canceled")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	if _, err := distributionSources(ctx, filepath.Join(root, "missing")); !errors.Is(err, cause) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}

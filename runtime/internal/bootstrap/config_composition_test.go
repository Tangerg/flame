package bootstrap

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/config"
)

func TestComposeConfigInjectsDurableRuntimePolicy(t *testing.T) {
	const buildID = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: t.TempDir(), DefaultWorkspacePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stores.Close() })
	got := ComposeConfig(config.Settings{}, stores, nil, nil, buildID)
	if got.BuildID != buildID {
		t.Fatalf("BuildID = %q, want %q", got.BuildID, buildID)
	}
	if got.Stores != stores {
		t.Fatal("composition replaced the durable graph")
	}
}

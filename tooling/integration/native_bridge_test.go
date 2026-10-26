package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestActualNativeBridgeProjectionIsCurrent(t *testing.T) {
	repository, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := t.TempDir()
	desktop := filepath.Join(output, "desktop.generated.ts")
	authorization := filepath.Join(output, "notificationAuthorization.ts")
	command := exec.CommandContext(t.Context(), "go", "run", "-mod=readonly", "./cmd/bridgegen",
		"--source", ".", "--receiver", "DesktopHost", "--out", desktop, "--foundation", authorization)
	command.Dir = filepath.Join(repository, "apps/desktop")
	command.Env = append(os.Environ(), "GOWORK=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generate actual Desktop bridge: %v\n%s", err, output)
	}
	for _, projection := range []struct {
		generated string
		published string
	}{
		{desktop, "apps/web/src/platform/desktop.generated.ts"},
		{authorization, "apps/web/src/foundation/notificationAuthorization.ts"},
	} {
		want, err := os.ReadFile(projection.generated)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(repository, projection.published))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is stale; run make generate", projection.published)
		}
	}
}

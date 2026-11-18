package exec

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/sandbox"
)

func unconfinedShells(t *testing.T) *Shells {
	t.Helper()
	shells, err := NewShells(nil, sandbox.ErrUnavailable, false)
	if err != nil {
		t.Fatal(err)
	}
	return shells
}

// TestLaunchIsolatedWithoutConfinerFailsClosed proves an isolated-session
// command without a confiner is refused rather than run unconfined, and that the
// refusal names why the confiner could not be built.
func TestLaunchIsolatedWithoutConfinerFailsClosed(t *testing.T) {
	cause := errors.New("sandbox: read-only path must be absolute")
	shells, err := NewShells(nil, cause, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shells.KillAll() })
	if _, err := shells.Launch(t.Context(), "", t.TempDir(), "true", Timeout{}, true); !errors.Is(err, cause) {
		t.Fatalf("isolated launch error = %v, want the confiner's cause", err)
	}
	// A non-isolated command on the same set still runs (unconfined).
	if _, err := shells.Launch(t.Context(), "", t.TempDir(), "true", Timeout{}, false); err != nil {
		t.Fatalf("non-isolated launch should succeed: %v", err)
	}
}

func waitDone(t *testing.T, shells *Shells, id string) {
	t.Helper()
	select {
	case <-mustShell(t, shells, "", id).Done():
	case <-time.After(10 * time.Second):
		t.Fatalf("shell %s did not finish in time", id)
	}
}

func mustShell(t *testing.T, shells *Shells, sessionID, id string) *Shell {
	t.Helper()
	sh, ok := shells.Get(sessionID, id)
	if !ok {
		t.Fatalf("shell %s not found", id)
	}
	return sh
}

func testTimeout(t *testing.T, duration time.Duration) Timeout {
	t.Helper()
	timeout, err := NewTimeout(duration)
	if err != nil {
		t.Fatal(err)
	}
	return timeout
}

package delivery

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestPresenterPanicReachesTheLoggingChannel pins the operator's half of a
// presenter defect. The client is told its stream failed; only the stack says
// which projection shape the presenter could not read.
func TestPresenterPanicReachesTheLoggingChannel(t *testing.T) {
	var diagnostics bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&diagnostics, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	if _, err := presentedRunEvent(nil); err == nil {
		t.Fatal("an unpresentable event did not fail the stream")
	}
	logged := diagnostics.String()
	if !strings.Contains(logged, "presenter panicked") {
		t.Fatalf("diagnostics = %q, want the presenter defect reported", logged)
	}
	if !strings.Contains(logged, "presenter_run_event.go") {
		t.Fatalf("diagnostics = %q, want a stack naming the failing presenter", logged)
	}
}

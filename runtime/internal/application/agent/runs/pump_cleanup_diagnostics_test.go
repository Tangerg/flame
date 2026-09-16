package runs

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// TestRunCleanupFailureReachesTheLoggingChannel pins the diagnostic for work a
// finished Run still owed — releasing its executor, closing its journal,
// aborting a reserved child start, post-Run maintenance. None of it can change
// the settled outcome, so the report is all there is; on a host that configured
// no TracerProvider a span carries nothing.
func TestRunCleanupFailureReachesTheLoggingChannel(t *testing.T) {
	var diagnostics bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&diagnostics, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	recordRunCleanupError(t.Context(), nil)
	if diagnostics.Len() != 0 {
		t.Fatalf("diagnostics = %q, want silence for a clean teardown", diagnostics.String())
	}

	recordRunCleanupError(t.Context(), errors.New("executor teardown refused"))
	if !strings.Contains(diagnostics.String(), "executor teardown refused") {
		t.Fatalf("diagnostics = %q, want the cleanup failure reported without tracing",
			diagnostics.String())
	}
}

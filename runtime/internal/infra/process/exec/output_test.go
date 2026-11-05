package exec

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestShellSpoolRetainsTheFirstDiagnosticAcrossPages(t *testing.T) {
	shells := unconfinedShells(t)
	t.Cleanup(func() { _ = shells.KillAll() })
	id, err := shells.Launch(t.Context(), "owner", t.TempDir(),
		"printf 'first diagnostic\\n'; head -c 1048576 /dev/zero; printf 'last diagnostic\\n'", Timeout{}, false)
	if err != nil {
		t.Fatal(err)
	}
	sh := mustShell(t, shells, "owner", id)
	file := sh.log
	waitForDone(t, shells, "owner", id)
	var combined strings.Builder
	for {
		page, err := shells.Read("owner", id)
		if err != nil || page.Dropped || !page.Finished || page.Offset != int64(combined.Len()) || len(page.Text) > maxOutputPageBytes {
			t.Fatalf("invalid retained output page: %+v, %v", page, err)
		}
		combined.WriteString(page.Text)
		if page.NextOffset != int64(combined.Len()) {
			t.Fatal("output page offset does not describe the original bytes")
		}
		if !page.HasMore {
			if page.ShellID != "" {
				t.Fatal("fully consumed command retained a handle")
			}
			break
		}
		if page.ShellID != id || len(shells.RetainedForSession("owner")) != 1 {
			t.Fatal("completed command lost its unread original output")
		}
	}
	if !strings.HasPrefix(combined.String(), "first diagnostic\n") || !strings.HasSuffix(combined.String(), "last diagnostic\n") || combined.Len() != 1048576+len("first diagnostic\nlast diagnostic\n") {
		t.Fatal("output before or after the former ring capacity was lost")
	}
	if _, err := file.Stat(); !errors.Is(err, os.ErrClosed) || shells.retainedBytes.Load() != 0 {
		t.Fatalf("final output did not reclaim the spool: %v, charged=%d", err, shells.retainedBytes.Load())
	}
}

func TestShellSpoolCapacityStopsOriginalProcessAndRetainsPrefix(t *testing.T) {
	shells := unconfinedShells(t)
	t.Cleanup(func() { _ = shells.KillAll() })
	id, err := shells.Launch(t.Context(), "owner", t.TempDir(),
		"printf 'first diagnostic\\n'; head -c 9437184 /dev/zero; sleep 30", Timeout{}, false)
	if err != nil {
		t.Fatal(err)
	}
	waitForDone(t, shells, "owner", id)
	var combined strings.Builder
	for {
		page, err := shells.Read("owner", id)
		if !errors.Is(err, ErrShellOutputLimit) || !page.Dropped || !page.Finished || !page.Killed {
			t.Fatalf("capacity silently discarded output or left its process alive: %+v, %v", page, err)
		}
		combined.WriteString(page.Text)
		if !page.HasMore {
			break
		}
	}
	if combined.Len() != maxShellOutputBytes || !strings.HasPrefix(combined.String(), "first diagnostic\n") {
		t.Fatal("capacity failure lost its admitted prefix")
	}
	if len(shells.RetainedForSession("owner")) != 0 || shells.retainedBytes.Load() != 0 {
		t.Fatal("consumed failed command leaked its spool or handle")
	}
}

func TestShellSpoolAggregateQuotaIsReclaimedByTheOriginalSessionOwner(t *testing.T) {
	shells := unconfinedShells(t)
	t.Cleanup(func() { _ = shells.KillAll() })
	page := []byte(strings.Repeat("x", maxShellOutputBytes))
	for range maxRetainedOutputBytes / maxShellOutputBytes {
		id, err := shells.Launch(t.Context(), "full", t.TempDir(), "sleep 30", Timeout{}, false)
		if err != nil {
			t.Fatal(err)
		}
		if n, err := mustShell(t, shells, "full", id).Write(page); err != nil || n != len(page) {
			t.Fatalf("admitted aggregate quota failed: %d, %v", n, err)
		}
	}
	id, err := shells.Launch(t.Context(), "overflow", t.TempDir(), "sleep 30", Timeout{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mustShell(t, shells, "overflow", id).Write([]byte("x")); !errors.Is(err, ErrShellOutputLimit) {
		t.Fatalf("aggregate quota was not enforced: %v", err)
	}
	if err := shells.StopSession("full"); err != nil || shells.retainedBytes.Load() != 0 {
		t.Fatalf("Session teardown retained consumed disk capacity: %v, %d", err, shells.retainedBytes.Load())
	}
	fresh, err := shells.Launch(t.Context(), "fresh", t.TempDir(), "printf restored", Timeout{}, false)
	if err != nil {
		t.Fatal(err)
	}
	waitForDone(t, shells, "fresh", fresh)
	output, err := shells.Read("fresh", fresh)
	if err != nil || output.Text != "restored" {
		t.Fatalf("reclaimed quota was not reusable: %+v, %v", output, err)
	}
	if err := shells.KillAll(); err != nil || shells.retainedBytes.Load() != 0 {
		t.Fatalf("shutdown leaked output: %v, %d", err, shells.retainedBytes.Load())
	}
}

func TestShellSpoolWriteFailureCancelsAndRemainsObservable(t *testing.T) {
	shells := unconfinedShells(t)
	t.Cleanup(func() { _ = shells.KillAll() })
	id, err := shells.Launch(t.Context(), "owner", t.TempDir(), "sleep 30", Timeout{}, false)
	if err != nil {
		t.Fatal(err)
	}
	sh := mustShell(t, shells, "owner", id)
	if err := sh.log.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := sh.Write([]byte("cannot record")); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("disk failure was hidden: %v", err)
	}
	waitForDone(t, shells, "owner", id)
	output, err := shells.Read("owner", id)
	if !errors.Is(err, os.ErrClosed) || !output.Finished || !output.Killed || !output.Dropped {
		t.Fatalf("disk failure did not settle its original process: %+v, %v", output, err)
	}
	if err := shells.StopSession("owner"); err != nil || shells.retainedBytes.Load() != 0 {
		t.Fatalf("disk failure prevented resource cleanup: %v, %d", err, shells.retainedBytes.Load())
	}
}

func TestShellCleanupFailureKeepsLaterPagesReachableWithoutReleasingTheOwner(t *testing.T) {
	shells := unconfinedShells(t)
	id, err := shells.Launch(t.Context(), "owner", t.TempDir(), "head -c 1048576 /dev/zero", Timeout{}, false)
	if err != nil {
		t.Fatal(err)
	}
	sh := mustShell(t, shells, "owner", id)
	waitForDone(t, shells, "owner", id)
	cause := errors.New("descendant cleanup is unconfirmed")
	sh.mu.Lock()
	sh.cleanup = cause
	sh.mu.Unlock()
	t.Cleanup(func() {
		sh.mu.Lock()
		sh.cleanup = nil
		sh.mu.Unlock()
		if err := shells.KillAll(); err != nil {
			t.Error(err)
		}
	})
	var observed int64
	for {
		page, err := shells.Read("owner", id)
		if !errors.Is(err, cause) || page.ShellID != id || page.Offset != observed || !page.Finished {
			t.Fatalf("cleanup failure page: offset=%d next=%d handle=%q, %v", page.Offset, page.NextOffset, page.ShellID, err)
		}
		observed += int64(len(page.Text))
		if observed > 1048576 || page.NextOffset != observed || len(page.Text) == 0 {
			t.Fatal("cleanup failure stalled or replayed a consumed page")
		}
		if !page.HasMore {
			break
		}
	}
	if observed != 1048576 || shells.retainedBytes.Load() != observed || len(shells.RetainedForSession("owner")) != 1 {
		t.Fatal("cleanup failure lost unread output or released unsettled ownership")
	}
}

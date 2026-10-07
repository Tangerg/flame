package exec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/infra/process/sandbox"
)

func TestLaunchRejectsCanceledCallerBeforeDetaching(t *testing.T) {
	shells := unconfinedShells(t)
	t.Cleanup(func() { _ = shells.KillAll() })
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("run stopped before shell launch")
	cancel(cause)
	id, err := shells.Launch(ctx, "session", t.TempDir(), "sleep 30", Timeout{}, false)
	if !errors.Is(err, cause) || id != "" {
		t.Fatalf("canceled launch = (%q, %v), want original cancellation", id, err)
	}
	if retained := shells.RetainedForSession("session"); len(retained) != 0 {
		t.Fatal("canceled caller launched a detached process")
	}
}

func TestShellLookupAndStopRequireTheOwningSession(t *testing.T) {
	shells := unconfinedShells(t)
	t.Cleanup(func() { _ = shells.KillAll() })
	id, err := shells.Launch(t.Context(), "owner", t.TempDir(), "sleep 30", Timeout{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if sh, found := shells.Get("other", id); found || sh != nil {
		t.Fatal("another Session acquired the shell handle")
	}
	if running, err := shells.Kill("other", id); running || !errors.Is(err, ErrShellNotFound) {
		t.Fatalf("another Session stopped shell: running=%t, error=%v", running, err)
	}
	sh := mustShell(t, shells, "owner", id)
	if finished, _ := sh.Status(); finished {
		t.Fatal("refused foreign stop terminated the shell")
	}
	if err := shells.StopSession("owner"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-sh.Done():
	default:
		t.Fatal("host Session cleanup did not join its shell")
	}
}

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

func TestNewShellsRequiresConfinerOutcome(t *testing.T) {
	if _, err := NewShells(nil, nil, false); err == nil {
		t.Fatal("NewShells without a confiner or its absence cause succeeded")
	}
	if _, err := NewShells(nil, sandbox.ErrUnavailable, true); !errors.Is(err, sandbox.ErrUnavailable) {
		t.Fatalf("always-jailed NewShells without a confiner = %v, want its cause", err)
	}
}

// TestShells_RunReadKill drives the background-command lifecycle end to end: a
// command's output is captured and read incrementally, completion is reported,
// and kill stops a still-running shell.
func TestShells_RunReadKill(t *testing.T) {
	shells := unconfinedShells(t)
	t.Cleanup(func() {
		if err := shells.KillAll(); err != nil {
			t.Errorf("KillAll: %v", err)
		}
	})

	// A quick command: capture output + completion.
	id, err := shells.Launch(context.Background(), "", "", "printf hello", Timeout{}, false)
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, shells, id)
	out, _ := mustShell(t, shells, "", id).Read()
	if !strings.Contains(out, "hello") {
		t.Errorf("output = %q, want hello", out)
	}
	done, info := mustShell(t, shells, "", id).Status()
	if !done || info != "exit 0" {
		t.Errorf("status = (%v, %q), want done exit 0", done, info)
	}
	// Second read returns only new output (none) — incremental.
	if out2, _ := mustShell(t, shells, "", id).Read(); out2 != "" {
		t.Errorf("second read = %q, want empty (incremental)", out2)
	}

	// A long-running command: kill it.
	longID, err := shells.Launch(context.Background(), "", "", "sleep 30", Timeout{}, false)
	if err != nil {
		t.Fatal(err)
	}
	running, err := shells.Kill("", longID)
	if err != nil || !running {
		t.Fatalf("kill = (running=%v err=%v), want a running shell stopped", running, err)
	}
	waitDone(t, shells, longID)
	if running2, err := shells.Kill("", longID); err != nil || running2 {
		t.Error("second kill should report not-running")
	}
}

// TestShells_TimeoutKills checks the hard-timeout path: a command outliving
// its timeout is killed, and Outcome reports it as killed with a duration.
func TestShells_TimeoutKills(t *testing.T) {
	shells := unconfinedShells(t)
	t.Cleanup(func() {
		if err := shells.KillAll(); err != nil {
			t.Errorf("KillAll: %v", err)
		}
	})

	id, err := shells.Launch(context.Background(), "", "", "sleep 30", testTimeout(t, 200*time.Millisecond), false)
	if err != nil {
		t.Fatal(err)
	}
	sh := mustShell(t, shells, "", id)
	select {
	case <-sh.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("timed-out command did not finish")
	}
	_, killed, dur, cleanupErr := sh.Outcome()
	if cleanupErr != nil {
		t.Fatalf("Outcome cleanup error = %v", cleanupErr)
	}
	if !killed {
		t.Error("Outcome.killed = false, want true (terminated by timeout)")
	}
	if dur <= 0 {
		t.Errorf("Outcome.duration = %v, want positive", dur)
	}
}

func TestShellsKillAllJoinsProcesses(t *testing.T) {
	shells := unconfinedShells(t)
	id, err := shells.Launch(context.Background(), "", "", "sleep 30", Timeout{}, false)
	if err != nil {
		t.Fatal(err)
	}
	sh := mustShell(t, shells, "", id)

	if err := shells.KillAll(); err != nil {
		t.Fatalf("KillAll: %v", err)
	}
	select {
	case <-sh.Done():
	default:
		t.Fatal("KillAll returned before the process wait goroutine finished")
	}
	if _, ok := shells.Get("", id); ok {
		t.Fatal("KillAll retained a stopped shell")
	}
}

func TestShellsRejectLaunchAfterKillAll(t *testing.T) {
	shells := unconfinedShells(t)
	if err := shells.KillAll(); err != nil {
		t.Fatalf("KillAll: %v", err)
	}
	if _, err := shells.Launch(context.Background(), "", "", "printf late", Timeout{}, false); !errors.Is(err, ErrShellsClosed) {
		t.Fatalf("Launch after KillAll = %v, want ErrShellsClosed", err)
	}
}

func TestShellsKillMissingHasStableIdentity(t *testing.T) {
	shells := unconfinedShells(t)
	if _, err := shells.Kill("", "bg_missing"); !errors.Is(err, ErrShellNotFound) {
		t.Fatalf("Kill missing shell = %v, want ErrShellNotFound", err)
	}
}

func TestShellIdentityDoesNotAliasAcrossOwners(t *testing.T) {
	first := unconfinedShells(t)
	second := unconfinedShells(t)
	t.Cleanup(func() {
		_ = first.KillAll()
		_ = second.KillAll()
	})
	firstID, err := first.Launch(t.Context(), "", "", "true", Timeout{}, false)
	if err != nil {
		t.Fatalf("launch first owner: %v", err)
	}
	secondID, err := second.Launch(t.Context(), "", "", "true", Timeout{}, false)
	if err != nil {
		t.Fatalf("launch second owner: %v", err)
	}
	if firstID == secondID {
		t.Fatalf("different Shells owners minted the same identity %q", firstID)
	}
	if _, found := second.Get("", firstID); found {
		t.Fatalf("second owner resolved first owner's identity %q", firstID)
	}
}

func TestShellIdentityCodecAndExhaustion(t *testing.T) {
	valid := newShellID(strings.Repeat("A", shellIDEpochBytes), 1)
	if parsed, ok := parseShellID(valid.String()); !ok || parsed != valid {
		t.Fatalf("parseShellID(%q) = %q, %t; want exact identity", valid.String(), parsed.String(), ok)
	}
	for _, invalid := range []string{"", "bg_1", "bg_" + strings.Repeat("A", shellIDEpochBytes) + "_0", "bg_" + strings.Repeat("a", shellIDEpochBytes) + "_1"} {
		if parsed, ok := parseShellID(invalid); ok || parsed != (shellID{}) {
			t.Errorf("parseShellID(%q) = %q, %t; want zero/false", invalid, parsed.String(), ok)
		}
	}
	shells := unconfinedShells(t)
	shells.nextID = ^uint64(0)
	if _, err := shells.Launch(t.Context(), "", "", "true", Timeout{}, false); !errors.Is(err, ErrShellIdentityExhausted) {
		t.Fatalf("Launch after identity exhaustion = %v, want ErrShellIdentityExhausted", err)
	}
}

func TestShellsFailedLaunchCanBeShutDown(t *testing.T) {
	shells := unconfinedShells(t)
	id, err := shells.Launch(t.Context(), "", t.TempDir()+"/missing", "printf unreachable", Timeout{}, false)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if id == "" {
		t.Fatal("failed launch has no shell identity")
	}
	if err := shells.KillAll(); err != nil {
		t.Fatalf("KillAll after failed launch: %v", err)
	}
}

func TestShellsLaunchRacesKillAll(t *testing.T) {
	for range 25 {
		shells := unconfinedShells(t)
		result := make(chan struct {
			id  string
			err error
		}, 1)
		go func() {
			id, err := shells.Launch(context.Background(), "", "", "sleep 30", Timeout{}, false)
			result <- struct {
				id  string
				err error
			}{id: id, err: err}
		}()
		if err := shells.KillAll(); err != nil {
			t.Errorf("KillAll: %v", err)
		}
		got := <-result
		if got.err != nil && !errors.Is(got.err, ErrShellsClosed) {
			t.Fatalf("Launch error = %v", got.err)
		}
		if got.id != "" {
			if _, ok := shells.Get("", got.id); ok {
				t.Fatalf("KillAll retained racing shell %q", got.id)
			}
		}
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

// Completed commands remain addressable until their final output is consumed.
func TestShells_RetainedForSession(t *testing.T) {
	shells := unconfinedShells(t)
	t.Cleanup(func() { _ = shells.KillAll() })

	if _, err := shells.Launch(context.Background(), "sess-a", "", "sleep 30", Timeout{}, false); err != nil {
		t.Fatalf("launch a1: %v", err)
	}
	if _, err := shells.Launch(context.Background(), "sess-a", "", "sleep 30", Timeout{}, false); err != nil {
		t.Fatalf("launch a2: %v", err)
	}
	bID, err := shells.Launch(context.Background(), "sess-b", "", "sleep 30", Timeout{}, false)
	if err != nil {
		t.Fatalf("launch b: %v", err)
	}

	if got := shells.RetainedForSession("sess-a"); len(got) != 2 {
		t.Fatalf("session a running = %d, want 2", len(got))
	}
	if got := shells.RetainedForSession("sess-a")[0].Command; got != "sleep 30" {
		t.Fatalf("running shell command = %q, want %q", got, "sleep 30")
	}
	if got := shells.RetainedForSession("other"); len(got) != 0 {
		t.Fatalf("unknown session running = %d, want 0", len(got))
	}

	// Stopping a command does not discard its unread final output.
	if _, err := shells.Kill("sess-b", bID); err != nil {
		t.Fatalf("kill b: %v", err)
	}
	waitForDone(t, shells, "sess-b", bID)
	if got := shells.RetainedForSession("sess-b"); len(got) != 1 || got[0].ID != bID {
		t.Fatalf("session b lost its unread command: %+v", got)
	}
	shells.Remove(bID)
	if got := shells.RetainedForSession("sess-b"); len(got) != 0 {
		t.Fatalf("session b after release = %d, want 0", len(got))
	}
}

func TestShellsStopOwnedProcesses(t *testing.T) {
	shells := unconfinedShells(t)
	t.Cleanup(func() { _ = shells.KillAll() })
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()

	rootID, err := shells.Launch(t.Context(), "owner", root, "sleep 30", Timeout{}, false)
	if err != nil {
		t.Fatal(err)
	}
	nestedID, err := shells.Launch(t.Context(), "sibling", nested, "sleep 30", Timeout{}, false)
	if err != nil {
		t.Fatal(err)
	}
	outsideID, err := shells.Launch(t.Context(), "owner", outside, "sleep 30", Timeout{}, false)
	if err != nil {
		t.Fatal(err)
	}
	rootShell := mustShell(t, shells, "owner", rootID)
	nestedShell := mustShell(t, shells, "sibling", nestedID)

	if err := shells.StopWorkspace(root); err != nil {
		t.Fatalf("StopWorkspace: %v", err)
	}
	for id, sh := range map[string]*Shell{rootID: rootShell, nestedID: nestedShell} {
		if _, exists := shells.Get(sh.sessionID, id); exists {
			t.Fatalf("StopWorkspace retained shell %s", id)
		}
		select {
		case <-sh.Done():
		default:
			t.Fatalf("StopWorkspace returned before shell %s joined", id)
		}
	}
	if _, exists := shells.Get("owner", outsideID); !exists {
		t.Fatal("StopWorkspace removed a shell outside its tree")
	}

	if err := shells.StopSession("owner"); err != nil {
		t.Fatalf("StopSession: %v", err)
	}
	if _, exists := shells.Get("owner", outsideID); exists {
		t.Fatal("StopSession retained its shell outside the restored tree")
	}
}

func waitForDone(t *testing.T, shells *Shells, sessionID, id string) {
	t.Helper()
	sh, ok := shells.Get(sessionID, id)
	if !ok {
		return
	}
	select {
	case <-sh.done:
	case <-time.After(2 * time.Second):
		t.Fatalf("shell %q did not finish after kill", id)
	}
}

// TestShellsStopEveryAliasOfTheRestoredTree pins the guarantee a destructive
// working-tree restore depends on: no detached process below that tree survives
// to rewrite the restored files. A sibling Session can hold the same tree
// through a symlink, and comparing the two spellings as text answers that the
// shell is somewhere else.
func TestShellsStopEveryAliasOfTheRestoredTree(t *testing.T) {
	shells := unconfinedShells(t)
	t.Cleanup(func() { _ = shells.KillAll() })

	tree := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(tree, alias); err != nil {
		t.Fatal(err)
	}

	aliasID, err := shells.Launch(t.Context(), "sibling", alias, "sleep 30", Timeout{}, false)
	if err != nil {
		t.Fatal(err)
	}
	aliasShell := mustShell(t, shells, "sibling", aliasID)

	if err := shells.StopWorkspace(tree); err != nil {
		t.Fatalf("StopWorkspace: %v", err)
	}
	if _, exists := shells.Get("sibling", aliasID); exists {
		t.Fatal("a shell inside the restored tree survived because its cwd spells the tree through a symlink")
	}
	select {
	case <-aliasShell.Done():
	default:
		t.Fatal("StopWorkspace returned before the aliased shell joined")
	}
}

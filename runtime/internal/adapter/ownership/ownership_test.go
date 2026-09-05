package ownership

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	appownership "github.com/Tangerg/flame/runtime/internal/application/ownership"
)

func TestLeaseSetsShareSessionAndGoalDriveOwnership(t *testing.T) {
	data := t.TempDir()
	first, err := New(data)
	if err != nil {
		t.Fatalf("New first: %v", err)
	}
	second, err := New(data)
	if err != nil {
		t.Fatalf("New second: %v", err)
	}

	sessionLease, ok, _ := first.TrySession("session-1")
	if !ok {
		t.Fatal("first Session lease was refused")
	}
	if _, trySessionOk, _ := second.TrySession("session-1"); trySessionOk {
		t.Fatal("second lease set acquired the same Session writer")
	}
	if other, trySessionOk, _ := second.TrySession("session-2"); !trySessionOk {
		t.Fatal("unrelated Session was blocked")
	} else {
		other.Release()
	}
	sessionLease.Release()
	if next, trySessionOk, _ := second.TrySession("session-1"); !trySessionOk {
		t.Fatal("Session writer did not transfer after release")
	} else {
		next.Release()
	}

	drive, ok, _ := first.TryGoalDrive("session-1")
	if !ok {
		t.Fatal("first Goal drive lease was refused")
	}
	if _, tryGoalDriveOk, _ := second.TryGoalDrive("session-1"); tryGoalDriveOk {
		t.Fatal("second lease set acquired the same Goal drive")
	}
	drive.Release()

	sweep, ok, _ := first.TryRecoverySweep()
	if !ok {
		t.Fatal("first recovery sweep lease was refused")
	}
	if _, ok, _ := second.TryRecoverySweep(); ok {
		t.Fatal("second lease set acquired the same recovery sweep")
	}
	sweep.Release()
}

func TestWorkingTreeRunsSharePhysicalIdentityAndExcludeMutation(t *testing.T) {
	data := t.TempDir()
	first, err := New(data)
	if err != nil {
		t.Fatalf("New first: %v", err)
	}
	second, err := New(data)
	if err != nil {
		t.Fatalf("New second: %v", err)
	}
	cwd := t.TempDir()
	alias := filepath.Join(t.TempDir(), "workspace")
	if err := os.Symlink(cwd, alias); err != nil {
		t.Fatalf("create workspace alias: %v", err)
	}

	runA, ok, _ := first.TryWorkingTree(cwd, true)
	if !ok {
		t.Fatal("first shared working-tree lease was refused")
	}
	runB, ok, _ := second.TryWorkingTree(alias, true)
	if !ok {
		t.Fatal("second shared working-tree lease through alias was refused")
	}
	if _, tryWorkingTreeOk, _ := second.TryWorkingTree(cwd, false); tryWorkingTreeOk {
		t.Fatal("destructive mutation crossed active Run leases")
	}
	runA.Release()
	if _, tryWorkingTreeOk, _ := first.TryWorkingTree(cwd, false); tryWorkingTreeOk {
		t.Fatal("destructive mutation crossed the remaining Run lease")
	}
	runB.Release()
	mutation, ok, _ := second.TryWorkingTree(alias, false)
	if !ok {
		t.Fatal("destructive mutation did not acquire after Runs released")
	}
	if _, ok, _ := first.TryWorkingTree(cwd, true); ok {
		t.Fatal("Run crossed active destructive mutation")
	}
	mutation.Release()
}

func TestSessionOwnershipTransfersAfterProcessKill(t *testing.T) {
	const childEnvironment = "FLAME_TEST_RUNTIME_OWNERSHIP_CHILD"
	if os.Getenv(childEnvironment) == "1" {
		leases, err := New(os.Getenv("FLAME_TEST_RUNTIME_OWNERSHIP_DATA"))
		if err != nil {
			t.Fatal(err)
		}
		lease, ok, _ := leases.TrySession("session-after-crash")
		if !ok {
			t.Fatal("child could not acquire Session ownership")
		}
		if err := os.WriteFile(os.Getenv("FLAME_TEST_RUNTIME_OWNERSHIP_READY"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		var oneByte [1]byte
		_, _ = os.Stdin.Read(oneByte[:])
		lease.Release()
		return
	}

	data := t.TempDir()
	ready := filepath.Join(t.TempDir(), "ready")
	command := exec.Command(os.Args[0], "-test.run=^TestSessionOwnershipTransfersAfterProcessKill$")
	command.Env = append(os.Environ(),
		childEnvironment+"=1",
		"FLAME_TEST_RUNTIME_OWNERSHIP_DATA="+data,
		"FLAME_TEST_RUNTIME_OWNERSHIP_READY="+ready,
	)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdin.Close() }()
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if startErr := command.Start(); startErr != nil {
		t.Fatal(startErr)
	}
	t.Cleanup(func() { _ = command.Process.Kill() })
	waitForFile(t, ready, &output)

	leases, err := New(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := leases.TrySession("session-after-crash"); ok {
		t.Fatal("parent acquired Session ownership while child was alive")
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("killed child exited successfully")
	}

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if lease, ok, _ := leases.TrySession("session-after-crash"); ok {
			lease.Release()
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("Session ownership did not transfer after process death: %s", output.String())
		}
	}
}

func waitForFile(t *testing.T, path string, childOutput *bytes.Buffer) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("ownership child did not become ready: %s", childOutput.String())
		}
	}
}

func TestLeaseAcquisitionPreservesFilesystemFailure(t *testing.T) {
	data := t.TempDir()
	leases, err := New(data)
	if err != nil {
		t.Fatal(err)
	}
	cwd := t.TempDir()
	if err := os.RemoveAll(data); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		acquire func() (appownership.Lease, bool, error)
	}{
		{"session", func() (appownership.Lease, bool, error) { return leases.TrySession("ses_1") }},
		{"working tree", func() (appownership.Lease, bool, error) { return leases.TryWorkingTree(cwd, false) }},
		{"Goal drive", func() (appownership.Lease, bool, error) { return leases.TryGoalDrive("ses_1") }},
		{"recovery", leases.TryRecoverySweep},
	} {
		t.Run(test.name, func(t *testing.T) {
			lease, acquired, err := test.acquire()
			if lease != nil || acquired || !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("acquisition = (%v, %t, %v), want filesystem cause", lease, acquired, err)
			}
		})
	}
}

func TestWorkingTreeIdentityFailureIsNotContention(t *testing.T) {
	leases, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	loop := filepath.Join(t.TempDir(), "loop")
	if err := os.Symlink("loop", loop); err != nil {
		t.Skipf("symlink: %v", err)
	}
	lease, acquired, err := leases.TryWorkingTree(loop, true)
	if lease != nil || acquired || err == nil {
		t.Fatalf("cyclic path acquisition = (%v, %t, %v), want identity failure", lease, acquired, err)
	}
}

// Package ownership maps application ownership identities to cross-process
// advisory leases rooted in one shared Runtime data directory.
package ownership

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/runtime/internal/application/automation/goals"
	appownership "github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/infra/advisorylock"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/pathidentity"
)

const ownershipDirectory = "ownership"

// LeaseSet owns the stable lock-file layout shared by every Runtime process
// using the same data directory. Lock files carry no ownership state; the OS
// lock on their first byte is authoritative and is released on process death.
type LeaseSet struct {
	sessions     string
	workingTrees string
	goalDrives   string
	recovery     string
}

// New prepares the private lock roots for one canonical data directory.
func New(dataDirectory string) (*LeaseSet, error) {
	if dataDirectory == "" || !filepath.IsAbs(dataDirectory) {
		return nil, errors.New("runtime ownership: absolute data directory is required")
	}
	root := filepath.Join(filepath.Clean(dataDirectory), ownershipDirectory)
	leases := &LeaseSet{
		sessions:     filepath.Join(root, "sessions"),
		workingTrees: filepath.Join(root, "working-trees"),
		goalDrives:   filepath.Join(root, "goal-drives"),
		recovery:     filepath.Join(root, "recovery"),
	}
	for _, directory := range []string{root, leases.sessions, leases.workingTrees, leases.goalDrives, leases.recovery} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, fmt.Errorf("runtime ownership: create %q: %w", directory, err)
		}
		if err := os.Chmod(directory, 0o700); err != nil {
			return nil, fmt.Errorf("runtime ownership: protect %q: %w", directory, err)
		}
	}
	return leases, nil
}

// TrySession acquires the exclusive writer lease for one Session.
func (l *LeaseSet) TrySession(sessionID string) (appownership.Lease, bool) {
	return tryLease(l.sessions, sessionID, false)
}

// TryWorkingTree acquires a shared Run lease or exclusive destructive-mutation
// lease for one physical working-tree identity.
func (l *LeaseSet) TryWorkingTree(cwd string, shared bool) (appownership.Lease, bool) {
	physical, err := pathidentity.Resolve("", cwd)
	if err != nil {
		return nil, false
	}
	return tryLease(l.workingTrees, physical, shared)
}

// TryGoalDrive acquires the single autonomous driver lease for one Session.
func (l *LeaseSet) TryGoalDrive(sessionID string) (goals.DriveLease, bool) {
	return tryLease(l.goalDrives, sessionID, false)
}

// TryRecoverySweep elects one Runtime to reconcile abandoned Runs before Goals.
func (l *LeaseSet) TryRecoverySweep() (appownership.Lease, bool) {
	lease, err := advisorylock.TryDirectory(l.recovery)
	if err != nil {
		return nil, false
	}
	return advisoryLease{lease: lease}, true
}

// AcquireRecoverySweep waits for the ordered startup recovery owner.
func (l *LeaseSet) AcquireRecoverySweep(ctx context.Context) (appownership.Lease, error) {
	lease, err := advisorylock.AcquireDirectory(ctx, l.recovery)
	if err != nil {
		return nil, err
	}
	return advisoryLease{lease: lease}, nil
}

type advisoryLease struct{ lease *advisorylock.Lease }

func (a advisoryLease) Release() { _ = a.lease.Release() }

type fileLease struct {
	file  *os.File
	lease *advisorylock.Lease
}

func (f *fileLease) Release() {
	if f == nil {
		return
	}
	// The descriptor is an independent OS-level release path. Always close it:
	// retaining it after an unlock error can retain ownership until process exit.
	_ = f.lease.Release()
	_ = f.file.Close()
}

func tryLease(directory, identity string, shared bool) (*fileLease, bool) {
	if identity == "" {
		return nil, false
	}
	digest := sha256.Sum256([]byte(identity))
	path := filepath.Join(directory, hex.EncodeToString(digest[:])+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false
	}
	var lease *advisorylock.Lease
	if shared {
		lease, err = advisorylock.TrySharedFile(file)
	} else {
		lease, err = advisorylock.TryFile(file)
	}
	if err != nil {
		_ = file.Close()
		return nil, false
	}
	return &fileLease{file: file, lease: lease}, true
}

var (
	_ appownership.AdmissionBackend = (*LeaseSet)(nil)
	_ goals.DriveOwnership          = (*LeaseSet)(nil)
	_ appownership.RecoveryBackend  = (*LeaseSet)(nil)
)

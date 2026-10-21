// Package ownership maps application ownership identities to kernel leases
// within Runtime's exclusively owned data directory.
package ownership

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/runtime/internal/application/goals"
	appownership "github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/pathidentity"
	"github.com/Tangerg/flame/runtime/storage/advisorylock"
)

const ownershipDirectory = "ownership"

// LeaseSet owns the stable lock-file layout for Runtime's logical owners.
// Lock files carry no ownership state; kernel leases end on release or process death.
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
func (l *LeaseSet) TrySession(sessionID string) (appownership.Lease, bool, error) {
	return tryLease(l.sessions, sessionID, false)
}

// TryWorkingTree acquires a shared Run lease or exclusive destructive-mutation
// lease for one physical working-tree identity.
func (l *LeaseSet) TryWorkingTree(cwd string, shared bool) (appownership.Lease, bool, error) {
	physical, err := pathidentity.Resolve("", cwd)
	if err != nil {
		return nil, false, fmt.Errorf("runtime ownership: resolve working tree: %w", err)
	}
	return tryLease(l.workingTrees, physical, shared)
}

// TryGoalDrive acquires the single autonomous driver lease for one Session.
func (l *LeaseSet) TryGoalDrive(sessionID string) (goals.DriveLease, bool, error) {
	return tryLease(l.goalDrives, sessionID, false)
}

// TryRecoverySweep admits one ordered reconciliation of Runs before Goals.
func (l *LeaseSet) TryRecoverySweep() (appownership.Lease, bool, error) {
	lease, err := advisorylock.TryDirectory(l.recovery)
	if err != nil {
		return nil, false, fmt.Errorf("runtime ownership: recovery sweep: %w", err)
	}
	if lease == nil {
		return nil, false, nil
	}
	return advisoryLease{lease: lease}, true, nil
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

func (a advisoryLease) Release() {
	if err := a.lease.Release(); err != nil {
		slog.Error("runtime ownership: release lease", "error", err)
	}
}

func tryLease(directory, identity string, shared bool) (appownership.Lease, bool, error) {
	if identity == "" {
		return nil, false, errors.New("runtime ownership: lease identity is required")
	}
	digest := sha256.Sum256([]byte(identity))
	path := filepath.Join(directory, hex.EncodeToString(digest[:])+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, err
	}
	var lease *advisorylock.Lease
	if shared {
		lease, err = advisorylock.TrySharedFile(file)
	} else {
		lease, err = advisorylock.TryFile(file)
	}
	if err != nil {
		return nil, false, fmt.Errorf("runtime ownership: acquire lease: %w", err)
	}
	if lease == nil {
		return nil, false, nil
	}
	return advisoryLease{lease: lease}, true, nil
}

var (
	_ appownership.AdmissionBackend = (*LeaseSet)(nil)
	_ goals.DriveOwnership          = (*LeaseSet)(nil)
	_ appownership.RecoveryBackend  = (*LeaseSet)(nil)
)

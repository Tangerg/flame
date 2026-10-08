//go:build unix

package pluginpackage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/runtime/internal/infra/advisorylock"
)

func TestExecutionPreparationCancellationIsLocalToTheCaller(t *testing.T) {
	for _, cause := range []error{context.Canceled, errors.New("connection superseded")} {
		t.Run(cause.Error(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r, _, release := executionFixture(t)
				root, err := r.currentRoot(t.Context(), release.Digest())
				if err != nil {
					t.Fatal(err)
				}
				defer root.Close()
				if err := os.MkdirAll(r.executionDirectory(), 0700); err != nil {
					t.Fatal(err)
				}
				namespace, err := advisorylock.AcquireDirectory(t.Context(), r.executionDirectory())
				if err != nil {
					t.Fatal(err)
				}
				defer namespace.Release()
				ctx, cancel := context.WithCancelCause(t.Context())
				defer cancel(context.Canceled)
				leader := make(chan error, 1)
				go func() {
					lease, err := r.execution(ctx, release.Digest(), root)
					if lease != nil {
						err = errors.Join(err, lease.Close())
					}
					leader <- err
				}()
				synctest.Wait()
				type outcome struct {
					lease *executionLease
					err   error
				}
				peer := make(chan outcome, 1)
				go func() {
					lease, err := r.execution(t.Context(), release.Digest(), root)
					peer <- outcome{lease, err}
				}()
				synctest.Wait()
				cancel(cause)
				if err := <-leader; !errors.Is(err, cause) {
					t.Fatalf("canceled preparation lost its cause: %v", err)
				}
				if err := namespace.Release(); err != nil {
					t.Fatal(err)
				}
				result := <-peer
				if result.err != nil || result.lease == nil {
					t.Fatalf("a canceled preparation decided a live peer's outcome: %v", result.err)
				}
				defer result.lease.Close()
				if content, err := os.ReadFile(filepath.Join(result.lease.path, "backend.sh")); err != nil || string(content) != "original" {
					t.Fatalf("surviving caller did not acquire admitted execution content: %q, %v", content, err)
				}
				if err := result.lease.Close(); err != nil {
					t.Fatal(err)
				}
				entries, err := os.ReadDir(r.executionDirectory())
				if err != nil || len(entries) != 0 {
					t.Fatalf("retirement retained execution content: %d, %v", len(entries), err)
				}
			})
		})
	}
}

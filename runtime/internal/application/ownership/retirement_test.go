package ownership

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
)

func TestForegroundAdmissionSettlesCommittedRetirementOnceCleanupRecovers(t *testing.T) {
	var cleanupFails atomic.Bool
	cleanupFails.Store(true)
	var attempts atomic.Int32
	gate, err := NewGate(GateConfig{
		TreesOverlap: filesystem.PathsOverlap,
		ResolveTree:  func(path string) (string, error) { return path, nil },
		SettleRetirement: func(_ context.Context, sessionID string) (bool, error) {
			if sessionID != "ses_1" {
				return true, nil
			}
			attempts.Add(1)
			return !cleanupFails.Load(), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if release, ok, err := gate.AcquireSession(t.Context(), "ses_1"); release != nil || ok || err != nil {
		t.Fatalf("Session admission with a failing retirement = %t, %v", ok, err)
	}
	if admission, ok, err := gate.AcquireRun(t.Context(), "ses_1", "/work"); ok || err != nil {
		admission.Release()
		t.Fatalf("Run admission with a failing retirement = %t, %v", ok, err)
	}
	other, ok, err := gate.AcquireRun(t.Context(), "ses_other", "/work")
	if err != nil || !ok {
		t.Fatalf("unrelated Session = %t, %v", ok, err)
	}
	other.Release()
	cleanupFails.Store(false)
	admission, ok, err := gate.AcquireRun(t.Context(), "ses_1", "/work")
	admission.Release()
	if err != nil || !ok {
		t.Fatalf("recovered cleanup still fences the Session: %t, %v", ok, err)
	}
	if attempts.Load() != 3 {
		t.Fatalf("settlement attempts = %d, want one per exclusive admission", attempts.Load())
	}
}

func TestRetirementReadOwnsLocalClaimWithoutHoldingGlobalLock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered, releaseRead := make(chan struct{}), make(chan struct{})
		var pending atomic.Bool
		gate, err := NewGate(GateConfig{
			TreesOverlap: filesystem.PathsOverlap,
			ResolveTree:  func(path string) (string, error) { return path, nil },
			SettleRetirement: func(_ context.Context, sessionID string) (bool, error) {
				if sessionID == "ses_1" {
					close(entered)
					<-releaseRead
					return !pending.Load(), nil
				}
				return true, nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		result := make(chan bool, 1)
		go func() {
			admission, ok, err := gate.AcquireRun(t.Context(), "ses_1", "/work")
			if err != nil {
				t.Error(err)
			}
			admission.Release()
			result <- ok
		}()
		<-entered
		if release, ok, err := gate.AcquireRecoverySession("ses_1"); release != nil || ok || err != nil {
			t.Fatalf("retirement read left Session unreserved: %t, %v", ok, err)
		}
		other := make(chan bool, 1)
		go func() {
			release, ok, err := gate.AcquireSession(t.Context(), "ses_other")
			if err != nil {
				t.Error(err)
			}
			if ok {
				release()
			}
			other <- ok
		}()
		synctest.Wait()
		select {
		case ok := <-other:
			if !ok {
				t.Fatal("unrelated Session was refused")
			}
		default:
			t.Fatal("global admission lock crossed the durable read")
		}
		pending.Store(true)
		close(releaseRead)
		if <-result {
			t.Fatal("Run admitted after its durable retirement appeared")
		}
		release, ok, err := gate.AcquireRecoverySession("ses_1")
		if err != nil || !ok {
			t.Fatalf("failed admission leaked local claim: %t, %v", ok, err)
		}
		release()
	})
}

func TestRetirementReadFailureReleasesBothAdmissionKinds(t *testing.T) {
	failure := errors.New("retirement catalog unavailable")
	gate, err := NewGate(GateConfig{
		TreesOverlap:     filesystem.PathsOverlap,
		ResolveTree:      func(path string) (string, error) { return path, nil },
		SettleRetirement: func(context.Context, string) (bool, error) { return false, failure },
	})
	if err != nil {
		t.Fatal(err)
	}
	if release, ok, err := gate.AcquireSession(t.Context(), "ses_1"); release != nil || ok || !errors.Is(err, failure) {
		t.Fatalf("Session read failure = %t, %v", ok, err)
	}
	if admission, ok, err := gate.AcquireRun(t.Context(), "ses_1", "/work"); ok || !errors.Is(err, failure) {
		admission.Release()
		t.Fatalf("Run read failure = %t, %v", ok, err)
	}
	release, ok, err := gate.AcquireRecoverySession("ses_1")
	if err != nil || !ok {
		t.Fatalf("read failures leaked claims: %t, %v", ok, err)
	}
	release()
}

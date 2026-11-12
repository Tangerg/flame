package client

import (
	"context"
	"io"
	"testing"
	"testing/synctest"
)

type heldResponseBody struct {
	entered chan struct{}
	release chan struct{}
}

func (b *heldResponseBody) Read([]byte) (int, error) { return 0, io.EOF }
func (b *heldResponseBody) Close() error {
	close(b.entered)
	<-b.release
	return nil
}

func TestClientCloseJoinsCancellationCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		owner, err := Connect(t.Context(), RemoteConfig{Endpoint: "http://127.0.0.1"})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		call, err := owner.begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		body := &heldResponseBody{entered: make(chan struct{}), release: make(chan struct{})}
		if err := call.attach(body); err != nil {
			t.Fatal(err)
		}
		cancel()
		<-body.entered
		closed := make(chan struct{})
		go func() {
			if err := owner.Close(); err != nil {
				t.Error(err)
			}
			close(closed)
		}()
		synctest.Wait()
		select {
		case <-closed:
			t.Error("client close returned before its canceled response body closed")
		default:
		}
		close(body.release)
		<-closed
		synctest.Wait()
	})
}

func TestConcurrentClientCloseJoinsOriginalCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		owner, err := Connect(t.Context(), RemoteConfig{Endpoint: "http://127.0.0.1"})
		if err != nil {
			t.Fatal(err)
		}
		call, err := owner.begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		body := &heldResponseBody{entered: make(chan struct{}), release: make(chan struct{})}
		if err := call.attach(body); err != nil {
			t.Fatal(err)
		}
		closeClient := func() <-chan struct{} {
			done := make(chan struct{})
			go func() {
				if err := owner.Close(); err != nil {
					t.Error(err)
				}
				close(done)
			}()
			return done
		}
		first := closeClient()
		<-body.entered
		second := closeClient()
		synctest.Wait()
		select {
		case <-second:
			t.Error("concurrent close returned before the original body cleanup completed")
		default:
		}
		if _, err := owner.begin(t.Context()); err != ErrClosed {
			t.Errorf("request admitted during close: %v", err)
		}
		close(body.release)
		<-first
		<-second
		if err := owner.Close(); err != nil {
			t.Fatal(err)
		}
	})
}

package lsp

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
)

func TestDisconnectedClientUsesOneJoinedReplacement(t *testing.T) {
	servers := mustNewServers(t, nil)
	first, remote := connectedTestClient(t, replyEmpty)
	second, _ := connectedTestClient(t, replyEmpty)
	started := make(chan struct{})
	release := make(chan struct{})
	releaseStart := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseStart)
	var launches atomic.Int32
	servers.launch = func(context.Context, ServerSpec, string) (*client, error) {
		if launches.Add(1) == 1 {
			return first, nil
		}
		close(started)
		<-release
		return second, nil
	}
	root := t.TempDir()
	spec := ServerSpec{Name: "test"}
	if _, err := servers.clientFor(t.Context(), root, spec); err != nil {
		t.Fatal(err)
	}
	_ = remote.Close()
	<-first.conn.DisconnectNotify()
	type result struct {
		client *client
		err    error
	}
	results := make(chan result, 8)
	for range cap(results) {
		go func() {
			c, err := servers.clientFor(t.Context(), root, spec)
			results <- result{c, err}
		}()
	}
	select {
	case <-started:
	case got := <-results:
		t.Fatalf("query reused disconnected client %p: %v", got.client, got.err)
	}
	releaseStart()
	for range cap(results) {
		got := <-results
		if got.client != second || got.err != nil {
			t.Fatalf("replacement = %p, %v; want %p", got.client, got.err, second)
		}
	}
	if launches.Load() != 2 {
		t.Fatalf("launches = %d, want initial and one replacement", launches.Load())
	}
	_ = first.close()
	if c, err := servers.clientFor(t.Context(), root, spec); err != nil || c != second {
		t.Fatalf("old retirement invalidated replacement: %p, %v", c, err)
	}
	if err := servers.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := servers.clientFor(t.Context(), root, spec); !errors.Is(err, errClosed) || launches.Load() != 2 {
		t.Fatalf("closed servers restarted: launches=%d, %v", launches.Load(), err)
	}
}

func TestReplacementJoinsRetirementAndCloseCanStopIt(t *testing.T) {
	for _, closing := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			servers := mustNewServers(t, nil)
			first, remote := connectedTestClient(t, replyEmpty)
			second, _ := connectedTestClient(t, replyEmpty)
			end := make(chan processEnd)
			first.wait = end
			var launches atomic.Int32
			servers.launch = func(context.Context, ServerSpec, string) (*client, error) {
				if launches.Add(1) == 1 {
					return first, nil
				}
				return second, nil
			}
			root, spec := t.TempDir(), ServerSpec{Name: "test"}
			if _, err := servers.clientFor(t.Context(), root, spec); err != nil {
				t.Fatal(err)
			}
			_ = remote.Close()
			<-first.conn.DisconnectNotify()
			result := make(chan error, 1)
			go func() {
				_, err := servers.clientFor(t.Context(), root, spec)
				result <- err
			}()
			synctest.Wait()
			if launches.Load() != 1 {
				t.Fatal("replacement started before the old process joined")
			}
			closed := make(chan error, 1)
			if closing {
				go func() { closed <- servers.Close() }()
				synctest.Wait()
				select {
				case err := <-closed:
					t.Fatalf("Close crossed retirement: %v", err)
				default:
				}
			}
			end <- processEnd{}
			close(end)
			err := <-result
			if closing {
				if !errors.Is(err, errClosed) || launches.Load() != 1 {
					t.Fatalf("replacement crossed Close: launches=%d, %v", launches.Load(), err)
				}
				if err := <-closed; err != nil {
					t.Fatal(err)
				}
			} else if err != nil || launches.Load() != 2 {
				t.Fatalf("joined replacement: launches=%d, %v", launches.Load(), err)
			}
		})
	}
}

func TestRetirementDistinguishesExitedProcessFromCleanupFailure(t *testing.T) {
	t.Setenv("FLAME_LSP_EXIT_FIXTURE", "1")
	exit := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestLSPExitFixture$").Run()
	if _, exited := errors.AsType[*exec.ExitError](exit); !exited {
		t.Fatalf("process fixture = %v, want nonzero exit", exit)
	}
	for _, cleanup := range []error{nil, errors.New("process group could not retire")} {
		c, remote := connectedTestClient(t, replyEmpty)
		end := make(chan processEnd, 1)
		end <- processEnd{exitErr: exit, cleanupErr: cleanup}
		close(end)
		c.wait = end
		_ = remote.Close()
		<-c.conn.DisconnectNotify()
		if err := c.retire(); !errors.Is(err, cleanup) {
			t.Fatalf("retirement = %v, want cleanup result %v", err, cleanup)
		}
		if err := c.close(); !errors.Is(err, exit) || cleanup != nil && !errors.Is(err, cleanup) {
			t.Fatalf("shutdown lost process evidence: %v", err)
		}
	}
}

func TestLSPExitFixture(_ *testing.T) {
	if os.Getenv("FLAME_LSP_EXIT_FIXTURE") == "1" {
		os.Exit(17)
	}
}

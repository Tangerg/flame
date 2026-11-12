package lsp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/sourcegraph/jsonrpc2"
)

func TestCloseDeadlineInterruptsBlockedNotification(t *testing.T) {
	for _, closeFails := range []bool{false, true} {
		t.Run(fmt.Sprintf("close_fails=%t", closeFails), func(t *testing.T) {
			agent, server := net.Pipe()
			writer := &blockedNotificationConn{Conn: agent, started: make(chan struct{})}
			closeFailure := errors.New("stdio close failed")
			if closeFails {
				writer.closeErr = closeFailure
			}
			defer agent.Close()
			defer server.Close()
			lifetime, cancel := context.WithCancel(t.Context())
			defer cancel()
			process := make(chan processEnd, 1)
			stopped := false
			c := &client{
				spec:         ServerSpec{Name: "blocked"},
				cancel:       cancel,
				shutdownBase: t.Context(),
				wait:         process,
				stopProcess: func() error {
					stopped = true
					process <- processEnd{}
					close(process)
					return nil
				},
			}
			c.conn = jsonrpc2.NewConn(lifetime, jsonrpc2.NewBufferedStream(writer, lspObjectCodec{}), jsonrpc2.AsyncHandler(c))
			written := make(chan error, 1)
			go func() { written <- c.conn.Notify(lifetime, "textDocument/didOpen", struct{}{}) }()
			<-writer.started
			ended := make(chan error, 1)
			go func() { ended <- c.close() }()
			deadline := time.NewTimer(clientShutdownTimeout + 3*time.Second)
			defer deadline.Stop()
			select {
			case err := <-ended:
				if !errors.Is(err, context.DeadlineExceeded) || !stopped || errors.Is(err, closeFailure) != closeFails {
					t.Fatalf("shutdown = %v; child stopped = %t", err, stopped)
				}
			case <-deadline.C:
				server.Close()
				<-ended
				<-written
				t.Fatal("shutdown deadline could not interrupt the blocked writer")
			}
			if err := <-written; err == nil {
				t.Fatal("blocked notification reported successful delivery")
			}
			if writer.closes != 1 {
				t.Fatalf("stdio closed %d times", writer.closes)
			}
		})
	}
}

type blockedNotificationConn struct {
	net.Conn
	started  chan struct{}
	once     sync.Once
	closeErr error
	closes   int
}

func (c *blockedNotificationConn) Write(data []byte) (int, error) {
	c.once.Do(func() { close(c.started) })
	return c.Conn.Write(data)
}

func (c *blockedNotificationConn) Close() error {
	c.closes++
	return errors.Join(c.Conn.Close(), c.closeErr)
}

package lsp

import (
	"errors"
	"net"
	"testing"

	"github.com/sourcegraph/jsonrpc2"
)

func TestDisconnectedConnectionRetainsPhysicalCleanupFailure(t *testing.T) {
	agent, server := net.Pipe()
	defer agent.Close()
	defer server.Close()
	failure := errors.New("stdio cleanup failed")
	stream := &blockedNotificationConn{Conn: agent, closeErr: failure}
	conn := newConnection(t.Context(), jsonrpc2.NewBufferedStream(stream, lspObjectCodec{}), jsonrpc2.AsyncHandler(&client{}))
	server.Close()
	<-conn.DisconnectNotify()
	for range 2 {
		if err := conn.Close(); !errors.Is(err, failure) {
			t.Fatalf("logical closure lost the physical cleanup failure: %v", err)
		}
	}
	if stream.closes != 1 {
		t.Fatalf("physical stream closed %d times", stream.closes)
	}
}

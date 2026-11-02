package lsp

import (
	"context"
	"os"
	"sync"

	"github.com/sourcegraph/jsonrpc2"
)

// client is one live connection to one language-server process, scoped to one
// workspace root. It owns the child process, the JSON-RPC connection over its
// stdio, the set of documents it has opened, and a cache of the diagnostics
// the server has pushed. It implements jsonrpc2.Handler to receive the
// server→client traffic (diagnostics pushes, configuration requests).
type client struct {
	spec       ServerSpec
	workspace  *os.Root
	syncPolicy documentSynchronization

	conn        *jsonrpc2.Conn
	cancel      context.CancelFunc
	wait        <-chan processEnd
	stopProcess func() error
	// shutdownBase retains the process-lifetime values while allowing Close to
	// use its own bounded graceful-shutdown budget after lifetime cancellation.
	shutdownBase context.Context

	closeOnce  sync.Once
	cleanupErr error
	processErr error

	mu sync.Mutex
	// Connection-wide versions distinguish a reopened URI from its evicted incarnation.
	version int
	open    map[string]openDoc // uri → last synced version + content hash
	diags   map[string]diagSet // uri → latest pushed diagnostics
	// diagnosticsErr carries the latest malformed publishDiagnostics
	// notification until a valid notification proves the stream healthy again.
	// The JSON-RPC handler cannot return notification errors to the server, so
	// the next diagnostics caller is the first honest observation boundary.
	diagnosticsErr error
	// updated is closed (and replaced) on every diagnostics push, so a waiter
	// can block for the next valid push or protocol error without polling.
	updated chan struct{}
}

type openDoc struct {
	version int
	hash    [32]byte
	end     Position
}

type diagSet struct {
	protocolErr error
	version     int
	diagnostics []Diagnostic
}

// processEnd separates observed process termination from failure to retire its
// resources. A disconnected server may restart after an ordinary failed exit.
type processEnd struct {
	exitErr    error
	cleanupErr error
}

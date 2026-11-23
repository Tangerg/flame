package lsp

import (
	"context"
	"os"
	"sync"
)

type client struct {
	spec       ServerSpec
	workspace  *os.Root
	syncPolicy documentSynchronization

	conn         *connection
	cancel       context.CancelFunc
	wait         <-chan processEnd
	stopProcess  func() error
	shutdownBase context.Context

	closeOnce  sync.Once
	cleanupErr error
	processErr error

	mu      sync.Mutex
	version int
	open    map[string]openDoc
	diags   map[string]diagSet
	// The JSON-RPC handler cannot return notification errors to the server, so the
	// next diagnostics caller is the first honest observation boundary.
	diagnosticsErr error
	updated        chan struct{}
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

type processEnd struct {
	exitErr    error
	cleanupErr error
}

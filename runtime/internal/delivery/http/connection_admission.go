package http

import (
	"net"
	"net/http"
	"sync"
)

// net/http gives StateNew connections time to send headers. Shutdown can
// retire those sockets immediately because request admission comes later.
type unstartedConnections struct {
	mu      sync.Mutex
	waiting map[net.Conn]struct{}
	sealed  bool
}

func (c *unstartedConnections) observe(conn net.Conn, state http.ConnState) {
	c.mu.Lock()
	if state != http.StateNew {
		delete(c.waiting, conn)
		c.mu.Unlock()
		return
	}
	if c.sealed {
		c.mu.Unlock()
		_ = conn.Close()
		return
	}
	c.waiting[conn] = struct{}{}
	c.mu.Unlock()
}

func (c *unstartedConnections) seal() {
	c.mu.Lock()
	c.sealed = true
	waiting := c.waiting
	c.waiting = nil
	c.mu.Unlock()
	for conn := range waiting {
		_ = conn.Close()
	}
}

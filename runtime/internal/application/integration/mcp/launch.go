package mcp

import (
	"fmt"
	"io"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

// Launch transfers a fresh launch to the connection owner. Server describes
// revocable source authority; Stdio projects it onto leased execution bytes.
// Retire must follow the process until teardown, including failed handshakes.
// Retained source descriptors never retain this resource or initiate a launch.
type Launch struct {
	Server mcpserver.Server
	Stdio  *Stdio
	Retire io.Closer
}

type Stdio struct {
	Command string
	Args    []string
	Env     map[string]string
	Dir     string
}

func (s Stdio) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "Stdio{Command:%q, Args:%q, Env:%s, Dir:%q}", s.Command, s.Args, mcpserver.SecretPresence(len(s.Env) > 0), s.Dir)
}

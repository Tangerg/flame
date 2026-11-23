package mcp

import (
	"fmt"
	"io"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/secret"

	framevalue "github.com/Tangerg/frame/value"
)

// Retire must follow the process until teardown, including failed handshakes.
type Launch struct {
	Server        mcpserver.Server
	Stdio         *Stdio
	Retire        io.Closer
	Exports       map[string]framevalue.JSON
	Requirements  *plugin.RequiredBindings
	Contributions []plugin.Contribution
}

type Stdio struct {
	ExpectedExecutable framevalue.Digest
	Command            string
	Args               []string
	Env                map[string]string
	Dir                string
}

func (s Stdio) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "Stdio{Command:%q, Args:%q, Env:%s, Dir:%q}", s.Command, s.Args, secret.Presence(len(s.Env) > 0), s.Dir)
}

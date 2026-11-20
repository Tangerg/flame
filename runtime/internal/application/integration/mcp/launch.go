package mcp

import (
	"fmt"
	"io"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/secret"

	framevalue "github.com/Tangerg/frame/value"
)

// Launch transfers a fresh launch to the connection owner. Server describes
// revocable source authority; Stdio projects it onto leased execution bytes.
// Retire must follow the process until teardown, including failed handshakes.
// Retained source descriptors never retain this resource or initiate a launch.
type Launch struct {
	Server mcpserver.Server
	Stdio  *Stdio
	Retire io.Closer
	// Callable entries must prove this declared set before connection publication.
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

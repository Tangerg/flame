package mcp

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

// Launch transfers a fresh configuration and its execution resource exactly
// once. SourceConfig snapshots carry neither this claim nor relocated paths.
// A rejected attempt retires its claim; a successful dial transfers it into
// the existing session ownership ledger until process teardown completes.
type Launch struct{ claim *launchClaim }

type launchClaim struct {
	mu      sync.Mutex
	pending *launch
}

type Stdio struct {
	Command string
	Args    []string
	Env     []string
	Dir     string
}

type launch struct {
	config ServerConfig
	stdio  *Stdio
	retire sessionCleanup
}

// NewLaunch consumes retire even when construction fails.
func NewLaunch(config ServerConfig, stdio *Stdio, retire func() error) (_ *Launch, err error) {
	defer func() {
		if err != nil && retire != nil {
			err = errors.Join(err, retire())
		}
	}()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if stdio != nil {
		if config.Transport != TransportStdio || stdio.Command == "" || retire == nil {
			return nil, errors.New("mcp: leased stdio requires a command, resource and stdio transport")
		}
		stdio = &Stdio{Command: stdio.Command, Args: slices.Clone(stdio.Args), Env: slices.Clone(stdio.Env), Dir: stdio.Dir}
	}
	if _, _, installed := config.Source.Release(); installed && config.Transport == TransportStdio && stdio == nil {
		return nil, errors.New("mcp: installation stdio requires leased execution content")
	}
	return &Launch{claim: &launchClaim{pending: &launch{config: config.Clone(), stdio: stdio, retire: retire}}}, nil
}

func (l *Launch) take() (*launch, error) {
	if l == nil || l.claim == nil {
		return nil, errors.New("mcp: launch is required")
	}
	l.claim.mu.Lock()
	defer l.claim.mu.Unlock()
	if l.claim.pending == nil {
		return nil, errors.New("mcp: launch was already consumed")
	}
	pending := l.claim.pending
	l.claim.pending = nil
	return pending, nil
}

func (l *Launch) Close() error {
	if l == nil || l.claim == nil {
		return nil
	}
	l.claim.mu.Lock()
	pending := l.claim.pending
	l.claim.pending = nil
	l.claim.mu.Unlock()
	if pending == nil {
		return nil
	}
	return pending.close()
}

func (l *launch) close() error {
	retire := l.retire
	l.retire = nil
	if retire == nil {
		return nil
	}
	return retire()
}

func (s Stdio) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "Stdio{Command:%q, Args:%q, Env:%s, Dir:%q}", s.Command, s.Args, mcpserver.SecretPresence(len(s.Env) > 0), s.Dir)
}

func (l *Launch) Format(state fmt.State, _ rune) {
	if l == nil || l.claim == nil {
		_, _ = fmt.Fprint(state, "Launch{nil}")
		return
	}
	l.claim.mu.Lock()
	defer l.claim.mu.Unlock()
	if l.claim.pending == nil {
		_, _ = fmt.Fprint(state, "Launch{consumed}")
		return
	}
	_, _ = fmt.Fprintf(state, "Launch{Config:%v, Stdio:%v}", l.claim.pending.config, l.claim.pending.stdio)
}

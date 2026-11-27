package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Tangerg/acp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/plugins/opencode/internal/wire"
	framesqlite "github.com/Tangerg/frame/sqlite"
)

type connector struct {
	ctx     context.Context
	stop    context.CancelFunc
	db      framesqlite.Database
	machine string
	exe     string
	actions sync.Mutex
	mu      sync.Mutex
	client  *acp.Client
	link    *agentLink
	joined  sync.WaitGroup
}

type agentLink struct {
	conn   *acp.ClientConn
	fork   bool
	name   string
	closed atomic.Bool
	bound  map[acp.SessionID]*attachedSession
}

type attachedSession struct {
	id          string
	handle      *acp.ClientSession
	replay      []wire.Event
	turn        *turnHandle
	permissions map[string]chan acp.RequestPermissionOutcome
}

func openConnector(ctx context.Context, data, exe string) (*connector, error) {
	machine, err := os.Hostname()
	if err != nil || machine == "" {
		return nil, errors.New("OpenCode connector could not identify its machine")
	}
	db, err := framesqlite.OpenSchema(ctx, filepath.Join(data, "opencode.db"), []string{pluginstorage.CommandsSchema, sessionSchema})
	if err != nil {
		return nil, err
	}
	life, stop := context.WithCancel(ctx)
	c := &connector{ctx: life, stop: stop, db: db, machine: machine, exe: exe}
	c.client, err = acp.NewClient(&acp.ClientConfig{
		Info:          &acp.Implementation{Name: "Flame", Version: "1"},
		SessionUpdate: c.onUpdate, RequestPermission: c.onPermission,
	})
	if err != nil {
		stop()
		return nil, errors.Join(err, db.Close())
	}
	return c, nil
}

func (c *connector) close() error {
	c.stop()
	c.mu.Lock()
	link := c.link
	c.mu.Unlock()
	var err error
	if link != nil {
		err = link.conn.Close()
	}
	c.joined.Wait()
	return errors.Join(err, c.db.Close())
}

func (c *connector) connect(ctx context.Context, cwd string) (*agentLink, error) {
	c.mu.Lock()
	link := c.link
	c.mu.Unlock()
	if link != nil && !link.closed.Load() {
		return link, nil
	}
	exe := c.exe
	if exe == "" {
		var err error
		exe, err = exec.LookPath("opencode")
		if err != nil {
			return nil, refuse("configure the absolute path to the OpenCode executable")
		}
	}
	if !filepath.IsAbs(exe) {
		return nil, refuse("the OpenCode executable must be an absolute path")
	}
	file, err := os.Stat(exe)
	self, selfErr := os.Executable()
	me, statErr := os.Stat(self)
	if err != nil || !file.Mode().IsRegular() || selfErr != nil || statErr != nil || os.SameFile(file, me) {
		return nil, refuse("the configured OpenCode executable is unavailable")
	}
	cmd := exec.Command(exe, "acp")
	cmd.Dir = cwd
	cmd.Stderr = os.Stderr
	ready, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	transport := &openCodeTransport{
		Transport: acp.NewCommandTransport(&acp.CommandConfig{Command: cmd}),
		promptSent: func(native acp.SessionID) {
			c.mu.Lock()
			defer c.mu.Unlock()
			if bound := link.bound[native]; bound != nil && bound.turn != nil {
				close(bound.turn.sent)
			}
		},
	}
	connection, err := c.client.Connect(ready, transport)
	if err != nil {
		return nil, observedFailure(err)
	}
	info, _ := connection.Negotiated().AgentInfo.Get()
	if !strings.EqualFold(info.Name, "opencode") {
		return nil, errors.Join(refuse("the configured ACP program did not identify itself as OpenCode"), connection.Close())
	}
	link = &agentLink{conn: connection, fork: transport.fork.Load(), name: fmt.Sprintf("%s %s", info.Name, info.Version), bound: map[acp.SessionID]*attachedSession{}}
	c.mu.Lock()
	c.link = link
	c.mu.Unlock()
	c.goLive(func() {
		if err := connection.Wait(); err != nil && c.ctx.Err() == nil {
			slog.WarnContext(c.ctx, "OpenCode connection ended", "error", err)
		}
		link.closed.Store(true)
	})
	return link, nil
}

func (c *connector) goLive(action func()) {
	c.joined.Go(action)
}

func physicalWorkspace(path string) (string, error) {
	if !filepath.IsAbs(path) || len(path) > 4096 {
		return "", refuse("select an absolute workspace path")
	}
	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", refuse("the workspace is unavailable")
	}
	info, err := os.Stat(physical)
	if err != nil || !info.IsDir() {
		return "", refuse("the workspace is unavailable")
	}
	return physical, nil
}

func (c *connector) list(ctx context.Context, in wire.ListRequest) (wire.Sessions, error) {
	path, err := physicalWorkspace(in.CWD)
	if err != nil {
		return wire.Sessions{}, err
	}
	c.actions.Lock()
	defer c.actions.Unlock()
	link, err := c.connect(ctx, path)
	if err != nil {
		return wire.Sessions{}, err
	}
	params := &acp.ListSessionsRequest{Cwd: acp.OptValue(path)}
	if in.Cursor != "" {
		params.Cursor = acp.OptValue(in.Cursor)
	}
	list, err := link.conn.ListSessions(ctx, params)
	if err != nil {
		return wire.Sessions{}, observedFailure(err)
	}
	next, _ := list.NextCursor.Get()
	out := wire.Sessions{Machine: c.machine, Agent: link.name, Rows: []wire.NativeSession{}, NextCursor: next}
	for _, row := range list.Sessions {
		cwd, err := physicalWorkspace(row.Cwd)
		if err != nil || cwd != path {
			continue
		}
		title, _ := row.Title.Get()
		out.Rows = append(out.Rows, wire.NativeSession{ID: string(row.SessionID), CWD: cwd, Title: title})
	}
	return out, nil
}

func (c *connector) open(ctx context.Context, in wire.OpenRequest) (wire.Session, error) {
	path, err := physicalWorkspace(in.CWD)
	if err != nil {
		return wire.Session{}, err
	}
	if in.NativeID == "" || len(in.NativeID) > 256 || strings.ContainsRune(in.NativeID, '\x00') {
		return wire.Session{}, refuse("select an existing OpenCode session")
	}
	link, err := c.connect(ctx, path)
	if err != nil {
		return wire.Session{}, err
	}
	return c.load(ctx, link, in.NativeID, path)
}

func (c *connector) load(ctx context.Context, link *agentLink, native, cwd string) (wire.Session, error) {
	initial := emptySession(c.machine, link.name, native, cwd)
	key := acp.SessionID(native)
	c.mu.Lock()
	existing := link.bound[key]
	if existing != nil {
		c.mu.Unlock()
		if initial.ID != existing.id {
			return wire.Session{}, refuse("the native session belongs to a different workspace")
		}
		return c.get(ctx, wire.SessionRequest{ID: existing.id})
	}
	bound := &attachedSession{id: initial.ID, replay: []wire.Event{}, permissions: map[string]chan acp.RequestPermissionOutcome{}}
	link.bound[key] = bound
	c.mu.Unlock()

	handle, _, err := link.conn.LoadSession(ctx, &acp.LoadSessionRequest{SessionID: key, Cwd: cwd, McpServers: []acp.McpServer{}})
	c.mu.Lock()
	if err != nil {
		delete(link.bound, key)
		c.mu.Unlock()
		return wire.Session{}, observedFailure(err)
	}
	replay := bound.replay
	bound.replay, bound.handle = nil, handle
	c.mu.Unlock()

	err = c.db.RunInTx(ctx, func(tx context.Context) error {
		old, err := c.stored(tx, initial.ID)
		if err != nil {
			if _, missing := errors.AsType[refusal](err); !missing {
				return err
			}
		} else {
			initial.Turn, initial.Permissions = old.Turn, old.Permissions
		}
		initial.Events = replay
		return c.save(tx, initial)
	})
	if err != nil {
		c.mu.Lock()
		delete(link.bound, key)
		c.mu.Unlock()
		return wire.Session{}, err
	}
	return c.get(ctx, wire.SessionRequest{ID: initial.ID})
}

func (c *connector) get(ctx context.Context, req wire.SessionRequest) (wire.Session, error) {
	value, err := c.stored(ctx, req.ID)
	if err != nil {
		return value, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	link, bound := c.live(value)
	value.Connected = bound != nil
	value.CanFork = bound != nil && link.fork
	if value.Turn != nil && value.Turn.Status == "running" && (bound == nil || bound.turn == nil || bound.turn.id != value.Turn.ID) {
		value.Turn.Status = "unconfirmed"
	}
	for i := range value.Permissions {
		permission := &value.Permissions[i]
		if permission.Status == "requested" && (bound == nil || bound.permissions[permission.ID] == nil) {
			permission.Status = "unconfirmed"
		}
	}
	return value, nil
}

// live is only read with mu held. The ACP handle is a lease on one connection.
func (c *connector) live(value wire.Session) (*agentLink, *attachedSession) {
	link := c.link
	if link == nil || link.closed.Load() || value.Machine != c.machine {
		return nil, nil
	}
	bound := link.bound[acp.SessionID(value.NativeID)]
	if bound == nil || bound.id != value.ID || bound.handle == nil {
		return link, nil
	}
	return link, bound
}

func (c *connector) fork(ctx context.Context, input wire.SessionRequest) (wire.Session, error) {
	parent, err := c.get(ctx, input)
	if err != nil {
		return wire.Session{}, err
	}
	c.mu.Lock()
	link, bound := c.live(parent)
	c.mu.Unlock()
	if bound == nil || !link.fork || parent.Turn != nil && parent.Turn.Status == "running" {
		return wire.Session{}, refuse("the current connection cannot fork this session")
	}
	var result struct {
		SessionID string `json:"sessionId"`
	}
	request := struct {
		SessionID string   `json:"sessionId"`
		CWD       string   `json:"cwd"`
		MCP       []string `json:"mcpServers"`
	}{parent.NativeID, parent.CWD, []string{}}
	if err := link.conn.Call(ctx, forkExtension, request, &result); err != nil {
		return wire.Session{}, observedFailure(err)
	}
	if result.SessionID == "" || result.SessionID == parent.NativeID {
		return wire.Session{}, errors.New("OpenCode fork did not return a separate native session ID")
	}
	return c.load(ctx, link, result.SessionID, parent.CWD)
}

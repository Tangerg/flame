package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/Tangerg/acp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/plugins/opencode/internal/wire"
	framesqlite "github.com/Tangerg/frame/sqlite"
)

type connector struct {
	ctx      context.Context
	stop     context.CancelFunc
	db       framesqlite.Database
	machine  string
	profiles map[string]agentProfile
	actions  sync.Mutex
	mu       sync.Mutex
	client   *acp.Client
	links    map[agentTarget]*agentLink
	joined   sync.WaitGroup
}

type agentTarget struct {
	agent string
	cwd   string
}

type agentLink struct {
	conn         *acp.ClientConn
	program      agentProgram
	capabilities wire.Capabilities
	name         string
	closed       atomic.Bool
	bound        map[acp.SessionID]*attachedSession
}

type attachedSession struct {
	id          string
	handle      *acp.ClientSession
	replay      []wire.Event
	turn        *turnHandle
	permissions map[string]chan acp.RequestPermissionOutcome
}

type nativeCreation struct {
	session wire.Session
	link    *agentLink
	handle  *acp.ClientSession
}

func openConnector(ctx context.Context, data string, profiles map[string]agentProfile) (*connector, error) {
	machine, err := os.Hostname()
	if err != nil || machine == "" {
		return nil, errors.New("acp: could not identify the execution machine")
	}
	db, err := framesqlite.OpenSchema(ctx, filepath.Join(data, "opencode.db"), []string{pluginstorage.CommandsSchema, sessionSchema})
	if err != nil {
		return nil, err
	}
	life, stop := context.WithCancel(ctx)
	c := &connector{ctx: life, stop: stop, db: db, machine: machine, profiles: profiles, links: make(map[agentTarget]*agentLink)}
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
	links := make([]*agentLink, 0, len(c.links))
	for _, link := range c.links {
		links = append(links, link)
	}
	c.mu.Unlock()
	var err error
	for _, link := range links {
		err = errors.Join(err, link.conn.Close())
	}
	c.joined.Wait()
	return errors.Join(err, c.db.Close())
}

func (c *connector) connect(ctx context.Context, target agentTarget, program agentProgram) (*agentLink, error) {
	c.mu.Lock()
	previous := c.links[target]
	c.mu.Unlock()
	if previous != nil && !previous.closed.Load() {
		return previous, nil
	}
	cmd := exec.Command(program.path, program.Args...)
	cmd.Dir = target.cwd
	cmd.Stderr = os.Stderr
	ready, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	link := &agentLink{program: program, bound: make(map[acp.SessionID]*attachedSession)}
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
	negotiated := connection.Negotiated()
	info, _ := negotiated.AgentInfo.Get()
	if program.NativeName != "" && !strings.EqualFold(info.Name, program.NativeName) {
		return nil, errors.Join(refuse("the ACP executable reported a different agent identity"), connection.Close())
	}
	if program.Version != "" && info.Version != program.Version {
		return nil, errors.Join(refuse("the ACP executable reported a different pinned version"), connection.Close())
	}
	_, list := negotiated.AgentCapabilities.SessionCapabilities.List.Get()
	_, resume := negotiated.AgentCapabilities.SessionCapabilities.Resume.Get()
	link.conn, link.name = connection, strings.TrimSpace(info.Name+" "+info.Version)
	if link.name == "" {
		link.name = program.Name
	}
	link.capabilities = wire.Capabilities{
		List: list, Load: negotiated.AgentCapabilities.LoadSession, Resume: resume,
		Fork: strings.EqualFold(info.Name, defaultAgent) && transport.fork.Load(),
	}
	c.mu.Lock()
	c.links[target] = link
	c.mu.Unlock()
	c.goLive(func() {
		if err := connection.Wait(); err != nil && c.ctx.Err() == nil {
			slog.WarnContext(c.ctx, "ACP connection ended", "agent", program.ID, "error", err)
		}
		link.closed.Store(true)
	})
	return link, nil
}

func (c *connector) goLive(action func()) { c.joined.Go(action) }

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

func validNativeID(value string) bool {
	return value != "" && len(value) <= 256 && !strings.ContainsFunc(value, unicode.IsControl)
}

func (c *connector) list(ctx context.Context, in wire.ListRequest) (wire.Sessions, error) {
	path, err := physicalWorkspace(in.CWD)
	if err != nil {
		return wire.Sessions{}, err
	}
	c.actions.Lock()
	defer c.actions.Unlock()
	target := agentTarget{agentID(in.AgentID), path}
	program, err := c.program(target)
	if err != nil {
		return wire.Sessions{}, err
	}
	link, err := c.connect(ctx, target, program)
	if err != nil {
		return wire.Sessions{}, err
	}
	return c.listConnected(ctx, link, path, in.Cursor)
}

func (c *connector) listConnected(ctx context.Context, link *agentLink, cwd, cursor string) (wire.Sessions, error) {
	out := wire.Sessions{Machine: c.machine, Agent: link.name, Capabilities: link.capabilities, Rows: []wire.NativeSession{}}
	if !link.capabilities.List {
		return out, nil
	}
	params := &acp.ListSessionsRequest{Cwd: acp.OptValue(cwd)}
	if cursor != "" {
		params.Cursor = acp.OptValue(cursor)
	}
	list, err := link.conn.ListSessions(ctx, params)
	if err != nil {
		return wire.Sessions{}, observedFailure(err)
	}
	out.NextCursor, _ = list.NextCursor.Get()
	for _, row := range list.Sessions {
		physical, err := physicalWorkspace(row.Cwd)
		if err != nil || physical != cwd || !validNativeID(string(row.SessionID)) {
			continue
		}
		title, _ := row.Title.Get()
		out.Rows = append(out.Rows, wire.NativeSession{ID: string(row.SessionID), CWD: cwd, Title: title})
	}
	return out, nil
}

func (c *connector) checkNativeOrigin(ctx context.Context, link *agentLink, cwd, native string) error {
	ctx, stop := context.WithTimeout(ctx, 15*time.Second)
	defer stop()
	cursor := ""
	seen := map[string]bool{}
	for {
		page, err := c.listConnected(ctx, link, cwd, cursor)
		if err != nil {
			return err
		}
		for _, item := range page.Rows {
			if item.ID == native {
				return nil
			}
		}
		if page.NextCursor == "" {
			return refuse("the agent did not list this native session in the chosen workspace")
		}
		if seen[page.NextCursor] {
			return refuse("the agent returned a repeated native session cursor")
		}
		seen[page.NextCursor], cursor = true, page.NextCursor
	}
}

func (c *connector) open(ctx context.Context, in wire.OpenRequest) (wire.Session, error) {
	var original wire.Session
	var err error
	target := agentTarget{agentID(in.AgentID), in.CWD}
	native := in.NativeID
	if in.ID != "" {
		if in.CWD != "" || in.NativeID != "" || in.AgentID != "" {
			return wire.Session{}, refuse("reopen a stored session by its original ID")
		}
		original, err = c.stored(ctx, in.ID)
		if err != nil {
			return wire.Session{}, err
		}
		target = agentTarget{agentID(original.AgentID), original.CWD}
		native = original.NativeID
	}
	if !validNativeID(native) {
		return wire.Session{}, refuse("select an existing native session")
	}
	target.cwd, err = physicalWorkspace(target.cwd)
	if err != nil {
		return wire.Session{}, err
	}
	if original.ID == "" {
		original, err = c.original(ctx, target.agent, native)
		if err != nil {
			return wire.Session{}, err
		}
	}
	program, err := c.program(target)
	if err != nil {
		return wire.Session{}, err
	}
	if original.ID != "" && (original.Machine != c.machine || original.CWD != target.cwd ||
		original.Authority != "" && original.Authority != program.authority) {
		return wire.Session{}, refuse("the original native session belongs to a different workspace or program")
	}
	link, err := c.connect(ctx, target, program)
	if err != nil {
		return wire.Session{}, err
	}
	if original.ID == "" && link.capabilities.List {
		if err := c.checkNativeOrigin(ctx, link, target.cwd, native); err != nil {
			return wire.Session{}, err
		}
	}
	return c.restore(ctx, link, native, target.cwd, in.Mode, original)
}

func (c *connector) restore(ctx context.Context, link *agentLink, native, cwd, mode string, original wire.Session) (wire.Session, error) {
	if mode == "" {
		if link.capabilities.Load {
			mode = "load"
		} else {
			mode = "resume"
		}
	}
	if !(mode == "load" && link.capabilities.Load || mode == "resume" && link.capabilities.Resume) {
		return wire.Session{}, refuse("the ACP agent did not advertise this restore operation")
	}
	initial := emptySession(c.machine, native, cwd, link)
	initial.Turn, initial.Permissions = original.Turn, original.Permissions
	if initial.Permissions == nil {
		initial.Permissions = []wire.PermissionRequest{}
	}
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
	bound := &attachedSession{id: initial.ID, replay: []wire.Event{}, permissions: make(map[string]chan acp.RequestPermissionOutcome)}
	link.bound[key] = bound
	c.mu.Unlock()

	var handle *acp.ClientSession
	var err error
	if mode == "load" {
		handle, _, err = link.conn.LoadSession(ctx, &acp.LoadSessionRequest{SessionID: key, Cwd: cwd, McpServers: []acp.McpServer{}})
	} else {
		handle, _, err = link.conn.ResumeSession(ctx, &acp.ResumeSessionRequest{SessionID: key, Cwd: cwd, McpServers: []acp.McpServer{}})
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		delete(link.bound, key)
		return wire.Session{}, observedFailure(err)
	}
	initial.Events = bound.replay
	if mode == "resume" {
		initial.Events = slices.Clone(original.Events)
		for _, event := range bound.replay {
			initial.Events = appendEvent(initial.Events, event)
		}
		if initial.Events == nil {
			initial.Events = []wire.Event{}
		}
	}
	if err := c.db.RunInTx(ctx, func(tx context.Context) error { return c.save(tx, initial) }); err != nil {
		delete(link.bound, key)
		return wire.Session{}, err
	}
	bound.replay, bound.handle = nil, handle
	return presentSession(initial, link, bound), nil
}

func (c *connector) get(ctx context.Context, req wire.SessionRequest) (wire.Session, error) {
	value, err := c.stored(ctx, req.ID)
	if err != nil {
		return value, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	link, bound := c.live(value)
	return presentSession(value, link, bound), nil
}

func presentSession(value wire.Session, link *agentLink, bound *attachedSession) wire.Session {
	value.Connected = bound != nil
	if link != nil {
		value.Capabilities = new(link.capabilities)
		value.CanFork = bound != nil && link.capabilities.Fork
	}
	if value.Turn != nil && value.Turn.Status == "running" && (bound == nil || bound.turn == nil || bound.turn.id != value.Turn.ID) {
		value.Turn.Status = "unconfirmed"
	}
	for i := range value.Permissions {
		permission := &value.Permissions[i]
		if (permission.Status == "requested" || permission.Status == "submitted") &&
			(bound == nil || bound.turn == nil || permission.TurnID != bound.turn.id ||
				permission.Status == "requested" && bound.permissions[permission.ID] == nil) {
			permission.Status = "unconfirmed"
		}
	}
	return value
}

// live is only read with mu held. The ACP handle is a lease on one connection.
func (c *connector) live(value wire.Session) (*agentLink, *attachedSession) {
	link := c.links[agentTarget{agentID(value.AgentID), value.CWD}]
	if link == nil || link.closed.Load() || value.Machine != c.machine || value.Authority != link.program.authority {
		return nil, nil
	}
	bound := link.bound[acp.SessionID(value.NativeID)]
	if bound == nil || bound.id != value.ID || bound.handle == nil {
		return link, nil
	}
	return link, bound
}

func (c *connector) nativeBinding(connection *acp.ClientConn, native acp.SessionID) *attachedSession {
	for _, link := range c.links {
		if link.conn == connection && !link.closed.Load() {
			return link.bound[native]
		}
	}
	return nil
}

func (c *connector) create(ctx context.Context, in wire.CreateRequest) (nativeCreation, error) {
	cwd, err := physicalWorkspace(in.CWD)
	if err != nil {
		return nativeCreation{}, err
	}
	target := agentTarget{agentID(in.AgentID), cwd}
	program, err := c.program(target)
	if err != nil {
		return nativeCreation{}, err
	}
	link, err := c.connect(ctx, target, program)
	if err != nil {
		return nativeCreation{}, err
	}
	handle, _, err := link.conn.NewSession(ctx, &acp.NewSessionRequest{Cwd: cwd, McpServers: []acp.McpServer{}})
	if err != nil {
		return nativeCreation{}, nativeCreateFailure(err)
	}
	return c.acceptCreation(ctx, link, handle, string(handle.ID()), cwd)
}

func (c *connector) fork(ctx context.Context, input wire.SessionRequest) (nativeCreation, error) {
	parent, err := c.get(ctx, input)
	if err != nil {
		return nativeCreation{}, err
	}
	c.mu.Lock()
	link, bound := c.live(parent)
	c.mu.Unlock()
	if bound == nil || !link.capabilities.Fork || parent.Turn != nil && parent.Turn.Status == "running" {
		return nativeCreation{}, refuse("the current connection cannot fork this session")
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
		return nativeCreation{}, nativeCreateFailure(err)
	}
	return c.acceptCreation(ctx, link, nil, result.SessionID, parent.CWD)
}

func nativeCreateFailure(err error) error {
	if errors.Is(err, acp.ErrAuthRequired) {
		return refuse("authenticate the ACP agent on the execution machine")
	}
	return observedFailure(err)
}

func (c *connector) acceptCreation(ctx context.Context, link *agentLink, handle *acp.ClientSession, native, cwd string) (nativeCreation, error) {
	if !validNativeID(native) {
		return nativeCreation{}, errors.New("acp: creation acknowledgement omitted a native session ID")
	}
	settle, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	original, err := c.original(settle, link.program.ID, native)
	if err != nil {
		return nativeCreation{}, err
	}
	if original.ID != "" {
		return nativeCreation{}, errors.New("acp: creation acknowledgement reused a known native session ID")
	}
	value := emptySession(c.machine, native, cwd, link)
	value.Capabilities = new(link.capabilities)
	value.Connected, value.CanFork = handle != nil, handle != nil && link.capabilities.Fork
	return nativeCreation{session: value, link: link, handle: handle}, nil
}

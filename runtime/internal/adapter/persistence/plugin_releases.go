package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/qimu/integrity"
	"github.com/Tangerg/qimu/jsonvalue"
	"github.com/Tangerg/qimu/sqlite"
)

// ReleaseStore is the admitted release catalog: the only owner of release
// content. Rows are insert-only by digest, so an installation names a release
// and can never replace its declaration.
type ReleaseStore struct {
	db          *sql.DB
	mu          sync.Mutex
	projections map[integrity.Digest]releaseProjection
}

type releaseProjection struct {
	declaration string
	release     plugin.Release
}

func newReleaseStore(db *sql.DB) *ReleaseStore {
	return &ReleaseStore{db: db, projections: make(map[integrity.Digest]releaseProjection)}
}

func (s *ReleaseStore) Get(ctx context.Context, digest integrity.Digest) (plugin.Release, error) {
	var body string
	err := sqlite.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT declaration FROM plugin_releases WHERE digest=?`, digest.String()).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return plugin.Release{}, plugin.ErrNotFound
	}
	if err != nil {
		return plugin.Release{}, err
	}
	// The current SQL read owns existence and transaction visibility. Only
	// admission of its complete declaration may be reused across snapshots.
	s.mu.Lock()
	defer s.mu.Unlock()
	if projection, ok := s.projections[digest]; ok && projection.declaration == body {
		return projection.release, nil
	}
	var stored storedDeclaration
	if err := decodeStoredJSON([]byte(body), &stored); err != nil {
		return plugin.Release{}, fmt.Errorf("sqlite: decode release %s: %w", digest, err)
	}
	declaration, err := stored.declaration()
	if err != nil {
		return plugin.Release{}, fmt.Errorf("sqlite: decode release %s: %w", digest, err)
	}
	release, err := plugin.NewRelease(digest, declaration)
	if err != nil {
		return plugin.Release{}, err
	}
	s.projections[digest] = releaseProjection{declaration: body, release: release}
	return release, nil
}

// Admit records a release and, in the same transaction, the names of the
// servers it declares. Membership is written once with the release and never
// changes, so installation sources follow it relationally.
func (s *ReleaseStore) Admit(ctx context.Context, release plugin.Release) error {
	declaration := release.Declaration()
	body, err := encodeStoredJSON(storeDeclaration(declaration))
	if err != nil {
		return err
	}
	digest := release.Digest().String()
	return sqlite.RunInTx(ctx, s.db, func(ctx context.Context) error {
		db := sqlite.Connection(ctx, s.db)
		result, err := db.ExecContext(ctx, `INSERT INTO plugin_releases(digest,declaration) VALUES(?,?) ON CONFLICT(digest) DO NOTHING`, digest, string(body))
		if err != nil {
			return err
		}
		inserted, err := result.RowsAffected()
		if err != nil || inserted == 0 {
			return err
		}
		for _, server := range declaration.Servers {
			if _, err := db.ExecContext(ctx, `INSERT INTO plugin_release_servers(digest,name) VALUES(?,?)`, digest, server.Name.String()); err != nil {
				return err
			}
		}
		return nil
	})
}

// Digests lists every admitted release.
func (s *ReleaseStore) Digests(ctx context.Context) ([]integrity.Digest, error) {
	rows, err := sqlite.Connection(ctx, s.db).QueryContext(ctx, `SELECT digest FROM plugin_releases ORDER BY digest`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []integrity.Digest
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return nil, err
		}
		digest, err := integrity.ParseDigest(text)
		if err != nil {
			return nil, fmt.Errorf("sqlite: decode release digest: %w", err)
		}
		result = append(result, digest)
	}
	return result, rows.Err()
}

// Remove deletes an unreferenced release with its server membership. An
// installation still naming it makes the delete fail, so a referenced release
// cannot be removed whatever its caller believes.
func (s *ReleaseStore) Remove(ctx context.Context, digest integrity.Digest) error {
	_, err := sqlite.Connection(ctx, s.db).ExecContext(ctx, `DELETE FROM plugin_releases WHERE digest=?`, digest.String())
	if err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.projections, digest)
	s.mu.Unlock()
	return nil
}

// storedDeclaration is the release record. Its servers are stored flat and its
// terminal views with an optional literal; the shadowing fields replace the
// domain's startup variants and always-present literal in the encoding.
type storedDeclaration struct {
	plugin.Declaration
	Servers       []storedServer
	TerminalViews []storedTerminalView
}

type storedTerminalView struct {
	ID           string
	Title        string
	Contribution plugin.ContributionID
	List         storedTerminalList
	Actions      []storedTerminalAction
}

type storedTerminalList struct {
	Method  string
	Input   []storedInputBinding
	Rows    string
	Key     string
	Paging  *plugin.TerminalPaging `json:",omitzero"`
	Columns []plugin.TerminalColumn
}

type storedTerminalAction struct {
	ID      string
	Title   string
	Method  string
	Target  extension.TerminalActionTarget
	Confirm bool
	Input   []storedInputBinding
}

type storedInputBinding struct {
	Path  string
	Value storedBinding
}

type storedBinding struct {
	Source  extension.TerminalBindingSource
	Literal *jsonvalue.Value `json:",omitzero"`
	Field   string
	Context extension.TerminalContext
	Form    *storedFormField `json:",omitzero"`
}

type storedFormField struct {
	Title   string
	Default *storedBinding `json:",omitzero"`
	Kind    extension.TerminalFormKind
	Options []string
}

func storeTerminalViews(views []plugin.TerminalView) []storedTerminalView {
	stored := make([]storedTerminalView, len(views))
	for index, view := range views {
		actions := make([]storedTerminalAction, len(view.Actions))
		for at, action := range view.Actions {
			actions[at] = storedTerminalAction{ID: action.ID, Title: action.Title, Method: action.Method, Target: action.Target, Confirm: action.Confirm, Input: storeInputBindings(action.Input)}
		}
		stored[index] = storedTerminalView{
			ID: view.ID, Title: view.Title, Contribution: view.Contribution, Actions: actions,
			List: storedTerminalList{Method: view.List.Method, Input: storeInputBindings(view.List.Input), Rows: view.List.Rows, Key: view.List.Key, Paging: view.List.Paging, Columns: view.List.Columns},
		}
	}
	return stored
}

func storeInputBindings(bindings []plugin.InputBinding) []storedInputBinding {
	stored := make([]storedInputBinding, len(bindings))
	for index, binding := range bindings {
		stored[index] = storedInputBinding{Path: binding.Path, Value: storeBinding(binding.Value)}
	}
	return stored
}

func storeBinding(binding plugin.Binding) storedBinding {
	stored := storedBinding{Source: binding.Source, Field: binding.Field, Context: binding.Context}
	if len(binding.Literal.JSON()) != 0 {
		stored.Literal = &binding.Literal
	}
	if binding.Form != nil {
		form := storedFormField{Title: binding.Form.Title, Kind: binding.Form.Kind, Options: binding.Form.Options}
		if binding.Form.Default != nil {
			value := storeBinding(*binding.Form.Default)
			form.Default = &value
		}
		stored.Form = &form
	}
	return stored
}

func (s storedTerminalView) view() plugin.TerminalView {
	actions := make([]plugin.TerminalAction, len(s.Actions))
	for index, action := range s.Actions {
		actions[index] = plugin.TerminalAction{ID: action.ID, Title: action.Title, Method: action.Method, Target: action.Target, Confirm: action.Confirm, Input: inputBindings(action.Input)}
	}
	return plugin.TerminalView{
		ID: s.ID, Title: s.Title, Contribution: s.Contribution, Actions: actions,
		List: plugin.TerminalList{Method: s.List.Method, Input: inputBindings(s.List.Input), Rows: s.List.Rows, Key: s.List.Key, Paging: s.List.Paging, Columns: s.List.Columns},
	}
}

func inputBindings(stored []storedInputBinding) []plugin.InputBinding {
	bindings := make([]plugin.InputBinding, len(stored))
	for index, binding := range stored {
		bindings[index] = plugin.InputBinding{Path: binding.Path, Value: binding.Value.binding()}
	}
	return bindings
}

func (s storedBinding) binding() plugin.Binding {
	binding := plugin.Binding{Source: s.Source, Field: s.Field, Context: s.Context}
	if s.Literal != nil {
		binding.Literal = *s.Literal
	}
	if s.Form != nil {
		form := plugin.FormField{Title: s.Form.Title, Kind: s.Form.Kind, Options: s.Form.Options}
		if s.Form.Default != nil {
			value := s.Form.Default.binding()
			form.Default = &value
		}
		binding.Form = &form
	}
	return binding
}

type storedServer struct {
	Name      mcpserver.ServerName
	Managed   *plugin.ManagedBackend `json:",omitzero"`
	Transport mcpserver.Transport
	Command   string
	Args      []string
	Env       map[string]string
	Dir       string
	URL       string
	Headers   map[string]string
}

func storeDeclaration(declaration plugin.Declaration) storedDeclaration {
	stored := storedDeclaration{Declaration: declaration, Servers: make([]storedServer, len(declaration.Servers)), TerminalViews: storeTerminalViews(declaration.TerminalViews)}
	stored.Declaration.Servers = nil
	stored.Declaration.TerminalViews = nil
	for index, server := range declaration.Servers {
		record := storedServer{Name: server.Name, Transport: server.Transport()}
		switch startup := server.Startup.(type) {
		case plugin.ManagedBackend:
			record.Managed = &startup
		case plugin.DeclaredConnection:
			switch connection := startup.Connection.(type) {
			case mcpserver.StdioConnection:
				record.Command, record.Args, record.Env, record.Dir = connection.Command, connection.Args, connection.Env, connection.Dir
			case mcpserver.HTTPConnection:
				record.URL, record.Headers = connection.URL, connection.Headers
			}
		}
		stored.Servers[index] = record
	}
	return stored
}

func (s storedDeclaration) declaration() (plugin.Declaration, error) {
	declaration := s.Declaration
	declaration.TerminalViews = make([]plugin.TerminalView, len(s.TerminalViews))
	for index, view := range s.TerminalViews {
		declaration.TerminalViews[index] = view.view()
	}
	declaration.Servers = make([]plugin.Server, len(s.Servers))
	for index, record := range s.Servers {
		server := plugin.Server{Name: record.Name}
		switch {
		case record.Managed != nil:
			server.Startup = *record.Managed
		case record.Transport == mcpserver.TransportStdio:
			server.Startup = plugin.DeclaredConnection{Connection: mcpserver.StdioConnection{Command: record.Command, Args: record.Args, Env: record.Env, Dir: record.Dir}}
		case record.Transport == mcpserver.TransportStreamableHTTP:
			server.Startup = plugin.DeclaredConnection{Connection: mcpserver.HTTPConnection{URL: record.URL, Headers: record.Headers}}
		default:
			return plugin.Declaration{}, fmt.Errorf("server %q has unknown transport %q", record.Name, record.Transport)
		}
		declaration.Servers[index] = server
	}
	return declaration, nil
}

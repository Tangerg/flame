package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/qimu/sqlite"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/models/catalog"
	otelchat "github.com/Tangerg/scope/otel/chat"
)

type inputTokenCounter interface {
	CountInputTokens(context.Context, *chat.Request) (int64, error)
}

type client struct {
	modelID string
	chat    chat.Model
	stream  chat.Streamer
	count   inputTokenCounter
	embed   embedding.Model
	closed  bool
	active  sync.WaitGroup
}

type store struct {
	db      *sql.DB
	mu      sync.Mutex
	clients map[string]*client
}

func openStore(ctx context.Context, path string) (*store, error) {
	db, err := sqlite.OpenSchema(ctx, path, []string{
		pluginstorage.CommandsSchema,
		`CREATE TABLE IF NOT EXISTS clients (id TEXT PRIMARY KEY, kind TEXT NOT NULL CHECK(kind IN ('chat','embedding')), snapshot TEXT CHECK(snapshot IS NULL OR json_valid(snapshot)))`,
	})
	if err != nil {
		return nil, err
	}
	return &store{db: db, clients: make(map[string]*client)}, nil
}

func constructClient(ctx context.Context, kind string, input modelprovider.Open) (*client, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	spec, err := newClientSpec(input.Connection)
	if err != nil {
		return nil, err
	}
	result := &client{modelID: input.Model}
	if kind == "embedding" {
		result.embed, err = buildEmbeddingModel(ctx, spec, input.Model)
		return result, err
	}
	raw, err := buildModel(ctx, spec, input.Model)
	if err != nil {
		return nil, err
	}
	middleware, err := otelchat.NewMiddleware(otelchat.MiddlewareConfig{Provider: input.Connection.Provider})
	if err != nil {
		return nil, err
	}
	result.chat = middleware.Call(raw)
	if stream, ok := raw.(chat.Streamer); ok {
		result.stream = middleware.Stream(stream)
	}
	result.count, _ = raw.(inputTokenCounter)
	return result, nil
}

// The receipt and immutable connection snapshot commit together. A restart can
// reconstruct this handle without consulting current credentials or defaults.
func openClient[R any](ctx context.Context, s *store, method, kind string, command qimumcp.Command[modelprovider.Open], project func(modelprovider.Handle, *client, modelprovider.Open) R) (R, error) {
	var zero R
	input, err := command.RequestInput()
	if err != nil {
		return zero, err
	}
	var created *client
	var handle modelprovider.Handle
	value, err := pluginstorage.Command(ctx, s.db, method, command.ID, input, nil, func(ctx context.Context, input modelprovider.Open) (R, error) {
		var err error
		handle, created, err = s.insertClient(ctx, kind, input)
		if err != nil {
			return zero, err
		}
		return project(handle, created, input), nil
	})
	if err == nil && created != nil {
		s.mu.Lock()
		s.clients[handle.ID] = created
		s.mu.Unlock()
	}
	return value, err
}

func (s *store) insertClient(ctx context.Context, kind string, input modelprovider.Open) (modelprovider.Handle, *client, error) {
	created, err := constructClient(ctx, kind, input)
	if err != nil {
		return modelprovider.Handle{}, nil, &portable.Refusal{Code: "model_unavailable", Message: "model provider: client construction was refused"}
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return modelprovider.Handle{}, nil, err
	}
	handle := modelprovider.Handle{ID: hex.EncodeToString(token[:])}
	encoded, err := json.Marshal(input)
	if err != nil {
		return modelprovider.Handle{}, nil, err
	}
	if _, err := sqlite.Connection(ctx, s.db).ExecContext(ctx, `INSERT INTO clients(id,kind,snapshot) VALUES(?,?,?)`, handle.ID, kind, string(encoded)); err != nil {
		return modelprovider.Handle{}, nil, err
	}
	return handle, created, nil
}

func (s *store) reopenChat(ctx context.Context, method, originalMethod string, command qimumcp.Command[modelprovider.Reopen]) (modelprovider.Chat, error) {
	input, err := command.RequestInput()
	if err != nil {
		return modelprovider.Chat{}, err
	}
	var created *client
	value, err := pluginstorage.Command(ctx, s.db, method, command.ID, input, nil, func(ctx context.Context, input modelprovider.Reopen) (modelprovider.Chat, error) {
		if err := input.Validate(); err != nil {
			return modelprovider.Chat{}, err
		}
		var storedMethod, snapshot string
		var reply []byte
		if err := sqlite.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT method,business_input_json,reply FROM commands WHERE id=?`, input.Original).Scan(&storedMethod, &snapshot, &reply); err != nil {
			return modelprovider.Chat{}, err
		}
		if storedMethod != originalMethod || len(reply) == 0 {
			return modelprovider.Chat{}, errors.New("model provider: original chat construction is unconfirmed")
		}
		original, err := qimumcp.DecodeReply[modelprovider.Chat](reply)
		if err != nil || original.Original != input.Original {
			return modelprovider.Chat{}, errors.New("model provider: original chat construction was refused")
		}
		var specification modelprovider.Open
		if err := json.Unmarshal([]byte(snapshot), &specification, json.RejectUnknownMembers(true)); err != nil {
			return modelprovider.Chat{}, err
		}
		if err := original.Validate(); err != nil || original.Model != specification.Model || original.Provider != specification.Connection.Provider {
			return modelprovider.Chat{}, errors.Join(err, errors.New("model provider: original construction identity changed"))
		}
		var handle modelprovider.Handle
		handle, created, err = s.insertClient(ctx, "chat", specification)
		if err != nil {
			return modelprovider.Chat{}, err
		}
		restored := chatProjection(handle, created, specification, original.Original)
		expected := original
		expected.Handle = handle
		expectedJSON, err := json.Marshal(expected)
		if err != nil {
			return modelprovider.Chat{}, err
		}
		restoredJSON, err := json.Marshal(restored)
		if err != nil {
			return modelprovider.Chat{}, err
		}
		if string(expectedJSON) != string(restoredJSON) {
			return modelprovider.Chat{}, errors.New("model provider: original construction facts changed")
		}
		return restored, nil
	})
	if err == nil && created != nil {
		s.mu.Lock()
		s.clients[value.ID] = created
		s.mu.Unlock()
	}
	return value, err
}

func (s *store) acquire(ctx context.Context, handle modelprovider.Handle, kind string) (*client, func(), error) {
	if err := handle.Validate(); err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.clients[handle.ID]
	if entry == nil {
		var storedKind string
		var snapshot sql.NullString
		if err := s.db.QueryRowContext(ctx, `SELECT kind,snapshot FROM clients WHERE id=?`, handle.ID).Scan(&storedKind, &snapshot); err != nil {
			return nil, nil, err
		}
		if !snapshot.Valid || storedKind != kind {
			return nil, nil, errors.New("model provider: captured client is unavailable")
		}
		var input modelprovider.Open
		if err := json.Unmarshal([]byte(snapshot.String), &input, json.RejectUnknownMembers(true)); err != nil {
			return nil, nil, err
		}
		var err error
		entry, err = constructClient(ctx, kind, input)
		if err != nil {
			return nil, nil, err
		}
		s.clients[handle.ID] = entry
	}
	if entry.closed || (kind == "chat" && entry.chat == nil) || (kind == "embedding" && entry.embed == nil) {
		return nil, nil, errors.New("model provider: captured client is unavailable")
	}
	entry.active.Add(1)
	return entry, entry.active.Done, nil
}

func (s *store) closeClient(ctx context.Context, handle modelprovider.Handle) error {
	if err := handle.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	entry := s.clients[handle.ID]
	if entry != nil {
		entry.closed = true
	}
	// The durable tombstone fences lazy reconstruction before accepted calls
	// join. Existing users hold the concrete client, rather than re-reading it.
	_, err := s.db.ExecContext(ctx, `UPDATE clients SET snapshot=NULL WHERE id=?`, handle.ID)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if entry != nil {
		entry.active.Wait()
		s.mu.Lock()
		delete(s.clients, handle.ID)
		s.mu.Unlock()
	}
	return nil
}

func (s *store) Close() error {
	s.mu.Lock()
	entries := make([]*client, 0, len(s.clients))
	for _, entry := range s.clients {
		entry.closed = true
		entries = append(entries, entry)
	}
	s.mu.Unlock()
	for _, entry := range entries {
		entry.active.Wait()
	}
	return s.db.Close()
}

func chatProjection(handle modelprovider.Handle, client *client, input modelprovider.Open, original string) modelprovider.Chat {
	result := modelprovider.Chat{Handle: handle, Original: original, Provider: input.Connection.Provider, Model: input.Model, Streaming: client.stream != nil, TokenCounting: client.count != nil}
	if metadata, ok := catalog.Default.Lookup(input.Connection.Provider, input.Model); ok {
		copy := metadata.Clone()
		result.Metadata = &copy
	}
	return result
}

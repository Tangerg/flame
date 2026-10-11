package managedmcp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"

	memory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const maximumModelInput = 512 * 1024

// Two active targets, with six bytes per code point for worst-case JSON escaping.
const maximumEmbeddingInput = memory.MaxVisiblePerTarget*2*(memory.MaxContentCharacters*6+3) + 1

// The embedding JSON is itself encoded as a string inside the model envelope.
const maximumModelEnvelope = maximumEmbeddingInput*2 + 64*1024
const maximumModelResult = 64 * 1024 * 1024

type Embeddings interface {
	ID() string
	Embed(context.Context, []string) ([][]float32, error)
}
type Prompt struct {
	Operation, SystemPrompt, UserPrompt string
	MaxInputBytes                       int
	MaxOutputTokens                     int64
}
type Completion func(context.Context, Prompt) (string, error)

func (c Completion) Complete(ctx context.Context, prompt Prompt) (string, error) {
	return c(ctx, prompt)
}

type modelGrant struct {
	session   *sdk.ClientSession
	context   context.Context
	complete  Completion
	embed     Embeddings
	remaining int
	cancel    context.CancelFunc
	pending   sync.WaitGroup
	release   func()
}

// Models owns an invocation-scoped loopback capability. Only the selected
// managed backend receives its unguessable URL. Closing the invocation or
// retiring its MCP session revokes the listener and joins admitted calls.
// This closed operation does not expose the Runtime protocol or provider secrets.
type Models struct {
	mu       sync.Mutex
	grants   map[string]*modelGrant
	sessions map[*sdk.ClientSession]struct{}
}

func NewModels() *Models {
	return &Models{grants: make(map[string]*modelGrant), sessions: make(map[*sdk.ClientSession]struct{})}
}

// AdmitSession delegates model-capability admission from the connection owner.
// Retirement removes admission before joining grants, including when no grant existed.
func (m *Models) AdmitSession(session *sdk.ClientSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session] = struct{}{}
}

func (m *Models) Grant(ctx context.Context, session *sdk.ClientSession, complete Completion, embed Embeddings) (string, func(), error) {
	if session == nil || ctx == nil {
		return "", nil, errors.New("managed plugin: invocation and backend session are required")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("managed plugin: open model capability: %w", err)
	}
	var entropy [32]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		listener.Close()
		return "", nil, err
	}
	token := hex.EncodeToString(entropy[:])
	lifetime, cancel := context.WithCancel(ctx)
	grant := &modelGrant{session: session, context: lifetime, complete: complete, embed: embed, remaining: 2, cancel: cancel}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/"+token || r.URL.RawQuery != "" {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maximumModelEnvelope))
		if err != nil {
			http.Error(w, "model envelope refused", http.StatusBadRequest)
			return
		}
		var prompt Prompt
		if err := json.Unmarshal(body, &prompt, json.RejectUnknownMembers(true)); err != nil {
			http.Error(w, "model envelope refused", http.StatusBadRequest)
			return
		}
		value, err := m.complete(r.Context(), session, token, prompt)
		if err != nil {
			http.Error(w, "model operation failed", http.StatusUnprocessableEntity)
			return
		}
		if len(value) > maximumModelResult {
			http.Error(w, "model result exceeds limit", http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		encoded, err := json.Marshal(value)
		if err != nil {
			http.Error(w, "model result encoding failed", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(encoded)
	})}
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Serve(listener) }()
	var once sync.Once
	release := func() {
		once.Do(func() {
			m.mu.Lock()
			delete(m.grants, token)
			m.mu.Unlock()
			cancel()
			_ = server.Close()
			<-done
			grant.pending.Wait()
		})
	}
	grant.release = release
	m.mu.Lock()
	if _, admitted := m.sessions[session]; !admitted {
		m.mu.Unlock()
		release()
		return "", nil, errors.New("managed plugin: backend model admission was withdrawn")
	}
	m.grants[token] = grant
	m.mu.Unlock()
	stop := context.AfterFunc(lifetime, release)
	return "http://" + listener.Addr().String() + "/" + token, func() { stop(); release() }, nil
}

func (m *Models) RevokeSession(session *sdk.ClientSession) {
	m.mu.Lock()
	delete(m.sessions, session)
	var releases []func()
	for _, grant := range m.grants {
		if grant.session == session {
			releases = append(releases, grant.release)
		}
	}
	m.mu.Unlock()
	for _, release := range releases {
		release()
	}
}

func (m *Models) complete(ctx context.Context, session *sdk.ClientSession, token string, prompt Prompt) (string, error) {
	m.mu.Lock()
	grant := m.grants[token]
	if grant == nil || grant.session != session || grant.context.Err() != nil || grant.remaining == 0 {
		m.mu.Unlock()
		return "", errors.New("managed plugin: model grant is absent, expired or exhausted")
	}
	grant.remaining--
	grant.pending.Add(1)
	m.mu.Unlock()
	defer grant.pending.Done()
	limit := maximumModelInput
	if prompt.Operation == "memory.embed" {
		limit = maximumEmbeddingInput
	}
	if prompt.MaxInputBytes <= 0 || prompt.MaxInputBytes > limit || prompt.MaxOutputTokens <= 0 || prompt.MaxOutputTokens > 2048 || len(prompt.SystemPrompt)+len(prompt.UserPrompt) > prompt.MaxInputBytes {
		return "", errors.New("managed plugin: model envelope refused")
	}
	lifetime, cancel := context.WithCancelCause(ctx)
	stop := context.AfterFunc(grant.context, func() { cancel(context.Cause(grant.context)) })
	defer func() { stop(); cancel(nil) }()
	if err := grant.context.Err(); err != nil {
		return "", err
	}
	switch prompt.Operation {
	case "memory.extract", "memory.curate":
		if grant.complete == nil {
			return "", errors.New("managed plugin: completion was not granted")
		}
		return grant.complete(lifetime, prompt)
	case "memory.embed":
		if grant.embed == nil {
			return "", errors.New("managed plugin: embedding was not granted")
		}
		var texts []string
		if err := json.Unmarshal([]byte(prompt.UserPrompt), &texts); err != nil {
			return "", err
		}
		if len(texts) == 0 || len(texts) > memory.MaxVisiblePerTarget*2 {
			return "", errors.New("managed plugin: embedding count refused")
		}
		vectors, err := grant.embed.Embed(lifetime, texts)
		if err != nil {
			return "", err
		}
		encoded, err := json.Marshal(vectors)
		return string(encoded), err
	default:
		return "", errors.New("managed plugin: model operation refused")
	}
}

type RemoteModels struct{ Grant, Space string }

func (r RemoteModels) Complete(ctx context.Context, prompt Prompt) (string, error) {
	endpoint, err := url.Parse(r.Grant)
	if err != nil || endpoint.Scheme != "http" || endpoint.Hostname() != "127.0.0.1" || endpoint.Port() == "" || len(endpoint.Path) != 65 || endpoint.RawQuery != "" || endpoint.User != nil || endpoint.Fragment != "" {
		return "", errors.New("managed plugin: model capability refused")
	}
	encoded, err := json.Marshal(prompt)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.Grant, bytes.NewReader(encoded))
	if err != nil {
		return "", err
	}
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		// The URL is a live capability; retain only the transport cause.
		if transport, ok := errors.AsType[*url.Error](err); ok {
			return "", fmt.Errorf("managed plugin: model transport: %w", transport.Err)
		}
		return "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumModelResult+1))
	if err != nil {
		return "", err
	}
	if len(body) > maximumModelResult {
		return "", errors.New("managed plugin: model result exceeds limit")
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("managed plugin: model capability failed (%d)", response.StatusCode)
	}
	var value string
	err = json.Unmarshal(body, &value)
	return value, err
}
func (r RemoteModels) ID() string { return r.Space }
func (r RemoteModels) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	encoded, err := json.Marshal(texts)
	if err != nil {
		return nil, err
	}
	text, err := r.Complete(ctx, Prompt{Operation: "memory.embed", UserPrompt: string(encoded), MaxInputBytes: maximumEmbeddingInput, MaxOutputTokens: 2048})
	if err != nil {
		return nil, err
	}
	var vectors [][]float32
	err = json.Unmarshal([]byte(text), &vectors)
	return vectors, err
}

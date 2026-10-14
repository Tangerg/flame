package plugin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"sync"
	"time"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

var ErrStaleCall = errors.New("plugin: callback call is no longer active")

const callbackMetadataKey = "io.github.tangerg.flame/callback.v1"

type callbackRefKey struct{}

func CallbackMetadata(ref CallbackRef) (sdk.ParamsBase, error) {
	if err := ref.Validate(); err != nil {
		return sdk.ParamsBase{}, err
	}
	return sdk.ParamsBase{Meta: sdk.Meta{callbackMetadataKey: ref}}, nil
}

func CallbackFromContext(ctx context.Context) (CallbackRef, error) {
	ref, present := ctx.Value(callbackRefKey{}).(CallbackRef)
	if !present {
		return CallbackRef{}, errors.New("plugin: captured callback reference is required")
	}
	return ref, nil
}

func bindCallbackMetadata(ctx context.Context, metadata sdk.Meta) (context.Context, error) {
	value, present := metadata[callbackMetadataKey]
	if !present {
		return ctx, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var ref CallbackRef
	if err := json.Unmarshal(encoded, &ref, json.RejectUnknownMembers(true)); err != nil {
		return nil, errors.New("plugin: callback metadata is invalid")
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	return context.WithValue(ctx, callbackRefKey{}, ref), nil
}

// Callback is an immutable binding to an existing operation contract. The
// handler owns its business behavior, effect identities and resource budget.
type Callback struct{ method boundMethod }

func NewCallback[P, R any](method *Method[P, R], classify func(error) string, handler func(context.Context, Command[P]) (R, error)) (Callback, error) {
	if method == nil || handler == nil {
		return Callback{}, errors.New("plugin: callback method and handler are required")
	}
	bound := method.bind(classify, func(ctx context.Context, _ *sdk.ServerSession, command Command[P]) (R, error) {
		return handler(ctx, command)
	})
	return Callback{method: bound}, nil
}

// CallbackRef is an ephemeral projection of one activation and owning call.
// It must not be logged, put in a package or reused after the call ends.
type CallbackRef struct {
	Endpoint         string `json:"endpoint"`
	Credential       string `json:"credential"`
	Call             string `json:"call"`
	MaxResponseBytes int64  `json:"maxResponseBytes"`
}

func (r CallbackRef) Validate() error {
	endpoint, err := url.Parse(r.Endpoint)
	if err != nil || endpoint.Scheme != "http" || endpoint.Hostname() != "127.0.0.1" || endpoint.Port() == "" || endpoint.Path != "/" || endpoint.RawQuery != "" || endpoint.User != nil || endpoint.Fragment != "" || r.MaxResponseBytes <= 0 || r.MaxResponseBytes == math.MaxInt64 {
		return errors.New("plugin: callback reference is invalid")
	}
	for _, token := range []string{r.Credential, r.Call} {
		if len(token) != 64 {
			return errors.New("plugin: callback reference is invalid")
		}
		if _, err := hex.DecodeString(token); err != nil {
			return errors.New("plugin: callback reference is invalid")
		}
	}
	return nil
}

type CallbackLimits struct {
	MaxRequestBytes  int64
	MaxResponseBytes int64
}

type callbackActivation struct {
	credential string
	calls      map[string]*callbackCall
	retiring   bool
	done       chan struct{}
}

type callbackCall struct {
	context  context.Context
	cancel   context.CancelCauseFunc
	handler  http.Handler
	limits   CallbackLimits
	pending  sync.WaitGroup
	done     chan struct{}
	stop     func() bool
	released bool
}

// Callbacks owns one lazily opened loopback endpoint for the Runtime instance.
// Connection retirement withdraws admission before canceling and joining user
// code. No owner lock is held while a handler performs I/O or a nested call.
type Callbacks struct {
	mu          sync.Mutex
	activations map[*sdk.ClientSession]*callbackActivation
	credentials map[string]*callbackActivation
	server      *http.Server
	endpoint    string
	served      chan struct{}
	closed      bool
	done        chan struct{}
	closeErr    error
	serveErr    error
	rpc         http.Handler
	rpcReady    chan struct{}
}

func NewCallbacks() *Callbacks {
	return &Callbacks{activations: make(map[*sdk.ClientSession]*callbackActivation), credentials: make(map[string]*callbackActivation), done: make(chan struct{}), rpcReady: make(chan struct{})}
}

func (c *Callbacks) AdmitSession(session *sdk.ClientSession) {
	if session == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed && c.activations[session] == nil {
		activation := &callbackActivation{credential: callbackToken(), calls: make(map[string]*callbackCall), done: make(chan struct{})}
		c.activations[session] = activation
		c.credentials[activation.credential] = activation
	}
}

func callbackToken() string {
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(token[:])
}

func (c *Callbacks) Open(ctx context.Context, session *sdk.ClientSession, limits CallbackLimits, callbacks ...Callback) (CallbackRef, func(), error) {
	if len(callbacks) == 0 {
		return CallbackRef{}, nil, errors.New("plugin: callback operations are required")
	}
	methods, err := callbackMethods(callbacks)
	if err != nil {
		return CallbackRef{}, nil, err
	}
	return c.open(ctx, session, limits, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		serveCallback(w, r, methods, limits)
	}))
}

func callbackMethods(callbacks []Callback) (map[string]boundMethod, error) {
	methods := make(map[string]boundMethod, len(callbacks))
	for _, callback := range callbacks {
		method := callback.method
		if method.contract == nil || method.invoke == nil && method.stream == nil || methods[method.contract.Name()].contract != nil {
			return nil, errors.New("plugin: callback operations must be valid and unique")
		}
		methods[method.contract.Name()] = method
	}
	return methods, nil
}

type callbackLimitsKey struct{}

// PublishRPC publishes the canonical Runtime RPC transport once before any
// backend can receive its reference. Publication owns the startup dependency
// knot. Call-bound host collaborators share its endpoint; core business
// operations retain the canonical RPC handler and its Delivery semantics.
func (c *Callbacks) PublishRPC(handler http.Handler, callbacks ...Callback) error {
	if handler == nil {
		return errors.New("plugin: runtime RPC handler is required")
	}
	switch value := reflect.ValueOf(handler); value.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Map, reflect.Slice:
		if value.IsNil() {
			return errors.New("plugin: runtime RPC handler is required")
		}
	}
	methods, err := callbackMethods(callbacks)
	if err != nil {
		return err
	}
	if len(methods) > 0 {
		rpc := handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				rpc.ServeHTTP(w, r)
				return
			}
			limits, ok := r.Context().Value(callbackLimitsKey{}).(CallbackLimits)
			if !ok {
				http.Error(w, "callback call limits are required", http.StatusForbidden)
				return
			}
			serveCallback(w, r, methods, limits)
		})
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.rpc != nil {
		return errors.New("plugin: runtime RPC publication is closed")
	}
	c.rpc = handler
	close(c.rpcReady)
	return nil
}

func (c *Callbacks) WaitRPC(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return errors.Join(ctx.Err(), context.Cause(ctx))
	case <-c.rpcReady:
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.rpc == nil {
		return ErrStaleCall
	}
	return nil
}

// OpenRPC captures one call lifetime for the already published Runtime contract.
func (c *Callbacks) OpenRPC(ctx context.Context, session *sdk.ClientSession, limits CallbackLimits) (CallbackRef, func(), error) {
	c.mu.Lock()
	handler := c.rpc
	c.mu.Unlock()
	if handler == nil {
		return CallbackRef{}, nil, errors.New("plugin: runtime RPC is not published")
	}
	return c.open(ctx, session, limits, handler)
}

// OpenRPCWithInvocation binds backend-owned background work through an admission
// projection. The authenticated activation owns the Actor; wire context cannot
// replace it. The projection must validate retained calling-owner evidence.
func (c *Callbacks) OpenRPCWithInvocation(ctx context.Context, session *sdk.ClientSession, limits CallbackLimits, project func(context.Context, capability.Invocation) (context.Context, error)) (CallbackRef, func(), error) {
	if project == nil {
		return CallbackRef{}, nil, errors.New("plugin: background invocation projection is required")
	}
	c.mu.Lock()
	handler := c.rpc
	c.mu.Unlock()
	if handler == nil {
		return CallbackRef{}, nil, errors.New("plugin: runtime RPC is not published")
	}
	return c.open(ctx, session, limits, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if value := r.Header.Get(InvocationHeader); value != "" {
			var invocation capability.Invocation
			if err := json.Unmarshal([]byte(value), &invocation, json.RejectUnknownMembers(true)); err != nil {
				http.Error(w, "invalid background invocation", http.StatusBadRequest)
				return
			}
			calling, err := project(r.Context(), invocation)
			if err != nil || calling == nil {
				http.Error(w, "background invocation refused", http.StatusForbidden)
				return
			}
			r = r.WithContext(calling)
		}
		handler.ServeHTTP(w, r)
	}))
}

const InvocationHeader = "Flame-Invocation"

func (c *Callbacks) open(ctx context.Context, session *sdk.ClientSession, limits CallbackLimits, handler http.Handler) (CallbackRef, func(), error) {
	if ctx == nil || session == nil || limits.MaxRequestBytes <= 0 || limits.MaxResponseBytes <= 0 || limits.MaxResponseBytes == math.MaxInt64 {
		return CallbackRef{}, nil, errors.New("plugin: callback context, session and capacities are required")
	}
	c.mu.Lock()
	activation := c.activations[session]
	if c.closed || activation == nil || activation.retiring {
		c.mu.Unlock()
		return CallbackRef{}, nil, ErrStaleCall
	}
	if c.serveErr != nil {
		c.mu.Unlock()
		return CallbackRef{}, nil, fmt.Errorf("plugin: callback endpoint stopped: %w", c.serveErr)
	}
	if err := ctx.Err(); err != nil {
		c.mu.Unlock()
		return CallbackRef{}, nil, errors.Join(err, context.Cause(ctx))
	}
	if c.server == nil {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			c.mu.Unlock()
			return CallbackRef{}, nil, fmt.Errorf("plugin: open callback endpoint: %w", err)
		}
		c.server = &http.Server{Handler: http.HandlerFunc(c.serve)}
		c.endpoint = "http://" + listener.Addr().String() + "/"
		c.served = make(chan struct{})
		server, served := c.server, c.served
		go func() {
			defer close(served)
			if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
				c.mu.Lock()
				c.serveErr = err
				for _, activation := range c.activations {
					for _, call := range activation.calls {
						call.cancel(err)
					}
				}
				c.mu.Unlock()
			}
		}()
	}
	lifetime, cancel := context.WithCancelCause(ctx)
	call := &callbackCall{context: lifetime, cancel: cancel, handler: handler, limits: limits, done: make(chan struct{})}
	id := callbackToken()
	activation.calls[id] = call
	release := func() { c.release(activation, id, call) }
	call.stop = context.AfterFunc(lifetime, release)
	ref := CallbackRef{Endpoint: c.endpoint, Credential: activation.credential, Call: id, MaxResponseBytes: limits.MaxResponseBytes}
	c.mu.Unlock()
	return ref, release, nil
}

func (c *Callbacks) release(activation *callbackActivation, id string, call *callbackCall) {
	c.mu.Lock()
	owned := c.withdrawCallLocked(call)
	c.mu.Unlock()
	c.finishCall(activation, id, call, owned)
}

func (c *Callbacks) withdrawCallLocked(call *callbackCall) bool {
	if call.released {
		return false
	}
	call.released = true
	call.stop()
	call.cancel(ErrStaleCall)
	return true
}

func (c *Callbacks) finishCall(activation *callbackActivation, id string, call *callbackCall, owned bool) {
	if !owned {
		<-call.done
		return
	}
	call.pending.Wait()
	c.mu.Lock()
	delete(activation.calls, id)
	c.mu.Unlock()
	close(call.done)
}

func (c *Callbacks) RevokeSession(session *sdk.ClientSession) {
	c.mu.Lock()
	activation := c.activations[session]
	if activation == nil {
		c.mu.Unlock()
		return
	}
	if activation.retiring {
		c.mu.Unlock()
		<-activation.done
		return
	}
	activation.retiring = true
	delete(c.credentials, activation.credential)
	type retiringCall struct {
		call  *callbackCall
		owned bool
	}
	calls := make(map[string]retiringCall, len(activation.calls))
	for id, call := range activation.calls {
		calls[id] = retiringCall{call, c.withdrawCallLocked(call)}
	}
	c.mu.Unlock()
	for id, call := range calls {
		c.finishCall(activation, id, call.call, call.owned)
	}
	c.mu.Lock()
	delete(c.activations, session)
	c.mu.Unlock()
	close(activation.done)
}

func (c *Callbacks) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		<-c.done
		return c.closeErr
	}
	c.closed = true
	if c.rpc == nil {
		close(c.rpcReady)
	}
	sessions := make([]*sdk.ClientSession, 0, len(c.activations))
	for session := range c.activations {
		sessions = append(sessions, session)
		for _, call := range c.activations[session].calls {
			call.cancel(ErrStaleCall)
		}
	}
	server, served := c.server, c.served
	c.mu.Unlock()
	for _, session := range sessions {
		c.RevokeSession(session)
	}
	if server != nil {
		c.closeErr = server.Close()
		<-served
		c.mu.Lock()
		c.closeErr = errors.Join(c.closeErr, c.serveErr)
		c.mu.Unlock()
	}
	close(c.done)
	return c.closeErr
}

func (c *Callbacks) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	c.mu.Lock()
	activation := c.credentials[r.Header.Get("Authorization")]
	var call *callbackCall
	if activation != nil {
		call = activation.calls[r.Header.Get("Flame-Call")]
	}
	if c.closed || call == nil || call.released || call.context.Err() != nil {
		c.mu.Unlock()
		http.Error(w, "callback call is no longer active", http.StatusGone)
		return
	}
	call.pending.Add(1)
	c.mu.Unlock()
	defer call.pending.Done()
	lifetime, cancel := context.WithCancelCause(call.context)
	networkContext, body := r.Context(), r.Body
	stop := context.AfterFunc(networkContext, func() { cancel(context.Cause(networkContext)) })
	bodyClosed := make(chan struct{})
	closeBody := context.AfterFunc(lifetime, func() {
		defer close(bodyClosed)
		controller := http.NewResponseController(w)
		_ = controller.SetReadDeadline(time.Now())
		_ = controller.SetWriteDeadline(time.Now())
		_ = body.Close()
	})
	defer func() {
		stop()
		if !closeBody() {
			<-bodyClosed
		}
		cancel(nil)
	}()
	// The owner context supplies target, origin and parent projections. The
	// network request can cancel them, but cannot replace them with input fields.
	request := r.WithContext(context.WithValue(lifetime, callbackLimitsKey{}, call.limits))
	request.Body = http.MaxBytesReader(w, body, call.limits.MaxRequestBytes)
	call.handler.ServeHTTP(w, request)
}

func serveCallback(w http.ResponseWriter, r *http.Request, methods map[string]boundMethod, limits CallbackLimits) {
	body, err := io.ReadAll(r.Body)
	var wire invokeParams
	if err == nil {
		err = json.Unmarshal(body, &wire, json.RejectUnknownMembers(true))
	}
	method, found := methods[wire.Method]
	if err != nil || !found || wire.Contract != method.contract.Identity().Digest() {
		http.Error(w, "callback contract or input refused", http.StatusBadRequest)
		return
	}
	if r.Context().Err() != nil {
		http.Error(w, "callback call is no longer active", http.StatusGone)
		return
	}
	if method.stream != nil {
		if r.Header.Get("Accept") != "text/event-stream" {
			http.Error(w, "callback requires streaming consumption", http.StatusNotAcceptable)
			return
		}
		serveCallbackStream(w, r, method, &wire, limits)
		return
	}
	result, err := method.invoke(r.Context(), nil, &wire)
	if err != nil {
		http.Error(w, "callback operation could not be confirmed", http.StatusUnprocessableEntity)
		return
	}
	encoded, err := json.Marshal(result)
	if err != nil || int64(len(encoded)) > limits.MaxResponseBytes {
		http.Error(w, "callback response capacity exceeded", http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(encoded)
}

func CallHost[P, R any](ctx context.Context, ref CallbackRef, method *Method[P, R], id string, input P) (R, error) {
	var zero R
	if err := ref.Validate(); err != nil {
		return zero, err
	}
	if method == nil {
		return zero, errors.New("plugin: callback method is required")
	}
	if method.contract.Kind() == capability.Mutation {
		if err := validateCommandID(id); err != nil {
			return zero, err
		}
	} else if id != "" {
		return zero, errors.New("plugin: query must not carry a command identity")
	}
	admitted, err := jsoninput.Encode(input)
	if err != nil {
		return zero, err
	}
	if err := method.contract.ValidateInput(admitted); err != nil {
		return zero, err
	}
	encoded, err := json.Marshal(invokeParams{Protocol: capability.Protocol, Method: method.Name(), Contract: method.contract.Identity().Digest(), Input: admitted.JSON(), CommandID: id})
	if err != nil {
		return zero, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, ref.Endpoint, bytes.NewReader(encoded))
	if err != nil {
		return zero, err
	}
	request.Header.Set("Authorization", ref.Credential)
	request.Header.Set("Flame-Call", ref.Call)
	request.Header.Set("Content-Type", "application/json")
	if err := setCallbackInvocation(request, ctx); err != nil {
		return zero, err
	}
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		if failure, ok := errors.AsType[*url.Error](err); ok {
			return zero, fmt.Errorf("plugin: callback transport: %w", errors.Join(failure.Err, ctx.Err(), context.Cause(ctx)))
		}
		return zero, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, ref.MaxResponseBytes+1))
	if err != nil {
		return zero, err
	}
	if int64(len(body)) > ref.MaxResponseBytes {
		return zero, errors.New("plugin: callback response capacity exceeded")
	}
	if response.StatusCode == http.StatusGone {
		return zero, ErrStaleCall
	}
	if response.StatusCode != http.StatusOK {
		return zero, fmt.Errorf("plugin: callback could not be confirmed (%d)", response.StatusCode)
	}
	var result Result[jsontext.Value]
	if err := json.Unmarshal(body, &result, json.RejectUnknownMembers(true)); err != nil {
		return zero, err
	}
	if _, err := result.value(method.Name()); err != nil {
		return zero, err
	}
	if err := method.contract.ValidateOutput(result.valueJSON); err != nil {
		return zero, fmt.Errorf("plugin: callback output: %w", err)
	}
	typed, err := decodeBoundResult[R](&result)
	if err != nil {
		return zero, err
	}
	return typed.value(method.Name())
}

// Only activation-scoped RPC grants admit a projected background invocation.
// Ordinary call/next grants keep their captured owner regardless of this header.
func setCallbackInvocation(request *http.Request, ctx context.Context) error {
	invocation, found := ctx.Value(invocationKey{}).(capability.Invocation)
	if !found {
		return nil
	}
	if err := invocation.Validate(); err != nil {
		return err
	}
	encoded, err := json.Marshal(invocation)
	if err != nil {
		return err
	}
	request.Header.Set(InvocationHeader, string(encoded))
	return nil
}

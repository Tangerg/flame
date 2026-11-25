package mcp

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"reflect"

	framecall "github.com/Tangerg/frame/call"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type callbackLimitsKey struct{}

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
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.rpc != nil {
		return errors.New("plugin: runtime RPC publication is closed")
	}
	c.rpc = handler
	c.rpcMethods = methods
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

func (c *Callbacks) OpenRPC(ctx context.Context, session *sdk.ClientSession, limits CallbackLimits, callbacks ...Callback) (CallbackRef, func(), error) {
	handler, err := c.rpcHandler(callbacks)
	if err != nil {
		return CallbackRef{}, nil, err
	}
	return c.open(ctx, session, limits, handler)
}

func (c *Callbacks) rpcHandler(callbacks []Callback) (http.Handler, error) {
	methods, err := callbackMethods(callbacks)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	handler := c.rpc
	published := c.rpcMethods
	c.mu.Unlock()
	if handler == nil {
		return nil, errors.New("plugin: runtime RPC is not published")
	}
	for name, method := range published {
		if _, found := methods[name]; found {
			return nil, errors.New("plugin: call callback cannot replace a published host method")
		}
		methods[name] = method
	}
	if len(methods) == 0 {
		return handler, nil
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			handler.ServeHTTP(w, r)
			return
		}
		limits, ok := r.Context().Value(callbackLimitsKey{}).(CallbackLimits)
		if !ok {
			http.Error(w, "callback call limits are required", http.StatusForbidden)
			return
		}
		serveCallback(w, r, methods, limits)
	}), nil
}

func (c *Callbacks) OpenRPCWithInvocation(ctx context.Context, session *sdk.ClientSession, limits CallbackLimits, project func(context.Context, framecall.Invocation) (context.Context, error)) (CallbackRef, func(), error) {
	if project == nil {
		return CallbackRef{}, nil, errors.New("plugin: background invocation projection is required")
	}
	handler, err := c.rpcHandler(nil)
	if err != nil {
		return CallbackRef{}, nil, err
	}
	return c.open(ctx, session, limits, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if value := r.Header.Get(InvocationHeader); value != "" {
			var invocation framecall.Invocation
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

const (
	InvocationHeader = "Flame-Invocation"
	CallHeader       = "Flame-Call"
)

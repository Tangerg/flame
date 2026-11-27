package main

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/Tangerg/acp"
	"github.com/Tangerg/acp/jsonrpc"
)

// session/fork is an advertised OpenCode operation that predates the standard
// ACP methods implemented by the SDK. Keep that compatibility at the wire boundary.
const forkExtension = "_flame/opencode-fork"

type openCodeTransport struct {
	acp.Transport
	fork       atomic.Bool
	promptSent func(acp.SessionID)
}

func (t *openCodeTransport) Connect(ctx context.Context) (acp.Connection, error) {
	connection, err := t.Transport.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &openCodeConnection{Connection: connection, supported: &t.fork, promptSent: t.promptSent}, nil
}

type openCodeConnection struct {
	acp.Connection
	supported  *atomic.Bool
	mu         sync.Mutex
	initID     jsonrpc.ID
	promptSent func(acp.SessionID)
}

func (c *openCodeConnection) Write(ctx context.Context, message jsonrpc.Message) error {
	var prompt acp.PromptRequest
	if req, ok := message.(*jsonrpc.Request); ok {
		switch req.Method {
		case "initialize":
			c.mu.Lock()
			c.initID = req.ID
			c.mu.Unlock()
		case "session/prompt":
			_ = json.Unmarshal(req.Params, &prompt)
		case forkExtension:
			message = &jsonrpc.Request{ID: req.ID, Params: req.Params, Method: "session/fork"}
		}
	}
	err := c.Connection.Write(ctx, message)
	if err == nil && prompt.SessionID != "" {
		c.promptSent(prompt.SessionID)
	}
	return err
}

func (c *openCodeConnection) Read(ctx context.Context) (jsonrpc.Message, error) {
	message, err := c.Connection.Read(ctx)
	if response, ok := message.(*jsonrpc.Response); ok && err == nil {
		c.mu.Lock()
		initial := c.initID.IsValid() && response.ID == c.initID
		if initial {
			c.initID = jsonrpc.ID{}
		}
		c.mu.Unlock()
		if initial {
			var result struct {
				Capabilities struct {
					Session struct {
						Fork *struct{} `json:"fork"`
					} `json:"sessionCapabilities"`
				} `json:"agentCapabilities"`
			}
			if json.Unmarshal(response.Result, &result) == nil {
				c.supported.Store(result.Capabilities.Session.Fork != nil)
			}
		}
	}
	return message, err
}

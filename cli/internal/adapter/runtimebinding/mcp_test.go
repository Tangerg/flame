package runtimebinding

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/application/integration/mcp"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

const (
	adapterMCPAuthorizationAttemptID      = "mcpauth_AAAAAAAAAAAAAAAAAAAAAAAAAA"
	adapterOtherMCPAuthorizationAttemptID = "mcpauth_BBBBBBBBBBBBBBBBBBBBBBBBBB"
)

type mcpBindingStub struct {
	t            *testing.T
	actions      []string
	created      protocol.MCPServerCandidate
	updated      protocol.UpdateMCPServerRequest
	authErr      error
	authGet      *protocol.MCPAuthorizationAttempt
	now          time.Time
	createResult *protocol.MCPServer
	updateResult *protocol.MCPServer
}

func (m *mcpBindingStub) ListMCPServers(_ context.Context, options flameruntime.CallOptions) (*protocol.Page[protocol.MCPServer], error) {
	m.assertMeta(options.RequestMeta)
	return protocol.NewPage([]protocol.MCPServer{wireMCPServer()}), nil
}

func (m *mcpBindingStub) CreateMCPServer(_ context.Context, request protocol.MCPServerCandidate, options flameruntime.CommandOptions) (*protocol.MCPServer, error) {
	m.assertCommand("create", options)
	m.created = request
	if m.createResult != nil {
		return m.createResult, nil
	}
	server := wireMCPServerFromCandidate(request)
	return &server, nil
}

func (m *mcpBindingStub) UpdateMCPServer(_ context.Context, request protocol.UpdateMCPServerRequest, options flameruntime.CommandOptions) (*protocol.MCPServer, error) {
	m.assertCommand("update", options)
	m.updated = request
	if m.updateResult != nil {
		return m.updateResult, nil
	}
	server := wireMCPServer()
	server.Name = request.Server
	if request.Enabled != nil {
		if *request.Enabled {
			server.Status = protocol.MCPServerState{Type: protocol.MCPServerDisconnected}
		} else {
			server.Status = protocol.MCPServerState{Type: protocol.MCPServerDisabled}
		}
	}
	if request.Description != nil {
		server.Description = *request.Description
	}
	if request.Connection != nil {
		server.Connection = wireMCPConnection(*request.Connection)
	}
	if request.HandshakeTimeout != nil {
		server.HandshakeTimeout = *request.HandshakeTimeout
	}
	if request.DisabledTools != nil {
		server.DisabledTools = append([]string(nil), (*request.DisabledTools)...)
	}
	if request.AutoApproveTools != nil {
		server.AutoApproveTools = append([]string(nil), (*request.AutoApproveTools)...)
	}
	return &server, nil
}

func (m *mcpBindingStub) DeleteMCPServer(_ context.Context, request protocol.MCPServerRequest, options flameruntime.CommandOptions) error {
	m.assertCommand("delete:"+request.Server, options)
	return nil
}

func (m *mcpBindingStub) TestMCPServer(_ context.Context, request protocol.MCPServerCandidate, options flameruntime.CallOptions) (*protocol.MCPTestResult, error) {
	m.assertMeta(options.RequestMeta)
	m.actions = append(m.actions, "test:"+request.Name)
	return &protocol.MCPTestResult{Error: &protocol.ProblemData{Type: protocol.ProblemMCPDialFailed}}, nil
}

func (m *mcpBindingStub) ListMCPTools(_ context.Context, request protocol.MCPListToolsRequest, options flameruntime.CallOptions) (*protocol.Page[protocol.MCPTool], error) {
	m.assertMeta(options.RequestMeta)
	m.actions = append(m.actions, "tools:"+request.Server)
	return protocol.NewPage([]protocol.MCPTool{{
		Server: "docs", Name: "search", Description: "Search docs",
		InputSchema: map[string]any{"type": "object"},
	}}), nil
}

func (m *mcpBindingStub) ReconnectMCPServer(_ context.Context, request protocol.MCPServerRequest, options flameruntime.CommandOptions) error {
	m.assertCommand("reconnect:"+request.Server, options)
	return nil
}

func (m *mcpBindingStub) CreateMCPAuthorizationAttempt(_ context.Context, request protocol.CreateMCPAuthorizationAttemptRequest, options flameruntime.CommandOptions) (*protocol.MCPAuthorizationAttempt, error) {
	m.assertCommand("authorize:"+request.Server, options)
	return &protocol.MCPAuthorizationAttempt{
		ID: adapterMCPAuthorizationAttemptID, Server: request.Server,
		Status:    protocol.MCPAuthorizationAttemptStatus{Type: protocol.MCPAuthorizationAttemptPending},
		CreatedAt: m.now,
	}, nil
}

func (m *mcpBindingStub) GetMCPAuthorizationAttempt(_ context.Context, request protocol.MCPAuthorizationAttemptRequest, options flameruntime.CallOptions) (*protocol.MCPAuthorizationAttempt, error) {
	m.assertMeta(options.RequestMeta)
	m.actions = append(m.actions, "authorization:"+request.AttemptID)
	if m.authErr != nil {
		return nil, m.authErr
	}
	if m.authGet != nil {
		return m.authGet, nil
	}
	finished := m.now.Add(time.Second)
	return &protocol.MCPAuthorizationAttempt{
		ID: request.AttemptID, Server: "docs", Status: protocol.MCPAuthorizationAttemptStatus{Type: protocol.MCPAuthorizationAttemptSucceeded},
		CreatedAt: m.now, FinishedAt: &finished,
	}, nil
}

func (m *mcpBindingStub) assertMeta(meta protocol.RequestMeta) {
	m.t.Helper()
	if meta.ProtocolVersion != protocol.ProtocolVersion {
		m.t.Fatalf("MCP request meta = %+v", meta)
	}
}

func (m *mcpBindingStub) assertCommand(action string, options flameruntime.CommandOptions) {
	m.t.Helper()
	m.assertMeta(options.RequestMeta)
	if options.IdempotencyKey == "" {
		m.t.Fatalf("MCP command options = %+v", options)
	}
	m.actions = append(m.actions, action)
}

func wireMCPServer() protocol.MCPServer {
	count := 1
	return protocol.MCPServer{
		Name: "docs", Description: "Documentation", HandshakeTimeout: boundedWireHandshakeTimeout(15),
		Connection: protocol.MCPConnection{
			Type: protocol.MCPTransportStreamableHTTP, URL: "https://mcp.example/tools",
			AuthorizationMasked: "Bearer ****", HeadersMasked: map[string]string{"X-Key": "****"},
		},
		Status: protocol.MCPServerState{Type: protocol.MCPServerConnected, ToolCount: &count},
	}
}

func wireMCPServerFromCandidate(candidate protocol.MCPServerCandidate) protocol.MCPServer {
	state := protocol.MCPServerState{Type: protocol.MCPServerDisconnected}
	if !candidate.Enabled {
		state.Type = protocol.MCPServerDisabled
	}
	return protocol.MCPServer{
		Name: candidate.Name, Description: candidate.Description,
		Connection: wireMCPConnection(candidate.Connection), HandshakeTimeout: candidate.HandshakeTimeout,
		DisabledTools:    append([]string(nil), candidate.DisabledTools...),
		AutoApproveTools: append([]string(nil), candidate.AutoApproveTools...), Status: state,
	}
}

func boundedWireHandshakeTimeout(seconds int) protocol.MCPHandshakeTimeout {
	return protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeBounded, Seconds: &seconds}
}

func wireMCPConnection(input protocol.MCPConnectionInput) protocol.MCPConnection {
	connection := protocol.MCPConnection{
		Type: input.Type, URL: input.URL, Command: input.Command,
		Args: append([]string(nil), input.Args...), Dir: input.Dir,
	}
	if input.Authorization != nil && input.Authorization.Type == protocol.MCPSecretSet {
		connection.AuthorizationMasked = "****"
	}
	if input.Headers != nil && input.Headers.Type == protocol.MCPSecretSet {
		connection.HeadersMasked = make(map[string]string, len(input.Headers.Value))
		for key := range input.Headers.Value {
			connection.HeadersMasked[key] = "****"
		}
	}
	if input.Env != nil && input.Env.Type == protocol.MCPSecretSet {
		connection.EnvMasked = make(map[string]string, len(input.Env.Value))
		for key := range input.Env.Value {
			connection.EnvMasked[key] = "****"
		}
	}
	return connection
}

func TestMCPAdapterProjectsEveryServerToolAndAuthorizationOperation(t *testing.T) {
	stub := &mcpBindingStub{t: t, now: time.Unix(100, 0)}
	runtime := &Connection{mcp: stub, meta: requestMeta("test")}
	servers, err := runtime.Servers(t.Context())
	if err != nil || len(servers) != 1 || servers[0].State.Type != protocol.MCPServerConnected || servers[0].Connection.AuthorizationMasked == "" {
		t.Fatalf("Servers = (%+v, %v)", servers, err)
	}
	authorization := mcp.AuthorizationChange{Kind: protocol.MCPSecretSet, Value: "Bearer secret"}
	candidate := mcp.Candidate{
		Name: "new-docs", Enabled: true,
		Connection: mcp.ConnectionInput{Transport: protocol.MCPTransportStreamableHTTP, URL: "https://new.example/tools", Authorization: &authorization},
	}
	if _, createServerErr := runtime.CreateServer(t.Context(), candidate); createServerErr != nil {
		t.Fatal(createServerErr)
	}
	if stub.created.Connection.Authorization == nil || stub.created.Connection.Authorization.Value != "Bearer secret" {
		t.Fatalf("created candidate = %+v", stub.created)
	}
	description := "Updated docs"
	enabled := false
	update := mcp.ServerUpdate{Server: "docs", Enabled: &enabled, Description: &description}
	if _, updateServerErr := runtime.UpdateServer(t.Context(), update); updateServerErr != nil {
		t.Fatal(updateServerErr)
	}
	if stub.updated.Enabled == nil || *stub.updated.Enabled || stub.updated.Description == nil || *stub.updated.Description != description {
		t.Fatalf("updated request = %+v", stub.updated)
	}
	if deleteServerErr := runtime.DeleteServer(t.Context(), "docs"); deleteServerErr != nil {
		t.Fatal(deleteServerErr)
	}
	tested, err := runtime.TestServer(t.Context(), candidate)
	if err != nil || tested.OK || tested.Problem == nil || tested.Problem.Type != "mcp_dial_failed" {
		t.Fatalf("TestServer = (%+v, %v)", tested, err)
	}
	tools, err := runtime.Tools(t.Context(), "docs")
	if err != nil || len(tools) != 1 || string(tools[0].InputSchema) != `{"type":"object"}` {
		t.Fatalf("Tools = (%+v, %v)", tools, err)
	}
	if reconnectServerErr := runtime.ReconnectServer(t.Context(), "docs"); reconnectServerErr != nil {
		t.Fatal(reconnectServerErr)
	}
	attempt, err := runtime.StartAuthorization(t.Context(), "docs")
	if err != nil || attempt.Status.Type != protocol.MCPAuthorizationAttemptPending || attempt.ID != adapterMCPAuthorizationAttemptID {
		t.Fatalf("StartAuthorization = (%+v, %v)", attempt, err)
	}
	attempt, err = runtime.GetAuthorization(t.Context(), mcp.AuthorizationReferenceFrom(attempt))
	if err != nil || attempt.Status.Type != protocol.MCPAuthorizationAttemptSucceeded || attempt.FinishedAt == nil {
		t.Fatalf("GetAuthorization = (%+v, %v)", attempt, err)
	}
	if len(stub.actions) != 8 {
		t.Fatalf("MCP actions = %v", stub.actions)
	}
}

func TestMCPAuthorizationAdapterClassifiesAbsenceAndEnforcesReferenceIdentity(t *testing.T) {
	server := wireMCPServer()
	if _, err := projectMCPServerResult("update MCP server", "other", &server, nil); !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("mismatched MCP server identity = %v, want ErrIncompatibleRuntime", err)
	}

	stub := &mcpBindingStub{t: t, now: time.Unix(100, 0), authErr: protocol.ErrMCPAuthorizationAttemptNotFound}
	runtime := &Connection{mcp: stub, meta: requestMeta("test")}
	if _, err := runtime.GetAuthorization(t.Context(), mcp.AuthorizationReference{ID: "auth_1", Server: "docs"}); err == nil || !strings.Contains(err.Error(), "attemptId") {
		t.Fatalf("non-canonical authorization reference = %v, want attemptId error", err)
	}
	if len(stub.actions) != 0 {
		t.Fatalf("invalid authorization reference reached Runtime: %v", stub.actions)
	}
	reference := mcp.AuthorizationReference{ID: adapterMCPAuthorizationAttemptID, Server: "docs"}
	if _, err := runtime.GetAuthorization(t.Context(), reference); !errors.Is(err, mcp.ErrAuthorizationAttemptNotFound) {
		t.Fatalf("missing authorization = %v, want ErrAuthorizationAttemptNotFound", err)
	}

	finished := stub.now.Add(time.Second)
	stub.authErr = nil
	stub.authGet = &protocol.MCPAuthorizationAttempt{
		ID: adapterOtherMCPAuthorizationAttemptID, Server: "docs",
		Status:    protocol.MCPAuthorizationAttemptStatus{Type: protocol.MCPAuthorizationAttemptSucceeded},
		CreatedAt: stub.now, FinishedAt: &finished,
	}
	if _, err := runtime.GetAuthorization(t.Context(), reference); !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("mismatched authorization identity = %v, want ErrIncompatibleRuntime", err)
	}
	stub.authGet.ID = reference.ID
	stub.authGet.Server = "other"
	if _, err := runtime.GetAuthorization(t.Context(), reference); !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("mismatched authorization server = %v, want ErrIncompatibleRuntime", err)
	}
	stub.authGet.ID = "auth_1"
	stub.authGet.Server = reference.Server
	if _, err := runtime.GetAuthorization(t.Context(), reference); !errors.Is(err, agent.ErrIncompatibleRuntime) || !strings.Contains(err.Error(), "id") {
		t.Fatalf("invalid Runtime authorization identity = %v, want contract violation for id", err)
	}
}

func TestMCPAdapterRejectsMutationAcknowledgementDrift(t *testing.T) {
	t.Parallel()
	authorization := mcp.AuthorizationChange{Kind: protocol.MCPSecretSet, Value: "Bearer secret"}
	candidate := mcp.Candidate{
		Name: "docs", Enabled: true, Description: "Documentation",
		Connection: mcp.ConnectionInput{
			Transport: protocol.MCPTransportStreamableHTTP, URL: "https://mcp.example/tools", Authorization: &authorization,
		},
	}
	projectedCandidate, err := projectMCPCandidate(candidate)
	if err != nil {
		t.Fatal(err)
	}
	createResult := wireMCPServerFromCandidate(projectedCandidate)
	createResult.Description = "ignored"
	description := "Updated"
	enabled := false
	update := mcp.ServerUpdate{Server: candidate.Name, Enabled: &enabled, Description: &description}
	updateResult := wireMCPServer()
	updateResult.Status = protocol.MCPServerState{Type: protocol.MCPServerDisabled}
	updateResult.Description = "ignored"
	tests := []struct {
		name   string
		stub   *mcpBindingStub
		invoke func(*Connection) error
	}{
		{
			name: "create fields",
			stub: &mcpBindingStub{createResult: &createResult},
			invoke: func(runtime *Connection) error {
				_, err := runtime.CreateServer(t.Context(), candidate)
				return err
			},
		},
		{
			name: "update fields",
			stub: &mcpBindingStub{updateResult: &updateResult},
			invoke: func(runtime *Connection) error {
				_, err := runtime.UpdateServer(t.Context(), update)
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.stub.t, test.stub.now = t, time.Unix(100, 0)
			runtime := &Connection{mcp: test.stub, meta: requestMeta("test")}
			requireRuntimeContractViolation(t, test.invoke(runtime))
		})
	}
}

func TestMCPAdapterRejectsWritesOutsideRuntimeWireContract(t *testing.T) {
	t.Parallel()
	validConnection := mcp.ConnectionInput{
		Transport: protocol.MCPTransportStreamableHTTP,
		URL:       "https://mcp.example/tools",
	}
	tests := []struct {
		name   string
		invoke func(*Connection) error
		field  string
	}{
		{
			name: "candidate server name",
			invoke: func(runtime *Connection) error {
				_, err := runtime.CreateServer(t.Context(), mcp.Candidate{Name: "Docs", Connection: validConnection})
				return err
			},
			field: "name",
		},
		{
			name: "candidate tool name",
			invoke: func(runtime *Connection) error {
				_, err := runtime.TestServer(t.Context(), mcp.Candidate{
					Name: "docs", Connection: validConnection, DisabledTools: []string{"invalid tool"},
				})
				return err
			},
			field: "disabledTools[0]",
		},
		{
			name: "update server name",
			invoke: func(runtime *Connection) error {
				description := "updated"
				_, err := runtime.UpdateServer(t.Context(), mcp.ServerUpdate{Server: "Docs", Description: &description})
				return err
			},
			field: "server",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stub := &mcpBindingStub{t: t}
			runtime := &Connection{mcp: stub, meta: requestMeta("test")}
			err := test.invoke(runtime)
			if err == nil || !strings.Contains(err.Error(), test.field) {
				t.Fatalf("write error = %v, want field %q", err, test.field)
			}
			if len(stub.actions) != 0 {
				t.Fatalf("invalid write reached Runtime: %v", stub.actions)
			}
		})
	}
}

func TestMCPAdapterRejectsInvalidServerIdentityBeforeDispatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		invoke func(*Connection) error
	}{
		{
			name: "delete",
			invoke: func(runtime *Connection) error {
				return runtime.DeleteServer(t.Context(), "Docs")
			},
		},
		{
			name: "reconnect",
			invoke: func(runtime *Connection) error {
				return runtime.ReconnectServer(t.Context(), "Docs")
			},
		},
		{
			name: "list tools",
			invoke: func(runtime *Connection) error {
				_, err := runtime.Tools(t.Context(), "Docs")
				return err
			},
		},
		{
			name: "start authorization",
			invoke: func(runtime *Connection) error {
				_, err := runtime.StartAuthorization(t.Context(), "Docs")
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stub := &mcpBindingStub{t: t}
			runtime := &Connection{mcp: stub, meta: requestMeta("test")}
			err := test.invoke(runtime)
			if err == nil || !strings.Contains(err.Error(), "server") {
				t.Fatalf("operation error = %v, want server field", err)
			}
			if len(stub.actions) != 0 {
				t.Fatalf("invalid operation reached Runtime: %v", stub.actions)
			}
		})
	}
}

func TestMCPReadProjectionsRejectValuesOutsideRuntimeWireContract(t *testing.T) {
	t.Parallel()
	server := wireMCPServer()
	server.Name = "Docs"
	if _, err := projectMCPServer(server); err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("server projection error = %v, want name field", err)
	}
	tool := protocol.MCPTool{Server: "docs", Name: "invalid tool"}
	if _, err := projectMCPTool(tool); err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("tool projection error = %v, want name field", err)
	}
}

func TestMCPAdapterClassifiesBoundedContextErrors(t *testing.T) {
	tests := []struct {
		source error
		target error
	}{
		{protocol.ErrMCPServerNotFound, mcp.ErrServerNotFound},
		{protocol.ErrMCPServerAlreadyExists, mcp.ErrServerAlreadyExists},
		{protocol.ErrMCPServerDisabled, mcp.ErrServerDisabled},
		{protocol.ErrMCPAuthorizationAttemptNotFound, mcp.ErrAuthorizationAttemptNotFound},
	}
	for _, test := range tests {
		classified := classifyMCPError(test.source)
		if !errors.Is(classified, test.source) || !errors.Is(classified, test.target) {
			t.Errorf("classifyMCPError(%v) = %v, want source and bounded-context identities", test.source, classified)
		}
	}
}

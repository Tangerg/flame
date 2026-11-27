package terminal

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/integration/mcp"
	"github.com/Tangerg/flame/apps/cli/internal/delivery/format"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const mcpToolCommandArguments = 3

type mcpScreens struct {
	screenHost
	service                    MCPManagement
	form                       *mcpFormFlow
	pendingAuthorizationStarts map[protocol.MCPServerID]mcpAuthorizationStart
	setApprovalRule            func(protocol.SetApprovalRuleRequest) error
}

func (m *mcpScreens) ShowMCPServers() {
	m.reader.show(m.mcpServersReaderQuery())
}

func (m *mcpScreens) mcpServersReaderQuery() *readerQuery {
	return &readerQuery{
		status:  "loading MCP servers",
		follows: followsTopic(protocol.TopicMCPChanged),
		read: func(ctx context.Context) (readerDocument, error) {
			servers, err := m.service.Servers(ctx)
			if err != nil {
				return readerDocument{}, err
			}
			return mcpServersDocument(servers)
		},
	}
}

func mcpServersDocument(servers []protocol.MCPServer) (readerDocument, error) {
	if len(servers) == 0 {
		return paragraphDocument("MCP servers", "none configured", []string{"No MCP servers are configured."}), nil
	}
	sections := make([]toolSection, 0, len(servers))
	for _, server := range servers {
		detail, err := mcpServerDetail(server)
		if err != nil {
			return readerDocument{}, err
		}
		sections = append(sections, toolSection{
			Title: mcp.ServerLabel(server.ID) + format.Separator + mcpStateLabel(server.Status), Style: toolSectionCode,
			Text: detail,
		})
	}
	return readerDocument{Title: "MCP servers", Detail: fmt.Sprintf("%d configured", len(servers)), Sections: sections}, nil
}

func mcpServerDetail(server protocol.MCPServer) (string, error) {
	lines := []string{}
	if server.ID.Origin.Type == protocol.MCPOriginInstallation {
		lines = append(lines, "installation  "+server.ID.Origin.InstallationID, "configuration  managed through flame plugins")
	}
	if server.Description != "" {
		lines = append(lines, "description  "+server.Description)
	}
	switch server.Connection.Type {
	case protocol.MCPTransportStreamableHTTP:
		lines = append(lines, "transport    streamable HTTP", "url          "+server.Connection.URL)
		if server.Connection.AuthorizationMasked != "" {
			lines = append(lines, "authorization  "+server.Connection.AuthorizationMasked)
		}
		if len(server.Connection.HeadersMasked) > 0 {
			lines = append(lines, "headers      "+formatMaskedMap(server.Connection.HeadersMasked))
		}
	case protocol.MCPTransportStdio:
		lines = append(lines, "transport    stdio", "command      "+server.Connection.Command)
		if len(server.Connection.Args) > 0 {
			lines = append(lines, "args         "+strings.Join(server.Connection.Args, " "))
		}
		if server.Connection.Dir != "" {
			lines = append(lines, "directory    "+server.Connection.Dir)
		}
		if len(server.Connection.EnvMasked) > 0 {
			lines = append(lines, "environment  "+formatMaskedMap(server.Connection.EnvMasked))
		}
	}
	if server.HandshakeTimeout.Type == protocol.MCPHandshakeBounded {
		lines = append(lines, fmt.Sprintf("handshake timeout  %ds", *server.HandshakeTimeout.Seconds))
	}

	problem, err := mcpStatusProblem(server.Status.Error)
	if err != nil {
		return "", err
	}
	if problem != "" {
		lines = append(lines, "problem      "+problem)
	}
	return strings.Join(lines, "\n"), nil
}

func mcpStatusProblem(problem *protocol.MCPStatusProblem) (string, error) {
	if problem == nil {
		return "", nil
	}
	switch problem.Type {
	case protocol.MCPStatusAuthorizationRequired:
		return "authorization is required; sign in to the MCP server or update its credentials", nil
	case protocol.MCPStatusAuthorizationFailed:
		return "sign-in did not complete; start authorization again", nil
	case protocol.MCPStatusDialFailed:
		return "the connection failed; check the endpoint or command and reconnect", nil
	case protocol.MCPStatusToolDiscoveryFailed:
		return "the server connected but did not provide a valid tool list", nil
	case protocol.MCPStatusConfigurationFailed:
		return "the configuration or stored credentials could not be used; review the server settings", nil
	default:
		return "", fmt.Errorf("runtime contract violation: unknown MCP status category %q", problem.Type)
	}
}

func mcpStateLabel(state protocol.MCPServerState) string {
	if state.Type == protocol.MCPServerConnected && state.ToolCount != nil {
		return fmt.Sprintf("connected · %d tools", *state.ToolCount)
	}
	return string(state.Type)
}

func formatMaskedMap(values map[string]string) string {
	keys := sortedKeys(values)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values[key])
	}
	return strings.Join(parts, ", ")
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func (m *mcpScreens) ShowMCPTools(argument string) {
	var server *protocol.MCPServerID
	if strings.TrimSpace(argument) != "" {
		parsed, err := mcp.ParseServerReference(argument)
		if err != nil {
			m.message(err.Error())
			return
		}
		server = &parsed
	}
	m.reader.show(m.mcpToolsReaderQuery(server))
}

func (m *mcpScreens) mcpToolsReaderQuery(server *protocol.MCPServerID) *readerQuery {
	return &readerQuery{
		status:  "loading MCP tools",
		follows: followsTopic(protocol.TopicMCPChanged),
		read: func(ctx context.Context) (readerDocument, error) {
			tools, err := m.service.Tools(ctx, server)
			if err != nil {
				return readerDocument{}, err
			}
			document, err := mcpToolsDocument(server, tools)
			if err != nil {
				return readerDocument{}, err
			}
			var servers []protocol.MCPServerID
			if server != nil {
				servers = append(servers, *server)
			} else {
				configured, err := m.service.Servers(ctx)
				if err != nil {
					return readerDocument{}, err
				}
				for _, source := range configured {
					servers = append(servers, source.ID)
				}
			}
			for _, id := range servers {
				exposure, err := m.service.ToolExposure(ctx, id)
				if err != nil {
					return readerDocument{}, err
				}
				if len(exposure.DisabledTools) > 0 {
					document.Sections = append(document.Sections, toolSection{Title: "Disabled tools · " + mcp.ServerLabel(id), Style: toolSectionParagraph, Text: strings.Join(exposure.DisabledTools, ", ")})
				}
			}
			return document, nil
		},
	}
}

func mcpToolsDocument(server *protocol.MCPServerID, tools []protocol.MCPTool) (readerDocument, error) {
	detail := fmt.Sprintf("%d advertised", len(tools))
	if server != nil {
		detail += format.Separator + mcp.ServerLabel(*server)
	}
	if len(tools) == 0 {
		return paragraphDocument("MCP tools", detail, []string{"No MCP tools match this server filter."}), nil
	}
	sections := make([]toolSection, 0, len(tools)*2)
	for _, tool := range tools {
		title := mcp.ServerLabel(tool.Server) + "/" + tool.Name
		sections = append(sections, toolSection{Title: title, Style: toolSectionParagraph, Text: tool.Description})
		if len(tool.NameConflicts) > 0 {
			sources := make([]string, 0, len(tool.NameConflicts))
			for _, ref := range tool.NameConflicts {
				source, err := mcp.ToolLabel(ref)
				if err != nil {
					return readerDocument{}, err
				}
				sources = append(sources, source)
			}
			sections = append(sections, toolSection{Title: "Instance-qualified model name", Style: toolSectionParagraph, Text: tool.ModelName + " distinguishes this source from " + strings.Join(sources, ", ")})
		}
		if tool.InputSchema != nil {
			schema, err := json.Marshal(tool.InputSchema, jsontext.WithIndent("  "), json.Deterministic(true))
			if err != nil {
				return readerDocument{}, fmt.Errorf("format MCP tool %s input schema: %w", title, err)
			}
			sections = append(sections, jsonSection("Input schema", string(schema)))
		}
	}
	return readerDocument{Title: "MCP tools", Detail: detail, Sections: sections}, nil
}

func (m *mcpScreens) OpenMCPCreateForm() error {
	m.openMCPServerForm(mcpFormCreate, protocol.MCPServer{})
	return nil
}

func (m *mcpScreens) OpenMCPProbeForm() error {
	m.openMCPServerForm(mcpFormProbe, protocol.MCPServer{})
	return nil
}

func (m *mcpScreens) EditMCPServer(argument string) error {
	if strings.TrimSpace(argument) == "" {
		return errors.New("usage: /mcp-edit <server>")
	}
	id, err := mcp.ParseServerReference(argument)
	if err != nil {
		return err
	}
	return m.loadEditableMCPServer(id, func(server protocol.MCPServer) {
		m.openMCPServerForm(mcpFormUpdate, server)
	})
}

func (m *mcpScreens) loadEditableMCPServer(id protocol.MCPServerID, apply func(protocol.MCPServer)) error {
	presentation := m.session.context
	m.status.note("loading MCP server " + mcp.ServerLabel(id))
	started := m.tasks.application(mcpOperation,
		func(ctx context.Context) (protocol.MCPServer, error) {
			servers, err := m.service.Servers(ctx)
			if err != nil {
				return protocol.MCPServer{}, err
			}
			for _, server := range servers {
				if server.ID == id {
					return server, nil
				}
			}
			return protocol.MCPServer{}, errors.New("MCP server not found: " + mcp.ServerLabel(id))
		},
		func(server protocol.MCPServer, err error) {
			if err != nil {
				m.message("load MCP server failed: " + err.Error())
				return
			}
			if !m.session.context.current(presentation) {
				m.message("MCP server loaded after the active session changed; repeat the command to continue")
				return
			}
			if server.ID.Origin.Type == protocol.MCPOriginInstallation {
				m.message("MCP connection configuration is owned by installation " + server.ID.Origin.InstallationID + "; use flame plugins configure")
				return
			}
			apply(server)
		},
	)
	if !started {
		return mcpOperation.busy()
	}
	return nil
}

func (m *mcpScreens) PrepareDeleteMCPServer(argument string) error {
	if strings.TrimSpace(argument) == "" {
		return errors.New("usage: /mcp-delete <server>")
	}
	id, err := mcp.ParseServerReference(argument)
	if err != nil {
		return err
	}
	return m.loadEditableMCPServer(id, func(server protocol.MCPServer) {
		target := protocol.MCPDirectoryTarget{Server: server.ID, ExpectedRevision: server.Revision}
		label := mcp.ServerLabel(server.ID)
		m.confirm("Delete MCP server", "Delete "+label+" and its live connection?", "Delete permanently", "", func() {
			m.runMCPAck("deleting MCP server "+label, func(ctx context.Context) error {
				return m.service.DeleteServer(ctx, target)
			})
		})
	})
}

func (m *mcpScreens) ReconnectMCPServer(argument string) error {
	if strings.TrimSpace(argument) == "" {
		return errors.New("usage: /mcp-reconnect <server>")
	}
	server, err := mcp.ParseServerReference(argument)
	if err != nil {
		return err
	}
	m.runMCPAck("requesting MCP reconnect "+mcp.ServerLabel(server), func(ctx context.Context) error { return m.service.ReconnectServer(ctx, server) })
	return nil
}

func (m *mcpScreens) runMCPAck(label string, command func(context.Context) error) {
	m.status.note(label)
	started := m.tasks.admissionMutation(mcpOperation,
		func(ctx context.Context) (struct{}, error) { return struct{}{}, command(ctx) },
		func(_ struct{}, err error) {
			if err != nil {
				m.message(label + " failed: " + err.Error())
				return
			}
			m.message(label + " accepted")
		},
	)
	if !started {
		m.message(mcpOperation.busy().Error())
	}
}

const (
	mcpToolEnable  = "enable"
	mcpToolDisable = "disable"
)

func (m *mcpScreens) ConfigureMCPTool(arguments string) error {
	parts := strings.Fields(arguments)
	if len(parts) != mcpToolCommandArguments {
		return errors.New("usage: /mcp-tool <server> <tool> <enable|disable|allow|deny>")
	}
	server, err := mcp.ParseServerReference(parts[0])
	if err != nil {
		return err
	}
	name, action := parts[1], parts[2]
	switch action {
	case mcpToolEnable, mcpToolDisable:
		request := protocol.SetMCPToolExposureRequest{Server: server, Name: name, Disabled: action == mcpToolDisable}
		if err := protocol.ValidateWireTree(request); err != nil {
			return err
		}
		m.runMCPAck("setting tool exposure "+mcp.ServerLabel(server)+"/"+name, func(ctx context.Context) error { return m.service.SetToolExposure(ctx, request) })
	case string(protocol.ApprovalRuleDecisionAllow), string(protocol.ApprovalRuleDecisionDeny):
		request := protocol.SetApprovalRuleRequest{Subject: protocol.ApprovalSubject{Type: protocol.ApprovalSubjectAll}, Tool: protocol.ToolRef{Type: protocol.ToolRefMCP, Server: &server, Name: name}, Scope: protocol.ApprovalRuleScopeGlobal, Decision: protocol.ApprovalRuleDecision(action)}
		if err := protocol.ValidateWireTree(request); err != nil {
			return err
		}
		return m.setApprovalRule(request)
	default:
		return errors.New("tool action must be enable, disable, allow, or deny")
	}
	return nil
}

package terminal

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/cli/internal/application/integration/mcp"
	"github.com/Tangerg/flame/cli/internal/application/retry"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
)

const mcpAuthorizationPollInterval = 500 * time.Millisecond

func (a *app) ShowMCPServers() {
	if a.mcp == nil {
		a.message("this runtime composition has no MCP service")
		return
	}
	a.executeRuntimeReaderQuery(a.mcpServersReaderQuery())
}

func (a *app) mcpServersReaderQuery() runtimeReaderQuery {
	return runtimeReaderQuery{
		status: "loading MCP servers",
		mode:   runtimeReaderMCPServers,
		read: func(ctx context.Context) (readerDocument, error) {
			servers, err := a.mcp.Servers(ctx)
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
	sections := make([]ToolSection, 0, len(servers))
	for _, server := range servers {
		detail, err := mcpServerDetail(server)
		if err != nil {
			return readerDocument{}, err
		}
		sections = append(sections, ToolSection{
			Title: mcp.ServerLabel(server.ID) + " · " + mcpStateLabel(server.Status), Style: toolSectionCode,
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

// mcpStatusProblem renders the closed MCP status category as the action it
// asks of the user. The wire contract, validated at the binding, already ties
// each category to the states that may carry it, so this only names it.
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
	case protocol.MCPStatusReleaseUnavailable:
		return "the plugin release cannot be verified; reinstall or select another release", nil
	case protocol.MCPStatusBackendUnavailable:
		return "the plugin backend cannot be prepared; check the plugin's data directory", nil
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

func (a *app) ShowMCPTools(argument string) {
	if a.mcp == nil {
		a.message("this runtime composition has no MCP service")
		return
	}
	var server *protocol.MCPServerID
	if strings.TrimSpace(argument) != "" {
		parsed, err := mcp.ParseServerReference(argument)
		if err != nil {
			a.message(err.Error())
			return
		}
		server = &parsed
	}
	a.dialogs.mcpToolServer = server
	a.executeRuntimeReaderQuery(a.mcpToolsReaderQuery(server))
}

func (a *app) mcpToolsReaderQuery(server *protocol.MCPServerID) runtimeReaderQuery {
	return runtimeReaderQuery{
		status: "loading MCP tools",
		mode:   runtimeReaderMCPTools,
		read: func(ctx context.Context) (readerDocument, error) {
			tools, err := a.mcp.Tools(ctx, server)
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
				configured, err := a.mcp.Servers(ctx)
				if err != nil {
					return readerDocument{}, err
				}
				for _, source := range configured {
					servers = append(servers, source.ID)
				}
			}
			for _, id := range servers {
				exposure, err := a.mcp.ToolExposure(ctx, id)
				if err != nil {
					return readerDocument{}, err
				}
				if len(exposure.DisabledTools) > 0 {
					document.Sections = append(document.Sections, ToolSection{Title: "Disabled tools · " + mcp.ServerLabel(id), Style: toolSectionParagraph, Text: strings.Join(exposure.DisabledTools, ", ")})
				}
			}
			return document, nil
		},
	}
}

func mcpToolsDocument(server *protocol.MCPServerID, tools []protocol.MCPTool) (readerDocument, error) {
	detail := fmt.Sprintf("%d advertised", len(tools))
	if server != nil {
		detail += " · " + mcp.ServerLabel(*server)
	}
	if len(tools) == 0 {
		return paragraphDocument("MCP tools", detail, []string{"No MCP tools match this server filter."}), nil
	}
	sections := make([]ToolSection, 0, len(tools)*2)
	for _, tool := range tools {
		title := mcp.ServerLabel(tool.Server) + "/" + tool.Name
		sections = append(sections, ToolSection{Title: title, Style: toolSectionParagraph, Text: tool.Description})
		if len(tool.NameConflicts) > 0 {
			sources := make([]string, 0, len(tool.NameConflicts))
			for _, ref := range tool.NameConflicts {
				source, err := mcp.ToolLabel(ref)
				if err != nil {
					return readerDocument{}, err
				}
				sources = append(sources, source)
			}
			sections = append(sections, ToolSection{Title: "Excluded from model tools", Style: toolSectionParagraph, Text: tool.ModelName + " conflicts with " + strings.Join(sources, ", ")})
		}
		if tool.InputSchema != nil {
			schema, err := json.Marshal(tool.InputSchema, jsontext.WithIndent("  "), json.Deterministic(true))
			if err != nil {
				return readerDocument{}, fmt.Errorf("format MCP tool %s input schema: %w", title, err)
			}
			sections = append(sections, ToolSection{Title: "Input schema", Style: toolSectionCode, Language: "json", Text: string(schema)})
		}
	}
	return readerDocument{Title: "MCP tools", Detail: detail, Sections: sections}, nil
}

func (a *app) OpenMCPCreateForm() error {
	if a.mcp == nil {
		return errors.New("this runtime composition has no MCP service")
	}
	a.openMCPServerForm(mcpFormCreate, protocol.MCPServer{})
	return nil
}

func (a *app) OpenMCPProbeForm() error {
	if a.mcp == nil {
		return errors.New("this runtime composition has no MCP service")
	}
	a.openMCPServerForm(mcpFormProbe, protocol.MCPServer{})
	return nil
}

func (a *app) EditMCPServer(argument string) error {
	if a.mcp == nil {
		return errors.New("this runtime composition has no MCP service")
	}
	if strings.TrimSpace(argument) == "" {
		return errors.New("usage: /mcp-edit <server>")
	}
	id, err := mcp.ParseServerReference(argument)
	if err != nil {
		return err
	}
	presentation := a.session.context
	a.status.note("loading MCP server " + mcp.ServerLabel(id))
	started := a.runApplicationOperation(mcpOperation, false,
		func(ctx context.Context) (protocol.MCPServer, error) {
			servers, err := a.mcp.Servers(ctx)
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
				a.message("load MCP server failed: " + err.Error())
				return
			}
			if !a.session.context.current(presentation) {
				a.message("MCP server loaded after the active session changed; reopen the editor to continue")
				return
			}
			if server.ID.Origin.Type == protocol.MCPOriginInstallation {
				a.message("MCP connection configuration is owned by installation " + server.ID.Origin.InstallationID + "; use flame plugins configure")
				return
			}
			a.openMCPServerForm(mcpFormUpdate, server)
		},
	)
	if !started {
		return errors.New("another MCP operation is running")
	}
	return nil
}

func (a *app) createMCPServer(candidate mcp.Candidate) {
	a.runMCPServerOperation("creating MCP server "+candidate.Name,
		func(ctx context.Context) (protocol.MCPServer, error) { return a.mcp.CreateServer(ctx, candidate) })
}

func (a *app) updateMCPServer(update mcp.ServerUpdate) {
	a.runMCPServerOperation("updating MCP server "+mcp.ServerLabel(update.Server),
		func(ctx context.Context) (protocol.MCPServer, error) { return a.mcp.UpdateServer(ctx, update) })
}

func (a *app) runMCPServerOperation(label string, change func(context.Context) (protocol.MCPServer, error)) {
	presentation := a.session.context
	a.status.note(label)
	started := a.runAdmissionMutation(mcpOperation, false, change, func(server protocol.MCPServer, err error) {
		if err != nil {
			a.message(label + " failed: " + err.Error())
			return
		}
		document, err := mcpServersDocument([]protocol.MCPServer{server})
		if err != nil {
			a.message(label + " returned an invalid result: " + err.Error())
			return
		}
		a.message(label + " complete")
		if !a.session.context.current(presentation) {
			return
		}
		a.setRuntimeReader(runtimeReaderMCPServers)
		a.dialogs.workspaceReader = workspaceReaderNone
		a.openReaderDocument(document)
		a.status.note("MCP server · " + mcp.ServerLabel(server.ID))
	})
	if !started {
		a.message("another MCP operation is running")
	}
}

func (a *app) probeMCPServer(candidate mcp.Candidate) {
	label := "testing MCP candidate " + candidate.Name
	a.status.note(label)
	started := a.runApplicationOperation(mcpOperation, false,
		func(ctx context.Context) (protocol.MCPTestOutcome, error) { return a.mcp.TestServer(ctx, candidate) },
		func(outcome protocol.MCPTestOutcome, err error) {
			if err == nil {
				var message string
				message, err = mcpProbeMessage(candidate.Name, outcome)
				if err == nil {
					a.message(message)
					return
				}
			}
			a.message(label + " failed: " + err.Error())
		},
	)
	if !started {
		a.message("another MCP operation is running")
	}
}

func (a *app) PrepareDeleteMCPServer(argument string) error {
	if a.mcp == nil {
		return errors.New("this runtime composition has no MCP service")
	}
	if strings.TrimSpace(argument) == "" {
		return errors.New("usage: /mcp-delete <server>")
	}
	server, err := mcp.ParseServerReference(argument)
	if err != nil {
		return err
	}
	a.confirmAction("Delete MCP server", "Delete "+mcp.ServerLabel(server)+" and its live connection?", "Delete permanently", func() {
		a.deleteMCPServer(server)
	})
	return nil
}

func (a *app) deleteMCPServer(server protocol.MCPServerID) {
	a.runMCPAck("deleting MCP server "+mcp.ServerLabel(server), func(ctx context.Context) error { return a.mcp.DeleteServer(ctx, server) })
}

func (a *app) ReconnectMCPServer(argument string) error {
	if a.mcp == nil {
		return errors.New("this runtime composition has no MCP service")
	}
	if strings.TrimSpace(argument) == "" {
		return errors.New("usage: /mcp-reconnect <server>")
	}
	server, err := mcp.ParseServerReference(argument)
	if err != nil {
		return err
	}
	a.runMCPAck("requesting MCP reconnect "+mcp.ServerLabel(server), func(ctx context.Context) error { return a.mcp.ReconnectServer(ctx, server) })
	return nil
}

func (a *app) runMCPAck(label string, command func(context.Context) error) {
	a.status.note(label)
	started := a.runAdmissionMutation(mcpOperation, false,
		func(ctx context.Context) (struct{}, error) { return struct{}{}, command(ctx) },
		func(_ struct{}, err error) {
			if err != nil {
				a.message(label + " failed: " + err.Error())
				return
			}
			a.message(label + " accepted")
		},
	)
	if !started {
		a.message("another MCP operation is running")
	}
}

func (a *app) AuthorizeMCPServer(argument string) error {
	if a.mcp == nil {
		return errors.New("this runtime composition has no MCP service")
	}
	if strings.TrimSpace(argument) == "" {
		return errors.New("usage: /mcp-auth <server>")
	}
	server, err := mcp.ParseServerReference(argument)
	if err != nil {
		return err
	}
	presentation := a.session.context
	a.status.note("starting MCP authorization " + mcp.ServerLabel(server))
	started := a.runAdmissionMutation(mcpAuthorizationOperation, false,
		func(ctx context.Context) (protocol.MCPAuthorizationAttempt, error) {
			return a.mcp.StartAuthorization(ctx, server)
		},
		func(attempt protocol.MCPAuthorizationAttempt, err error) {
			if err != nil {
				a.message("start MCP authorization failed: " + err.Error())
				return
			}
			if a.session.context.current(presentation) {
				a.dialogs.mcpAuthorizationID = attempt.ID
				a.setRuntimeReader(runtimeReaderMCPAuthorization)
				a.dialogs.workspaceReader = workspaceReaderNone
				a.openReaderDocument(mcpAuthorizationDocument(attempt))
			}
			if attempt.Status.Type == protocol.MCPAuthorizationAttemptPending {
				a.pollMCPAuthorization(attempt)
			}
		},
	)
	if !started {
		return errors.New("another MCP authorization is running")
	}
	return nil
}

func (a *app) pollMCPAuthorization(initial protocol.MCPAuthorizationAttempt) {
	observer := mcpAuthorizationObserver{
		service: a.mcp, pollInterval: mcpAuthorizationPollInterval, recovery: runtimeRecoveryBackoff,
	}
	started := a.runApplicationOperation(mcpAuthorizationOperation, false,
		func(ctx context.Context) (protocol.MCPAuthorizationAttempt, error) {
			return observer.observe(ctx, initial)
		},
		func(attempt protocol.MCPAuthorizationAttempt, err error) {
			if err != nil {
				a.message("observe MCP authorization failed: " + err.Error())
				return
			}
			if a.dialogs.runtimeReader == runtimeReaderMCPAuthorization && a.dialogs.mcpAuthorizationID == attempt.ID && a.dialogs.readerDialog.Open() {
				a.dialogs.reader.replace(mcpAuthorizationDocument(attempt), true, false)
			}
			a.message("MCP authorization " + string(attempt.Status.Type) + " · " + mcp.ServerLabel(attempt.Server))
		},
	)
	if !started {
		a.message("could not observe MCP authorization " + initial.ID)
	}
}

type mcpAuthorizationObserver struct {
	service      mcpAuthorizationReader
	pollInterval time.Duration
	recovery     retry.Backoff
}

type mcpAuthorizationReader interface {
	GetAuthorization(context.Context, mcp.AuthorizationReference) (protocol.MCPAuthorizationAttempt, error)
}

func (m mcpAuthorizationObserver) observe(
	ctx context.Context,
	initial protocol.MCPAuthorizationAttempt,
) (protocol.MCPAuthorizationAttempt, error) {
	if err := mcp.ValidateAuthorizationAttempt(initial); err != nil {
		return protocol.MCPAuthorizationAttempt{}, fmt.Errorf("observe MCP authorization: %w", err)
	}
	current := initial
	reference := mcp.AuthorizationReferenceFrom(initial)
	delay := m.pollInterval
	failures := 0
	for current.Status.Type == protocol.MCPAuthorizationAttemptPending {
		if err := retry.Wait(ctx, delay); err != nil {
			return protocol.MCPAuthorizationAttempt{}, err
		}
		next, err := m.service.GetAuthorization(ctx, reference)
		if err != nil {
			if !retry.IsReconnectable(err) {
				return protocol.MCPAuthorizationAttempt{}, err
			}
			failures++
			delay, err = m.recovery.Delay(failures)
			if err != nil {
				return protocol.MCPAuthorizationAttempt{}, err
			}
			continue
		}
		if err := mcp.ValidateAuthorizationAttempt(next); err != nil {
			return protocol.MCPAuthorizationAttempt{}, fmt.Errorf("observe MCP authorization: %w", err)
		}
		nextReference := mcp.AuthorizationReferenceFrom(next)
		if nextReference != reference {
			return protocol.MCPAuthorizationAttempt{}, fmt.Errorf(
				"%w: authorization observation moved from %+v to %+v",
				conversation.ErrIncompatibleRuntime,
				reference,
				nextReference,
			)
		}
		current = next
		failures = 0
		delay = m.pollInterval
	}
	return current, nil
}

func mcpAuthorizationDocument(attempt protocol.MCPAuthorizationAttempt) readerDocument {
	lines := []string{
		"attempt  " + attempt.ID,
		"server   " + mcp.ServerLabel(attempt.Server),
		"status   " + string(attempt.Status.Type),
		"started  " + attempt.CreatedAt.Format(time.RFC3339),
	}
	if attempt.FinishedAt != nil {
		lines = append(lines, "finished "+attempt.FinishedAt.Format(time.RFC3339))
	}
	problem, err := mcpStatusProblem(attempt.Status.Error)
	if err != nil {
		problem = err.Error()
	}
	if problem != "" {
		lines = append(lines, "problem  "+problem)
	}
	detail := "complete the sign-in in your browser"
	if attempt.Status.Type != protocol.MCPAuthorizationAttemptPending {
		detail = string(attempt.Status.Type)
	}
	return paragraphDocument("MCP authorization", detail, lines)
}

func (a *app) ConfigureMCPTool(arguments string) error {
	if a.mcp == nil {
		return errors.New("this runtime composition has no MCP service")
	}
	parts := strings.Fields(arguments)
	if len(parts) != 3 {
		return errors.New("usage: /mcp-tool <server> <tool> <enable|disable|allow|deny>")
	}
	server, err := mcp.ParseServerReference(parts[0])
	if err != nil {
		return err
	}
	name, action := parts[1], parts[2]
	switch action {
	case "enable", "disable":
		request := protocol.SetMCPToolExposureRequest{Server: server, Name: name, Disabled: action == "disable"}
		if err := protocol.ValidateWireTree(request); err != nil {
			return err
		}
		a.runMCPAck("setting tool exposure "+mcp.ServerLabel(server)+"/"+name, func(ctx context.Context) error { return a.mcp.SetToolExposure(ctx, request) })
	case "allow", "deny":
		request := protocol.SetApprovalRuleRequest{Subject: protocol.ApprovalSubject{Type: protocol.ApprovalSubjectAll}, Tool: protocol.ToolRef{Type: protocol.ToolRefMCP, Server: &server, Name: name}, Scope: protocol.ApprovalRuleScopeGlobal, Decision: protocol.ApprovalRuleDecision(action)}
		if err := protocol.ValidateWireTree(request); err != nil {
			return err
		}
		return a.setApprovalRule(request)
	default:
		return errors.New("tool action must be enable, disable, allow, or deny")
	}
	return nil
}

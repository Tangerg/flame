package terminal

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/flame/cli/internal/application/integration/mcp"
)

type mcpFormMode uint8

const (
	mcpFormCreate mcpFormMode = iota + 1
	mcpFormProbe
	mcpFormUpdate
)

type mcpFormDraft struct {
	name              string
	enabled           bool
	description       string
	replaceConnection bool
	transport         protocol.MCPTransport
	url               string
	authorizationMode formChange
	authorization     string
	headersMode       formChange
	headers           string
	command           string
	arguments         string
	environmentMode   formChange
	environment       string
	directory         string
	timeoutSeconds    string
	disabledTools     string
	autoApproveTools  string
}

func newMCPFormDraft(mode mcpFormMode, server protocol.MCPServer) mcpFormDraft {
	draft := mcpFormDraft{
		enabled: true, replaceConnection: true, transport: protocol.MCPTransportStreamableHTTP,
		authorizationMode: formChangeKeep, headersMode: formChangeKeep, environmentMode: formChangeKeep,
	}
	if mode != mcpFormUpdate {
		return draft
	}
	draft.name = server.Name
	if server.Status.Type == protocol.MCPServerDisabled {
		draft.enabled = false
	}
	draft.description = server.Description
	draft.replaceConnection = false
	draft.transport = server.Connection.Type
	draft.url = server.Connection.URL
	draft.command = server.Connection.Command
	if len(server.Connection.Args) > 0 {
		encoded, _ := json.Marshal(server.Connection.Args)
		draft.arguments = string(encoded)
	}
	draft.directory = server.Connection.Dir
	if server.HandshakeTimeout.Type == protocol.MCPHandshakeBounded {
		draft.timeoutSeconds = strconv.Itoa(*server.HandshakeTimeout.Seconds)
	}
	draft.disabledTools = strings.Join(server.DisabledTools, ", ")
	draft.autoApproveTools = strings.Join(server.AutoApproveTools, ", ")
	return draft
}

func (m mcpFormDraft) candidate() (mcp.Candidate, error) {
	if err := m.validateSelections(); err != nil {
		return mcp.Candidate{}, err
	}
	connection, err := m.connection()
	if err != nil {
		return mcp.Candidate{}, err
	}
	timeout, err := parseMCPTimeout(m.timeoutSeconds)
	if err != nil {
		return mcp.Candidate{}, err
	}
	candidate := mcp.Candidate{
		Name: strings.TrimSpace(m.name), Enabled: m.enabled,
		Description: strings.TrimSpace(m.description), Connection: connection,
		HandshakeTimeout: timeout, DisabledTools: parseMCPToolNames(m.disabledTools),
		AutoApproveTools: parseMCPToolNames(m.autoApproveTools),
	}
	return candidate, candidate.Validate()
}

func (m mcpFormDraft) update(original protocol.MCPServer) (mcp.ServerUpdate, bool, error) {
	if err := m.validateSelections(); err != nil {
		return mcp.ServerUpdate{}, false, err
	}
	timeout, err := parseMCPTimeout(m.timeoutSeconds)
	if err != nil {
		return mcp.ServerUpdate{}, false, err
	}
	update := mcp.ServerUpdate{Server: original.Name}
	enabled := m.enabled
	if enabled != (original.Status.Type != protocol.MCPServerDisabled) {
		update.Enabled = &enabled
	}
	description := strings.TrimSpace(m.description)
	if description != original.Description {
		update.Description = &description
	}
	if !timeout.Matches(original.HandshakeTimeout) {
		update.HandshakeTimeout = &timeout
	}
	disabledTools := parseMCPToolNames(m.disabledTools)
	if !slices.Equal(disabledTools, original.DisabledTools) {
		update.DisabledTools = &disabledTools
	}
	autoApproveTools := parseMCPToolNames(m.autoApproveTools)
	if !slices.Equal(autoApproveTools, original.AutoApproveTools) {
		update.AutoApproveTools = &autoApproveTools
	}
	if m.replaceConnection {
		connection, err := m.connection()
		if err != nil {
			return mcp.ServerUpdate{}, false, err
		}
		update.Connection = &connection
	}
	if !update.HasChanges() {
		return update, false, nil
	}
	if err := update.Validate(); err != nil {
		return mcp.ServerUpdate{}, false, err
	}
	return update, true, nil
}

func (m mcpFormDraft) connection() (mcp.ConnectionInput, error) {
	connection := mcp.ConnectionInput{Transport: m.transport}
	switch connection.Transport {
	case protocol.MCPTransportStreamableHTTP:
		connection.URL = strings.TrimSpace(m.url)
		authorization, err := mcpAuthorizationChange(m.authorizationMode, m.authorization)
		if err != nil {
			return mcp.ConnectionInput{}, err
		}
		connection.Authorization = authorization
		headers, err := mcpHeadersChange(m.headersMode, m.headers)
		if err != nil {
			return mcp.ConnectionInput{}, err
		}
		connection.Headers = headers
	case protocol.MCPTransportStdio:
		connection.Command = strings.TrimSpace(m.command)
		arguments, err := parseMCPArguments(m.arguments)
		if err != nil {
			return mcp.ConnectionInput{}, err
		}
		connection.Args = arguments
		connection.Directory = strings.TrimSpace(m.directory)
		environment, err := mcpEnvironmentChange(m.environmentMode, m.environment)
		if err != nil {
			return mcp.ConnectionInput{}, err
		}
		connection.Environment = environment
	}
	return connection, connection.Validate()
}

func (m mcpFormDraft) validateSelections() error {
	for _, change := range []formChange{m.authorizationMode, m.headersMode, m.environmentMode} {
		if err := change.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) openMCPServerForm(mode mcpFormMode, server protocol.MCPServer) {
	a.showMCPFormStep(newMCPFormFlow(mode, server))
}

func (a *app) showMCPFormStep(flow *mcpFormFlow) {
	if a.dialogs.mcpDialog != nil {
		a.dialogs.mcpDialog.Controller().Dismiss()
		a.dialogs.mcpDialog = nil
	}
	fields, secretFields := a.mcpFormFields(flow)
	flow.secretFields = append(flow.secretFields, secretFields...)
	form := headless.NewForm(fields...)
	form.Keys = headless.DefaultFormKeys()
	var dialog *kit.Dialog
	form.Done = func() {
		if a.dialogs.mcpDialog != dialog {
			return
		}
		if flow.advance() {
			a.showMCPFormStep(flow)
			return
		}
		a.submitMCPForm(flow)
	}
	form.GaveUp = func() {
		if a.dialogs.mcpDialog != dialog {
			return
		}
		if flow.back() {
			a.showMCPFormStep(flow)
			return
		}
		a.closeMCPForm(flow)
	}
	body := kit.NewForm(kit.FormConfig{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	title := "Create MCP server"
	switch flow.mode {
	case mcpFormProbe:
		title = "Test MCP candidate"
	case mcpFormUpdate:
		title = "Configure MCP server · " + flow.server.Name
	}
	step, total, label := flow.progress()
	title += fmt.Sprintf(" · %d/%d", step, total)
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: title, Body: body,
		Where: layout.Placement{Width: 92, Height: formDialogHeight(body.Measure(88), len(fields), 24)},
	})
	dialog.Controller().SetDescription(label)
	a.dialogs.mcpDialog = dialog
	dialog.Controller().Show()
}

func (a *app) submitMCPForm(flow *mcpFormFlow) {
	switch flow.mode {
	case mcpFormCreate, mcpFormProbe:
		candidate, err := flow.draft.candidate()
		if err != nil {
			a.message("MCP form: " + err.Error())
			return
		}
		a.closeMCPForm(flow)
		if flow.mode == mcpFormCreate {
			a.createMCPServer(candidate)
		} else {
			a.probeMCPServer(candidate)
		}
	case mcpFormUpdate:
		update, changed, err := flow.draft.update(flow.server)
		if err != nil {
			a.message("MCP form: " + err.Error())
			return
		}
		a.closeMCPForm(flow)
		if !changed {
			a.message("MCP server configuration unchanged")
			return
		}
		a.updateMCPServer(update)
	}
}

func (a *app) closeMCPForm(flow *mcpFormFlow) {
	flow.clearSecrets()
	if a.dialogs.mcpDialog != nil {
		a.dialogs.mcpDialog.Controller().Dismiss()
		a.dialogs.mcpDialog = nil
	}
}

func (a *app) mcpFormFields(flow *mcpFormFlow) ([]headless.Field, []*headless.Text) {
	draft := &flow.draft
	fields := make([]headless.Field, 0, 5)
	secretFields := make([]*headless.Text, 0, 3)
	textField := func(label, placeholder string, value *string, check func(string) error) *headless.Text {
		field := &headless.Text{Label: label, Placeholder: placeholder, Value: headless.Bind(value), Check: check}
		field.Editor().Clipboard = a.loop.Clipboard()
		fields = append(fields, field)
		return field
	}
	switch flow.step {
	case mcpFormGeneral:
		if flow.mode != mcpFormUpdate {
			textField("Server name", "docs", &draft.name, requiredText)
		}
		enabled := &headless.Select[bool]{Label: "Enabled", Value: headless.Bind(&draft.enabled), Rows: 2}
		enabled.SetOptions([]headless.Option[bool]{{Label: "Enabled", Value: true}, {Label: "Disabled", Value: false}})
		fields = append(fields, enabled)
		textField("Description", "Optional description", &draft.description, nil)
		if flow.mode == mcpFormUpdate {
			connection := &headless.Select[bool]{Label: "Connection change", Value: headless.Bind(&draft.replaceConnection), Rows: 2}
			connection.SetOptions([]headless.Option[bool]{{Label: "Keep current connection", Value: false}, {Label: "Replace connection", Value: true}})
			fields = append(fields, connection)
		}
		transportLabel := "Transport"
		if flow.mode == mcpFormUpdate {
			transportLabel = "Replacement transport"
		}
		transport := &headless.Select[protocol.MCPTransport]{Label: transportLabel, Value: headless.Bind(&draft.transport), Rows: 2}
		transport.SetOptions([]headless.Option[protocol.MCPTransport]{{Label: "Streamable HTTP", Value: protocol.MCPTransportStreamableHTTP}, {Label: "stdio process", Value: protocol.MCPTransportStdio}})
		fields = append(fields, transport)
	case mcpFormHTTP:
		textField("HTTP URL", "https://mcp.example/tools", &draft.url, requiredText)
		secretOptions := mcpSecretOptions(flow.mode)
		authorizationMode := &headless.Select[formChange]{Label: "Authorization change", Value: headless.Bind(&draft.authorizationMode), Rows: len(secretOptions)}
		authorizationMode.SetOptions(secretOptions)
		fields = append(fields, authorizationMode)
		authorization := textField("Authorization value", "Bearer …", &draft.authorization, func(value string) error {
			if draft.authorizationMode.SetsValue() {
				return requiredText(value)
			}
			return nil
		})
		authorization.Editor().SetMask("•")
		headersMode := &headless.Select[formChange]{Label: "Headers change", Value: headless.Bind(&draft.headersMode), Rows: len(secretOptions)}
		headersMode.SetOptions(secretOptions)
		fields = append(fields, headersMode)
		headers := textField("Headers JSON", `{"X-Key":"secret"}`, &draft.headers, func(value string) error {
			if !draft.headersMode.SetsValue() {
				return nil
			}
			_, err := parseMCPStringMap(value)
			return err
		})
		headers.Editor().SetMask("•")
		secretFields = append(secretFields, authorization, headers)
	case mcpFormStdio:
		textField("stdio command", "mcp-server", &draft.command, requiredText)
		textField("stdio args JSON", `["--stdio"]`, &draft.arguments, func(value string) error {
			_, err := parseMCPArguments(value)
			return err
		})
		secretOptions := mcpSecretOptions(flow.mode)
		environmentMode := &headless.Select[formChange]{Label: "Environment change", Value: headless.Bind(&draft.environmentMode), Rows: len(secretOptions)}
		environmentMode.SetOptions(secretOptions)
		fields = append(fields, environmentMode)
		environment := textField("Environment JSON", `{"TOKEN":"secret"}`, &draft.environment, func(value string) error {
			if !draft.environmentMode.SetsValue() {
				return nil
			}
			_, err := parseMCPStringMap(value)
			return err
		})
		environment.Editor().SetMask("•")
		secretFields = append(secretFields, environment)
		textField("Working directory", "Optional absolute path", &draft.directory, nil)
	case mcpFormPolicy:
		textField("Handshake timeout seconds", "Blank leaves the handshake unbounded", &draft.timeoutSeconds, func(value string) error {
			_, err := parseMCPTimeout(value)
			return err
		})
		textField("Disabled tools", "comma-separated remote names", &draft.disabledTools, validateMCPToolNames)
		textField("Auto-approved tools", "comma-separated remote names", &draft.autoApproveTools, validateMCPToolNames)
	}
	return fields, secretFields
}

func mcpSecretOptions(mode mcpFormMode) []headless.Option[formChange] {
	if mode == mcpFormUpdate {
		return []headless.Option[formChange]{
			{Label: "Keep current secret", Value: formChangeKeep},
			{Label: "Set replacement", Value: formChangeSet},
			{Label: "Clear secret", Value: formChangeClear},
		}
	}
	return []headless.Option[formChange]{{Label: "No secret", Value: formChangeKeep}, {Label: "Set secret", Value: formChangeSet}}
}

func mcpAuthorizationChange(mode formChange, value string) (*mcp.AuthorizationChange, error) {
	if err := mode.Validate(); err != nil {
		return nil, err
	}
	switch mode {
	case formChangeSet:
		change := &mcp.AuthorizationChange{Kind: protocol.MCPSecretSet, Value: strings.TrimSpace(value)}
		return change, change.Validate()
	case formChangeClear:
		return &mcp.AuthorizationChange{Kind: protocol.MCPSecretClear}, nil
	case formChangeKeep:
		return nil, nil
	}
	panic("validated form change became unreachable")
}

func mcpHeadersChange(mode formChange, value string) (*mcp.HeadersChange, error) {
	if err := mode.Validate(); err != nil {
		return nil, err
	}
	if mode.ClearsValue() {
		return &mcp.HeadersChange{Kind: protocol.MCPSecretClear}, nil
	}
	if !mode.SetsValue() {
		return nil, nil
	}
	values, err := parseMCPStringMap(value)
	if err != nil {
		return nil, err
	}
	change := &mcp.HeadersChange{Kind: protocol.MCPSecretSet, Value: values}
	return change, change.Validate()
}

func mcpEnvironmentChange(mode formChange, value string) (*mcp.EnvironmentChange, error) {
	if err := mode.Validate(); err != nil {
		return nil, err
	}
	if mode.ClearsValue() {
		return &mcp.EnvironmentChange{Kind: protocol.MCPSecretClear}, nil
	}
	if !mode.SetsValue() {
		return nil, nil
	}
	values, err := parseMCPStringMap(value)
	if err != nil {
		return nil, err
	}
	change := &mcp.EnvironmentChange{Kind: protocol.MCPSecretSet, Value: values}
	return change, change.Validate()
}

func parseMCPStringMap(value string) (map[string]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("a non-empty JSON object is required")
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return nil, fmt.Errorf("parse JSON object: %w", err)
	}
	if len(parsed) == 0 {
		return nil, errors.New("a non-empty JSON object is required")
	}
	for key, item := range parsed {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(item) == "" {
			return nil, errors.New("JSON object names and values must be non-empty")
		}
	}
	return parsed, nil
}

func parseMCPArguments(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	var arguments []string
	if err := json.Unmarshal([]byte(value), &arguments); err != nil {
		return nil, fmt.Errorf("parse argument array: %w", err)
	}
	return arguments, nil
}

func parseMCPTimeout(value string) (mcp.HandshakeTimeout, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return mcp.HandshakeTimeout{}, nil
	}
	seconds, err := strconv.Atoi(value)
	if err != nil {
		return mcp.HandshakeTimeout{}, errors.New("handshake timeout must be a positive integer")
	}
	return mcp.NewHandshakeTimeout(seconds)
}

func parseMCPToolNames(value string) []string {
	fields := strings.Split(value, ",")
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		if name := strings.TrimSpace(field); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func validateMCPToolNames(value string) error {
	seen := make(map[string]struct{})
	for _, name := range parseMCPToolNames(value) {
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("tool %q is duplicated", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

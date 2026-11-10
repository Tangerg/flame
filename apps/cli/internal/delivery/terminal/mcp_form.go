package terminal

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/integration/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
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
}

func newMCPFormDraft(mode mcpFormMode, server protocol.MCPServer) mcpFormDraft {
	draft := mcpFormDraft{
		enabled: true, replaceConnection: true, transport: protocol.MCPTransportStreamableHTTP,
		authorizationMode: formChangeKeep, headersMode: formChangeKeep, environmentMode: formChangeKeep,
	}
	if mode != mcpFormUpdate {
		return draft
	}
	draft.name = server.ID.Name
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
		HandshakeTimeout: timeout,
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
	update := mcp.ServerUpdate{Server: original.ID}
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
		connection.Command = m.command
		arguments, err := parseMCPArguments(m.arguments)
		if err != nil {
			return mcp.ConnectionInput{}, err
		}
		connection.Args = arguments
		connection.Directory = m.directory
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

func (m *mcpScreens) openMCPServerForm(mode mcpFormMode, server protocol.MCPServer) {
	m.showMCPFormStep(newMCPFormFlow(mode, server))
}

func (m *mcpScreens) showMCPFormStep(flow *mcpFormFlow) {
	if flow.state != mcpFormEditing {
		return
	}
	if m.form != nil && m.form != flow {
		m.closeMCPForm(m.form)
	}
	m.form = flow
	if flow.dialog != nil {
		flow.dialog.Dismiss()
	}
	fields, secretFields, capture := m.mcpFormFields(flow)
	flow.secretFields = append(flow.secretFields, secretFields...)
	form := kit.FormConfig{Fields: fields, Theme: m.theme, Glyphs: m.glyphs,
		Hints: []keymap.Action{base.Submit, base.Cancel}}
	form.Keys = base.DefaultFormKeys()
	var dialog *presentationDialog
	form.Submit = func(base.SubmissionID) (scene.ActionState, error) {
		if m.form != flow || flow.dialog != dialog || flow.state != mcpFormEditing {
			return scene.Unchanged, nil
		}
		capture()
		if flow.advance() {
			m.showMCPFormStep(flow)
			return scene.Unchanged, nil
		}
		m.submitMCPForm(flow)
		return scene.Applied, nil
	}
	leave := func() {
		if m.form != flow || flow.dialog != dialog || flow.state != mcpFormEditing {
			return
		}
		capture()
		if flow.back() {
			m.showMCPFormStep(flow)
			return
		}
		m.closeMCPForm(flow)
	}
	body := kit.NewForm(form)
	title := "Create MCP server"
	switch flow.mode {
	case mcpFormProbe:
		title = "Test MCP candidate"
	case mcpFormUpdate:
		title = "Configure MCP server · " + mcp.ServerLabel(flow.server.ID)
	}
	step, total, label := flow.progress()
	title += fmt.Sprintf(" · %d/%d", step, total)
	dialog = newPresentationDialog(m.stack, kit.DialogConfig{Theme: m.theme, Glyphs: m.glyphs,
		Title: title, Body: formNavigation(body.Element(), leave),
		Where: layout.Placement{Width: 92, Height: formDialogHeight(body.HeightForWidth(88), len(fields), 24)},
	})
	dialog.Controller().SetDescription(label)
	flow.dialog = dialog
	flow.view = body
	dialog.Show()
}

func (m *mcpScreens) submitMCPForm(flow *mcpFormFlow) {
	if m.form != flow || (flow.state != mcpFormEditing && flow.state != mcpFormUncertain) {
		return
	}
	submission, err := flow.prepare(m.service, m.replayPolicy)
	if err != nil {
		flow.feedback("MCP form: " + err.Error())
		return
	}
	if submission == nil {
		m.closeMCPForm(flow)
		m.message("MCP server configuration unchanged")
		return
	}
	presentation := m.session.context
	apply := func(result mcpFormResult, err error) {
		if flow.state == mcpFormClosed {
			return
		}
		flow.settle(err)
		if err != nil {
			message := submission.label + " failed: " + err.Error()
			if flow.state == mcpFormEditing {
				m.showMCPFormStep(flow)
				flow.feedback(message)
			} else {
				message = submission.label + " outcome unknown: " + err.Error()
				m.showMCPFormSubmission(flow, message)
			}
			m.message(message)
			return
		}
		m.closeMCPForm(flow)
		if flow.mode == mcpFormProbe {
			message, err := mcpProbeMessage(result.probeName, result.probe)
			if err != nil {
				m.message(submission.label + " returned an invalid result: " + err.Error())
				return
			}
			m.message(message)
			return
		}
		document, err := mcpServersDocument([]protocol.MCPServer{result.server})
		if err != nil {
			m.message(submission.label + " returned an invalid result: " + err.Error())
			return
		}
		m.message(submission.label + " complete")
		if m.session.context.current(presentation) {
			m.reader.open(m.mcpServersReaderQuery(), document)
			m.status.note("MCP server · " + mcp.ServerLabel(result.server.ID))
		}
	}
	var started bool
	if flow.mode == mcpFormProbe {
		started = m.tasks.application(mcpOperation, false, submission.work, apply)
	} else {
		started = m.tasks.admissionMutation(mcpOperation, false, submission.work, apply)
	}
	if !started {
		flow.feedback("another MCP operation is running")
		m.message("another MCP operation is running")
		return
	}
	flow.admitted(submission)
	m.status.note(submission.label)
	m.showMCPFormSubmission(flow, submission.label)
}

func (m *mcpScreens) showMCPFormSubmission(flow *mcpFormFlow, message string) {
	flow.dialog.Dismiss()
	form := kit.FormConfig{Theme: m.theme, Glyphs: m.glyphs}
	form.Keys = base.DefaultFormKeys()
	var dialog *presentationDialog
	form.Submit = func(base.SubmissionID) (scene.ActionState, error) {
		if m.form == flow && flow.dialog == dialog {
			m.submitMCPForm(flow)
		}
		return scene.Applied, nil
	}
	leave := func() {
		if m.form == flow && flow.dialog == dialog {
			m.closeMCPForm(flow)
		}
	}
	title := "Submitting MCP form"
	hints := []keymap.Action{base.Cancel}
	if flow.state == mcpFormUncertain {
		title = "Outcome unknown · Enter retries the original request"
		hints = append([]keymap.Action{base.Submit}, hints...)
	} else if flow.state == mcpFormBlocked {
		title = "Outcome unknown · replay is unavailable"
	}
	form.Title, form.Hints = title+"\n"+message, hints
	body := kit.NewForm(form)
	if err := body.Controller().Element().SetFocusable(func() bool { return true }); err != nil {
		panic(err)
	}
	dialog = newPresentationDialog(m.stack, kit.DialogConfig{Theme: m.theme, Glyphs: m.glyphs,
		Title: "MCP submission", Description: message, Body: formNavigation(body.Element(), leave),
		Where: layout.Placement{Width: 92, Height: formDialogHeight(body.HeightForWidth(88), 0, 24)},
	})
	flow.dialog = dialog
	flow.view = body
	dialog.Show()
}

func (m *mcpScreens) closeMCPForm(flow *mcpFormFlow) {
	flow.close()
	if m.form == flow {
		m.form = nil
	}
}

func (m *mcpScreens) mcpFormFields(flow *mcpFormFlow) ([]base.Field, []*base.Text, func()) {
	draft := &flow.draft
	var capture []func()
	fields := make([]base.Field, 0, 5)
	secretFields := make([]*base.Text, 0, 3)
	textField := func(label, placeholder string, value *string, check func(string) error) *base.Text {
		field := base.NewText(base.TextConfig{Label: label, Placeholder: placeholder, Initial: *value, Check: check})
		field.SetClipboard(m.clipboard)
		capture = append(capture, func() { *value = field.Text() })
		fields = append(fields, field)
		return field
	}
	switch flow.step {
	case mcpFormGeneral:
		if flow.mode != mcpFormUpdate {
			textField("Server name", "docs", &draft.name, requiredText)
		}
		enabled := base.NewSelect(base.SelectConfig[bool]{Same: base.Equal[bool], Label: "Enabled", Initial: draft.enabled, Rows: 2})
		enabled.SetOptions([]base.Option[bool]{{Label: "Enabled", Value: true}, {Label: "Disabled", Value: false}})
		enabled.Choose(draft.enabled)
		capture = append(capture, func() {
			if selected, ok := enabled.Chosen(); ok {
				draft.enabled = selected.Value
			}
		})
		fields = append(fields, enabled)
		textField("Description", "Optional description", &draft.description, nil)
		if flow.mode == mcpFormUpdate {
			connection := base.NewSelect(base.SelectConfig[bool]{Same: base.Equal[bool], Label: "Connection change", Initial: draft.replaceConnection, Rows: 2})
			connection.SetOptions([]base.Option[bool]{{Label: "Keep current connection", Value: false}, {Label: "Replace connection", Value: true}})
			connection.Choose(draft.replaceConnection)
			capture = append(capture, func() {
				if selected, ok := connection.Chosen(); ok {
					draft.replaceConnection = selected.Value
				}
			})
			fields = append(fields, connection)
		}
		transportLabel := "Transport"
		if flow.mode == mcpFormUpdate {
			transportLabel = "Replacement transport"
		}
		transport := base.NewSelect(base.SelectConfig[protocol.MCPTransport]{Same: base.Equal[protocol.MCPTransport], Label: transportLabel, Initial: draft.transport, Rows: 2})
		transport.SetOptions([]base.Option[protocol.MCPTransport]{{Label: "Streamable HTTP", Value: protocol.MCPTransportStreamableHTTP}, {Label: "stdio process", Value: protocol.MCPTransportStdio}})
		transport.Choose(draft.transport)
		capture = append(capture, func() {
			if selected, ok := transport.Chosen(); ok {
				draft.transport = selected.Value
			}
		})
		fields = append(fields, transport)
	case mcpFormHTTP:
		textField("HTTP URL", "https://mcp.example/tools", &draft.url, requiredText)
		secretOptions := mcpSecretOptions(flow.mode)
		authorizationMode := base.NewSelect(base.SelectConfig[formChange]{Same: base.Equal[formChange], Label: "Authorization change", Initial: draft.authorizationMode, Rows: len(secretOptions)})
		authorizationMode.SetOptions(secretOptions)
		authorizationMode.Choose(draft.authorizationMode)
		capture = append(capture, func() {
			if selected, ok := authorizationMode.Chosen(); ok {
				draft.authorizationMode = selected.Value
			}
		})
		fields = append(fields, authorizationMode)
		authorization := textField("Authorization value", "Bearer …", &draft.authorization, func(value string) error {
			if selected, ok := authorizationMode.Chosen(); ok && selected.Value.SetsValue() {
				return requiredText(value)
			}
			return nil
		})
		authorization.SetMask("•")
		headersMode := base.NewSelect(base.SelectConfig[formChange]{Same: base.Equal[formChange], Label: "Headers change", Initial: draft.headersMode, Rows: len(secretOptions)})
		headersMode.SetOptions(secretOptions)
		headersMode.Choose(draft.headersMode)
		capture = append(capture, func() {
			if selected, ok := headersMode.Chosen(); ok {
				draft.headersMode = selected.Value
			}
		})
		fields = append(fields, headersMode)
		headers := textField("Headers JSON", `{"X-Key":"secret"}`, &draft.headers, func(value string) error {
			_, err := mcpHeadersChange(func() formChange { selected, _ := headersMode.Chosen(); return selected.Value }(), value)
			return err
		})
		headers.SetMask("•")
		secretFields = append(secretFields, authorization, headers)
	case mcpFormStdio:
		textField("stdio command", "mcp-server", &draft.command, func(value string) error {
			return (mcp.ConnectionInput{Transport: protocol.MCPTransportStdio, Command: value}).Validate()
		})
		textField("stdio args JSON", `["--stdio"]`, &draft.arguments, func(value string) error {
			_, err := parseMCPArguments(value)
			return err
		})
		secretOptions := mcpSecretOptions(flow.mode)
		environmentMode := base.NewSelect(base.SelectConfig[formChange]{Same: base.Equal[formChange], Label: "Environment change", Initial: draft.environmentMode, Rows: len(secretOptions)})
		environmentMode.SetOptions(secretOptions)
		environmentMode.Choose(draft.environmentMode)
		capture = append(capture, func() {
			if selected, ok := environmentMode.Chosen(); ok {
				draft.environmentMode = selected.Value
			}
		})
		fields = append(fields, environmentMode)
		environment := textField("Environment JSON", `{"TOKEN":"secret"}`, &draft.environment, func(value string) error {
			_, err := mcpEnvironmentChange(func() formChange { selected, _ := environmentMode.Chosen(); return selected.Value }(), value)
			return err
		})
		environment.SetMask("•")
		secretFields = append(secretFields, environment)
		textField("Working directory", "Optional absolute path", &draft.directory, nil)
	case mcpFormPolicy:
		textField("Handshake timeout seconds", "Blank leaves the handshake unbounded", &draft.timeoutSeconds, func(value string) error {
			_, err := parseMCPTimeout(value)
			return err
		})
	}
	return fields, secretFields, func() {
		for _, collect := range capture {
			collect()
		}
	}
}

func mcpSecretOptions(mode mcpFormMode) []base.Option[formChange] {
	if mode == mcpFormUpdate {
		return []base.Option[formChange]{
			{Label: "Keep current secret", Value: formChangeKeep},
			{Label: "Set replacement", Value: formChangeSet},
			{Label: "Clear secret", Value: formChangeClear},
		}
	}
	return []base.Option[formChange]{{Label: "No secret", Value: formChangeKeep}, {Label: "Set secret", Value: formChangeSet}}
}

func mcpAuthorizationChange(mode formChange, value string) (*mcp.AuthorizationChange, error) {
	if err := mode.Validate(); err != nil {
		return nil, err
	}
	switch mode {
	case formChangeSet:
		change := &mcp.AuthorizationChange{Kind: protocol.MCPSecretSet, Value: value}
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

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
	form := kit.FormConfig{
		Behavior: base.FormConfig{
			Fields: fields,
		},
		Theme:  m.theme,
		Glyphs: m.glyphs,
		Hints:  []keymap.Action{base.Submit, base.Cancel},
	}
	form.Behavior.Keys = base.DefaultFormKeys()
	var dialog *presentationDialog
	// A step's callbacks act only while this step is the one presented.
	superseded := func() bool {
		return m.form != flow || flow.dialog != dialog || flow.state != mcpFormEditing
	}
	form.Behavior.Submit = func(base.SubmissionID) (scene.ActionState, error) {
		if superseded() {
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
		if superseded() {
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
	_, _, label := flow.progress()
	dialog = newPresentationDialog(m.stack, kit.DialogConfig{
		Behavior: base.DialogConfig{
			Title:   flow.title(),
			Content: formNavigation(body.Element(), leave),
		},
		Theme:  m.theme,
		Glyphs: m.glyphs,
		Where:  formDialogPlacement(wideDialogWidth, len(fields), tallDialogHeight, body),
	})
	dialog.Controller().SetDescription(label)
	flow.dialog = dialog
	flow.view = body
	dialog.Show()
}

func (f *mcpFormFlow) title() string {
	title := "Create MCP server"
	switch f.mode {
	case mcpFormProbe:
		title = "Test MCP candidate"
	case mcpFormUpdate:
		title = "Configure MCP server · " + mcp.ServerLabel(f.server.ID)
	case mcpFormCreate:
	}
	step, total, _ := f.progress()
	return title + fmt.Sprintf(" · %d/%d", step, total)
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
		started = m.tasks.application(mcpOperation, submission.work, apply)
	} else {
		started = m.tasks.admissionMutation(mcpOperation, submission.work, apply)
	}
	if !started {
		flow.feedback(mcpOperation.busy().Error())
		m.message(mcpOperation.busy().Error())
		return
	}
	flow.admitted(submission)
	m.status.note(submission.label)
	m.showMCPFormSubmission(flow, submission.label)
}

func (m *mcpScreens) showMCPFormSubmission(flow *mcpFormFlow, message string) {
	flow.dialog.Dismiss()
	form := kit.FormConfig{Theme: m.theme, Glyphs: m.glyphs}
	form.Behavior.Keys = base.DefaultFormKeys()
	var dialog *presentationDialog
	form.Behavior.Submit = func(base.SubmissionID) (scene.ActionState, error) {
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
	switch flow.state {
	case mcpFormUncertain:
		title = "Outcome unknown · Enter retries the original request"
		hints = append([]keymap.Action{base.Submit}, hints...)
	case mcpFormBlocked:
		title = "Outcome unknown · replay is unavailable"
	case mcpFormEditing, mcpFormPending, mcpFormClosed:
	}
	form.Behavior.Title, form.Hints = title+"\n"+message, hints
	body := kit.NewForm(form)
	dialog = newPresentationDialog(m.stack, kit.DialogConfig{
		Behavior: base.DialogConfig{
			Title:       "MCP submission",
			Description: message,
			Content:     formNavigation(body.Element(), leave),
		},
		Theme:  m.theme,
		Glyphs: m.glyphs,
		Where:  formDialogPlacement(wideDialogWidth, 0, tallDialogHeight, body),
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
	set := &mcpFieldSet{clipboard: m.clipboard}
	switch flow.step {
	case mcpFormGeneral:
		set.generalFields(flow)
	case mcpFormHTTP:
		set.httpFields(flow)
	case mcpFormStdio:
		set.stdioFields(flow)
	case mcpFormPolicy:
		set.text("Handshake timeout seconds", "Blank leaves the handshake unbounded", &flow.draft.timeoutSeconds, func(value string) error {
			_, err := parseMCPTimeout(value)
			return err
		})
	}
	return set.fields, set.secrets, set.collect
}

type mcpFieldSet struct {
	clipboard base.Clipboard
	fields    []base.Field
	secrets   []*base.Text
	capture   []func()
}

func (s *mcpFieldSet) collect() {
	for _, capture := range s.capture {
		capture()
	}
}

func (s *mcpFieldSet) text(label, placeholder string, value *string, check func(string) error) *base.Text {
	field := base.NewText(base.TextConfig{Label: label, Placeholder: placeholder, Initial: *value, Check: check})
	field.SetClipboard(s.clipboard)
	s.capture = append(s.capture, func() { *value = field.Text() })
	s.fields = append(s.fields, field)
	return field
}

func (s *mcpFieldSet) secret(label, placeholder string, value *string, check func(string) error) {
	field := s.text(label, placeholder, value, check)
	field.SetMask("•")
	s.secrets = append(s.secrets, field)
}

func choiceField[T comparable](s *mcpFieldSet, label string, options []base.Option[T], value *T) *base.Select[T] {
	choice := base.NewSelect(base.SelectConfig[T]{Same: base.Equal[T], Label: label, Initial: *value, Rows: len(options)})
	choice.SetOptions(options)
	choice.Choose(*value)
	s.capture = append(s.capture, func() {
		if selected, ok := choice.Chosen(); ok {
			*value = selected.Value
		}
	})
	s.fields = append(s.fields, choice)
	return choice
}

func chosenChange(choice *base.Select[formChange]) formChange {
	selected, _ := choice.Chosen()
	return selected.Value
}

func (s *mcpFieldSet) generalFields(flow *mcpFormFlow) {
	draft := &flow.draft
	updating := flow.mode == mcpFormUpdate
	if !updating {
		s.text("Server name", "docs", &draft.name, requiredText)
	}
	choiceField(s, "Enabled", []base.Option[bool]{{Label: "Enabled", Value: true}, {Label: "Disabled", Value: false}}, &draft.enabled)
	s.text("Description", "Optional description", &draft.description, nil)
	transportLabel := "Transport"
	if updating {
		choiceField(s, "Connection change", []base.Option[bool]{{Label: "Keep current connection", Value: false}, {Label: "Replace connection", Value: true}}, &draft.replaceConnection)
		transportLabel = "Replacement transport"
	}
	choiceField(s, transportLabel, []base.Option[protocol.MCPTransport]{{Label: "Streamable HTTP", Value: protocol.MCPTransportStreamableHTTP}, {Label: "stdio process", Value: protocol.MCPTransportStdio}}, &draft.transport)
}

func (s *mcpFieldSet) httpFields(flow *mcpFormFlow) {
	draft := &flow.draft
	s.text("HTTP URL", "https://mcp.example/tools", &draft.url, requiredText)
	secretOptions := mcpSecretOptions(flow.mode)
	authorizationMode := choiceField(s, "Authorization change", secretOptions, &draft.authorizationMode)
	s.secret("Authorization value", "Bearer …", &draft.authorization, func(value string) error {
		if chosenChange(authorizationMode).SetsValue() {
			return requiredText(value)
		}
		return nil
	})
	headersMode := choiceField(s, "Headers change", secretOptions, &draft.headersMode)
	s.secret("Headers JSON", `{"X-Key":"secret"}`, &draft.headers, func(value string) error {
		_, err := mcpHeadersChange(chosenChange(headersMode), value)
		return err
	})
}

func (s *mcpFieldSet) stdioFields(flow *mcpFormFlow) {
	draft := &flow.draft
	s.text("stdio command", "mcp-server", &draft.command, func(value string) error {
		return (mcp.ConnectionInput{Transport: protocol.MCPTransportStdio, Command: value}).Validate()
	})
	s.text("stdio args JSON", `["--stdio"]`, &draft.arguments, func(value string) error {
		_, err := parseMCPArguments(value)
		return err
	})
	environmentMode := choiceField(s, "Environment change", mcpSecretOptions(flow.mode), &draft.environmentMode)
	s.secret("Environment JSON", `{"TOKEN":"secret"}`, &draft.environment, func(value string) error {
		_, err := mcpEnvironmentChange(chosenChange(environmentMode), value)
		return err
	})
	s.text("Working directory", "Optional absolute path", &draft.directory, nil)
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

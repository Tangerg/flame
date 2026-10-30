package terminal

import (
	"context"

	"github.com/Tangerg/flame/apps/cli/internal/application/integration/mcp"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
)

type mcpFormStep uint8

const (
	mcpFormGeneral mcpFormStep = iota + 1
	mcpFormHTTP
	mcpFormStdio
	mcpFormPolicy
)

type mcpFormState uint8

const (
	mcpFormEditing mcpFormState = iota + 1
	mcpFormPending
	mcpFormUncertain
	mcpFormBlocked
	mcpFormClosed
)

type mcpFormResult struct {
	server    protocol.MCPServer
	probe     protocol.MCPTestOutcome
	probeName string
}

type mcpFormSubmission struct {
	label string
	// The admitted snapshot outlives UI cleanup. Retrying it must never read the
	// live draft or allocate another command identity.
	work func(context.Context) (mcpFormResult, error)
}

type mcpFormFlow struct {
	mode         mcpFormMode
	server       protocol.MCPServer
	draft        mcpFormDraft
	step         mcpFormStep
	secretFields []*headless.Text
	state        mcpFormState
	submission   *mcpFormSubmission
	dialog       *kit.Dialog
	view         *kit.Form
}

// newMCPFormFlow takes ownership of the fresh Runtime result selected for editing.
func newMCPFormFlow(mode mcpFormMode, server protocol.MCPServer) *mcpFormFlow {
	return &mcpFormFlow{
		mode: mode, server: server, draft: newMCPFormDraft(mode, server), step: mcpFormGeneral, state: mcpFormEditing,
	}
}

func (m *mcpFormFlow) replacesConnection() bool {
	return m.mode != mcpFormUpdate || m.draft.replaceConnection
}

func (m *mcpFormFlow) connectionStep() mcpFormStep {
	if m.draft.transport == protocol.MCPTransportStdio {
		return mcpFormStdio
	}
	return mcpFormHTTP
}

func (m *mcpFormFlow) advance() bool {
	if m.state != mcpFormEditing {
		return false
	}
	switch m.step {
	case mcpFormGeneral:
		if m.replacesConnection() {
			m.step = m.connectionStep()
		} else {
			m.step = mcpFormPolicy
		}
		return true
	case mcpFormHTTP, mcpFormStdio:
		m.step = mcpFormPolicy
		return true
	default:
		return false
	}
}

func (m *mcpFormFlow) back() bool {
	if m.state != mcpFormEditing {
		return false
	}
	switch m.step {
	case mcpFormHTTP, mcpFormStdio:
		m.step = mcpFormGeneral
		return true
	case mcpFormPolicy:
		if m.replacesConnection() {
			m.step = m.connectionStep()
		} else {
			m.step = mcpFormGeneral
		}
		return true
	default:
		return false
	}
}

func (m *mcpFormFlow) progress() (int, int, string) {
	total := 2
	if m.replacesConnection() {
		total = 3
	}
	switch m.step {
	case mcpFormGeneral:
		return 1, total, "General"
	case mcpFormHTTP:
		return 2, total, "HTTP connection"
	case mcpFormStdio:
		return 2, total, "stdio connection"
	default:
		return total, total, "Tool policy"
	}
}

func (m *mcpFormFlow) prepare(service MCPManagement, policy mutation.ReplayPolicy) (*mcpFormSubmission, error) {
	if m.state == mcpFormUncertain {
		return m.submission, nil
	}
	if m.state != mcpFormEditing {
		return nil, nil
	}
	if m.mode == mcpFormUpdate {
		update, changed, err := m.draft.update(m.server)
		if err != nil || !changed {
			return nil, err
		}
		return newMCPFormMutation("updating MCP server "+mcp.ServerLabel(update.Server), policy,
			func(ctx context.Context, commandID authoring.CommandID) (protocol.MCPServer, error) {
				return service.UpdateServer(ctx, commandID, update)
			})
	}
	candidate, err := m.draft.candidate()
	if err != nil {
		return nil, err
	}
	if m.mode == mcpFormProbe {
		return &mcpFormSubmission{
			label: "testing MCP candidate " + candidate.Name,
			work: func(ctx context.Context) (mcpFormResult, error) {
				outcome, err := service.TestServer(ctx, candidate)
				return mcpFormResult{probe: outcome, probeName: candidate.Name}, err
			},
		}, nil
	}
	return newMCPFormMutation("creating MCP server "+candidate.Name, policy,
		func(ctx context.Context, commandID authoring.CommandID) (protocol.MCPServer, error) {
			return service.CreateServer(ctx, commandID, candidate)
		})
}

func newMCPFormMutation(label string, policy mutation.ReplayPolicy, change func(context.Context, authoring.CommandID) (protocol.MCPServer, error)) (*mcpFormSubmission, error) {
	guard, err := policy.NewGuard()
	if err != nil {
		return nil, err
	}
	commandID := mutation.NewCommandID()
	admit := mutation.ReplayAdmission(policy, guard)
	return &mcpFormSubmission{
		label: label,
		work: func(ctx context.Context) (mcpFormResult, error) {
			if err := admit(); err != nil {
				return mcpFormResult{}, err
			}
			server, err := change(ctx, commandID)
			return mcpFormResult{server: server}, err
		},
	}, nil
}

func (m *mcpFormFlow) admitted(submission *mcpFormSubmission) {
	m.submission, m.state = submission, mcpFormPending
}

func (m *mcpFormFlow) settle(err error) {
	if m.state != mcpFormPending {
		return
	}
	switch {
	case err == nil:
		m.close()
	case m.mode != mcpFormProbe && mutation.AcknowledgementUncertain(err):
		m.state = mcpFormUncertain
	case m.mode != mcpFormProbe && mutation.OutcomeUnknown(err):
		m.state = mcpFormBlocked
	default:
		m.state, m.submission = mcpFormEditing, nil
	}
}

func (m *mcpFormFlow) close() {
	m.state, m.submission = mcpFormClosed, nil
	m.clearSecrets()
	if m.dialog != nil {
		m.dialog.Controller().Dismiss()
		m.dialog = nil
	}
	m.view = nil
}

func (m *mcpFormFlow) feedback(message string) {
	m.view.Title = message
	m.dialog.Controller().SetDescription(message)
}

func (m *mcpFormFlow) clearSecrets() {
	m.draft.authorization, m.draft.headers, m.draft.environment = "", "", ""
	for _, field := range m.secretFields {
		field.SetText("")
	}
	clear(m.secretFields)
	m.secretFields = nil
}

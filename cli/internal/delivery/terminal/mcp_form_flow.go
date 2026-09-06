package terminal

import (
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/oolong/components/headless"
)

type mcpFormStep uint8

const (
	mcpFormGeneral mcpFormStep = iota + 1
	mcpFormHTTP
	mcpFormStdio
	mcpFormPolicy
)

type mcpFormFlow struct {
	mode         mcpFormMode
	server       protocol.MCPServer
	draft        mcpFormDraft
	step         mcpFormStep
	secretFields []*headless.Text
}

// newMCPFormFlow takes ownership of the fresh Runtime result selected for editing.
func newMCPFormFlow(mode mcpFormMode, server protocol.MCPServer) *mcpFormFlow {
	return &mcpFormFlow{
		mode: mode, server: server, draft: newMCPFormDraft(mode, server), step: mcpFormGeneral,
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

func (m *mcpFormFlow) clearSecrets() {
	m.draft.authorization, m.draft.headers, m.draft.environment = "", "", ""
	for _, field := range m.secretFields {
		field.Editor().SetText("")
	}
	clear(m.secretFields)
	m.secretFields = nil
}

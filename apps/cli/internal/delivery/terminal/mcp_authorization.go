package terminal

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/application/integration/mcp"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/retry"
	"github.com/Tangerg/flame/apps/cli/internal/delivery/format"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const (
	mcpAuthorizationCommand      = "mcp-auth"
	mcpAuthorizationPollInterval = 500 * time.Millisecond
)

type mcpAuthorizationStart struct {
	commandID authoring.CommandID
	replay    authoring.ReplayGuard
}

func (m *mcpScreens) AuthorizeMCPServer(argument string) error {
	if strings.TrimSpace(argument) == "" {
		return errors.New("usage: /" + mcpAuthorizationCommand + " <server>")
	}
	server, err := mcp.ParseServerReference(argument)
	if err != nil {
		return err
	}
	original, recovering := m.pendingAuthorizationStarts[server]
	if !recovering {
		guard, err := m.replayPolicy.NewGuard()
		if err != nil {
			return err
		}
		original = mcpAuthorizationStart{commandID: mutation.NewCommandID(), replay: guard}
	}
	label := mcp.ServerLabel(server)
	presentation := m.session.context
	admit := mutation.ReplayAdmission(m.replayPolicy, original.replay)
	started := m.tasks.admissionMutation(mcpAuthorizationOperation,
		func(ctx context.Context) (mcp.AuthorizationReference, error) {
			if err := admit(); err != nil {
				return mcp.AuthorizationReference{}, err
			}
			return m.service.StartAuthorization(ctx, original.commandID, server)
		},
		func(reference mcp.AuthorizationReference, err error) {
			if mutation.AcknowledgementUncertain(err) ||
				errors.Is(err, authoring.ErrCommandOutcomeUnknown) ||
				(recovering && errors.Is(err, authoring.ErrCommandNotDispatched)) {
				m.pendingAuthorizationStarts[server] = original
				m.message("MCP authorization outcome unknown · repeat /" + mcpAuthorizationCommand + " " + label + ": " + err.Error())
				return
			}
			delete(m.pendingAuthorizationStarts, server)
			if err != nil {
				if mutation.OutcomeUnknown(err) {
					m.message("MCP authorization outcome unknown · could not recover " + label + ": " + err.Error())
				} else {
					m.message("start MCP authorization failed: " + err.Error())
				}
				return
			}
			view := &readerQuery{status: "MCP authorization"}
			if m.session.context.current(presentation) {
				m.reader.open(view, paragraphDocument("MCP authorization", "reading authorization status", []string{
					"attempt  " + reference.ID,
					"server   " + mcp.ServerLabel(reference.Server),
				}))
			}
			m.pollMCPAuthorization(reference, view)
		},
	)
	if !started {
		return errors.New("another MCP authorization is running")
	}
	if recovering {
		m.status.note("checking original MCP authorization " + label)
	} else {
		m.status.note("starting MCP authorization " + label)
	}
	return nil
}

func (m *mcpScreens) pollMCPAuthorization(reference mcp.AuthorizationReference, view *readerQuery) {
	started := m.tasks.application(mcpAuthorizationOperation,
		func(ctx context.Context) (protocol.MCPAuthorizationAttempt, error) {
			return m.awaitMCPAuthorization(ctx, reference)
		},
		func(attempt protocol.MCPAuthorizationAttempt, err error) {
			if err != nil {
				m.message("observe MCP authorization failed: " + err.Error())
				return
			}
			m.reader.update(view, mcpAuthorizationDocument(attempt))
			m.message("MCP authorization " + string(attempt.Status.Type) + format.Separator + mcp.ServerLabel(attempt.Server))
		},
	)
	if !started {
		m.message("could not observe MCP authorization " + reference.ID)
	}
}

func (m *mcpScreens) awaitMCPAuthorization(ctx context.Context, reference mcp.AuthorizationReference) (protocol.MCPAuthorizationAttempt, error) {
	var delay time.Duration
	failures := 0
	for {
		if err := retry.Wait(ctx, delay); err != nil {
			return protocol.MCPAuthorizationAttempt{}, err
		}
		attempt, err := m.service.GetAuthorization(ctx, reference)
		if err != nil {
			if !retry.IsReconnectable(err) {
				return protocol.MCPAuthorizationAttempt{}, err
			}
			failures++
			delay, err = runtimeRecoveryBackoff.Delay(failures)
			if err != nil {
				return protocol.MCPAuthorizationAttempt{}, err
			}
			continue
		}
		if attempt.Status.Type != protocol.MCPAuthorizationAttemptPending {
			return attempt, nil
		}
		failures = 0
		delay = mcpAuthorizationPollInterval
	}
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

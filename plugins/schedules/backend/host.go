package main

import (
	"context"
	"errors"

	schedule "github.com/Tangerg/flame/plugins/schedules/backend/domain"
	"github.com/Tangerg/flame/plugins/schedules/backend/transport"
	flameruntime "github.com/Tangerg/flame/runtime"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/flame/runtime/protocol"
)

type runHost struct{ client *flameruntime.Client }

func connectRunHost(ctx context.Context, ref pluginsdk.CallbackRef) (transport.RunHost, func() error, error) {
	client, err := flameruntime.ConnectCallback(ctx, ref)
	if err != nil {
		return nil, nil, err
	}
	return runHost{client: client}, client.Close, nil
}

func (h runHost) Namespace(ctx context.Context) (string, error) {
	discovery, err := h.client.Discover(ctx, flameruntime.CallOptions{})
	if err != nil {
		return "", err
	}
	return discovery.Capabilities.Limits.Idempotency.Namespace, nil
}

func (h runHost) Admit(ctx context.Context, request schedule.RunRequest) error {
	execution := request.Execution()
	selection := execution.ModelSelection()
	start := protocol.StartRunRequest{
		NewSession: &protocol.RunSessionOpening{SessionID: request.SessionID(), RunID: request.RunID(), Title: execution.Title(), Workspace: protocol.WorkspaceRef{Path: execution.CWD()}},
		Input:      []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: execution.Instructions()}},
		Provider:   selection.Provider(), Model: selection.Model(), ReasoningEffort: selection.ReasoningEffort(),
	}
	key := "schedules:run:" + request.RunID()
	namespace, err := h.Namespace(ctx)
	if err != nil {
		return err
	}
	acceptance, err := h.client.GetRunAcceptance(ctx, protocol.GetRunAcceptanceRequest{Namespace: namespace, Key: key, Operation: protocol.RunCommandStart, Request: &start}, flameruntime.CallOptions{})
	if err != nil {
		return err
	}
	switch acceptance.Type {
	case protocol.RunAcceptanceAccepted, protocol.RunAcceptanceRetired:
		if acceptance.SessionID != request.SessionID() || acceptance.RunID != request.RunID() {
			return errors.New("schedules: Runtime acceptance differs from captured dispatch")
		}
		return nil
	case protocol.RunAcceptanceNotRecorded:
	default:
		return flameruntime.ErrInvalidResponse
	}
	ack, _, err := h.client.StartRun(ctx, start, flameruntime.RunCommandOptions{IdempotencyKey: key})
	if err != nil {
		return err
	}
	if ack.RunID != request.RunID() {
		return flameruntime.ErrInvalidResponse
	}
	return nil
}

func (h runHost) Notify(ctx context.Context, contribution string) error {
	return h.client.NotifyPluginChange(ctx, protocol.PluginChangeNotice{Contribution: contribution}, flameruntime.CommandOptions{})
}

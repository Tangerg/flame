package main

import (
	"context"
	"errors"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
)

type remoteRuns struct{ client *runtimeclient.Client }

func connectRunHost(ctx context.Context, ref pluginsdk.CallbackRef) (runHost, func() error, error) {
	client, err := runtimeclient.ConnectCallback(ctx, ref)
	if err != nil {
		return nil, nil, err
	}
	return remoteRuns{client: client}, client.Close, nil
}

func (h remoteRuns) Namespace(ctx context.Context) (string, error) {
	discovery, err := h.client.Discover(ctx, protocol.CallOptions{})
	if err != nil {
		return "", err
	}
	return discovery.Capabilities.Limits.Idempotency.Namespace, nil
}

func (h remoteRuns) Admit(ctx context.Context, request schedule.RunRequest) error {
	execution := request.Execution()
	selection := execution.ModelSelection()
	start := protocol.StartRunRequest{
		NewSession: &protocol.RunSessionOpening{SessionID: request.SessionID(), RunID: request.RunID(), Title: execution.Title()},
		Input:      []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: execution.Instructions()}},
		Provider:   selection.Provider(), Model: selection.Model(), ReasoningEffort: selection.ReasoningEffort(),
	}
	if execution.CWD() != "" {
		start.NewSession.Workspace = &protocol.WorkspaceRef{Path: execution.CWD()}
	}
	key := "schedules:run:" + request.RunID()
	namespace, err := h.Namespace(ctx)
	if err != nil {
		return err
	}
	acceptance, err := h.client.GetRunAcceptance(ctx, protocol.GetRunAcceptanceRequest{Namespace: namespace, Key: key, Operation: protocol.RunCommandStart, Request: &start}, protocol.CallOptions{})
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
		return protocol.ErrInvalidResponse
	}
	observation, cancel := context.WithCancel(ctx)
	defer cancel()
	ack, _, err := h.client.StartRun(observation, start, protocol.RunCommandOptions{IdempotencyKey: key})
	if err != nil {
		return err
	}
	if ack.RunID != request.RunID() {
		return protocol.ErrInvalidResponse
	}
	return nil
}

func (h remoteRuns) Notify(ctx context.Context, contribution string) error {
	return h.client.NotifyPluginChange(ctx, protocol.PluginChangeNotice{Contribution: contribution}, protocol.CommandOptions{})
}

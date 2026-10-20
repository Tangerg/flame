package main

import (
	"context"
	"errors"
	"iter"

	runtimeclient "github.com/Tangerg/flame/packages/runtime-client/go"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	"github.com/Tangerg/flame/plugins/filewatch/backend/domain"
	"github.com/Tangerg/flame/runtime/protocol"
)

type runHost struct{ client *runtimeclient.Client }

func connectHost(ctx context.Context, ref pluginsdk.CallbackRef) (*runHost, error) {
	client, err := runtimeclient.ConnectCallback(ctx, ref)
	if err != nil {
		return nil, err
	}
	return &runHost{client: client}, nil
}

func (h *runHost) Namespace(ctx context.Context) (string, error) {
	discovery, err := h.client.Discover(ctx, protocol.CallOptions{})
	if err != nil {
		return "", err
	}
	return discovery.Capabilities.Limits.Idempotency.Namespace, nil
}

func (h *runHost) Watch(ctx context.Context, generation string, trigger domain.Trigger) (iter.Seq2[protocol.RuntimeEvent, error], error) {
	draft := trigger.Draft()
	workspace, err := h.client.ResolveWorkspace(ctx, protocol.ResolveWorkspaceRequest{Ref: &protocol.WorkspaceRef{Path: draft.Workspace}}, protocol.CallOptions{})
	if err != nil {
		return nil, err
	}
	if workspace.Availability != protocol.WorkspaceAvailable || workspace.Ref.Path != draft.Workspace {
		return nil, errors.New("filewatch: configure an available canonical workspace from workspaces.resolve")
	}
	_, events, err := h.client.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged},
		Watches: []protocol.WatchSpec{{WatchID: generation, Workspace: protocol.WorkspaceRef{Path: draft.Workspace}, Paths: draft.Paths}},
	}, protocol.SubscriptionOptions{})
	return events, err
}

func (h *runHost) Admit(ctx context.Context, namespace string, observation domain.Observation) error {
	capture := observation.Capture()
	input, err := observation.Input()
	if err != nil {
		return err
	}
	start := protocol.StartRunRequest{
		NewSession: &protocol.RunSessionOpening{SessionID: capture.SessionID, RunID: capture.RunID, Title: capture.Trigger.Title, Workspace: protocol.WorkspaceRef{Path: capture.Trigger.Workspace}},
		Input:      []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: input}},
		Provider:   capture.Trigger.Provider, Model: capture.Trigger.Model, ReasoningEffort: capture.Trigger.ReasoningEffort,
	}
	key := "filewatch:run:" + capture.RunID
	acceptance, err := h.client.GetRunAcceptance(ctx, protocol.GetRunAcceptanceRequest{Namespace: namespace, Key: key, Operation: protocol.RunCommandStart, Request: &start}, protocol.CallOptions{})
	if err != nil {
		return err
	}
	switch acceptance.Type {
	case protocol.RunAcceptanceAccepted, protocol.RunAcceptanceRetired:
		if acceptance.SessionID != capture.SessionID || acceptance.RunID != capture.RunID {
			return errors.New("filewatch: Runtime acceptance differs from captured intent")
		}
		return nil
	case protocol.RunAcceptanceNotRecorded:
	default:
		return protocol.ErrInvalidResponse
	}
	ack, _, err := h.client.StartRun(ctx, start, protocol.RunCommandOptions{IdempotencyKey: key})
	if err != nil {
		return err
	}
	if ack.RunID != capture.RunID {
		return protocol.ErrInvalidResponse
	}
	return nil
}

func (h *runHost) Notify(ctx context.Context, contribution string) error {
	return h.client.NotifyPluginChange(ctx, protocol.PluginChangeNotice{Contribution: contribution}, protocol.CommandOptions{})
}

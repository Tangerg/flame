package main

import (
	"context"
	"errors"
	"iter"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/plugins/filewatch/internal/filewatch"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
)

type runHost struct{ client *runtimeclient.Client }

func connectHost(ctx context.Context, ref pluginmcp.CallbackRef) (*runHost, error) {
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

func (h *runHost) Watch(ctx context.Context, generation string, trigger filewatch.Trigger) (iter.Seq2[protocol.RuntimeEvent, error], error) {
	draft := trigger.Draft()
	workspace, err := h.client.ResolveWorkspace(ctx, protocol.ResolveWorkspaceRequest{Ref: &protocol.WorkspaceRef{Path: draft.Workspace}}, protocol.CallOptions{})
	if err != nil {
		return nil, err
	}
	if workspace.Availability != protocol.WorkspaceAvailable || workspace.Ref.Path != draft.Workspace {
		return nil, errors.New("filewatch: configure an available canonical workspace from workspaces.resolve")
	}
	if draft.TargetSessionID != "" {
		if err := h.checkTarget(ctx, draft.TargetSessionID, draft.Workspace); err != nil {
			return nil, err
		}
	}
	_, events, err := h.client.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged},
		Watches: []protocol.WatchSpec{{WatchID: generation, Workspace: protocol.WorkspaceRef{Path: draft.Workspace}, Paths: draft.Paths}},
	}, protocol.SubscriptionOptions{})
	return events, err
}

func (h *runHost) checkTarget(ctx context.Context, id, workspace string) error {
	session, err := h.client.GetSession(ctx, protocol.GetSessionRequest{SessionID: id}, protocol.CallOptions{})
	if err != nil {
		return err
	}
	if session.ID != id || session.Workspace.Ref.Path != workspace {
		return errors.New("filewatch: the target Session must own the watched workspace")
	}
	return nil
}

func (h *runHost) readPartitions(ctx context.Context, draft filewatch.Draft) ([]filewatch.Partition, error) {
	if err := h.checkTarget(ctx, draft.TargetSessionID, draft.Workspace); err != nil {
		return nil, err
	}
	values := make([]filewatch.Partition, 0, len(draft.Paths))
	for _, path := range draft.Paths {
		result, err := h.client.ReadWorkspaceFile(ctx, protocol.ReadFileRequest{
			Workspace: protocol.WorkspaceRef{Path: draft.Workspace}, Path: path, MaxBytes: new(filewatch.PartitionReadLimit),
		}, protocol.CallOptions{})
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		partition := filewatch.Partition{Path: path}
		switch {
		case errors.Is(err, protocol.ErrWorkspaceFileNotFound):
			partition.State = filewatch.PartitionMissing
		case err != nil:
			partition.State = filewatch.PartitionUnavailable
		case result.Truncated:
			partition.State = filewatch.PartitionTruncated
		default:
			partition = filewatch.ReadPartition(path, result.Content)
		}
		values = append(values, partition)
	}
	return values, nil
}

func (h *runHost) Admit(ctx context.Context, namespace string, observation filewatch.Observation) (identity.RunID, error) {
	capture := observation.Capture()
	input, err := observation.Input()
	if err != nil {
		return identity.RunID{}, err
	}
	start := protocol.StartRunRequest{
		Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: input}},
	}
	key := "filewatch:observation:" + capture.Invocation.ID
	if capture.RunID.IsZero() {
		start.SessionID = capture.SessionID.String()
	} else {
		key = "filewatch:run:" + capture.RunID.String()
		start.NewSession = &protocol.RunSessionOpening{
			SessionID: capture.SessionID.String(), RunID: capture.RunID.String(),
			Title: capture.Trigger.Title, Workspace: &protocol.WorkspaceRef{Path: capture.Trigger.Workspace},
		}
		start.Provider, start.Model, start.ReasoningEffort = capture.Trigger.Provider, capture.Trigger.Model, capture.Trigger.ReasoningEffort
	}
	options := protocol.RunCommandOptions{IdempotencyKey: key, IdempotencyNamespace: namespace}
	if capture.Trigger.CapturePartitions {
		options.RequestMeta.ClientCapabilities = &protocol.ClientCapabilities{
			InterruptTypes: []protocol.InterruptType{protocol.InterruptQuestion, protocol.InterruptApproval},
		}
	}
	acceptance, err := h.client.GetRunAcceptance(ctx, protocol.GetRunAcceptanceRequest{Namespace: namespace, Key: key, Operation: protocol.RunCommandStart, Request: &start}, protocol.CallOptions{RequestMeta: options.RequestMeta})
	if err != nil {
		return identity.RunID{}, err
	}
	switch acceptance.Type {
	case protocol.RunAcceptanceAccepted, protocol.RunAcceptanceRetired:
		if acceptance.SessionID != capture.SessionID.String() {
			return identity.RunID{}, errors.New("filewatch: Runtime acceptance belongs to another Session")
		}
		return admittedRun(capture.RunID, acceptance.RunID)
	case protocol.RunAcceptanceNotRecorded:
	default:
		return identity.RunID{}, protocol.ErrInvalidResponse
	}
	if start.NewSession == nil {
		if err := h.checkTarget(ctx, start.SessionID, capture.Trigger.Workspace); err != nil {
			return identity.RunID{}, err
		}
	}
	stream, detach := context.WithCancel(ctx)
	defer detach()
	ack, _, err := h.client.StartRun(stream, start, options)
	if err != nil {
		return identity.RunID{}, err
	}
	return admittedRun(capture.RunID, ack.RunID)
}

func admittedRun(expected identity.RunID, received string) (identity.RunID, error) {
	id, err := identity.ParseRun(received)
	if err != nil || !expected.IsZero() && id != expected {
		return identity.RunID{}, protocol.ErrInvalidResponse
	}
	return id, nil
}

func (h *runHost) Notify(ctx context.Context, contribution string) error {
	return h.client.NotifyPluginChange(ctx, protocol.PluginChangeNotice{Contribution: contribution}, protocol.CallOptions{})
}

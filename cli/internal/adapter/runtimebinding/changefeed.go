package runtimebinding

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/application/changefeed"
)

type changeBinding interface {
	SubscribeRuntime(context.Context, protocol.RuntimeSubscribeRequest, flameruntime.SubscriptionOptions) (*protocol.RuntimeSubscribeResponse, iter.Seq2[protocol.RuntimeEvent, error], error)
}

func (r *Connection) Supports(topic protocol.RuntimeTopic) bool {
	return r.profile.SupportsRuntimeTopic(topic)
}

func (r *Connection) Subscribe(ctx context.Context, subscription changefeed.Subscription) (changefeed.EventStream, error) {
	if err := subscription.Validate(); err != nil {
		return nil, err
	}
	if len(subscription.Watches) != 0 {
		if err := r.requireFeature(protocol.FeatureFileWatch); err != nil {
			return nil, err
		}
	}
	limits := r.profile.discovery.Capabilities.Limits.RuntimeSubscription
	if limits.MaxTopics > 0 && len(subscription.Topics) > limits.MaxTopics {
		return nil, fmt.Errorf("runtime change subscription has %d topics; maximum is %d", len(subscription.Topics), limits.MaxTopics)
	}
	if limits.MaxWatches > 0 && len(subscription.Watches) > limits.MaxWatches {
		return nil, fmt.Errorf("runtime change subscription has %d watches; maximum is %d", len(subscription.Watches), limits.MaxWatches)
	}
	for _, topic := range subscription.Topics {
		if !r.Supports(topic) {
			return nil, errors.New("runtime does not support change topic " + string(topic))
		}
	}
	// Event validation retains this declaration after Subscribe returns.
	// The wire call borrows the same owned topics while registering the stream.
	subscription.Topics = slices.Clone(subscription.Topics)
	subscription.Watches = slices.Clone(subscription.Watches)
	wire := protocol.RuntimeSubscribeRequest{
		Topics:  subscription.Topics,
		Watches: make([]protocol.WatchSpec, 0, len(subscription.Watches)),
	}
	for _, watch := range subscription.Watches {
		wire.Watches = append(wire.Watches, protocol.WatchSpec{
			WatchID: watch.ID, Workspace: protocol.WorkspaceRef{Path: watch.Workspace},
		})
	}
	ack, events, err := r.changes.SubscribeRuntime(ctx, wire, r.changeSubscriptionOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	if ack == nil || events == nil {
		return nil, runtimeContractViolation("subscribe runtime changes returned an incomplete stream")
	}
	return func(yield func(changefeed.Event, error) bool) {
		for event, err := range events {
			if err != nil {
				yield(changefeed.Event{}, classifyError(err))
				return
			}
			if err := protocol.ValidateWireTree(event); err != nil {
				yield(changefeed.Event{}, runtimeContractViolation("runtime change event is invalid: %v", err))
				return
			}
			projected := projectRuntimeEvent(event)
			if err := subscription.ValidateEvent(projected); err != nil {
				yield(changefeed.Event{}, runtimeContractViolation("runtime change event cannot be projected: %v", err))
				return
			}
			if !yield(projected, nil) {
				return
			}
		}
	}, nil
}

func projectRuntimeEvent(event protocol.RuntimeEvent) changefeed.Event {
	projected := changefeed.Event{
		Type: event.Type, Sequence: event.Sequence, WatchID: event.WatchID,
		Paths: slices.Clone(event.Paths), Names: slices.Clone(event.Names), ServerIDs: slices.Clone(event.ServerIDs),
		ScheduleIDs: slices.Clone(event.ScheduleIDs), SessionIDs: slices.Clone(event.SessionIDs),
		RunIDs: slices.Clone(event.RunIDs), Topics: slices.Clone(event.Topics), WatchIDs: slices.Clone(event.WatchIDs),
	}
	if event.Workspace != nil {
		projected.Workspace = event.Workspace.Path
	}
	return projected
}

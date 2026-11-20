package delivery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"slices"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
)

// SubscribeRuntime opens the change-signal stream. The stream's lifetime is
// bounded by the request ctx and by the consumer's range: it ends on client
// disconnect, transport shutdown (the transport force-closes the connection), or an
// early range stop.
//
// The caller says which topics it can fold. There is no wildcard: a client that has
// not named a topic cannot be sent it, because a signal it does not understand is a
// refetch it will not perform. Frames for topics it did not ask for are filtered here
// rather than at the producer — one hub, many subscriptions, each with its own set.
//
// Watches observe Git state and the bounded exact paths the client is currently
// reading. Content notifications preserve workspace/path identity; Git notices
// invalidate only the watch whose semantic HEAD/index state changed.
// watchFiles publishes each change under its watch; a change without paths
// means the watcher lost track, so the client resyncs that watch.
func (s *Handler) watchFiles(watches []protocol.WatchSpec, subscription *runtimeSubscription, report func(error)) (io.Closer, error) {
	scopes := make([]workspaceapp.WatchScope, len(watches))
	for index, watch := range watches {
		scopes[index] = workspaceapp.WatchScope{Key: watch.WatchID, Root: watch.Workspace.Path, Paths: watch.Paths}
	}
	return s.workspaceWatch.Watch(scopes, func(change workspaceapp.ObservationChange) {
		event := protocol.RuntimeEvent{
			Type: protocol.RuntimeFilesChanged, WatchID: change.Key,
			Workspace: &protocol.WorkspaceRef{Path: change.Root}, Paths: change.Paths,
		}
		if len(change.Paths) == 0 {
			event = protocol.RuntimeEvent{Type: protocol.RuntimeResync, Topics: []protocol.RuntimeTopic{protocol.TopicFilesChanged}, WatchIDs: []string{change.Key}}
		}
		s.runtimeEvents.publishTo(subscription, event)
	}, report)
}

func (s *Handler) SubscribeRuntime(ctx context.Context, request protocol.RuntimeSubscribeRequest) (*protocol.RuntimeSubscribeResponse, iter.Seq2[protocol.RuntimeEvent, error], error) {
	topics, err := s.subscribedTopics(request.Topics)
	if err != nil {
		return nil, nil, err
	}
	workingDirectories, watchIDs, err := validateWorkspaceWatches(request.Watches, topics)
	if err != nil {
		return nil, nil, err
	}

	// SubscribeRuntime owns the channel: the hub broadcasts to it and (when
	// watches are present) the application-owned watcher emits to it. Closing it
	// only after that watcher has stopped keeps emit from racing the close.
	events := make(chan protocol.RuntimeEvent, runtimeSubscriptionQueueCapacity)
	subscription, unregister, registered := s.runtimeEvents.register(events, topics, watchIDs)
	if !registered {
		close(events)
		return nil, nil, errSubscriptionAdmissionsClosed
	}

	observationErrors := make(chan error, 1)
	reportObservationError := func(err error) {
		select {
		case observationErrors <- err:
		default:
		}
	}
	var fileWatcher io.Closer
	if len(workingDirectories) > 0 {
		fileWatcher, err = s.watchFiles(request.Watches, subscription, reportObservationError)
		if err != nil {
			unregister()
			close(events)
			return nil, nil, wireWorkspaceError(fmt.Errorf("start workspace watcher: %w", err))
		}
	}
	var authoredWatcher io.Closer
	authoredResources := subscribedAuthoredResources(topics)
	if len(authoredResources) > 0 {
		authoredWatcher, err = s.workspaceAuthoredWatch.Watch(
			workingDirectories,
			authoredResources,
			func(resource workspaceapp.AuthoredResource) {
				invalidationResource := resource.InvalidationResource()
				if !invalidationResource.Valid() {
					return
				}
				if event, ok := runtimeEventFor(invalidation.Notice{Resource: invalidationResource}); ok {
					s.runtimeEvents.publishTo(subscription, event)
				}
			},
			reportObservationError,
		)
		if err != nil {
			var cleanupErr error
			if fileWatcher != nil {
				cleanupErr = fileWatcher.Close()
			}
			unregister()
			close(events)
			return nil, nil, wireWorkspaceError(fmt.Errorf("start authored resource watcher: %w", errors.Join(err, cleanupErr)))
		}
	}

	releaseSubscription := sync.OnceValue(func() error {
		var authoredErr, fileErr error
		if authoredWatcher != nil {
			authoredErr = authoredWatcher.Close()
		}
		if fileWatcher != nil {
			fileErr = fileWatcher.Close()
		}
		unregister()
		close(events)
		err := errors.Join(authoredErr, fileErr)
		if err != nil {
			// An abandoned consumer has no terminal frame to receive cleanup failure.
			slog.ErrorContext(ctx, "runtime subscription cleanup failed", "error", err)
		}
		return err
	})
	stopContextRelease := context.AfterFunc(ctx, func() { _ = releaseSubscription() })
	stopSubscription := func() error {
		stopContextRelease()
		return releaseSubscription()
	}
	return &protocol.RuntimeSubscribeResponse{}, subscriptionEventSequence(
		events,
		observationErrors,
		subscription.exhausted,
		func() { s.runtimeEvents.drained(subscription) },
		stopSubscription,
	), nil
}

func subscribedAuthoredResources(topics map[protocol.RuntimeTopic]bool) []workspaceapp.AuthoredResource {
	resources := make([]workspaceapp.AuthoredResource, 0, 2)
	if topics[protocol.TopicHooksChanged] {
		resources = append(resources, workspaceapp.AuthoredHooks)
	}
	if topics[protocol.TopicSkillsChanged] {
		resources = append(resources, workspaceapp.AuthoredSkills)
	}
	return resources
}

// subscriptionEventSequence presents a subscription channel as the iter.Seq
// used by the wire streaming contract. The coalescing fan-out hub keeps its
// channels internally; only this outer boundary is a sequence. queueDrained lets
// the hub flush a pending resync, and stopSubscription releases an early-ending
// consumer.
func subscriptionEventSequence(
	events <-chan protocol.RuntimeEvent,
	failures <-chan error,
	exhausted <-chan struct{},
	queueDrained func(),
	stopSubscription func() error,
) iter.Seq2[protocol.RuntimeEvent, error] {
	return func(yield func(protocol.RuntimeEvent, error) bool) {
		defer stopSubscription()
		consume := func() (bool, error) {
			for {
				select {
				case err := <-failures:
					return true, err
				case event, open := <-events:
					if !open {
						return true, nil
					}
					queueDrained()
					if !yield(event, nil) {
						return false, nil
					}
				case <-exhausted:
					// Exhaustion fences producers, so only already accepted frames remain.
					for {
						select {
						case event, open := <-events:
							if !open {
								return true, nil
							}
							if !yield(event, nil) {
								return false, nil
							}
						default:
							return true, nil
						}
					}
				}
			}
		}
		completed, err := consume()
		cleanupErr := stopSubscription()
		if !completed {
			return
		}
		// Joined watchers cannot publish after this final failure read.
		select {
		case pending := <-failures:
			err = errors.Join(err, pending)
		default:
		}
		if err = errors.Join(err, cleanupErr); err != nil {
			yield(protocol.RuntimeEvent{}, wireWorkspaceError(fmt.Errorf("runtime subscription ended: %w", err)))
		}
	}
}

// subscribedTopics materializes the already shape-validated request as a lookup
// set. The generated RuntimeSubscribeRequest validator owns non-empty/unique
// semantics; this method retains the composition-dependent checks: the enforced
// fan-out cap and whether this runtime actually advertises each topic.
func (s *Handler) subscribedTopics(requested []protocol.RuntimeTopic) (map[protocol.RuntimeTopic]bool, error) {
	if len(requested) > protocol.MaxSubscriptionTopics {
		return nil, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("at most %d topics per subscription", protocol.MaxSubscriptionTopics))
	}
	advertised := s.capabilities().RuntimeTopics
	topics := make(map[protocol.RuntimeTopic]bool, len(requested))
	for _, topic := range requested {
		if !slices.Contains(advertised, topic) {
			return nil, NewCapabilityGapError(protocol.CapabilityRequirement{
				Type: protocol.RequirementRuntimeTopic, Name: string(topic),
			})
		}
		topics[topic] = true
	}
	return topics, nil
}

// validateWorkspaceWatches validates the wire-only portion of workspace watches
// and returns the working directories and stable watch identities they name. Root resolution,
// repository layout and filesystem notification are application/adapter concerns;
// Delivery retains only the protocol's required watch identifier.
//
// Watches are legal only with files.changed: the other topics are global, so a watch
// would narrow nothing — and a caller that registered one expecting it to is holding
// a belief the runtime would never honor.
func validateWorkspaceWatches(watches []protocol.WatchSpec, topics map[protocol.RuntimeTopic]bool) (workingDirectories, watchIDs []string, err error) {
	if len(watches) == 0 {
		return nil, nil, nil
	}
	if !topics[protocol.TopicFilesChanged] {
		return nil, nil, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("watches require the %s topic", protocol.TopicFilesChanged))
	}
	if len(watches) > protocol.MaxSubscriptionWatches {
		return nil, nil, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("at most %d watches per subscription", protocol.MaxSubscriptionWatches))
	}
	workingDirectories = make([]string, 0, len(watches))
	watchIDs = make([]string, 0, len(watches))
	for _, spec := range watches {
		if spec.WatchID == "" {
			return nil, nil, protocol.NewFailure(protocol.ErrInvalidParams, "watchId is required")
		}
		if slices.Contains(watchIDs, spec.WatchID) {
			return nil, nil, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("watchId %q is registered twice", spec.WatchID))
		}
		workingDirectories = append(workingDirectories, spec.Workspace.Path)
		watchIDs = append(watchIDs, spec.WatchID)
	}
	return workingDirectories, watchIDs, nil
}

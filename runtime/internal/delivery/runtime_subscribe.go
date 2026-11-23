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
	"github.com/Tangerg/flame/runtime/internal/telemetry"
)

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
			slog.ErrorContext(ctx, "runtime subscription cleanup failed", telemetry.Error(err))
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

func subscriptionEventSequence(
	events <-chan protocol.RuntimeEvent,
	failures <-chan error,
	exhausted <-chan struct{},
	queueDrained func(),
	stopSubscription func() error,
) iter.Seq2[protocol.RuntimeEvent, error] {
	return func(yield func(protocol.RuntimeEvent, error) bool) {
		defer stopSubscription()
		completed, err := consumeSubscription(events, failures, exhausted, queueDrained, yield)
		cleanupErr := stopSubscription()
		if !completed {
			return
		}
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

func consumeSubscription(
	events <-chan protocol.RuntimeEvent,
	failures <-chan error,
	exhausted <-chan struct{},
	queueDrained func(),
	yield func(protocol.RuntimeEvent, error) bool,
) (completed bool, err error) {
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
			return drainBufferedEvents(events, yield), nil
		}
	}
}

// An exhausted source publishes nothing new, so only events already buffered
// remain deliverable.
func drainBufferedEvents(events <-chan protocol.RuntimeEvent, yield func(protocol.RuntimeEvent, error) bool) (completed bool) {
	for {
		select {
		case event, open := <-events:
			if !open {
				return true
			}
			if !yield(event, nil) {
				return false
			}
		default:
			return true
		}
	}
}

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

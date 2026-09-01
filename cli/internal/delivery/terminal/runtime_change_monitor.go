package terminal

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/cli/internal/application/retry"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/runtime/protocol"
)

const workspaceWatchID = "flame-active-workspace"

func (a *app) followRuntimeChanges() {
	a.operations.Cancel(runtimeChangesOperation)
	workspacePath := a.session.current.Workspace.Path
	var repository WorkspaceChanges
	if a.runtimeSupports(protocol.FeatureGit) {
		repository = a.workspaces
	}
	if repository == nil && a.changes == nil {
		return
	}
	dispatcher := a.loop.Dispatcher()
	a.operations.Go(runtimeChangesOperation, true, func(ctx context.Context, lease operationLease) {
		monitor := runtimeChangeMonitor{
			workspace: workspacePath, repository: repository, source: a.changes,
			recovery:           runtimeRecoveryBackoff,
			subscriptionLimits: a.runtimeChangeSubscriptionLimits(),
			watchFiles:         a.runtimeSupports(protocol.FeatureFileWatch),
			resources:          a.observedRuntimeResources(),
			applyFiles: func(changes []workspace.Change) error {
				return post(ctx, dispatcher, func() {
					if !a.operations.Current(lease) || a.closed || a.session.current.Workspace.Path != workspacePath {
						return
					}
					a.applyWorkspaceChanges(changes)
				})
			},
			applyEvent: func(event changefeed.Event) error {
				return post(ctx, dispatcher, func() {
					if !a.operations.Current(lease) || a.closed || a.session.current.Workspace.Path != workspacePath {
						return
					}
					a.applyRuntimeInvalidation(event)
				})
			},
			applyResync: func(topics []changefeed.Topic) error {
				return post(ctx, dispatcher, func() {
					if !a.operations.Current(lease) || a.closed || a.session.current.Workspace.Path != workspacePath {
						return
					}
					a.applyRuntimeResync(topics)
				})
			},
		}
		if err := monitor.run(ctx); err != nil && context.Cause(ctx) == nil {
			_ = post(ctx, dispatcher, func() {
				if !a.operations.Current(lease) || a.closed || a.session.current.Workspace.Path != workspacePath {
					return
				}
				a.message("runtime change observation stopped: " + err.Error())
			})
		}
	})
}

func (a *app) applyWorkspaceChanges(changes []workspace.Change) {
	a.header.SetWorkspaceChanges(len(changes))
	if a.dialogs.workspaceReader == workspaceReaderChanges {
		follow := a.dialogs.reader.scroll.AtBottom()
		a.dialogs.reader.replace(workspaceChangesDocument(a.session.current.Workspace.Path, changes), true, follow)
	}
}

type runtimeChangeMonitor struct {
	workspace          string
	repository         WorkspaceChanges
	source             changefeed.Source
	recovery           retry.Backoff
	watchFiles         bool
	resources          runtimeResourceObservation
	applyFiles         func([]workspace.Change) error
	applyEvent         func(changefeed.Event) error
	applyResync        func([]changefeed.Topic) error
	subscriptionLimits changefeed.SubscriptionLimits
}

type runtimeResourceObservation struct {
	plan        bool
	goals       bool
	skills      bool
	mcp         bool
	schedules   bool
	knowledge   bool
	hooks       bool
	models      bool
	approvals   bool
	agentMemory bool
}

func (r runtimeResourceObservation) hasWorkspaceAuthoredResources() bool {
	return r.knowledge || r.hooks
}

func (a *app) observedRuntimeResources() runtimeResourceObservation {
	return runtimeResourceObservation{
		plan:        a.runtimeSupports(protocol.FeaturePlan),
		goals:       a.goals != nil && a.runtimeSupports(protocol.FeatureGoals),
		skills:      a.skills != nil && a.runtimeSupports(protocol.FeatureSkills),
		mcp:         a.mcp != nil && a.runtimeSupports(protocol.FeatureMCP),
		schedules:   a.schedules != nil && a.runtimeSupports(protocol.FeatureSchedules),
		knowledge:   a.knowledge != nil && a.runtimeSupports(protocol.FeatureKnowledge),
		hooks:       a.hooks != nil,
		models:      true,
		approvals:   true,
		agentMemory: a.agentMemory != nil && a.runtimeSupports(protocol.FeatureAgentMemory),
	}
}

func (r runtimeChangeMonitor) observesWorkspace() bool {
	return r.watchFiles && (r.repository != nil || r.resources.hasWorkspaceAuthoredResources())
}

func (r runtimeChangeMonitor) run(ctx context.Context) error {
	topics := r.supportedTopics()
	if r.source == nil || len(topics) == 0 {
		return r.runWithoutWatch(ctx)
	}
	requested := changefeed.Subscription{Topics: topics}
	if r.observesWorkspace() && containsTopic(topics, changefeed.FilesChanged) {
		requested.Watches = []changefeed.Watch{{ID: workspaceWatchID, Workspace: r.workspace}}
	}
	subscriptions, err := r.subscriptionLimits.Partition(requested)
	if err != nil {
		return fmt.Errorf("plan runtime change subscriptions: %w", err)
	}
	if len(subscriptions) == 1 {
		return r.runSubscription(ctx, subscriptions[0], r.repository != nil)
	}
	return r.runSubscriptions(ctx, subscriptions)
}

func (a *app) runtimeChangeSubscriptionLimits() changefeed.SubscriptionLimits {
	if a.runtimeProfile == nil {
		return changefeed.SubscriptionLimits{}
	}
	limits := a.runtimeProfile.Limits.RuntimeSubscription
	return changefeed.SubscriptionLimits{MaxTopics: limits.MaxTopics, MaxWatches: limits.MaxWatches}
}

func (r runtimeChangeMonitor) runSubscriptions(ctx context.Context, subscriptions []changefeed.Subscription) error {
	groupContext, cancelGroup := context.WithCancelCause(ctx)
	defer cancelGroup(nil)

	fileOwner := 0
	for index, subscription := range subscriptions {
		if containsTopic(subscription.Topics, changefeed.FilesChanged) {
			fileOwner = index
			break
		}
	}
	results := make(chan error, len(subscriptions))
	for index, subscription := range subscriptions {
		ownsFileProjection := r.repository != nil && index == fileOwner
		go func(subscription changefeed.Subscription, ownsFileProjection bool) {
			results <- r.runSubscription(groupContext, subscription, ownsFileProjection)
		}(subscription, ownsFileProjection)
	}

	first := <-results
	cancelGroup(first)
	for range len(subscriptions) - 1 {
		<-results
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return first
}

func (r runtimeChangeMonitor) runSubscription(
	ctx context.Context,
	subscription changefeed.Subscription,
	ownsFileProjection bool,
) error {
	setupFailures, streamFailures := 0, 0
	for context.Cause(ctx) == nil {
		attempt, err := r.openSubscriptionAttempt(ctx, subscription, ownsFileProjection)
		if err != nil {
			setupFailures++
			if retryErr := r.waitToRetry(ctx, err, setupFailures); retryErr != nil {
				return retryErr
			}
			continue
		}
		setupFailures = 0
		progressed, attemptErr := r.consumeSubscription(attempt, subscription.Topics, ownsFileProjection)
		attempt.cancel()
		if cause := context.Cause(ctx); cause != nil {
			return cause
		}
		if progressed {
			streamFailures = 0
		}
		streamFailures++
		if retryErr := r.waitToRetry(ctx, attemptErr, streamFailures); retryErr != nil {
			return retryErr
		}
	}
	return context.Cause(ctx)
}

type runtimeChangeAttempt struct {
	ctx    context.Context
	cancel context.CancelFunc
	stream changefeed.EventStream
}

func (r runtimeChangeMonitor) openSubscriptionAttempt(
	ctx context.Context,
	subscription changefeed.Subscription,
	ownsFileProjection bool,
) (runtimeChangeAttempt, error) {
	attemptContext, cancelAttempt := context.WithCancel(ctx)
	attempt := runtimeChangeAttempt{ctx: attemptContext, cancel: cancelAttempt}
	stream, err := r.source.Subscribe(attemptContext, subscription)
	if err != nil {
		cancelAttempt()
		return runtimeChangeAttempt{}, err
	}
	attempt.stream = stream

	// Register before every authoritative cold refresh. Events racing the reads
	// remain buffered and trigger a later replacement, closing read-then-subscribe
	// gaps. File query and watch support are independent, so the owner also installs
	// the file projection when files.changed itself was not negotiated.
	if ownsFileProjection {
		if err := r.refreshFiles(attemptContext); err != nil {
			cancelAttempt()
			return runtimeChangeAttempt{}, err
		}
	}
	if err := r.resync(subscription.Topics); err != nil {
		cancelAttempt()
		return runtimeChangeAttempt{}, err
	}
	return attempt, nil
}

func (r runtimeChangeMonitor) consumeSubscription(
	attempt runtimeChangeAttempt,
	topics []changefeed.Topic,
	ownsFileProjection bool,
) (bool, error) {
	sequences := changefeed.NewSequenceTracker()
	progressed := false
	for event, streamErr := range attempt.stream {
		if streamErr != nil {
			return progressed, streamErr
		}
		applied, err := r.consumeChangeEvent(attempt.ctx, topics, ownsFileProjection, sequences, event)
		if err != nil {
			return progressed, err
		}
		progressed = progressed || applied
	}
	return progressed, fmt.Errorf("%w: runtime change stream ended", agent.ErrDisconnected)
}

func (r runtimeChangeMonitor) consumeChangeEvent(
	ctx context.Context,
	topics []changefeed.Topic,
	ownsFileProjection bool,
	sequences *changefeed.SequenceTracker,
	event changefeed.Event,
) (bool, error) {
	disposition, err := sequences.Observe(event.Sequence)
	if err != nil {
		return false, err
	}
	if disposition == changefeed.SequenceStale {
		return false, nil
	}
	if disposition == changefeed.SequenceGap {
		if ownsFileProjection && containsTopic(topics, changefeed.FilesChanged) {
			if err := r.refreshFiles(ctx); err != nil {
				return false, err
			}
		}
		if err := r.resync(topics); err != nil {
			return false, err
		}
		// The reads started after this frame was observed, so they include both
		// the missing changes and this frame. Reapplying it can starve convergence
		// on a persistently gappy stream.
		return true, nil
	}
	if ownsFileProjection && r.invalidatesFiles(event) {
		if err := r.refreshFiles(ctx); err != nil {
			return false, err
		}
	}
	if event.Type != changefeed.EventType(changefeed.FilesChanged) {
		if err := r.invalidate(event); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (r runtimeChangeMonitor) waitToRetry(ctx context.Context, failure error, failures int) error {
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	if !retry.IsReconnectable(failure) {
		return failure
	}
	if err := r.recovery.Wait(ctx, failures); err != nil {
		return err
	}
	return nil
}

func (r runtimeChangeMonitor) runWithoutWatch(ctx context.Context) error {
	if r.repository == nil {
		return nil
	}
	failures := 0
	for context.Cause(ctx) == nil {
		if err := r.refreshFiles(ctx); err == nil {
			return nil
		} else if !retry.IsReconnectable(err) {
			return err
		}
		failures++
		if err := r.recovery.Wait(ctx, failures); err != nil {
			return err
		}
	}
	return context.Cause(ctx)
}

func (r runtimeChangeMonitor) supportedTopics() []changefeed.Topic {
	if r.source == nil {
		return nil
	}
	candidates := []changefeed.Topic{changefeed.SessionsChanged, changefeed.RunsChanged}
	if r.resources.plan {
		candidates = append(candidates, changefeed.PlanChanged)
	}
	candidates = append(candidates, changefeed.InterruptsChanged)
	if r.resources.goals {
		candidates = append(candidates, changefeed.GoalsChanged)
	}
	if r.resources.skills {
		candidates = append(candidates, changefeed.SkillsChanged)
	}
	if r.resources.mcp {
		candidates = append(candidates, changefeed.MCPChanged)
	}
	if r.resources.schedules {
		candidates = append(candidates, changefeed.SchedulesChanged)
	}
	if r.resources.knowledge {
		candidates = append(candidates, changefeed.KnowledgeChanged)
	}
	if r.resources.hooks {
		candidates = append(candidates, changefeed.HooksChanged)
	}
	if r.resources.models {
		candidates = append(candidates, changefeed.ModelsChanged)
	}
	if r.resources.approvals {
		candidates = append(candidates, changefeed.ApprovalsChanged)
	}
	if r.resources.agentMemory {
		candidates = append(candidates, changefeed.AgentMemoryChanged)
	}
	if r.observesWorkspace() {
		candidates = append([]changefeed.Topic{changefeed.FilesChanged}, candidates...)
	}
	topics := make([]changefeed.Topic, 0, len(candidates))
	for _, topic := range candidates {
		if r.source.Supports(topic) {
			topics = append(topics, topic)
		}
	}
	return topics
}

func (r runtimeChangeMonitor) refreshFiles(ctx context.Context) error {
	if r.repository == nil || r.applyFiles == nil {
		return nil
	}
	changes, err := r.repository.Changes(ctx, r.workspace)
	if errors.Is(err, workspace.ErrVersionControlUnavailable) {
		changes = nil
	} else if err != nil {
		return err
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return r.applyFiles(changes)
}

func (r runtimeChangeMonitor) invalidate(event changefeed.Event) error {
	if r.applyEvent == nil {
		return nil
	}
	return r.applyEvent(event)
}

func (r runtimeChangeMonitor) resync(topics []changefeed.Topic) error {
	if r.applyResync == nil {
		return nil
	}
	return r.applyResync(topics)
}

func (r runtimeChangeMonitor) invalidatesFiles(event changefeed.Event) bool {
	switch event.Type {
	case changefeed.EventType(changefeed.FilesChanged):
		if event.WatchID != "" {
			return event.WatchID == workspaceWatchID &&
				(event.Workspace == "" || event.Workspace == r.workspace)
		}
		// Agent tool writes are broad file invalidations. They carry the
		// affected workspace but no client watch identity, and must refresh the
		// same authoritative projection as a watch-produced signal.
		return event.Workspace == "" || event.Workspace == r.workspace
	case changefeed.Resync:
		return containsTopic(event.Topics, changefeed.FilesChanged) || containsString(event.WatchIDs, workspaceWatchID)
	default:
		return false
	}
}

func containsTopic(values []changefeed.Topic, target changefeed.Topic) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

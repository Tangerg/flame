package changefeed

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func Topics() []protocol.RuntimeTopic {
	return protocol.RuntimeTopics()
}

type Watch struct {
	ID        string
	Workspace string
}

func (w Watch) Validate() error {
	if strings.TrimSpace(w.ID) == "" || strings.TrimSpace(w.Workspace) == "" {
		return errors.New("change watch requires id and workspace")
	}
	return nil
}

type Subscription struct {
	Topics  []protocol.RuntimeTopic
	Watches []Watch
}

type SubscriptionLimits struct {
	MaxTopics  int
	MaxWatches int
}

func (s SubscriptionLimits) Partition(subscription Subscription) ([]Subscription, error) {
	if err := subscription.Validate(); err != nil {
		return nil, err
	}
	if s.MaxTopics <= 0 || s.MaxWatches <= 0 {
		return nil, errors.New("change subscription limits must be positive")
	}
	topicLimit := min(s.MaxTopics, len(subscription.Topics))
	watchLimit := min(s.MaxWatches, len(subscription.Watches))
	if len(subscription.Topics) <= topicLimit && len(subscription.Watches) <= watchLimit {
		return []Subscription{cloneSubscription(subscription)}, nil
	}
	if len(subscription.Watches) > 0 {
		workspaceTopics := workspaceObservedTopics(subscription.Topics)
		if len(workspaceTopics) > 0 {
			return partitionWorkspaceObservation(subscription, workspaceTopics, topicLimit, watchLimit)
		}
	}

	partitions := partitionTopics(subscription.Topics, topicLimit)
	if len(subscription.Watches) == 0 {
		return partitions, nil
	}

	filePartition := slices.Index(subscription.Topics, protocol.TopicFilesChanged) / topicLimit
	remainingWatches := subscription.Watches
	count := min(watchLimit, len(remainingWatches))
	partitions[filePartition].Watches = slices.Clone(remainingWatches[:count])
	remainingWatches = remainingWatches[count:]
	for len(remainingWatches) > 0 {
		count = min(watchLimit, len(remainingWatches))
		partitions = append(partitions, Subscription{
			Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged},
			Watches: slices.Clone(remainingWatches[:count]),
		})
		remainingWatches = remainingWatches[count:]
	}
	return partitions, nil
}

func workspaceObservedTopics(topics []protocol.RuntimeTopic) []protocol.RuntimeTopic {
	var observed []protocol.RuntimeTopic
	for _, topic := range topics {
		switch topic {
		case protocol.TopicHooksChanged, protocol.TopicSkillsChanged:
			observed = append(observed, topic)
		case protocol.TopicFilesChanged, protocol.TopicMCPChanged, protocol.TopicPluginsChanged, protocol.TopicPluginDataChanged, protocol.TopicSessionsChanged, protocol.TopicRunsChanged, protocol.TopicPlanChanged, protocol.TopicGoalsChanged, protocol.TopicInterruptsChanged, protocol.TopicModelsChanged, protocol.TopicApprovalsChanged:
		}
	}
	return observed
}

func partitionWorkspaceObservation(
	subscription Subscription,
	workspaceTopics []protocol.RuntimeTopic,
	topicLimit int,
	watchLimit int,
) ([]Subscription, error) {
	if topicLimit < 2 {
		return nil, errors.New("change subscription topic limit cannot preserve workspace observation")
	}
	workspaceTopicPartitions := partitionTopics(workspaceTopics, topicLimit-1)
	watchPartitions := partitionWatches(subscription.Watches, watchLimit)
	partitions := make([]Subscription, 0, len(workspaceTopicPartitions)*len(watchPartitions))
	for _, watches := range watchPartitions {
		for _, topics := range workspaceTopicPartitions {
			partitions = append(partitions, Subscription{
				Topics:  append([]protocol.RuntimeTopic{protocol.TopicFilesChanged}, topics.Topics...),
				Watches: slices.Clone(watches),
			})
		}
	}

	unscoped := make([]protocol.RuntimeTopic, 0, len(subscription.Topics))
	for _, topic := range subscription.Topics {
		if topic != protocol.TopicFilesChanged && !slices.Contains(workspaceTopics, topic) {
			unscoped = append(unscoped, topic)
		}
	}
	return append(partitions, partitionTopics(unscoped, topicLimit)...), nil
}

func partitionTopics(topics []protocol.RuntimeTopic, limit int) []Subscription {
	partitions := make([]Subscription, 0, (len(topics)+limit-1)/limit)
	for remaining := topics; len(remaining) > 0; {
		count := min(limit, len(remaining))
		partitions = append(partitions, Subscription{Topics: slices.Clone(remaining[:count])})
		remaining = remaining[count:]
	}
	return partitions
}

func partitionWatches(watches []Watch, limit int) [][]Watch {
	partitions := make([][]Watch, 0, (len(watches)+limit-1)/limit)
	for remaining := watches; len(remaining) > 0; {
		count := min(limit, len(remaining))
		partitions = append(partitions, slices.Clone(remaining[:count]))
		remaining = remaining[count:]
	}
	return partitions
}

func cloneSubscription(subscription Subscription) Subscription {
	return Subscription{
		Topics:  slices.Clone(subscription.Topics),
		Watches: slices.Clone(subscription.Watches),
	}
}

func (s Subscription) Validate() error {
	if len(s.Topics) == 0 {
		return errors.New("change subscription has no topics")
	}
	seen := make(map[protocol.RuntimeTopic]struct{}, len(s.Topics))
	for _, topic := range s.Topics {
		if !slices.Contains(Topics(), topic) {
			return fmt.Errorf("change subscription topic %q is invalid", topic)
		}
		if _, duplicate := seen[topic]; duplicate {
			return fmt.Errorf("change subscription repeats topic %q", topic)
		}
		seen[topic] = struct{}{}
	}
	if len(s.Watches) > 0 && !slices.Contains(s.Topics, protocol.TopicFilesChanged) {
		return errors.New("file watches require the files.changed topic")
	}
	watchIDs := make(map[string]struct{}, len(s.Watches))
	for _, watch := range s.Watches {
		if err := watch.Validate(); err != nil {
			return err
		}
		if _, duplicate := watchIDs[watch.ID]; duplicate {
			return fmt.Errorf("change subscription repeats watch %q", watch.ID)
		}
		watchIDs[watch.ID] = struct{}{}
	}
	return nil
}

func (s Subscription) ValidateEvent(event Event) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := event.Validate(); err != nil {
		return err
	}
	if event.Type == protocol.RuntimeResync {
		return s.validateResyncScope(event)
	}
	return s.validateChangeScope(event)
}

func (s Subscription) validateChangeScope(event Event) error {
	topic := protocol.RuntimeTopic(event.Type)
	if !slices.Contains(s.Topics, topic) {
		return fmt.Errorf("change event topic %q is outside the subscription", topic)
	}
	if topic != protocol.TopicFilesChanged || event.WatchID == "" {
		return nil
	}
	watchIndex := slices.IndexFunc(s.Watches, func(watch Watch) bool {
		return watch.ID == event.WatchID
	})
	if watchIndex < 0 {
		return fmt.Errorf("file change watch %q is outside the subscription", event.WatchID)
	}
	if event.Workspace != "" && event.Workspace != s.Watches[watchIndex].Workspace {
		return fmt.Errorf("file change watch %q names another workspace", event.WatchID)
	}
	return nil
}

func (s Subscription) validateResyncScope(event Event) error {
	for _, topic := range event.Topics {
		if !slices.Contains(s.Topics, topic) {
			return fmt.Errorf("resync topic %q is outside the subscription", topic)
		}
	}
	if len(event.WatchIDs) > 0 && !slices.Contains(event.Topics, protocol.TopicFilesChanged) {
		return errors.New("resync watch scope requires the files.changed topic")
	}
	for _, watchID := range event.WatchIDs {
		if !slices.ContainsFunc(s.Watches, func(watch Watch) bool { return watch.ID == watchID }) {
			return fmt.Errorf("resync watch %q is outside the subscription", watchID)
		}
	}
	return nil
}

type Event struct {
	Type       protocol.RuntimeEventType
	Sequence   uint64
	WatchID    string
	Workspace  string
	Paths      []string
	Names      []string
	Servers    []protocol.MCPServerID
	SessionIDs []string
	RunIDs     []string
	Topics     []protocol.RuntimeTopic
	WatchIDs   []string
}

func (e Event) Validate() error {
	if e.Sequence == 0 {
		return errors.New("change event sequence is zero")
	}
	if e.Type == protocol.RuntimeResync {
		if len(e.Topics) == 0 && len(e.WatchIDs) == 0 {
			return errors.New("resync event has no affected scope")
		}
		return nil
	}
	topic := protocol.RuntimeTopic(e.Type)
	if !slices.Contains(Topics(), topic) {
		return fmt.Errorf("change event type %q is invalid", e.Type)
	}
	if topic == protocol.TopicFilesChanged {
		if len(e.Paths) == 0 {
			return errors.New("file change event is incomplete")
		}
	}
	return nil
}

type EventStream = iter.Seq2[Event, error]

type Source interface {
	Supports(protocol.RuntimeTopic) bool
	Subscribe(context.Context, Subscription) (EventStream, error)
}

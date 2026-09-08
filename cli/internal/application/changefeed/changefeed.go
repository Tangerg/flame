// Package changefeed models runtime-wide invalidations. Events deliberately
// carry identities, not duplicated resource state; consumers refetch through
// the authoritative bounded-context query port.
package changefeed

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/protocol"
)

// Topics returns the complete change vocabulary understood by this client.
// Callers own the returned slice, so subscription policy cannot mutate the
// package's inventory.
func Topics() []protocol.RuntimeTopic {
	return []protocol.RuntimeTopic{
		protocol.TopicFilesChanged,
		protocol.TopicSkillsChanged,
		protocol.TopicMCPChanged,
		protocol.TopicSchedulesChanged,
		protocol.TopicSessionsChanged,
		protocol.TopicRunsChanged,
		protocol.TopicPlanChanged,
		protocol.TopicGoalsChanged,
		protocol.TopicInterruptsChanged,
		protocol.TopicKnowledgeChanged,
		protocol.TopicHooksChanged,
		protocol.TopicModelsChanged,
		protocol.TopicApprovalsChanged,
		protocol.TopicAgentMemoryChanged,
	}
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

// ValidateEvent checks both the event shape and the delivery scope owned by
// this subscription. Runtime events are invalidations, so accepting a frame
// for an undeclared topic or watch can make an unrelated local projection look
// authoritative.
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
	Type        protocol.RuntimeEventType
	Sequence    uint64
	WatchID     string
	Workspace   string
	Paths       []string
	Names       []string
	ServerIDs   []string
	ScheduleIDs []string
	SessionIDs  []string
	RunIDs      []string
	Topics      []protocol.RuntimeTopic
	WatchIDs    []string
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
		// Tool writes are broad invalidations and intentionally carry no watch ID.
		// Watch-produced signals may add WatchID and Workspace so consumers can
		// narrow the authoritative read, but neither is required by the protocol.
		if len(e.Paths) == 0 {
			return errors.New("file change event is incomplete")
		}
	}
	return nil
}

type EventStream = iter.Seq2[Event, error]

type Source interface {
	Supports(protocol.RuntimeTopic) bool
	// Subscribe borrows the request for the call. Implementations own any
	// declaration retained for the returned stream's lifetime.
	Subscribe(context.Context, Subscription) (EventStream, error)
}

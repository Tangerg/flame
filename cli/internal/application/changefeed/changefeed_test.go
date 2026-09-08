package changefeed

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestTopicsReturnsAnOwnedCompleteInventory(t *testing.T) {
	t.Parallel()
	want := []protocol.RuntimeTopic{
		protocol.TopicFilesChanged, protocol.TopicSkillsChanged, protocol.TopicMCPChanged, protocol.TopicSchedulesChanged,
		protocol.TopicSessionsChanged, protocol.TopicRunsChanged, protocol.TopicPlanChanged, protocol.TopicGoalsChanged, protocol.TopicInterruptsChanged,
		protocol.TopicKnowledgeChanged, protocol.TopicHooksChanged, protocol.TopicModelsChanged, protocol.TopicApprovalsChanged,
		protocol.TopicAgentMemoryChanged,
	}
	got := Topics()
	if !slices.Equal(got, want) {
		t.Fatalf("Topics = %v, want %v", got, want)
	}
	got[0] = "mutated"
	if Topics()[0] != protocol.TopicFilesChanged {
		t.Fatal("mutating a Topics result rewrote the package inventory")
	}
}

func TestSubscriptionMakesWatchScopeExplicit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		subscription Subscription
		want         string
	}{
		{name: "valid", subscription: Subscription{Topics: []protocol.RuntimeTopic{protocol.TopicFilesChanged}, Watches: []Watch{{ID: "active", Workspace: "/workspace"}}}},
		{name: "watch without topic", subscription: Subscription{Topics: []protocol.RuntimeTopic{protocol.TopicRunsChanged}, Watches: []Watch{{ID: "active", Workspace: "/workspace"}}}, want: "files.changed"},
		{name: "duplicate topic", subscription: Subscription{Topics: []protocol.RuntimeTopic{protocol.TopicFilesChanged, protocol.TopicFilesChanged}}, want: "repeats"},
		{name: "duplicate watch", subscription: Subscription{Topics: []protocol.RuntimeTopic{protocol.TopicFilesChanged}, Watches: []Watch{{ID: "active", Workspace: "/workspace"}, {ID: "active", Workspace: "/other"}}}, want: "repeats"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.subscription.Validate()
			if test.want == "" && err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("Validate = %v, want %q", err, test.want)
			}
		})
	}
}

func TestEventDistinguishesInvalidationFromResync(t *testing.T) {
	t.Parallel()
	changed := Event{Type: protocol.RuntimeFilesChanged, Sequence: 1, WatchID: "active", Workspace: "/workspace", Paths: []string{"main.go"}}
	if err := changed.Validate(); err != nil {
		t.Fatal(err)
	}
	resync := Event{Type: protocol.RuntimeResync, Sequence: 2, Topics: []protocol.RuntimeTopic{protocol.TopicFilesChanged}}
	if err := resync.Validate(); err != nil {
		t.Fatal(err)
	}
	resync.Topics = nil
	if err := resync.Validate(); err == nil {
		t.Fatal("scope-free resync was accepted")
	}
}

func TestEventAcceptsBroadFileInvalidations(t *testing.T) {
	t.Parallel()
	event := Event{
		Type: protocol.RuntimeFilesChanged, Sequence: 1,
		Workspace: "/workspace", Paths: []string{"main.go"},
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("broad file invalidation: %v", err)
	}
	event.Paths = nil
	if err := event.Validate(); err == nil {
		t.Fatal("pathless file invalidation was accepted")
	}
}

func TestSubscriptionRejectsEventsOutsideItsDeclaredScope(t *testing.T) {
	t.Parallel()
	subscription := Subscription{
		Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged, protocol.TopicSessionsChanged},
		Watches: []Watch{{ID: "active", Workspace: "/workspace"}},
	}
	tests := []struct {
		name  string
		event Event
		want  string
	}{
		{
			name: "broad file event", event: Event{
				Type: protocol.RuntimeFilesChanged, Sequence: 1,
				Workspace: "/workspace", Paths: []string{"main.go"},
			},
		},
		{
			name: "owned watch", event: Event{
				Type: protocol.RuntimeFilesChanged, Sequence: 1, WatchID: "active",
				Workspace: "/workspace", Paths: []string{"main.go"},
			},
		},
		{
			name: "foreign topic", want: "outside the subscription",
			event: Event{Type: protocol.RuntimeRunsChanged, Sequence: 1},
		},
		{
			name: "foreign watch", want: "outside the subscription",
			event: Event{
				Type: protocol.RuntimeFilesChanged, Sequence: 1, WatchID: "foreign",
				Workspace: "/workspace", Paths: []string{"main.go"},
			},
		},
		{
			name: "watch workspace mismatch", want: "another workspace",
			event: Event{
				Type: protocol.RuntimeFilesChanged, Sequence: 1, WatchID: "active",
				Workspace: "/other", Paths: []string{"main.go"},
			},
		},
		{
			name: "foreign resync topic", want: "outside the subscription",
			event: Event{Type: protocol.RuntimeResync, Sequence: 1, Topics: []protocol.RuntimeTopic{protocol.TopicRunsChanged}},
		},
		{
			name: "foreign resync watch", want: "outside the subscription",
			event: Event{
				Type: protocol.RuntimeResync, Sequence: 1, Topics: []protocol.RuntimeTopic{protocol.TopicFilesChanged}, WatchIDs: []string{"foreign"},
			},
		},
		{
			name: "watch without file resync", want: "files.changed",
			event: Event{
				Type: protocol.RuntimeResync, Sequence: 1, Topics: []protocol.RuntimeTopic{protocol.TopicSessionsChanged}, WatchIDs: []string{"active"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := subscription.ValidateEvent(test.event)
			if test.want == "" && err != nil {
				t.Fatalf("ValidateEvent: %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("ValidateEvent = %v, want %q", err, test.want)
			}
		})
	}
}

func TestPlanChangeIsAFirstClassInvalidation(t *testing.T) {
	t.Parallel()
	event := Event{Type: protocol.RuntimePlanChanged, Sequence: 1}
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
}

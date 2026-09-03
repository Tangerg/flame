package runtimebinding

import (
	"context"
	"errors"
	"iter"
	"slices"
	"testing"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

type changeBindingStub struct {
	request protocol.RuntimeSubscribeRequest
	events  iter.Seq2[protocol.RuntimeEvent, error]
	called  bool
}

func (c *changeBindingStub) SubscribeRuntime(_ context.Context, request protocol.RuntimeSubscribeRequest, _ flameruntime.SubscriptionOptions) (*protocol.RuntimeSubscribeResponse, iter.Seq2[protocol.RuntimeEvent, error], error) {
	c.called, c.request = true, request
	return &protocol.RuntimeSubscribeResponse{}, c.events, nil
}

func TestChangefeedAdapterNegotiatesAndProjectsRuntimeEvents(t *testing.T) {
	t.Parallel()
	workspaceRef := protocol.WorkspaceRef{Path: "/workspace"}
	stub := &changeBindingStub{events: func(yield func(protocol.RuntimeEvent, error) bool) {
		yield(protocol.RuntimeEvent{
			Type: protocol.RuntimeFilesChanged, Sequence: 1, WatchID: "active",
			Workspace: &workspaceRef, Paths: []string{"main.go"},
		}, nil)
	}}
	runtime := &Connection{
		changes: stub, meta: requestMeta("test"),
		profile: changefeedProfile(protocol.TopicFilesChanged),
	}
	runtime.profile.Features = map[string]Feature{
		protocol.FeatureFileWatch: {Enabled: true},
	}
	stream, err := runtime.Subscribe(t.Context(), changefeed.Subscription{
		Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged},
		Watches: []changefeed.Watch{{ID: "active", Workspace: "/workspace"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var events []changefeed.Event
	for event, err := range stream {
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	if len(events) != 1 || events[0].Workspace != "/workspace" || events[0].Paths[0] != "main.go" {
		t.Fatalf("events = %+v", events)
	}
	if !stub.called || len(stub.request.Watches) != 1 || stub.request.Watches[0].WatchID != "active" {
		t.Fatalf("request = %+v", stub.request)
	}
}

func TestChangefeedAdapterProjectsBroadFileInvalidations(t *testing.T) {
	t.Parallel()
	workspaceRef := protocol.WorkspaceRef{Path: "/workspace"}
	stub := &changeBindingStub{events: func(yield func(protocol.RuntimeEvent, error) bool) {
		yield(protocol.RuntimeEvent{
			Type: protocol.RuntimeFilesChanged, Sequence: 1,
			Workspace: &workspaceRef, Paths: []string{"main.go"},
		}, nil)
	}}
	runtime := &Connection{
		changes: stub, meta: requestMeta("test"),
		profile: changefeedProfile(protocol.TopicFilesChanged),
	}
	stream, err := runtime.Subscribe(t.Context(), changefeed.Subscription{
		Topics: []protocol.RuntimeTopic{protocol.TopicFilesChanged},
	})
	if err != nil {
		t.Fatal(err)
	}
	for event, eventErr := range stream {
		if eventErr != nil {
			t.Fatal(eventErr)
		}
		if event.WatchID != "" || event.Workspace != "/workspace" || !slices.Equal(event.Paths, []string{"main.go"}) {
			t.Fatalf("broad file event = %+v", event)
		}
		return
	}
	t.Fatal("broad file stream yielded no event")
}

func TestChangefeedAdapterRejectsEventsOutsideTheSubscription(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		subscription changefeed.Subscription
		event        protocol.RuntimeEvent
	}{
		{
			name:         "topic",
			subscription: changefeed.Subscription{Topics: []protocol.RuntimeTopic{protocol.TopicSessionsChanged}},
			event:        protocol.RuntimeEvent{Type: protocol.RuntimeRunsChanged, Sequence: 1},
		},
		{
			name: "watch",
			subscription: changefeed.Subscription{
				Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged},
				Watches: []changefeed.Watch{{ID: "active", Workspace: "/workspace"}},
			},
			event: protocol.RuntimeEvent{
				Type: protocol.RuntimeFilesChanged, Sequence: 1,
				WatchID: "foreign", Paths: []string{"main.go"},
			},
		},
		{
			name:         "resync",
			subscription: changefeed.Subscription{Topics: []protocol.RuntimeTopic{protocol.TopicSessionsChanged}},
			event: protocol.RuntimeEvent{
				Type: protocol.RuntimeResync, Sequence: 1,
				Topics: []protocol.RuntimeTopic{protocol.TopicRunsChanged},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stub := &changeBindingStub{events: func(yield func(protocol.RuntimeEvent, error) bool) {
				yield(test.event, nil)
			}}
			runtime := &Connection{
				changes: stub, meta: requestMeta("test"),
				profile: changefeedProfile(protocol.TopicFilesChanged, protocol.TopicSessionsChanged, protocol.TopicRunsChanged),
			}
			if len(test.subscription.Watches) != 0 {
				runtime.profile.Features = map[string]Feature{
					protocol.FeatureFileWatch: {Enabled: true},
				}
			}
			stream, err := runtime.Subscribe(t.Context(), test.subscription)
			if err != nil {
				t.Fatal(err)
			}
			for _, eventErr := range stream {
				requireRuntimeContractViolation(t, eventErr)
				return
			}
			t.Fatal("out-of-scope stream yielded no error")
		})
	}
}

func TestChangefeedAdapterRefusesAnUnadvertisedTopic(t *testing.T) {
	t.Parallel()
	stub := &changeBindingStub{}
	runtime := &Connection{changes: stub}
	_, err := runtime.Subscribe(t.Context(), changefeed.Subscription{Topics: []protocol.RuntimeTopic{protocol.TopicFilesChanged}})
	if err == nil {
		t.Fatal("unadvertised topic was accepted")
	}
	if stub.called {
		t.Fatal("runtime binding was called before capability validation")
	}
}

func TestChangefeedAdapterRejectsWatchesWithoutFileWatchCapability(t *testing.T) {
	t.Parallel()
	stub := &changeBindingStub{}
	runtime := &Connection{changes: stub, profile: changefeedProfile(protocol.TopicFilesChanged)}
	_, err := runtime.Subscribe(t.Context(), changefeed.Subscription{
		Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged},
		Watches: []changefeed.Watch{{ID: "active", Workspace: "/workspace"}},
	})
	if err == nil || !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("Subscribe error = %v, want ErrIncompatibleRuntime", err)
	}
	if stub.called {
		t.Fatal("workspace watch reached the binding without fileWatch capability")
	}
}

func TestChangefeedAdapterHonorsAdvertisedSubscriptionLimits(t *testing.T) {
	t.Parallel()
	stub := &changeBindingStub{}
	runtime := &Connection{
		changes: stub, profile: changefeedProfile(protocol.TopicFilesChanged),
	}
	runtime.profile.Limits.RuntimeSubscription.MaxWatches = 1
	_, err := runtime.Subscribe(t.Context(), changefeed.Subscription{
		Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged},
		Watches: []changefeed.Watch{{ID: "one", Workspace: "/one"}, {ID: "two", Workspace: "/two"}},
	})
	if err == nil {
		t.Fatal("subscription above the advertised watch limit was accepted")
	}
	if stub.called {
		t.Fatal("binding was called before subscription limit validation")
	}
}

func TestChangefeedAdapterRejectsMalformedWireEvent(t *testing.T) {
	t.Parallel()
	stub := &changeBindingStub{events: func(yield func(protocol.RuntimeEvent, error) bool) {
		yield(protocol.RuntimeEvent{Type: protocol.RuntimeFilesChanged}, nil)
	}}
	runtime := &Connection{
		changes: stub, profile: changefeedProfile(protocol.TopicFilesChanged),
	}
	stream, err := runtime.Subscribe(t.Context(), changefeed.Subscription{Topics: []protocol.RuntimeTopic{protocol.TopicFilesChanged}})
	if err != nil {
		t.Fatal(err)
	}
	for _, eventErr := range stream {
		if eventErr == nil {
			t.Fatal("malformed wire event was accepted")
		}
		requireRuntimeContractViolation(t, eventErr)
		return
	}
	t.Fatal("malformed stream yielded no error")
}

func TestChangefeedAdapterProjectsRuntimeResourceInvalidations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		topic protocol.RuntimeTopic
		event protocol.RuntimeEventType
	}{
		{name: "models", topic: protocol.TopicModelsChanged, event: protocol.RuntimeModelsChanged},
		{name: "approvals", topic: protocol.TopicApprovalsChanged, event: protocol.RuntimeApprovalsChanged},
		{name: "agent memory", topic: protocol.TopicAgentMemoryChanged, event: protocol.RuntimeAgentMemoryChanged},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stub := &changeBindingStub{events: func(yield func(protocol.RuntimeEvent, error) bool) {
				yield(protocol.RuntimeEvent{Type: test.event, Sequence: 1}, nil)
			}}
			runtime := &Connection{changes: stub, profile: changefeedProfile(test.topic)}
			stream, err := runtime.Subscribe(t.Context(), changefeed.Subscription{Topics: []protocol.RuntimeTopic{test.topic}})
			if err != nil {
				t.Fatal(err)
			}
			for event, eventErr := range stream {
				if eventErr != nil {
					t.Fatal(eventErr)
				}
				if event.Type != protocol.RuntimeEventType(test.topic) || event.Sequence != 1 {
					t.Fatalf("projected event = %+v, want %s sequence 1", event, test.topic)
				}
				return
			}
			t.Fatal("runtime resource stream yielded no event")
		})
	}
}

func TestChangefeedAdapterRejectsAnIncompleteRuntimeStream(t *testing.T) {
	t.Parallel()
	runtime := &Connection{
		changes: &changeBindingStub{}, profile: changefeedProfile(protocol.TopicFilesChanged),
	}
	_, err := runtime.Subscribe(t.Context(), changefeed.Subscription{Topics: []protocol.RuntimeTopic{protocol.TopicFilesChanged}})
	requireRuntimeContractViolation(t, err)
}

func TestChangefeedAdapterProjectsPlanInvalidation(t *testing.T) {
	t.Parallel()
	stub := &changeBindingStub{events: func(yield func(protocol.RuntimeEvent, error) bool) {
		yield(protocol.RuntimeEvent{
			Type: protocol.RuntimePlanChanged, Sequence: 1,
			SessionIDs: []string{"ses_1"},
		}, nil)
	}}
	runtime := &Connection{
		changes: stub, profile: changefeedProfile(protocol.TopicPlanChanged),
	}
	stream, err := runtime.Subscribe(t.Context(), changefeed.Subscription{Topics: []protocol.RuntimeTopic{protocol.TopicPlanChanged}})
	if err != nil {
		t.Fatal(err)
	}
	for event, eventErr := range stream {
		if eventErr != nil {
			t.Fatal(eventErr)
		}
		if len(event.SessionIDs) != 1 || event.SessionIDs[0] != "ses_1" {
			t.Fatalf("plan invalidation sessions = %v", event.SessionIDs)
		}
		return
	}
	t.Fatal("plan change stream yielded no event")
}

func changefeedProfile(topics ...protocol.RuntimeTopic) Profile {
	profile := Profile{
		RuntimeTopics: slices.Clone(topics),
		Limits:        Limits{RuntimeSubscription: protocol.SubscriptionLimits{MaxTopics: 32, MaxWatches: 32}},
	}
	return profile
}

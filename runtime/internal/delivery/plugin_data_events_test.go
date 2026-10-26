package delivery

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
)

func TestPluginDataInvalidationProjectsToARuntimeSignal(t *testing.T) {
	notifier := &testNotification[invalidation.Notice]{}
	s := &Handler{workspaceHub: newWorkspaceHub()}
	s.observeInvalidations(notifier.Observe)
	events, unsubscribe := s.workspaceHub.subscribe()
	defer unsubscribe()

	notifier.Publish(invalidation.Notice{Resource: invalidation.PluginData})
	got := <-events
	if got.Type != protocol.RuntimePluginDataChanged {
		t.Fatalf("runtime event = %+v", got)
	}
}

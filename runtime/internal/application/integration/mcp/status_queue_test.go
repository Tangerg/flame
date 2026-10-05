package mcp

import (
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"slices"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

func TestStatusQueuePublishesPreparedOrderAndRemainsReusable(t *testing.T) {
	var published []string
	queue := newStatusQueue(func(name mcpserver.ID) {
		published = append(published, name.String())
	})
	first := queue.prepare(testsupport.UserMCPServer("first"))
	second := queue.prepare(testsupport.UserMCPServer("second"))
	third := queue.prepare(testsupport.UserMCPServer("third"))

	queue.publish(second)
	queue.publish(third)
	if len(published) != 0 {
		t.Fatalf("later ready statuses bypassed the head: %v", published)
	}
	queue.publish(first)
	if !slices.Equal(published, []string{"first", "second", "third"}) {
		t.Fatalf("published status order = %v", published)
	}
	queue.publish(queue.prepare(testsupport.UserMCPServer("fourth")))
	if !slices.Equal(published, []string{"first", "second", "third", "fourth"}) {
		t.Fatalf("reused queue publication = %v, want each notification once in order", published)
	}
}

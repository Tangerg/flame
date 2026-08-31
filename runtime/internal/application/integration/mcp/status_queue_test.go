package mcp

import (
	"slices"
	"testing"
)

func TestStatusQueuePublishesPreparedOrderWithoutNumericSequence(t *testing.T) {
	var published []string
	queue := newStatusQueue(func(status ServerStatus) {
		published = append(published, status.Name.String())
	})
	first := queue.prepare(ServerStatus{Name: testMCPServerName("first")})
	second := queue.prepare(ServerStatus{Name: testMCPServerName("second")})
	third := queue.prepare(ServerStatus{Name: testMCPServerName("third")})

	queue.publish(second)
	queue.publish(third)
	if len(published) != 0 {
		t.Fatalf("later ready statuses bypassed the head: %v", published)
	}
	queue.publish(first)
	if !slices.Equal(published, []string{"first", "second", "third"}) {
		t.Fatalf("published status order = %v", published)
	}
	if queue.head != nil || queue.tail != nil || queue.draining {
		t.Fatalf("drained queue retained lifecycle state: %+v", queue)
	}
}

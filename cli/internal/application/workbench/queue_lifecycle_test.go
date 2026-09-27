package workbench

import "testing"

func TestQueueRequiresAnOpenWorkbench(t *testing.T) {
	if _, err := NewQueue(nil); err == nil {
		t.Fatal("queue was constructed without a workbench")
	}
	store, err := OpenMemory(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewQueue(store); err == nil {
		t.Fatal("queue was constructed with a closed workbench")
	}
}

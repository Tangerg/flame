package workbench

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

func TestCapacityRequiresAnExplicitPositiveBound(t *testing.T) {
	for _, value := range []int{-1, 0} {
		if _, err := NewCapacity(value); err == nil {
			t.Fatalf("NewCapacity(%d) unexpectedly succeeded", value)
		}
	}
	capacity, err := NewCapacity(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := capacity.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreRejectsPresentInvalidCapacities(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{name: "history", config: Config{HistoryCapacity: new(Capacity)}},
		{name: "stash", config: Config{StashCapacity: &Capacity{value: -1}}},
		{name: "workspace", config: Config{WorkspaceCapacity: new(Capacity)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := OpenMemory(test.config); err == nil {
				t.Fatal("present invalid capacity unexpectedly selected a default")
			}
		})
	}
}

func TestDirectoryPersistenceRequiresAnAbsoluteOwnedRoot(t *testing.T) {
	for _, directory := range []string{"", "   ", filepath.Join("relative", "state")} {
		if _, err := OpenDirectory(directory, Config{}); err == nil {
			t.Fatalf("OpenDirectory(%q) unexpectedly succeeded", directory)
		}
	}
	if _, err := newStore(persistence{}, Config{}); err == nil {
		t.Fatal("zero persistence unexpectedly constructed a Store")
	}
}

func TestMemoryPersistenceNeverCreatesWorkbenchFiles(t *testing.T) {
	workingDirectory := t.TempDir()
	previousDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousDirectory) })

	store, err := OpenMemory(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Remember(agent.Message{Text: "process-local prompt"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDraft("session", agent.Message{Text: "process-local draft"}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(workingDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("memory Store wrote files: %+v", entries)
	}
}

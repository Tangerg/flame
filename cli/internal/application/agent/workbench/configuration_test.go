package workbench

import (
	"path/filepath"
	"testing"
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
			if _, err := Open(new(closeTrackingPersistence), test.config); err == nil {
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
	if _, err := Open(nil, Config{}); err == nil {
		t.Fatal("zero persistence unexpectedly constructed a Store")
	}
	var typedNil *removeFailurePersistence
	if _, err := Open(typedNil, Config{}); err == nil {
		t.Fatal("typed-nil persistence unexpectedly constructed a Store")
	}
}

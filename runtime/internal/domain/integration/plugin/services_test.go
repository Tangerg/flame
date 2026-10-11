package plugin

import (
	"errors"
	"testing"
)

func TestManagedServerHasOneClosedServiceKind(t *testing.T) {
	declaration := Declaration{
		Name: "managed",
		Servers: []Server{
			{Name: serverName("memory"), Transport: stdio, Command: "memory"},
			{Name: serverName("schedules"), Transport: stdio, Command: "schedules"},
		},
		Services: []Service{
			{Kind: MemoryService, Server: serverName("memory")},
			{Kind: SchedulesService, Server: serverName("memory")},
		},
	}
	if _, err := NewRelease(testDigest("managed"), declaration); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ambiguous backend service admission = %v", err)
	}
	declaration.Services[1].Server = serverName("schedules")
	release, err := NewRelease(testDigest("managed"), declaration)
	if err != nil {
		t.Fatal(err)
	}
	for _, declared := range declaration.Services {
		if admitted, found := release.Service(declared.Kind); !found || admitted != declared {
			t.Fatalf("service admission lost its backend: %+v", declared)
		}
	}
}

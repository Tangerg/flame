package commandreplay

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCapabilityOwnsStoreIdentityRetentionAndDeadline(t *testing.T) {
	t.Parallel()

	capability, err := NewCapability(" runtime-a ", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if capability.Namespace() != "runtime-a" || capability.Retention() != 10*time.Minute {
		t.Fatalf("capability = %+v", capability)
	}
	stagedAt := time.Date(2026, 8, 29, 1, 2, 3, 0, time.FixedZone("test", 8*60*60))
	deadline, err := capability.Deadline(stagedAt)
	if err != nil {
		t.Fatal(err)
	}
	if want := stagedAt.UTC().Add(10 * time.Minute); !deadline.Equal(want) || deadline.Location() != time.UTC {
		t.Fatalf("deadline = %s, want %s UTC", deadline, want)
	}
}

func TestCapabilityRejectsPartialOrNonWireRepresentableFacts(t *testing.T) {
	t.Parallel()

	for _, input := range []struct {
		namespace string
		retention time.Duration
	}{
		{},
		{namespace: "runtime-a"},
		{retention: time.Minute},
		{namespace: "runtime-a", retention: -time.Second},
		{namespace: "runtime-a", retention: time.Millisecond},
	} {
		if _, err := NewCapability(input.namespace, input.retention); err == nil {
			t.Fatalf("NewCapability(%q, %s) succeeded", input.namespace, input.retention)
		}
	}
	if err := (Capability{}).Validate(); err == nil {
		t.Fatal("zero Capability was valid")
	}
}

func TestCapabilityJSONIsStrictAndRoundTrips(t *testing.T) {
	t.Parallel()

	want, err := NewCapability("runtime-a", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got Capability
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
	for _, invalid := range []string{
		`{}`,
		`{"namespace":"runtime-a","retentionSeconds":0}`,
		`{"namespace":"","retentionSeconds":1}`,
		`{"namespace":"runtime-a","retentionSeconds":1,"extra":true}`,
		`{"namespace":"runtime-a","retentionSeconds":1} {}`,
	} {
		var decoded Capability
		if err := json.Unmarshal([]byte(invalid), &decoded); err == nil {
			t.Fatalf("decoded invalid capability %s", invalid)
		}
	}
}

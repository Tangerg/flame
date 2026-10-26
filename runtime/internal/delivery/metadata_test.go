package delivery

import (
	"context"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestRequestMetaContextOwnsSnapshot(t *testing.T) {
	info := &protocol.ClientInfo{Name: "before"}
	capabilities := &protocol.ClientCapabilities{
		InterruptTypes:          []protocol.InterruptType{"approval"},
		ExcludedEphemeralEvents: []protocol.SuppressibleRunEventType{protocol.SuppressibleRunItemDelta},
		Features: map[string]protocol.FeaturePreference{
			"nested": {Enabled: true},
		},
	}
	ctx := WithRequestMeta(context.Background(), protocol.RequestMeta{
		ClientInfo: info, ClientCapabilities: capabilities,
	})
	info.Name = "after"
	capabilities.InterruptTypes[0] = "after"
	capabilities.ExcludedEphemeralEvents[0] = protocol.SuppressibleRunSegmentProgress
	capabilities.Features["nested"] = protocol.FeaturePreference{Enabled: false}

	first, ok := RequestMetaFrom(ctx)
	if !ok || first.ClientInfo.Name != "before" || first.ClientCapabilities.InterruptTypes[0] != "approval" {
		t.Fatalf("stored metadata retained caller state: %+v", first)
	}
	if first.ClientCapabilities.ExcludedEphemeralEvents[0] != protocol.SuppressibleRunItemDelta {
		t.Fatalf("exclusion list retained caller state: %+v", first.ClientCapabilities)
	}
	if !first.ClientCapabilities.Features["nested"].Enabled {
		t.Fatalf("nested feature retained caller state: %+v", first.ClientCapabilities.Features)
	}
	first.ClientCapabilities.InterruptTypes[0] = "after"
	second, _ := RequestMetaFrom(ctx)
	if second.ClientCapabilities.InterruptTypes[0] != "approval" {
		t.Fatal("RequestMetaFrom exposed context-owned backing storage")
	}
}

func TestEmptyOperationCursorReplacesParentCursor(t *testing.T) {
	parent := withAfterEventID(t.Context(), "parent-event")
	operation := withAfterEventID(parent, "")
	if got := AfterEventIDFrom(operation); got != "" {
		t.Fatalf("new operation inherited cursor %q", got)
	}
	if got := AfterEventIDFrom(parent); got != "parent-event" {
		t.Fatalf("binding changed parent cursor %q", got)
	}
}

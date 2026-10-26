package bridge

import "testing"

func TestNotificationTargetIsOptionalOnlyForSending(t *testing.T) {
	if err := ValidateNotification(DesktopNotification{ID: "run:1", Title: "Done"}); err != nil {
		t.Fatalf("a notification without a navigation target is valid: %v", err)
	}
	for _, value := range []DesktopNotification{{ID: "run:1"}, {Title: "Done"}} {
		if err := ValidateNotification(value); err == nil {
			t.Fatal("missing notification identity or title was accepted")
		}
	}
	if _, err := NewNotificationTarget(""); err == nil {
		t.Fatal("an opened event accepted an absent target")
	}
	if target, err := NewNotificationTarget("session-1"); err != nil || target != "session-1" {
		t.Fatalf("opaque target changed: %q: %v", target, err)
	}
}

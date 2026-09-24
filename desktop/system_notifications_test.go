package main

import (
	"context"
	"errors"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type fakeNotifications struct {
	startErr error
	allowed  bool
	answer   bool
	sent     []DesktopNotification
}

func (f *fakeNotifications) start(context.Context) error         { return f.startErr }
func (f *fakeNotifications) authorized() (bool, error)           { return f.allowed, nil }
func (f *fakeNotifications) requestAuthorization() (bool, error) { return f.answer, nil }
func (f *fakeNotifications) send(n DesktopNotification) error {
	f.sent = append(f.sent, n)
	return nil
}

func startedHost(t *testing.T, centre *fakeNotifications) *DesktopHost {
	t.Helper()
	host := mustDesktopHost(t, t.TempDir())
	host.useNotifications(centre)
	if err := host.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup() = %v", err)
	}
	return host
}

func TestNotificationsAreUnsupportedWhenThePlatformCentreCannotStart(t *testing.T) {
	host := startedHost(t, &fakeNotifications{startErr: errors.New("no bundle identifier")})

	if got, _ := host.NotificationAuthorization(); got != NotificationsUnsupported {
		t.Fatalf("NotificationAuthorization() = %q, want unsupported", got)
	}
	if got, _ := host.RequestNotificationAuthorization(); got != NotificationsUnsupported {
		t.Fatalf("RequestNotificationAuthorization() = %q, want unsupported", got)
	}
	if err := host.SendNotification(DesktopNotification{ID: "run:1", Title: "Done"}); err == nil {
		t.Fatal("SendNotification succeeded without a notification centre")
	}
}

func TestNotificationAuthorizationDistinguishesUndecidedFromRefused(t *testing.T) {
	centre := &fakeNotifications{}
	host := startedHost(t, centre)

	if got, _ := host.NotificationAuthorization(); got != NotificationsUndecided {
		t.Fatalf("before a request = %q, want default", got)
	}
	if got, _ := host.RequestNotificationAuthorization(); got != NotificationsDenied {
		t.Fatalf("a refused request = %q, want denied", got)
	}
	centre.allowed, centre.answer = true, true
	if got, _ := host.NotificationAuthorization(); got != NotificationsGranted {
		t.Fatalf("an allowed app = %q, want granted", got)
	}
}

func TestSendNotificationHandsTheCentreACompleteMessage(t *testing.T) {
	centre := &fakeNotifications{allowed: true}
	host := startedHost(t, centre)

	if err := host.SendNotification(DesktopNotification{Title: "Done"}); err == nil {
		t.Fatal("a notification without an id reached the centre")
	}
	message := DesktopNotification{ID: "run:1", Title: "Done", Body: "Finished", Target: "session-1"}
	if err := host.SendNotification(message); err != nil {
		t.Fatalf("SendNotification() = %v", err)
	}
	if len(centre.sent) != 1 || centre.sent[0] != message {
		t.Fatalf("sent %#v, want exactly %#v", centre.sent, message)
	}
}

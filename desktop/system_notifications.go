package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// notificationOpenedEvent carries the Target of a notification the user clicked back
// to the frontend, which owns what opening it means.
const notificationOpenedEvent = "desktop:notification-opened"

// NotificationAuthorization is what the platform lets the app do, as the frontend
// names it. The platform answers only "allowed" or "not allowed", so before a request
// "not allowed" is reported as not yet decided, and after a refused request as denied.
type NotificationAuthorization string

const (
	NotificationsUnsupported NotificationAuthorization = "unsupported"
	NotificationsUndecided   NotificationAuthorization = "default"
	NotificationsGranted     NotificationAuthorization = "granted"
	NotificationsDenied      NotificationAuthorization = "denied"
)

// DesktopNotification is one message for the platform notification centre. Target
// is opaque to the host: it comes back unchanged when the user clicks the message.
type DesktopNotification struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Body   string `json:"body,omitempty"`
	Target string `json:"target,omitempty"`
}

// systemNotifications is what DesktopHost needs of the platform notification centre.
type systemNotifications interface {
	start(ctx context.Context) error
	authorized() (bool, error)
	requestAuthorization() (bool, error)
	send(notification DesktopNotification) error
}

// wailsNotifications adapts Wails' native notification service. It is started by
// DesktopHost rather than registered with the application, so none of the service's
// own methods become IPC entry points.
type wailsNotifications struct {
	service *notifications.NotificationService
	opened  func(target string)
}

func (w wailsNotifications) start(ctx context.Context) error {
	w.service.OnNotificationResponse(func(result notifications.NotificationResult) {
		if result.Error != nil {
			log.Printf("desktop host: notification response: %v", result.Error)
			return
		}
		if result.Response.ActionIdentifier != notifications.DefaultActionIdentifier {
			return
		}
		if target, ok := result.Response.UserInfo["target"].(string); ok && target != "" {
			w.opened(target)
		}
	})
	return w.service.ServiceStartup(ctx, application.ServiceOptions{})
}

func (w wailsNotifications) authorized() (bool, error) {
	return w.service.CheckNotificationAuthorization()
}

func (w wailsNotifications) requestAuthorization() (bool, error) {
	return w.service.RequestNotificationAuthorization()
}

func (w wailsNotifications) send(notification DesktopNotification) error {
	return w.service.SendNotification(notifications.NotificationOptions{
		ID:    notification.ID,
		Title: notification.Title,
		Body:  notification.Body,
		Data:  map[string]any{"target": notification.Target},
	})
}

// useNotifications attaches the platform notification centre. Unexported on purpose:
// see the note on DesktopHost.
func (d *DesktopHost) useNotifications(centre systemNotifications) {
	d.notifications = centre
}

// ServiceStartup is the Wails lifecycle hook, which Wails never binds to IPC. It
// starts the notification centre; an unbundled build has none, and that is reported
// to the frontend as unsupported rather than failing the application.
func (d *DesktopHost) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	if d.notifications == nil {
		return nil
	}
	if err := d.notifications.start(ctx); err != nil {
		log.Printf("desktop host: system notifications unavailable: %v", err)
		return nil
	}
	d.notificationsReady.Store(true)
	return nil
}

// NotificationAuthorization reports whether the app may notify, without asking.
func (d *DesktopHost) NotificationAuthorization() (NotificationAuthorization, error) {
	if !d.notificationsReady.Load() {
		return NotificationsUnsupported, nil
	}
	allowed, err := d.notifications.authorized()
	if err != nil {
		return "", fmt.Errorf("desktop host: check notification authorization: %w", err)
	}
	if allowed {
		return NotificationsGranted, nil
	}
	return NotificationsUndecided, nil
}

// RequestNotificationAuthorization asks the platform once; a refusal the user made
// earlier comes back as denied without a prompt.
func (d *DesktopHost) RequestNotificationAuthorization() (NotificationAuthorization, error) {
	if !d.notificationsReady.Load() {
		return NotificationsUnsupported, nil
	}
	allowed, err := d.notifications.requestAuthorization()
	if err != nil {
		return "", fmt.Errorf("desktop host: request notification authorization: %w", err)
	}
	if allowed {
		return NotificationsGranted, nil
	}
	return NotificationsDenied, nil
}

// SendNotification delivers one message to the platform notification centre.
func (d *DesktopHost) SendNotification(notification DesktopNotification) error {
	if !d.notificationsReady.Load() {
		return errors.New("desktop host: system notifications are unavailable")
	}
	if notification.ID == "" || notification.Title == "" {
		return errors.New("desktop host: a notification needs an id and a title")
	}
	if err := d.notifications.send(notification); err != nil {
		return fmt.Errorf("desktop host: send notification: %w", err)
	}
	return nil
}

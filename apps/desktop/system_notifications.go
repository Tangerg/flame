package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/Tangerg/flame/apps/desktop/internal/bridge"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

type systemNotifications interface {
	start(ctx context.Context) error
	authorized() (bool, error)
	requestAuthorization() (bool, error)
	send(notification bridge.DesktopNotification) error
}

// wailsNotifications adapts Wails' native notification service. It is started by
// DesktopHost rather than registered with the application, so none of the service's
// own methods become IPC entry points.
type wailsNotifications struct {
	service *notifications.NotificationService
	opened  func(target bridge.NotificationTarget)
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
		if target, ok := result.Response.UserInfo["target"].(string); ok {
			if admitted, err := bridge.NewNotificationTarget(target); err == nil {
				w.opened(admitted)
			}
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

func (w wailsNotifications) send(notification bridge.DesktopNotification) error {
	return w.service.SendNotification(notifications.NotificationOptions{
		ID:    notification.ID,
		Title: notification.Title,
		Body:  notification.Body,
		Data:  map[string]any{"target": notification.Target},
	})
}

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

func (d *DesktopHost) NotificationAuthorization() (bridge.NotificationAuthorization, error) {
	if !d.notificationsReady.Load() {
		return bridge.NotificationsUnsupported, nil
	}
	allowed, err := d.notifications.authorized()
	if err != nil {
		return "", fmt.Errorf("desktop host: check notification authorization: %w", err)
	}
	if allowed {
		return bridge.NotificationsGranted, nil
	}
	return bridge.NotificationsUndecided, nil
}

// RequestNotificationAuthorization asks the platform once; a refusal the user made
// earlier comes back as denied without a prompt.
func (d *DesktopHost) RequestNotificationAuthorization() (bridge.NotificationAuthorization, error) {
	if !d.notificationsReady.Load() {
		return bridge.NotificationsUnsupported, nil
	}
	allowed, err := d.notifications.requestAuthorization()
	if err != nil {
		return "", fmt.Errorf("desktop host: request notification authorization: %w", err)
	}
	if allowed {
		return bridge.NotificationsGranted, nil
	}
	return bridge.NotificationsDenied, nil
}

func (d *DesktopHost) SendNotification(notification bridge.DesktopNotification) error {
	if !d.notificationsReady.Load() {
		return errors.New("desktop host: system notifications are unavailable")
	}
	if err := bridge.ValidateNotification(notification); err != nil {
		return fmt.Errorf("desktop host: notification: %w", err)
	}
	if err := d.notifications.send(notification); err != nil {
		return fmt.Errorf("desktop host: send notification: %w", err)
	}
	return nil
}

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

const notificationTargetKey = "target"

type systemNotifications interface {
	start(ctx context.Context) error
	authorization() (bridge.NotificationAuthorization, error)
	requestAuthorization() error
	send(notification bridge.DesktopNotification) error
}

// Only DesktopHost may expose IPC; registering the notification service directly
// would publish its native methods as additional entry points.
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
		if target, ok := result.Response.UserInfo[notificationTargetKey].(string); ok {
			if admitted, err := bridge.NewNotificationTarget(target); err == nil {
				w.opened(admitted)
			}
		}
	})
	return w.service.ServiceStartup(ctx, application.ServiceOptions{})
}

func (w wailsNotifications) requestAuthorization() error {
	_, err := w.service.RequestNotificationAuthorization()
	return err
}

func (w wailsNotifications) send(notification bridge.DesktopNotification) error {
	return w.service.SendNotification(notifications.NotificationOptions{
		ID:    notification.ID,
		Title: notification.Title,
		Body:  notification.Body,
		Data:  map[string]any{notificationTargetKey: string(notification.Target)},
	})
}

// Unbundled builds lack a notification centre; that absence must not prevent
// the rest of the Desktop from starting.
func (d *DesktopHost) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	if d.shell == nil {
		return errShellUnbound
	}
	if err := d.shell.notifications.start(ctx); err != nil {
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
	status, err := d.shell.notifications.authorization()
	if err != nil {
		return "", fmt.Errorf("desktop host: check notification authorization: %w", err)
	}
	return status, nil
}

func (d *DesktopHost) RequestNotificationAuthorization() (bridge.NotificationAuthorization, error) {
	status, err := d.NotificationAuthorization()
	if err != nil || status != bridge.NotificationsUndecided {
		return status, err
	}
	if err := d.shell.notifications.requestAuthorization(); err != nil {
		return "", fmt.Errorf("desktop host: request notification authorization: %w", err)
	}
	return d.NotificationAuthorization()
}

func (d *DesktopHost) SendNotification(notification bridge.DesktopNotification) error {
	if !d.notificationsReady.Load() {
		return errors.New("desktop host: system notifications are unavailable")
	}
	if err := bridge.ValidateNotification(notification); err != nil {
		return fmt.Errorf("desktop host: notification: %w", err)
	}
	if err := d.shell.notifications.send(notification); err != nil {
		return fmt.Errorf("desktop host: send notification: %w", err)
	}
	return nil
}

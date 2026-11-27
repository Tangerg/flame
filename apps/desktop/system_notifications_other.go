//go:build !darwin

package main

import "github.com/Tangerg/flame/apps/desktop/internal/bridge"

func (w wailsNotifications) authorization() (bridge.NotificationAuthorization, error) {
	allowed, err := w.service.CheckNotificationAuthorization()
	if err != nil {
		return "", err
	}
	if allowed {
		return bridge.NotificationsGranted, nil
	}
	return bridge.NotificationsUndecided, nil
}

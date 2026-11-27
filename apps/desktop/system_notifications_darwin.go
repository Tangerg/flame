package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework UserNotifications
#import <UserNotifications/UserNotifications.h>
#include <dispatch/dispatch.h>
#include <stdbool.h>

static bool readNotificationAuthorization(int64_t timeout, UNAuthorizationStatus *status) {
	@autoreleasepool {
		dispatch_semaphore_t done = dispatch_semaphore_create(0);
		__block UNAuthorizationStatus current;
		[[UNUserNotificationCenter currentNotificationCenter] getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *settings) {
			current = settings.authorizationStatus;
			dispatch_semaphore_signal(done);
		}];
		if (dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, timeout)) != 0) return false;
		*status = current;
		return true;
	}
}
*/
import "C"

import (
	"context"
	"fmt"
	"time"

	"github.com/Tangerg/flame/apps/desktop/internal/bridge"
)

const notificationSettingsReadTimeout = 15 * time.Second

func (wailsNotifications) authorization() (bridge.NotificationAuthorization, error) {
	// Wails reduces the OS setting to a boolean and loses explicit denials.
	var status C.UNAuthorizationStatus
	if !C.readNotificationAuthorization(C.int64_t(notificationSettingsReadTimeout), &status) {
		return "", context.DeadlineExceeded
	}
	switch status {
	case C.UNAuthorizationStatusNotDetermined:
		return bridge.NotificationsUndecided, nil
	case C.UNAuthorizationStatusDenied:
		return bridge.NotificationsDenied, nil
	case C.UNAuthorizationStatusAuthorized, C.UNAuthorizationStatusProvisional:
		return bridge.NotificationsGranted, nil
	default:
		return "", fmt.Errorf("unexpected notification authorization status %d", status)
	}
}

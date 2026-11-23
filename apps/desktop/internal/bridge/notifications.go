package bridge

//flame:enum
type NotificationAuthorization string

const (
	NotificationsUnsupported NotificationAuthorization = "unsupported"
	NotificationsUndecided   NotificationAuthorization = "default"
	NotificationsGranted     NotificationAuthorization = "granted"
	NotificationsDenied      NotificationAuthorization = "denied"
)

type DesktopNotification struct {
	ID     string             `json:"id"`
	Title  string             `json:"title"`
	Body   string             `json:"body,omitempty"`
	Target NotificationTarget `json:"target,omitempty"`
}

type NotificationTarget string

//flame:event NotificationTarget
const NotificationOpenedEvent = "desktop:notification-opened"

func NewNotificationTarget(value string) (NotificationTarget, error) {
	target := NotificationTarget(value)
	if err := validate(target); err != nil {
		return "", err
	}
	return target, nil
}

func ValidateNotification(value DesktopNotification) error { return validate(value) }

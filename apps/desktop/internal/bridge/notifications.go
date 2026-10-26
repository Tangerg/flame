package bridge

// NotificationAuthorization projects the platform's permission answer. Before a
// request a refusal is undecided; after the request it is denied.
//
//flame:enum
type NotificationAuthorization string

const (
	NotificationsUnsupported NotificationAuthorization = "unsupported"
	NotificationsUndecided   NotificationAuthorization = "default"
	NotificationsGranted     NotificationAuthorization = "granted"
	NotificationsDenied      NotificationAuthorization = "denied"
)

// Target is opaque to the host and may be absent when sending a notification.
type DesktopNotification struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Body   string `json:"body,omitempty"`
	Target string `json:"target,omitempty"`
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

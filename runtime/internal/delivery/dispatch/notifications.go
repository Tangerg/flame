package dispatch

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/go-sdk/jsonrpc"
)

const notificationNamespace = "notifications."

const (
	NotificationRunEvent     = notificationNamespace + "run.event"
	NotificationRuntimeEvent = notificationNamespace + "runtime.event"
	NotificationPluginStream = notificationNamespace + "plugin.stream"
)

func encodeRunEvent(event protocol.RunEvent) (jsonrpc.Message, error) {
	return protocol.NewRPCNotification(NotificationRunEvent, event)
}

func encodeRuntimeEvent(event protocol.RuntimeEvent) (jsonrpc.Message, error) {
	return protocol.NewRPCNotification(NotificationRuntimeEvent, protocol.RuntimeEventNotification{
		Event: event,
	})
}

func (r *Router) handleNotification(context.Context, *jsonrpc.Request) {}

func runEventToFrame(event protocol.RunEvent) (StreamFrame, bool) {
	notification, err := encodeRunEvent(event)
	if err != nil {
		return StreamFrame{}, false
	}
	sseID := ""
	if event.Event.Replayable() {
		sseID = event.EventID
	}
	return StreamFrame{Notification: notification, SSEID: sseID}, true
}

func runtimeEventToFrame(event protocol.RuntimeEvent) (StreamFrame, bool) {
	notification, err := encodeRuntimeEvent(event)
	if err != nil {
		return StreamFrame{}, false
	}
	return StreamFrame{Notification: notification}, true
}

func frameOperationEvent(event any) (StreamFrame, bool) {
	switch typed := event.(type) {
	case protocol.RunEvent:
		return runEventToFrame(typed)
	case protocol.RuntimeEvent:
		return runtimeEventToFrame(typed)
	case protocol.PluginStreamEvent:
		notification, err := protocol.NewRPCNotification(NotificationPluginStream, typed)
		return StreamFrame{Notification: notification}, err == nil
	default:
		return StreamFrame{}, false
	}
}

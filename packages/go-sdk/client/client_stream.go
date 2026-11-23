package client

import (
	"errors"
	"io"
	"iter"
	"reflect"
	"sync/atomic"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/go-sdk/jsonrpc"
	"github.com/Tangerg/sse"
)

func remoteEvents(call *remoteCall, reader *sse.Reader, eventType reflect.Type, operation operation, acknowledgement any) iter.Seq2[any, error] {
	var consumed atomic.Bool
	rootID, segmentID := streamIdentity(acknowledgement)
	return func(yield func(any, error) bool) {
		fail := func(err error) {
			if eventType == reflect.TypeFor[protocol.PluginStreamEvent]() && operation == commandOperation {
				err = errors.Join(ErrAcknowledgementUnknown, err)
			}
			yield(nil, err)
		}
		if !consumed.CompareAndSwap(false, true) {
			yield(nil, ErrClosed)
			return
		}
		defer call.finish(nil)
		for frame, err := range reader.Messages() {
			if err != nil {
				fail(call.readError(err))
				return
			}
			event, err := decodeRemoteEvent(frame, eventType)
			if err != nil {
				fail(err)
				return
			}
			terminal := false
			if run, ok := event.(protocol.RunEvent); ok && run.RunID == rootID {
				if run.SegmentID != segmentID {
					yield(nil, invalidRemote("event belongs to another root segment"))
					return
				}
				terminal = run.Event.Type == protocol.StreamSegmentFinished
			}
			if stream, ok := event.(protocol.PluginStreamEvent); ok {
				terminal = stream.Kind == protocol.PluginStreamResult || stream.Kind == protocol.PluginStreamFailure
			}
			if !yield(event, nil) || terminal {
				return
			}
		}
		// HTTP has no stream-error frame. EOF without the root boundary is a
		// detached observation, never evidence that durable execution finished.
		fail(call.readError(io.ErrUnexpectedEOF))
	}
}

const defaultSSEEvent = "message"

func decodeRemoteEvent(frame sse.Message, eventType reflect.Type) (any, error) {
	if frame.Event != defaultSSEEvent {
		return nil, invalidRemote("unexpected sse event type")
	}
	message, err := protocol.DecodeRPCMessage(frame.Data)
	if err != nil {
		return nil, invalidRemote("invalid event envelope")
	}
	notification, ok := message.(*jsonrpc.Request)
	if !ok || notification.IsCall() {
		return nil, invalidRemote("stream frame is not a notification")
	}
	switch eventType {
	case reflect.TypeFor[protocol.PluginStreamEvent]():
		if notification.Method != notificationPluginStream || frame.ID != "" {
			return nil, invalidRemote("unexpected plugin stream notification")
		}
		return decodeRemoteValue(notification.Params, eventType)
	case reflect.TypeFor[protocol.RunEvent]():
		return decodeRemoteRunEvent(frame, notification)
	case reflect.TypeFor[protocol.RuntimeEvent]():
		return decodeRemoteRuntimeEvent(frame, notification)
	default:
		return nil, invalidRemote("unknown operation event type")
	}
}

func decodeRemoteRunEvent(frame sse.Message, notification *jsonrpc.Request) (any, error) {
	if notification.Method != notificationRunEvent {
		return nil, invalidRemote("unexpected run notification")
	}
	event, err := decodeRemote[protocol.RunEvent](notification.Params)
	if err != nil {
		return nil, err
	}
	// SSE retains the previous id on ephemeral frames. Only a replayable
	// event publishes a cursor; its two wire projections must agree.
	if event.Event.Replayable() && frame.ID != event.EventID {
		return nil, invalidRemote("event cursor does not match its sse id")
	}
	return event, nil
}

func decodeRemoteRuntimeEvent(frame sse.Message, notification *jsonrpc.Request) (any, error) {
	if notification.Method != notificationRuntimeEvent || frame.ID != "" {
		return nil, invalidRemote("unexpected runtime notification")
	}
	decoded, err := decodeRemote[protocol.RuntimeEventNotification](notification.Params)
	if err != nil {
		return nil, err
	}
	return decoded.Event, nil
}

func streamIdentity(acknowledgement any) (string, string) {
	switch ack := acknowledgement.(type) {
	case *protocol.StartRunResponse:
		return ack.RunID, ack.SegmentID
	case *protocol.ResumeRunResponse:
		return ack.RunID, ack.SegmentID
	case *protocol.SubscribeRunResponse:
		return ack.RunID, ack.SegmentID
	default:
		return "", ""
	}
}

package runtime

import (
	"io"
	"iter"
	"reflect"
	"sync/atomic"

	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/flame/runtime/internal/delivery/transport"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/sse"
)

func remoteEvents(call *remoteCall, reader *sse.Reader, eventType reflect.Type, acknowledgement any) iter.Seq2[any, error] {
	var consumed atomic.Bool
	rootID, segmentID := streamIdentity(acknowledgement)
	return func(yield func(any, error) bool) {
		if !consumed.CompareAndSwap(false, true) {
			yield(nil, ErrClosed)
			return
		}
		defer call.finish(nil)
		for frame, err := range reader.Messages() {
			if err != nil {
				yield(nil, call.readError(err))
				return
			}
			event, err := decodeRemoteEvent(frame, eventType)
			if err != nil {
				yield(nil, err)
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
			if !yield(event, nil) || terminal {
				return
			}
		}
		// HTTP has no stream-error frame. EOF without the root boundary is a
		// detached observation, never evidence that durable execution finished.
		yield(nil, call.readError(io.ErrUnexpectedEOF))
	}
}

func decodeRemoteEvent(frame sse.Message, eventType reflect.Type) (any, error) {
	if frame.Event != "message" {
		return nil, invalidRemote("unexpected sse event type")
	}
	message, err := transport.DecodeMessage(frame.Data)
	if err != nil {
		return nil, invalidRemote("invalid event envelope")
	}
	notification, ok := message.(*transport.Request)
	if !ok || notification.IsCall() {
		return nil, invalidRemote("stream frame is not a notification")
	}
	switch eventType {
	case reflect.TypeFor[protocol.RunEvent]():
		if notification.Method != dispatch.NotificationRunEvent {
			return nil, invalidRemote("unexpected run notification")
		}
		decoded, err := decodeRemoteValue(notification.Params, eventType)
		if err != nil {
			return nil, err
		}
		event := decoded.(protocol.RunEvent)
		// SSE retains the previous id on ephemeral frames. Only a replayable
		// event publishes a cursor; its two wire projections must agree.
		if event.Event.Replayable() && frame.ID != event.EventID {
			return nil, invalidRemote("event cursor does not match its sse id")
		}
		return event, nil
	case reflect.TypeFor[protocol.RuntimeEvent]():
		if notification.Method != dispatch.NotificationRuntimeEvent || frame.ID != "" {
			return nil, invalidRemote("unexpected runtime notification")
		}
		decoded, err := decodeRemoteValue(notification.Params, reflect.TypeFor[protocol.RuntimeEventNotification]())
		if err != nil {
			return nil, err
		}
		return decoded.(protocol.RuntimeEventNotification).Event, nil
	default:
		return nil, invalidRemote("unknown operation event type")
	}
}

func validateStreamIdentity(request, acknowledgement any) error {
	runID, segmentID := streamIdentity(acknowledgement)
	switch request := request.(type) {
	case protocol.SubscribeRunRequest:
		ack := acknowledgement.(*protocol.SubscribeRunResponse)
		if runID != request.RunID || segmentID != request.SegmentID || request.Snapshot != (ack.Snapshot != nil) {
			return invalidRemote("subscription acknowledgement does not match its request")
		}
	case protocol.ResumeRunRequest:
		if runID != request.RunID {
			return invalidRemote("resume acknowledgement names another run")
		}
	}
	return nil
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

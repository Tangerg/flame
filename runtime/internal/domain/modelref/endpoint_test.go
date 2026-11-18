package modelref

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"

	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/core/chat"
)

func endpointRecord(t *testing.T) EndpointRecord {
	t.Helper()
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("admitted input")))
	if err != nil {
		t.Fatal(err)
	}
	invocation := EndpointInvocation{ID: "physical-request", Caller: framecall.Invocation{ID: "logical-call", Origin: extension.Automatic, Target: framecall.Target{Scope: extension.ScopeCall, Reference: "logical-call"}}, Provider: "provider", Model: "selected", Request: ChatEndpointRequest{Chat: request}, StartedAt: time.Unix(100, 0).UTC()}
	input, err := framevalue.EncodeJSON(invocation)
	if err != nil {
		t.Fatal(err)
	}
	requestJSON, err := framevalue.EncodeJSON(request)
	if err != nil {
		t.Fatal(err)
	}
	return EndpointRecord{Invocation: invocation, InvocationJSON: input, RequestJSON: requestJSON}
}

func endpointResponse(t *testing.T, usage *chat.Usage) framevalue.JSON {
	t.Helper()
	message := chat.NewAssistantMessage(chat.NewTextPart("known response"))
	value, err := framevalue.EncodeJSON(&chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, Metadata: &chat.ResponseMetadata{Model: "served", Usage: usage}})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestEndpointObservationAdmitsOneEvidenceMatrix(t *testing.T) {
	for _, test := range []struct {
		name    string
		state   InvocationState
		known   bool
		failure string
		settled bool
		valid   bool
	}{
		{"open", InvocationStarted, false, "", false, true},
		{"complete", InvocationCompleted, true, "", true, true},
		{"known failed response", InvocationFailed, true, "outer operation failed", true, true},
		{"unknown", InvocationUnknown, false, "provider acknowledgement missing", true, true},
		{"open finished", InvocationStarted, false, "", true, false},
		{"open response", InvocationStarted, true, "", false, false},
		{"open failure", InvocationStarted, false, "failure", false, false},
		{"completed missing response", InvocationCompleted, false, "", true, false},
		{"completed failure", InvocationCompleted, true, "failure", true, false},
		{"failed missing response", InvocationFailed, false, "failure", true, false},
		{"failed missing failure", InvocationFailed, true, "", true, false},
		{"unknown response", InvocationUnknown, true, "failure", true, false},
		{"unknown missing failure", InvocationUnknown, false, "", true, false},
		{"unknown unfinished", InvocationUnknown, false, "failure", false, false},
		{"undeclared state", InvocationState("other"), false, "failure", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			record := endpointRecord(t)
			record.State, record.Failure = test.state, test.failure
			if test.known {
				record.ResponseJSON = endpointResponse(t, nil)
			}
			if test.settled {
				record.FinishedAt = record.Invocation.StartedAt.Add(time.Second)
			}
			observation, err := RestoreEndpointObservation(record)
			if (err == nil) != test.valid {
				t.Fatalf("admission = %v, valid = %t", err, test.valid)
			}
			if err == nil && (observation.State() != test.state || observation.HasResponse() != test.known || observation.Open() != !test.settled) {
				t.Fatal("observation changed admitted evidence")
			}
		})
	}
}

func TestEndpointOutcomeAndRestorationShareAdmission(t *testing.T) {
	for _, known := range []bool{false, true} {
		for _, failure := range []string{"", "outer failure"} {
			record := endpointRecord(t)
			outcome := EndpointOutcome{FinishedAt: record.Invocation.StartedAt.Add(time.Second), Failure: failure}
			if known {
				message := chat.NewAssistantMessage(chat.NewTextPart("known"))
				response := &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}}
				outcome.Response = ChatEndpointResponse{Chat: response}
				encoded, err := framevalue.EncodeJSON(response)
				if err != nil {
					t.Fatal(err)
				}
				record.ResponseJSON = encoded
			}
			state, err := outcome.State(record.Invocation)
			if !known && failure == "" {
				if err == nil {
					t.Fatal("missing evidence became a completed outcome")
				}
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			record.State, record.Failure, record.FinishedAt = state, outcome.Failure, outcome.FinishedAt
			if _, err := RestoreEndpointObservation(record); err != nil {
				t.Fatalf("settled outcome cannot restore: %v", err)
			}
		}
	}
}

func TestEndpointEvidenceRequiresExactTimingAndAccounting(t *testing.T) {
	for _, change := range []func(*EndpointRecord){
		func(r *EndpointRecord) { r.FinishedAt = r.Invocation.StartedAt.Add(-time.Second) },
		func(r *EndpointRecord) { r.Usage = &chat.Usage{InputTokens: 1} },
		func(r *EndpointRecord) { r.Cost = mustCost(t, 1.0) },
		func(r *EndpointRecord) { r.ResponseJSON = framevalue.JSON{} },
	} {
		record := endpointRecord(t)
		record.State, record.FinishedAt, record.ResponseJSON = InvocationCompleted, record.Invocation.StartedAt.Add(time.Second), endpointResponse(t, nil)
		change(&record)
		if _, err := RestoreEndpointObservation(record); err == nil {
			t.Fatal("inconsistent endpoint evidence was admitted")
		}
	}
	record := endpointRecord(t)
	record.State, record.ResponseJSON = InvocationCompleted, endpointResponse(t, nil)
	record.Invocation.StartedAt, record.FinishedAt = time.Unix(-1, 0).UTC(), time.Unix(0, 0).UTC()
	if observation, err := RestoreEndpointObservation(record); err != nil || observation.Open() {
		t.Fatalf("epoch settlement = %v", err)
	}
	if err := (EndpointObservation{}).Validate(); err == nil {
		t.Fatal("uninitialized observation was admitted")
	}
}

func mustCost(t *testing.T, usd float64) accounting.Cost {
	t.Helper()
	cost, err := accounting.NewCost(usd)
	if err != nil {
		t.Fatal(err)
	}
	return cost
}

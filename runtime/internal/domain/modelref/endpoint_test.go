package modelref

import (
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/scope/core/chat"
)

func endpointRecord(t *testing.T) EndpointRecord {
	t.Helper()
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("admitted input")))
	if err != nil {
		t.Fatal(err)
	}
	invocation := EndpointInvocation{ID: "physical-request", Caller: portable.Invocation{ID: "logical-call", Origin: extension.Automatic, Target: portable.Target{Scope: extension.ScopeCall, Reference: "logical-call"}}, Provider: "provider", Model: "selected", Request: ChatEndpointRequest{Chat: request}, StartedAt: time.Unix(100, 0).UTC()}
	input, err := jsonvalue.Encode(invocation)
	if err != nil {
		t.Fatal(err)
	}
	requestJSON, err := jsonvalue.Encode(request)
	if err != nil {
		t.Fatal(err)
	}
	return EndpointRecord{Invocation: invocation, InvocationJSON: input, RequestJSON: requestJSON}
}

func endpointResponse(t *testing.T, usage *chat.Usage) jsonvalue.Value {
	t.Helper()
	message := chat.NewAssistantMessage(chat.NewTextPart("known response"))
	value, err := jsonvalue.Encode(&chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, Metadata: &chat.ResponseMetadata{Model: "served", Usage: usage}})
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
				encoded, err := jsonvalue.Encode(response)
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

func TestEndpointObservationProtectsItsAdmittedRecord(t *testing.T) {
	record := endpointRecord(t)
	record.State, record.FinishedAt = InvocationCompleted, record.Invocation.StartedAt.Add(time.Second)
	record.CostUSD, record.Usage = new(1.5), &chat.Usage{InputTokens: 7}
	record.ResponseJSON = endpointResponse(t, record.Usage)
	observation, err := RestoreEndpointObservation(record)
	if err != nil {
		t.Fatal(err)
	}
	before := observation.Snapshot()
	*record.CostUSD, record.Usage.InputTokens = 99, 99
	other, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("different input")))
	if err != nil {
		t.Fatal(err)
	}
	*record.Invocation.Request.(ChatEndpointRequest).Chat = *other
	projection := observation.Snapshot()
	*projection.CostUSD, projection.Usage.InputTokens = 88, 88
	*projection.Invocation.Request.(ChatEndpointRequest).Chat = *other
	response := observation.Response()
	response.Metadata.Model, response.Metadata.Usage.InputTokens = "different", 88
	if !reflect.DeepEqual(before, observation.Snapshot()) || observation.Response().Metadata.Usage.InputTokens != 7 || observation.ServedModel() != "served" {
		t.Fatal("a projection changed the admitted observation")
	}
}

func TestEndpointEvidenceRequiresExactTimingAndAccounting(t *testing.T) {
	for _, change := range []func(*EndpointRecord){
		func(r *EndpointRecord) { r.FinishedAt = r.Invocation.StartedAt.Add(-time.Second) },
		func(r *EndpointRecord) { r.Usage = &chat.Usage{InputTokens: 1} },
		func(r *EndpointRecord) { r.CostUSD = new(1.0) },
		func(r *EndpointRecord) { r.ResponseJSON = jsonvalue.Value{} },
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

func TestEndpointObservationConcurrentProjectionsRemainIsolated(t *testing.T) {
	record := endpointRecord(t)
	record.State, record.FinishedAt = InvocationCompleted, record.Invocation.StartedAt.Add(time.Second)
	record.CostUSD, record.Usage = new(1.5), &chat.Usage{InputTokens: 7}
	record.ResponseJSON = endpointResponse(t, record.Usage)
	observation, err := RestoreEndpointObservation(record)
	if err != nil {
		t.Fatal(err)
	}
	before := observation.Snapshot()
	for range 4 {
		t.Run("independent reader", func(t *testing.T) {
			t.Parallel()
			projection := observation.Snapshot()
			*projection.CostUSD, projection.Usage.InputTokens = 99, 99
			projection.Invocation.Request.(ChatEndpointRequest).Chat.Messages = nil
			response := observation.Response()
			response.Metadata.Model, response.Metadata.Usage.InputTokens = "different", 99
			if !reflect.DeepEqual(before, observation.Snapshot()) || observation.Response().Metadata.Usage.InputTokens != 7 || observation.ServedModel() != "served" {
				t.Fatal("another reader advanced the admitted observation")
			}
		})
	}
}

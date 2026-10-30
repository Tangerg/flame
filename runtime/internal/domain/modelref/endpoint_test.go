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
	invocation := EndpointInvocation{ID: "physical-request", Kind: EndpointChat, Caller: portable.Invocation{ID: "logical-call", Origin: extension.Automatic, Target: portable.Target{Scope: extension.ScopeCall, Reference: "logical-call"}}, Provider: "provider", Model: "selected", Request: request, StartedAt: time.Unix(100, 0).UTC()}
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
		state   EndpointState
		known   bool
		failure string
		settled bool
		valid   bool
	}{
		{"open", EndpointStarted, false, "", false, true},
		{"complete", EndpointCompleted, true, "", true, true},
		{"known failed response", EndpointFailed, true, "outer operation failed", true, true},
		{"unknown", EndpointUnknown, false, "provider acknowledgement missing", true, true},
		{"open finished", EndpointStarted, false, "", true, false},
		{"open response", EndpointStarted, true, "", false, false},
		{"open failure", EndpointStarted, false, "failure", false, false},
		{"completed missing response", EndpointCompleted, false, "", true, false},
		{"completed failure", EndpointCompleted, true, "failure", true, false},
		{"failed missing response", EndpointFailed, false, "failure", true, false},
		{"failed missing failure", EndpointFailed, true, "", true, false},
		{"unknown response", EndpointUnknown, true, "failure", true, false},
		{"unknown missing failure", EndpointUnknown, false, "", true, false},
		{"unknown unfinished", EndpointUnknown, false, "failure", false, false},
		{"undeclared state", EndpointState("other"), false, "failure", true, false},
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
				outcome.Response = &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}}
				encoded, err := jsonvalue.Encode(outcome.Response)
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
	record.State, record.FinishedAt = EndpointCompleted, record.Invocation.StartedAt.Add(time.Second)
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
	*record.Invocation.Request = *other
	projection := observation.Snapshot()
	*projection.CostUSD, projection.Usage.InputTokens = 88, 88
	*projection.Invocation.Request = *other
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
		record.State, record.FinishedAt, record.ResponseJSON = EndpointCompleted, record.Invocation.StartedAt.Add(time.Second), endpointResponse(t, nil)
		change(&record)
		if _, err := RestoreEndpointObservation(record); err == nil {
			t.Fatal("inconsistent endpoint evidence was admitted")
		}
	}
	record := endpointRecord(t)
	record.State, record.ResponseJSON = EndpointCompleted, endpointResponse(t, nil)
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
	record.State, record.FinishedAt = EndpointCompleted, record.Invocation.StartedAt.Add(time.Second)
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
			projection.Invocation.Request.Messages = nil
			response := observation.Response()
			response.Metadata.Model, response.Metadata.Usage.InputTokens = "different", 99
			if !reflect.DeepEqual(before, observation.Snapshot()) || observation.Response().Metadata.Usage.InputTokens != 7 || observation.ServedModel() != "served" {
				t.Fatal("another reader advanced the admitted observation")
			}
		})
	}
}

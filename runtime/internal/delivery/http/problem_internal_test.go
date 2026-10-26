package http

import (
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A client decides from a transport problem whether a command certainly never
// reached an operation, so the decision must agree with what writeProblem sends.
func TestRefusedBeforeDispatchMatchesWrittenProblems(t *testing.T) {
	for _, kind := range problemKinds {
		recorder := httptest.NewRecorder()
		writeProblem(recorder, kind, "detail", false)
		var problem Problem
		if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
			t.Fatalf("%s: decode problem: %v", kind.name, err)
		}
		if got := RefusedBeforeDispatch(recorder.Code, problem.Type); got != kind.refusal {
			t.Errorf("%s: RefusedBeforeDispatch = %t, want %t", kind.name, got, kind.refusal)
		}
		if kind.refusal && RefusedBeforeDispatch(http.StatusBadGateway, problem.Type) {
			t.Errorf("%s: a refusal at a foreign status was treated as definitive", kind.name)
		}
	}
	if RefusedBeforeDispatch(http.StatusBadRequest, "urn:flame:transport:unknown") {
		t.Error("an unknown transport problem was treated as definitive")
	}
}

package http

import (
	json "encoding/json/v2"
	"net/http/httptest"
	"testing"
)

func TestWrittenProblemPreservesCatalogIdentityAndStatus(t *testing.T) {
	for _, kind := range problemKinds {
		recorder := httptest.NewRecorder()
		writeProblem(recorder, kind, "detail", false)
		var problem Problem
		if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != kind.status || problem.Status != kind.status || problem.Type != kind.problemType() || problem.Detail != "detail" {
			t.Fatalf("problem %s: status=%d body=%+v", kind.name, recorder.Code, problem)
		}
	}
}

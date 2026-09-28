package delivery

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/protocol"
)

type failingTrajectoryExporter struct{ err error }

func (f failingTrajectoryExporter) Export(context.Context, string) (sessions.TrajectoryExport, error) {
	return sessions.TrajectoryExport{}, f.err
}

func TestTrajectoryExportFailureReturnsNoDocument(t *testing.T) {
	for _, test := range []struct {
		source error
		want   error
	}{
		{sessions.ErrSessionBusy, protocol.ErrSessionBusy},
		{sessions.ErrExportTooLarge, protocol.ErrExportTooLarge},
	} {
		handler := &Handler{trajectoryExports: failingTrajectoryExporter{err: test.source}}
		response, err := handler.ExportTrajectory(t.Context(), protocol.ExportTrajectoryRequest{SessionID: "ses_1"})
		if response != nil || !errors.Is(err, test.want) {
			t.Fatalf("failed export = %+v, %v", response, err)
		}
	}
}

func TestTrajectoryEncodingLimitCountsEscapedJSON(t *testing.T) {
	value := struct {
		Text string `json:"text"`
	}{Text: strings.Repeat("\x00", 20)}
	if len(value.Text) >= 100 {
		t.Fatal("fixture already exceeds the source budget")
	}
	err := json.MarshalWrite(&trajectorySizeWriter{remaining: 100}, value)
	if !errors.Is(err, sessions.ErrExportTooLarge) {
		t.Fatalf("escaped JSON bypassed final encoding budget: %v", err)
	}
	if projected := wireTrajectoryExportError(err); !errors.Is(projected, protocol.ErrExportTooLarge) {
		t.Fatalf("encoding failure lost stable wire problem: %v", projected)
	}
}

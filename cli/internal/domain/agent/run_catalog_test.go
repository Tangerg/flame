package agent

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestRunQueryRejectsInvalidFilters(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		query RunQuery
		want  string
	}{
		{name: "negative page size", query: RunQuery{PageSize: PageSize{kind: explicitPageSize, rows: -1}}, want: "page size"},
		{name: "unknown status", query: RunQuery{PageSize: DefaultPageSize(), Statuses: []protocol.RunStatus{"paused"}}, want: "paused"},
		{name: "duplicate status", query: RunQuery{PageSize: DefaultPageSize(), Statuses: []protocol.RunStatus{protocol.RunStatusRunning, protocol.RunStatusRunning}}, want: "not repeat"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.query.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() = %v, want error containing %q", err, test.want)
			}
		})
	}
}

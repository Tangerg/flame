package toolset

import "testing"

func TestMethodSelectableHTTPDoesNotClaimAReadOnlyOutcome(t *testing.T) {
	built, err := buildOnline(OnlineConfig{HTTPAllowedHosts: []string{"api.example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(built) != 1 || built[0].Definition().Name != "http_request" {
		t.Fatalf("HTTP tool: %+v", built)
	}
	if _, definite := built[0].(*callDecorator); definite {
		t.Fatal("method-selectable HTTP claimed a definite read outcome")
	}
}

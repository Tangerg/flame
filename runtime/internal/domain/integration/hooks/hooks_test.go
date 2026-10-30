package hooks

import "testing"

// TestFoldDecidesTheSameCallWhateverOrderHooksFire pins that a denial is final.
// Ask and RewriteArguments only mean anything for a call that proceeds, so a
// hook set that escalates and then denies must read as a denial — the same
// decision it would be had the denying hook matched first.
func TestFoldDecidesTheSameCallWhateverOrderHooksFire(t *testing.T) {
	deny := func(d *Decision) { d.Fold(VerdictBlock, "blocked by policy", "", "") }
	ask := func(d *Decision) { d.Fold(VerdictAsk, "please review", "", "") }
	rewrite := func(d *Decision) { d.Fold(VerdictAllow, "", "", `{"path":"/tmp"}`) }

	for _, test := range []struct {
		name  string
		order []func(*Decision)
	}{
		{name: "deny first", order: []func(*Decision){deny, ask, rewrite}},
		{name: "ask then deny", order: []func(*Decision){ask, deny}},
		{name: "rewrite then deny", order: []func(*Decision){rewrite, deny}},
		{name: "ask, rewrite, then deny", order: []func(*Decision){ask, rewrite, deny}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var decision Decision
			for _, fold := range test.order {
				fold(&decision)
			}
			if decision.Verdict != VerdictBlock || decision.Reason != "blocked by policy" {
				t.Fatalf("decision = %+v, want a denial stating its reason", decision)
			}
			if decision.RewriteArguments != "" {
				t.Fatalf("denied decision still rewrites: %+v", decision)
			}
		})
	}
}

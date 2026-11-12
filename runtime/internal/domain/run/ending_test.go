package run

import (
	"testing"
	"time"
)

func TestRunRejectsMutableAndNilPointerEndings(t *testing.T) {
	created := time.Unix(1, 0).UTC()
	current, err := Admit(Draft{
		RunID: "run_ending", SessionID: "ses_ending", SegmentID: "seg_ending",
		ModelSelection: mustRunSelection(t), CreatedAt: created,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		value    Ending
		pointer  Ending
		typedNil Ending
	}{
		{"completed", Completed{}, &Completed{}, (*Completed)(nil)},
		{"canceled", Canceled{Detail: "stopped"}, &Canceled{Detail: "stopped"}, (*Canceled)(nil)},
		{"failed", Failed{Failure: Failure{Kind: FailureInternal}}, &Failed{Failure: Failure{Kind: FailureInternal}}, (*Failed)(nil)},
		{"timed_out", TimedOut{Detail: "deadline"}, &TimedOut{Detail: "deadline"}, (*TimedOut)(nil)},
		{"lost", Lost{Loss: Loss{Reason: LossRestart}}, &Lost{Loss: Loss{Reason: LossRestart}}, (*Lost)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			termination := Termination{Ending: test.value, FinishedAt: created.Add(time.Second), MessageMark: MessageMarkAt(0)}
			finished, err := current.Terminate(termination)
			if err != nil {
				t.Fatalf("value ending: %v", err)
			}
			for _, invalid := range []struct {
				name   string
				ending Ending
			}{{"mutable", test.pointer}, {"typed_nil", test.typedNil}} {
				t.Run(invalid.name, func(t *testing.T) {
					termination.Ending = invalid.ending
					if _, err := current.Terminate(termination); err == nil {
						t.Fatal("termination accepted a pointer-owned ending")
					}
					snapshot := finished.Snapshot()
					snapshot.Ending = invalid.ending
					if _, err := Restore(snapshot); err == nil {
						t.Fatal("restoration accepted a pointer-owned ending")
					}
				})
			}
		})
	}
}

package tool

import "testing"

func TestFailureValidate(t *testing.T) {
	for _, test := range []struct {
		name    string
		kind    FailureKind
		wantErr bool
	}{
		{name: "unknown", kind: FailureKind("invalid"), wantErr: true},
		{name: "execution", kind: FailureExecution},
		{name: "canceled", kind: FailureCanceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			failure := Failure{Kind: test.kind}
			if err := failure.Validate(); (err != nil) != test.wantErr {
				t.Fatalf("validation error = %v, want error %t", err, test.wantErr)
			}
		})
	}
}

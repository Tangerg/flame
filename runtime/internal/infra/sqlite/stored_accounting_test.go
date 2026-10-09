package sqlite

import "testing"

func TestRunUsageDistinguishesMissingReportFromReportedZero(t *testing.T) {
	missing, err := decodeRunUsage("")
	if err != nil || missing != nil {
		t.Fatalf("missing report = %+v, %v", missing, err)
	}
	reported, err := decodeRunUsage(`{}`)
	if err != nil || reported == nil {
		t.Fatalf("reported zero = %+v, %v", reported, err)
	}
}

func TestRunAccountingAndFailureRejectInvalidStoredValues(t *testing.T) {
	for _, encoded := range []string{
		`null`,
		`{"InputTokens":7}`,
		`{"inputTokens":7,"future":true}`,
		`{"byModel":{"provider/model":{"OutputTokens":7}}}`,
		`{"byModel":{"provider/model":{"future":true}}}`,
	} {
		t.Run(encoded, func(t *testing.T) {
			if _, err := decodeRunUsage(encoded); err == nil {
				t.Fatal("invalid stored accounting became reported usage")
			}
		})
	}
	for _, encoded := range []string{
		`{"kind":"internal","Detail":"lost diagnostic"}`,
		`{"kind":"internal","future":true}`,
	} {
		t.Run(encoded, func(t *testing.T) {
			if _, err := decodeRunFailure(encoded); err == nil {
				t.Fatal("unknown stored failure field was silently discarded")
			}
		})
	}
}

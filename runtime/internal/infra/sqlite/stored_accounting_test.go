package sqlite

import "testing"

func TestRunAccountingAndFailureRejectUnknownStoredFields(t *testing.T) {
	for _, encoded := range []string{
		`{"InputTokens":7}`,
		`{"inputTokens":7,"future":true}`,
		`{"byModel":{"provider/model":{"OutputTokens":7}}}`,
		`{"byModel":{"provider/model":{"future":true}}}`,
	} {
		t.Run(encoded, func(t *testing.T) {
			if _, err := decodeRunUsage(encoded); err == nil {
				t.Fatal("unknown stored accounting field was silently discarded")
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

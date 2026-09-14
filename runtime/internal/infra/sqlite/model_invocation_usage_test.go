package sqlite

import "testing"

func TestModelInvocationUsageRejectsMalformedStoredObservations(t *testing.T) {
	for _, encoded := range []string{`{}`, `{"inputTokens":1}`, `{"inputTokens":null,"outputTokens":0,"cacheReadTokens":0,"cacheWriteTokens":0,"reasoningTokens":0}`, `null`, `[]`, `{"inputTokens":-1}`, `{"inputTokens":1,"inputTokens":2}`, `{"unexpected":3}`, `{} {}`} {
		t.Run(encoded, func(t *testing.T) {
			if _, err := decodeModelInvocationUsage(encoded); err == nil {
				t.Fatalf("accepted malformed usage %s", encoded)
			}
		})
	}
}

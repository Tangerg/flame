package persistence

import (
	"testing"
)

func TestRunCapabilitiesCodecRejectsNonCanonicalOrFormerShapes(t *testing.T) {
	for name, encoded := range map[string]string{
		"null capabilities":         `null`,
		"empty capabilities object": `{}`,
		"former vocabulary":         `{"interruptKinds":["approval"]}`,
		"unknown field":             `{"childRuns":true,"extra":true}`,
		"duplicate type":            `{"interruptTypes":["approval","approval"]}`,
		"unsorted types":            `{"interruptTypes":["question","approval"]}`,
		"trailing value":            `{} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeRunCapabilities(encoded); err == nil {
				t.Fatalf("decodeRunCapabilities(%s) succeeded", encoded)
			}
		})
	}
}

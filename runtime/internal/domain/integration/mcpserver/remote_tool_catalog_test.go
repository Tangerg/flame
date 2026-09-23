package mcpserver

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRemoteToolCatalogEnvelope(t *testing.T) {
	if err := ValidateRemoteToolCount(MaxRemoteToolsPerServer); err != nil {
		t.Fatalf("exact tool-count boundary: %v", err)
	}
	if err := ValidateRemoteToolCount(MaxRemoteToolsPerServer + 1); !errors.Is(err, ErrInvalidRemoteToolCatalog) {
		t.Fatalf("over-capacity tool count error = %v", err)
	}

	description := strings.Repeat("x", MaxRemoteToolDescriptionBytes)
	if err := ValidateRemoteToolDescription(description); err != nil {
		t.Fatalf("exact description boundary: %v", err)
	}
	if err := ValidateRemoteToolDescription(description + "x"); !errors.Is(err, ErrInvalidRemoteToolCatalog) {
		t.Fatalf("oversized description error = %v", err)
	}
	if err := ValidateRemoteToolDescription(string([]byte{utf8.RuneSelf})); !errors.Is(err, ErrInvalidRemoteToolCatalog) {
		t.Fatalf("invalid UTF-8 description error = %v", err)
	}
}

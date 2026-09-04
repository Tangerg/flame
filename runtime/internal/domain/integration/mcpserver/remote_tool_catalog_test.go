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

func TestRemoteToolInputSchemaEnvelope(t *testing.T) {
	prefix := `{"type":"object","description":"`
	suffix := `"}`
	exact := []byte(prefix + strings.Repeat("x", MaxRemoteToolInputSchemaBytes-len(prefix)-len(suffix)) + suffix)
	if len(exact) != MaxRemoteToolInputSchemaBytes {
		t.Fatalf("fixture length = %d", len(exact))
	}
	if _, err := ParseInputSchema(exact); err != nil {
		t.Fatalf("exact schema boundary: %v", err)
	}
	oversized := append(exact[:len(exact)-len(suffix)], append([]byte("x"), []byte(suffix)...)...)
	if _, err := ParseInputSchema(oversized); !errors.Is(err, ErrInvalidInputSchema) {
		t.Fatalf("oversized schema error = %v, want ErrInvalidInputSchema", err)
	}

	expanding := []byte(prefix + strings.Repeat("<", MaxRemoteToolInputSchemaBytes/5) + suffix)
	if len(expanding) >= MaxRemoteToolInputSchemaBytes {
		t.Fatalf("expanding fixture is already oversized: %d", len(expanding))
	}
	if _, err := ParseInputSchema(expanding); !errors.Is(err, ErrInvalidInputSchema) {
		t.Fatalf("oversized normalized schema error = %v, want ErrInvalidInputSchema", err)
	}
}

func TestAdvertisedToolValidatesItsCompleteDescriptor(t *testing.T) {
	valid := AdvertisedTool{Server: testMCPServerName("files"), Name: testRemoteToolName("read")}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid tool rejected: %v", err)
	}

	for name, tool := range map[string]AdvertisedTool{
		"missing server":      {Name: valid.Name},
		"missing tool":        {Server: valid.Server},
		"invalid schema":      {Server: valid.Server, Name: valid.Name, InputSchema: InputSchema{object: `{"type":"array"}`}},
		"noncanonical schema": {Server: valid.Server, Name: valid.Name, InputSchema: InputSchema{object: `{"z":1,"type":"object"}`}},
		"invalid UTF-8":       {Server: valid.Server, Name: valid.Name, Description: string([]byte{utf8.RuneSelf})},
	} {
		t.Run(name, func(t *testing.T) {
			if err := tool.Validate(); !errors.Is(err, ErrInvalidRemoteToolCatalog) {
				t.Fatalf("Validate error = %v, want ErrInvalidRemoteToolCatalog", err)
			}
		})
	}
}

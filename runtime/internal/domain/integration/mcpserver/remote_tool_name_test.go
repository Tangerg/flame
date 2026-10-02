package mcpserver

import (
	"errors"
	"strings"
	"testing"
)

func TestRemoteToolName(t *testing.T) {
	valid := []string{
		"read",
		"DATA_EXPORT_v2.1",
		"-",
		strings.Repeat("a", MaximumRemoteToolNameCharacters),
	}
	for _, raw := range valid {
		name, err := ParseRemoteToolName(raw)
		if err != nil || name.String() != raw || name.Validate() != nil {
			t.Errorf("ParseRemoteToolName(%q) = %q, %v", raw, name, err)
		}
	}

	invalid := []string{
		"",
		"with space",
		"tool/name",
		"tool@name",
		"工具",
		strings.Repeat("a", MaximumRemoteToolNameCharacters+1),
	}
	for _, raw := range invalid {
		if _, err := ParseRemoteToolName(raw); !errors.Is(err, ErrInvalidRemoteToolName) {
			t.Errorf("ParseRemoteToolName(%q) error = %v, want ErrInvalidRemoteToolName", raw, err)
		}
	}
	var zero RemoteToolName
	if !errors.Is(zero.Validate(), ErrInvalidRemoteToolName) {
		t.Fatal("zero RemoteToolName validated")
	}
}

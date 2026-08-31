package mcpserver

import (
	"errors"
	"strings"
	"testing"
)

func TestServerNameOwnsCanonicalRegistryIdentity(t *testing.T) {
	valid := []string{
		"a",
		"github",
		"html.to_design-v2",
		strings.Repeat("a", MaximumServerNameCharacters),
	}
	for _, raw := range valid {
		name, err := ParseServerName(raw)
		if err != nil {
			t.Errorf("ParseServerName(%q) error = %v", raw, err)
			continue
		}
		if name.String() != raw || name.Validate() != nil {
			t.Errorf("ParseServerName(%q) = %q, Validate = %v", raw, name.String(), name.Validate())
		}
	}

	invalid := []string{
		"",
		"GitHub",
		" github",
		"github ",
		"github/server",
		"服务",
		strings.Repeat("a", MaximumServerNameCharacters+1),
	}
	for _, raw := range invalid {
		if _, err := ParseServerName(raw); !errors.Is(err, ErrInvalidServerName) {
			t.Errorf("ParseServerName(%q) error = %v, want ErrInvalidServerName", raw, err)
		}
	}

	var zero ServerName
	if !errors.Is(zero.Validate(), ErrInvalidServerName) {
		t.Fatalf("zero ServerName Validate error = %v", zero.Validate())
	}
}

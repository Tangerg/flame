package localruntime

import (
	"path/filepath"
	"testing"
)

func TestResolveProductRoot(t *testing.T) {
	home := t.TempDir()
	configured := filepath.Join(t.TempDir(), " product root ")
	for _, test := range []struct {
		name, home, configured, want string
	}{
		{name: "default", home: home, want: filepath.Join(home, ".flame")},
		{name: "configured", home: home, configured: configured, want: configured},
		{name: "configured without home", configured: configured, want: configured},
		{name: "configured with relative home", home: "relative", configured: configured, want: configured},
		{name: "missing home"},
		{name: "relative home", home: "relative"},
		{name: "relative configuration", home: home, configured: "relative"},
		{name: "whitespace configuration", home: home, configured: " "},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, err := ResolveProductRoot(test.home, test.configured)
			if test.want == "" {
				if err == nil || root != "" {
					t.Fatalf("invalid product root resolved as %q: %v", root, err)
				}
				return
			}
			if err != nil || root != test.want {
				t.Fatalf("product root = %q, error = %v, want %q", root, err, test.want)
			}
		})
	}
}

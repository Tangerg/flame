package sandbox

import "testing"

func TestUnconstructedConfinerCannotAuthorizeACommand(t *testing.T) {
	for _, confiner := range []*Confiner{nil, {}} {
		if _, err := confiner.Confine(t.TempDir(), "printf test"); err == nil {
			t.Fatal("unconstructed confiner authorized a command")
		}
	}
}

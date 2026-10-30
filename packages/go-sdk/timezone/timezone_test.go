package timezone_test

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/timezone"
)

func TestHostPrefersAValidTZName(t *testing.T) {
	t.Setenv("TZ", ":Asia/Shanghai")
	name, err := timezone.Host()
	if err != nil || name != "Asia/Shanghai" {
		t.Fatalf("Host() = %q, %v", name, err)
	}
}

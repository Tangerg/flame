package terminal

import (
	"os"
	"testing"

	"github.com/Tangerg/flame/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/runtime/localruntime"
)

func TestMain(tests *testing.M) {
	os.Exit(runtimefixture.RunWithFirstPartyPackages(tests, localruntime.FirstPartyDirectory))
}

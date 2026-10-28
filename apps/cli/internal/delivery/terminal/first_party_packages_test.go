package terminal

import (
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	"os"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
)

func TestMain(tests *testing.M) {
	os.Exit(runtimefixture.RunWithRuntimeCompanions(tests, localruntime.FirstPartyDirectory, localruntime.RunnerExecutable))
}

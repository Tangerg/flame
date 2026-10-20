package cmd

import (
	"os"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
)

func TestMain(tests *testing.M) {
	os.Exit(runtimefixture.RunWithFirstPartyPackages(tests, localruntime.FirstPartyDirectory))
}

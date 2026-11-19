package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

var firstPartyPackages string

func TestMain(tests *testing.M) {
	executable, err := os.Executable()
	if err != nil {
		panic(err)
	}
	firstPartyPackages = filepath.Join(filepath.Dir(executable), "plugins")
	// A crash helper re-executes this binary; only the process that built the
	// fixture may remove it while the parent's tests still use it.
	owner := os.Getenv("FLAME_TEST_FIRST_PARTY_DIRECTORY") != firstPartyPackages
	if owner {
		if err := preparePluginFixture(executable); err != nil {
			panic(err)
		}
	}
	status := testsupport.RunWithFirstPartyPackages(tests)
	if owner {
		_ = os.Remove(filepath.Join(filepath.Dir(executable), "mcp-fixture"))
	}
	os.Exit(status)
}

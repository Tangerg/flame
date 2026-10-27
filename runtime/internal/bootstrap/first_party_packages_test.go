package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

var firstPartyPackages string

func TestMain(tests *testing.M) {
	if os.Getenv(messageLossProcessEnv) == "1" {
		runMessageLossFrameServer()
		return
	}
	executable, err := os.Executable()
	if err != nil {
		panic(err)
	}
	firstPartyPackages = filepath.Join(filepath.Dir(executable), "plugins")
	if os.Getenv("FLAME_TEST_FIRST_PARTY_DIRECTORY") != firstPartyPackages {
		if err := preparePluginFixture(executable); err != nil {
			panic(err)
		}
	}
	status := testsupport.RunWithFirstPartyPackages(tests)
	_ = os.Remove(filepath.Join(filepath.Dir(executable), "mcp-fixture"))
	os.Exit(status)
}

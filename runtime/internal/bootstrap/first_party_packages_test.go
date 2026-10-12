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
	os.Exit(testsupport.RunWithFirstPartyPackages(tests))
}

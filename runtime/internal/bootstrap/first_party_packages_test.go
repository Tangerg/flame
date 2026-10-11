package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"testing"
)

var firstPartyPackages string

func TestMain(tests *testing.M) {
	owned, err := os.MkdirTemp("", "flame-first-party-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	firstPartyPackages = filepath.Join(owned, "packages")
	command := exec.Command("node", "../../../plugins/build.mjs", "--os", goruntime.GOOS, "--arch", goruntime.GOARCH, "--output", firstPartyPackages)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	status := 1
	if err := command.Run(); err == nil {
		status = tests.Run()
	} else {
		fmt.Fprintln(os.Stderr, err)
	}
	if err := os.RemoveAll(owned); err != nil {
		fmt.Fprintln(os.Stderr, err)
		status = 1
	}
	os.Exit(status)
}

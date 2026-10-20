package main

import (
	"os"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestMain(tests *testing.M) { os.Exit(testsupport.RunWithFirstPartyPackages(tests)) }

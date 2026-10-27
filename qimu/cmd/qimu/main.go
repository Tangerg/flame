// Command qimu generates public contracts and builds or validates immutable releases.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/Tangerg/flame/qimu/contractgen"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "qimu:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("expected contract, package or verify")
	}
	switch args[0] {
	case "contract":
		return runContract(args[1:])
	case "package":
		return runPackage(args[1:], os.Stdout)
	case "verify":
		return runVerify(args[1:], os.Stdout)
	default:
		return errors.New("expected contract, package or verify")
	}
}

func runContract(args []string) error {
	flags := flag.NewFlagSet("qimu contract", flag.ContinueOnError)
	feature := flags.String("feature", "", "published capability descriptor, or - for stdin")
	out := flags.String("out", ".", "artifact directory; empty writes only requested projections")
	ts := flags.String("ts", "", "TypeScript projection directory")
	goOut := flags.String("go-out", "", "Go projection file")
	goPackage := flags.String("go-package", "", "Go projection package")
	goPrefix := flags.String("go-prefix", "", "Go exported prefix")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *feature == "" {
		return errors.New("one feature descriptor is required")
	}
	return contractgen.Generate(*feature, *out, *ts, contractgen.GoOutput{Path: *goOut, Package: *goPackage, Prefix: *goPrefix})
}

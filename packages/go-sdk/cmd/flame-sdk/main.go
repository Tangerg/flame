package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "flame-sdk:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		return fmt.Errorf("expected contract or backend-protocol")
	}
	switch arguments[0] {
	case "contract":
		return runContract(arguments[1:])
	case "backend-protocol":
		return runBackendProtocol(arguments[1:])
	default:
		return fmt.Errorf("unknown command %q", arguments[0])
	}
}

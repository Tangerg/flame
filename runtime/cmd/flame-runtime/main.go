package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

const executableName = identity.ProductName

// version is stamped at link time via -ldflags "-X main.version=..."; empty
// means an unstamped build, whose version bootstrap derives.
var version string

func main() {
	if len(os.Args) > 1 {
		fmt.Fprintf(os.Stderr, "%s: unexpected argument %q; this binary only starts the HTTP runtime server\n", executableName, os.Args[1])
		os.Exit(2)
	}
	if err := run(context.Background(), os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", executableName, err)
		os.Exit(1)
	}
}

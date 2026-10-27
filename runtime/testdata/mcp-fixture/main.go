package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/Tangerg/flame/runtime/internal/testsupport/mcpfixture"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 3 {
		return fmt.Errorf("fixture: expected proxy endpoint or blocked marker")
	}
	switch os.Args[1] {
	case "proxy":
		return mcpfixture.Forward(context.Background(), os.Args[2])
	case "blocked":
		if err := os.WriteFile(os.Args[2], []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			return err
		}
		_, err := io.Copy(io.Discard, os.Stdin)
		return err
	default:
		return fmt.Errorf("fixture: unknown mode")
	}
}

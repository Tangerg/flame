package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "worktree writer: the original receipt directory is required")
		os.Exit(2)
	}
	if err := workspace.RunTaskWriter(context.Background(), os.Args[1], os.Stdin, delivery.EncodeTaskWriteRefusal); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

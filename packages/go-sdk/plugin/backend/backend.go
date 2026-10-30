// Package backend runs a package backend process under the Runtime's contract.
package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/process"
)

// Main serves one backend until the Runtime stops it and exits non-zero on
// failure. serve receives the private data directory under an exclusive lease,
// so a second process of the same installation cannot share its state.
func Main(name string, serve func(ctx context.Context, dataDirectory string) error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, name, serve)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, name string, serve func(context.Context, string) error) (err error) {
	data := os.Getenv(process.DataVariable)
	if !filepath.IsAbs(data) {
		return fmt.Errorf("%s: %s must be absolute", name, process.DataVariable)
	}
	lease, err := filesystem.AcquireDirectoryLease(ctx, data)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lease.Release()) }()
	return serve(ctx, data)
}

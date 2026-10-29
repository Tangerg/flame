package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/muesli/cancelreader"
)

// A prompt read exclusively borrows its source. An embedded blocking source
// must interrupt its own ReadContext; the command never closes a borrowed reader.
type contextPromptReader interface {
	ReadContext(context.Context, []byte) (int, error)
}

type promptFile interface {
	cancelreader.File
	Stat() (os.FileInfo, error)
}

type promptReadFunc func([]byte) (int, error)

func (f promptReadFunc) Read(buffer []byte) (int, error) { return f(buffer) }

func promptReader(ctx context.Context, input io.Reader) (io.Reader, func() error, error) {
	if cause := context.Cause(ctx); cause != nil {
		return nil, nil, cause
	}
	noClose := func() error { return nil }
	if reader, ok := input.(contextPromptReader); ok {
		return promptReadFunc(func(buffer []byte) (int, error) {
			return reader.ReadContext(ctx, buffer)
		}), noClose, nil
	}
	if file, ok := input.(promptFile); ok {
		info, err := file.Stat()
		if err != nil {
			return nil, nil, fmt.Errorf("inspect stdin: %w", err)
		}
		if info.Mode()&os.ModeCharDevice != 0 {
			return nil, nil, nil
		}
		if !info.Mode().IsRegular() {
			// These deployment targets provide descriptor pollers. Other backends
			// may use an uncancelable fallback or open the Windows console instead.
			if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
				return nil, nil, fmt.Errorf("piped prompts on %s require a context-aware reader", runtime.GOOS)
			}
			reader, err := cancelreader.NewReader(file)
			if err != nil {
				return nil, nil, fmt.Errorf("prepare stdin cancellation: %w", err)
			}
			done := make(chan struct{})
			var cancelErr error
			stop := context.AfterFunc(ctx, func() {
				defer close(done)
				if !reader.Cancel() {
					cancelErr = errors.New("interrupt stdin read failed")
				}
			})
			return reader, func() error {
				if !stop() {
					<-done
				}
				return errors.Join(cancelErr, reader.Close())
			}, nil
		}
		return checkedPromptReader(ctx, file), noClose, nil
	}
	switch input.(type) {
	case *bytes.Reader, *bytes.Buffer, *strings.Reader:
		return checkedPromptReader(ctx, input), noClose, nil
	default:
		return nil, nil, errors.New("piped prompt source must provide context-aware reads")
	}
}

func checkedPromptReader(ctx context.Context, input io.Reader) io.Reader {
	return promptReadFunc(func(buffer []byte) (int, error) {
		if cause := context.Cause(ctx); cause != nil {
			return 0, cause
		}
		return input.Read(buffer)
	})
}

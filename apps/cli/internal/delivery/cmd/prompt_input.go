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

// pipedPrompt is a borrowed prompt source and the release its reader requires.
type pipedPrompt struct {
	reader io.Reader
	close  func() error
}

// pipedPromptSource reports false for a terminal, which carries no piped prompt.
func pipedPromptSource(ctx context.Context, input io.Reader) (pipedPrompt, bool, error) {
	if cause := context.Cause(ctx); cause != nil {
		return pipedPrompt{}, false, cause
	}
	noClose := func() error { return nil }
	if reader, ok := input.(contextPromptReader); ok {
		return pipedPrompt{reader: promptReadFunc(func(buffer []byte) (int, error) {
			return reader.ReadContext(ctx, buffer)
		}), close: noClose}, true, nil
	}
	if file, ok := input.(promptFile); ok {
		info, err := file.Stat()
		if err != nil {
			return pipedPrompt{}, false, fmt.Errorf("inspect stdin: %w", err)
		}
		if info.Mode()&os.ModeCharDevice != 0 {
			return pipedPrompt{}, false, nil
		}
		if !info.Mode().IsRegular() {
			// These deployment targets provide descriptor pollers. Other backends
			// may use an uncancelable fallback or open the Windows console instead.
			if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
				return pipedPrompt{}, false, fmt.Errorf("piped prompts on %s require a context-aware reader", runtime.GOOS)
			}
			reader, err := cancelreader.NewReader(file)
			if err != nil {
				return pipedPrompt{}, false, fmt.Errorf("prepare stdin cancellation: %w", err)
			}
			done := make(chan struct{})
			var cancelErr error
			stop := context.AfterFunc(ctx, func() {
				defer close(done)
				if !reader.Cancel() {
					cancelErr = errors.New("interrupt stdin read failed")
				}
			})
			return pipedPrompt{reader: reader, close: func() error {
				if !stop() {
					<-done
				}
				return errors.Join(cancelErr, reader.Close())
			}}, true, nil
		}
		return pipedPrompt{reader: checkedPromptReader(ctx, file), close: noClose}, true, nil
	}
	switch input.(type) {
	case *bytes.Reader, *bytes.Buffer, *strings.Reader:
		return pipedPrompt{reader: checkedPromptReader(ctx, input), close: noClose}, true, nil
	default:
		return pipedPrompt{}, false, errors.New("piped prompt source must provide context-aware reads")
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

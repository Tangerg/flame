package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
)

type observedPromptFile struct {
	*os.File
	read chan struct{}
	once sync.Once
}

func (f *observedPromptFile) Read(buffer []byte) (int, error) {
	n, err := f.File.Read(buffer)
	f.once.Do(func() { close(f.read) })
	return n, err
}

func TestRunCancellationInterruptsOpenPromptPipeBeforeRuntime(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("raw prompt pipes require a Linux or macOS descriptor poller")
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	var closeOnce sync.Once
	closeInput := func() {
		closeOnce.Do(func() {
			if err := errors.Join(reader.Close(), writer.Close()); err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(closeInput)
	for range 4 {
		source := &observedPromptFile{File: reader, read: make(chan struct{})}
		ctx, cancel := context.WithCancelCause(t.Context())
		cause := errors.New("prompt input interrupted")
		var opens atomic.Int32
		root := NewRoot(Dependencies{OpenRuntime: func(context.Context, settings.Runtime) (Runtime, RuntimeProfile, error) {
			opens.Add(1)
			return nil, nil, errors.New("runtime must remain unopened")
		}})
		root.SetIn(source)
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		root.SetArgs([]string{"run", "an argument does not end piped input"})
		done := make(chan error, 1)
		go func() {
			done <- root.ExecuteContext(ctx)
			close(done)
		}()
		t.Cleanup(func() {
			cancel(context.Canceled)
			closeInput()
			<-done
		})
		if _, err := writer.Write([]byte("partial prompt")); err != nil {
			t.Fatal(err)
		}
		select {
		case <-source.read:
		case <-time.After(5 * time.Second):
			t.Fatal("command did not enter prompt reading")
		}
		cancel(cause)
		cancel(errors.New("later cancellation cannot replace the cause"))
		select {
		case err := <-done:
			if !errors.Is(err, cause) || opens.Load() != 0 {
				t.Fatalf("canceled input = %v, Runtime opens = %d", err, opens.Load())
			}
		case <-time.After(5 * time.Second):
			t.Fatal("canceled command remained blocked behind an open writer")
		}
		if _, err := reader.Stat(); err != nil {
			t.Fatalf("command closed borrowed stdin: %v", err)
		}
	}
}

func TestPromptPipeEOFReturnsCompleteInput(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("raw prompt pipes require a Linux or macOS descriptor poller")
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
	})
	_, writeErr := writer.Write([]byte("complete prompt\n"))
	if err := errors.Join(writeErr, writer.Close()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if got, err := readPipedPrompt(ctx, reader); err != nil || got != "complete prompt" {
		t.Fatalf("pipe EOF = %q, %v", got, err)
	}
}

type promptContextFunc func(context.Context, []byte) (int, error)

func (f promptContextFunc) ReadContext(ctx context.Context, buffer []byte) (int, error) {
	return f(ctx, buffer)
}
func (promptContextFunc) Read([]byte) (int, error) { panic("input requires ReadContext") }

func TestPromptInputRetainsReadFailureAndCancellationCause(t *testing.T) {
	readFailure := errors.New("source failed")
	cause := errors.New("source canceled")
	ctx, cancel := context.WithCancelCause(t.Context())
	input := promptContextFunc(func(context.Context, []byte) (int, error) {
		cancel(cause)
		return 0, readFailure
	})
	if _, err := readPipedPrompt(ctx, input); !errors.Is(err, readFailure) || !errors.Is(err, cause) {
		t.Fatalf("concurrent source failure and cancellation = %v", err)
	}
	if _, err := readPipedPrompt(t.Context(), promptContextFunc(func(context.Context, []byte) (int, error) {
		return 0, readFailure
	})); !errors.Is(err, readFailure) {
		t.Fatalf("source failure = %v", err)
	}
}

type borrowedPromptReader struct{ reads, closes int }

func (r *borrowedPromptReader) Read([]byte) (int, error) { r.reads++; return 0, io.EOF }
func (r *borrowedPromptReader) Close() error             { r.closes++; return nil }

func TestPromptInputRefusesAnUnboundedBorrowWithoutReadingOrClosingIt(t *testing.T) {
	input := new(borrowedPromptReader)
	if _, err := readPipedPrompt(t.Context(), input); err == nil || !strings.Contains(err.Error(), "context-aware reads") {
		t.Fatalf("unbounded borrowed reader = %v", err)
	}
	if input.reads != 0 || input.closes != 0 {
		t.Fatalf("command touched an unowned input: %+v", input)
	}
}

func TestPromptInputReadsRegularFileAndEOFWithoutClosingSource(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "prompt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := file.WriteString("  prompt from file\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if got, err := readPipedPrompt(t.Context(), file); err != nil || got != "prompt from file" {
		t.Fatalf("file prompt = %q, %v", got, err)
	}
	if got, err := readPipedPrompt(t.Context(), file); err != nil || got != "" {
		t.Fatalf("EOF = %q, %v", got, err)
	}
}

package main

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/capture"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/codeintel"
	"github.com/Tangerg/flame/packages/go-sdk/procgroup"
	"github.com/Tangerg/flame/qimu/cancellation"
)

var errFormattedTooLarge = errors.New("code intelligence: formatted output exceeds document capacity")

func formatDocument(ctx context.Context, source codeintel.Document) (codeintel.Transformation, error) {
	if err := source.Validate(); err != nil {
		return codeintel.Transformation{}, err
	}
	if err := cancellation.Err(ctx); err != nil {
		return codeintel.Transformation{}, err
	}
	result := codeintel.Transformation{Version: source.Version}
	input := []byte(source.Content)
	var formatted []byte
	var err error
	switch strings.ToLower(filepath.Ext(source.Path)) {
	case ".go":
		formatted, err = format.Source(input)
	case ".json":
		formatted = bytes.Clone(bytes.TrimSpace(input))
		err = (*jsontext.Value)(&formatted).Indent(jsontext.WithIndent("  "))
		if err == nil {
			formatted = append(formatted, '\n')
		}
	case ".js", ".jsx", ".ts", ".tsx", ".css", ".scss", ".html", ".md", ".yaml", ".yml":
		prettier, lookupErr := exec.LookPath("prettier")
		if errors.Is(lookupErr, exec.ErrNotFound) {
			return result, nil
		}
		if lookupErr != nil {
			return result, lookupErr
		}
		formatted, err = runFormatter(ctx, input, prettier, "--stdin-filepath", filepath.Join(source.Workspace, source.Path))
	default:
		return result, nil
	}
	if err := errors.Join(err, cancellation.Err(ctx)); err != nil {
		return result, err
	}
	result.Content = new(string(formatted))
	return result, result.Validate(source)
}

func runFormatter(ctx context.Context, input []byte, name string, args ...string) ([]byte, error) {
	if err := cancellation.Err(ctx); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, name, args...)
	stdout := capture.NewWriter(int(codeintel.MaxDocumentBytes))
	stderr := capture.NewWriter(codeintel.MaxSummaryBytes)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = time.Second
	procgroup.Prepare(cmd)
	cmd.Cancel = func() error { return procgroup.Stop(cmd) }
	runErr := cmd.Run()
	cleanupErr := procgroup.Stop(cmd)
	if errors.Is(cleanupErr, os.ErrProcessDone) {
		cleanupErr = nil
	}
	failure := errors.Join(runErr, cleanupErr, cancellation.Err(ctx))
	target := name
	if len(args) > 0 {
		target = args[len(args)-1]
	}
	if stdout.Truncated() {
		failure = errors.Join(failure, fmt.Errorf("%s output: %w", name, errFormattedTooLarge))
	}
	if stderr.Truncated() {
		failure = errors.Join(failure, fmt.Errorf("%s diagnostic output exceeded %d bytes", name, codeintel.MaxSummaryBytes))
	}
	if failure == nil {
		return bytes.Clone(stdout.Bytes()), nil
	}
	msg := strings.TrimSpace(stderr.Diagnostic())
	if msg != "" && stderr.Truncated() {
		msg += "\n... [formatter diagnostic truncated] ..."
	}
	if msg == "" {
		return nil, fmt.Errorf("%s: run %s: %w", target, name, failure)
	}
	return nil, fmt.Errorf("%s: %s: %w", target, msg, failure)
}

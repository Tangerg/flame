package toolset

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/codeintel"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"
)

type CodeIntelligence interface {
	AcquireCodeIntelligence(context.Context) (CodeIntelligenceUse, error)
}

// A use retains one selected backend and the original invocation's cancellation
// through baseline capture, formatting, file commit and final diagnostics.
type CodeIntelligenceUse interface {
	Format(codeintel.Document) (codeintel.Transformation, error)
	Inspect(codeintel.Inspection) (codeintel.Report, error)
	Close() error
}

func withCodeIntelligence(inner toolcontract.Tool, intelligence CodeIntelligence, root *filesystemRoot, executor *fs.LocalExecutor) toolcontract.Tool {
	if intelligence == nil {
		return inner
	}
	return decorateCall(inner, func(ctx context.Context, invocation toolcontract.Invocation) (out toolcontract.Result, err error) {
		paths, err := mutationPaths(inner, invocation)
		if err != nil {
			return out, err
		}
		use, acquireErr := intelligence.AcquireCodeIntelligence(ctx)
		var notices []string
		defer func() {
			if use != nil {
				if closeErr := use.Close(); closeErr != nil {
					if err != nil {
						err = errors.Join(err, closeErr)
					} else {
						notices = append(notices, closeErr.Error())
					}
				}
			}
			if err == nil && len(notices) > 0 {
				out.Output = appendToolOutputText(out.Output, "\n\nCode intelligence:\n"+strings.Join(notices, "\n"))
			}
		}()
		if acquireErr != nil {
			notices = append(notices, "Unavailable: "+acquireErr.Error())
			return inner.Call(ctx, invocation)
		}
		if use == nil {
			return inner.Call(ctx, invocation)
		}
		baseline := make(map[string][]string, len(paths))
		for _, path := range paths {
			document, _, found, readErr := captureCodeDocument(ctx, root, executor, path)
			if readErr != nil {
				notices = append(notices, fmt.Sprintf("%s baseline unavailable: %v", path, readErr))
				continue
			}
			if !found {
				continue
			}
			report, inspectErr := use.Inspect(codeintel.Inspection{Document: document})
			if inspectErr == nil {
				inspectErr = report.Validate(document)
			}
			if inspectErr != nil {
				notices = append(notices, fmt.Sprintf("%s baseline unavailable: %v", path, inspectErr))
				continue
			}
			baseline[path] = report.Problems
		}
		out, err = inner.Call(ctx, invocation)
		if err != nil {
			return out, err
		}
		for _, path := range paths {
			if formatErr := formatPath(ctx, use, root, executor, path); formatErr != nil {
				notices = append(notices, fmt.Sprintf("%s formatting unavailable: %v", path, formatErr))
			}
			document, source, found, readErr := captureCodeDocument(ctx, root, executor, path)
			if readErr != nil {
				notices = append(notices, fmt.Sprintf("%s diagnostics unavailable: %v", path, readErr))
				continue
			}
			if !found {
				continue
			}
			report, inspectErr := use.Inspect(codeintel.Inspection{Document: document, Baseline: baseline[path]})
			if inspectErr == nil {
				inspectErr = report.Validate(document)
			}
			if inspectErr == nil {
				inspectErr = verifyCodeSource(root, document.Path, source)
			}
			if inspectErr != nil {
				notices = append(notices, fmt.Sprintf("%s diagnostics unavailable: %v", path, inspectErr))
				continue
			}
			if report.Summary != "" {
				notices = append(notices, report.Summary)
			}
		}
		return out, nil
	})
}

func captureCodeDocument(ctx context.Context, root *filesystemRoot, executor *fs.LocalExecutor, path string) (codeintel.Document, autoFormatSource, bool, error) {
	path, err := rootRelative(root.Name(), path)
	if err != nil {
		return codeintel.Document{}, autoFormatSource{}, false, err
	}
	info, err := root.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return codeintel.Document{}, autoFormatSource{}, false, nil
	}
	if err != nil {
		return codeintel.Document{}, autoFormatSource{}, false, fmt.Errorf("inspect before formatting: %w", err)
	}
	if !info.Mode().IsRegular() {
		return codeintel.Document{}, autoFormatSource{}, false, nil
	}
	if err := verifyWorkspacePath(root); err != nil {
		return codeintel.Document{}, autoFormatSource{}, false, err
	}
	source, err := readAutoFormatFile(ctx, root, executor, path)
	if err != nil {
		return codeintel.Document{}, autoFormatSource{}, false, err
	}
	document, err := codeintel.NewDocument(root.Name(), path, source.content)
	return document, source, err == nil, err
}

func formatPath(ctx context.Context, use CodeIntelligenceUse, root *filesystemRoot, executor *fs.LocalExecutor, path string) error {
	document, source, found, err := captureCodeDocument(ctx, root, executor, path)
	if err != nil || !found {
		return err
	}
	transformation, err := use.Format(document)
	if err != nil {
		return err
	}
	if err := transformation.Validate(document); err != nil {
		return err
	}
	if err := verifyCodeSource(root, document.Path, source); err != nil {
		return err
	}
	if transformation.Content == nil || *transformation.Content == source.content {
		return nil
	}
	return applyFormattedFile(ctx, root, executor, document.Path, []byte(*transformation.Content), source)
}

func verifyCodeSource(root *filesystemRoot, path string, source autoFormatSource) error {
	if err := verifyWorkspacePath(root); err != nil {
		return err
	}
	current, err := root.Lstat(path)
	if err != nil || !filesystem.SameVersion(source.info, current) {
		return errors.Join(errors.New("file changed during code analysis"), err)
	}
	return nil
}

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
	AcquireCodeIntelligence(context.Context) (CodeIntelligenceUse, bool, error)
}

type CodeIntelligenceUse interface {
	Format(codeintel.Document) (codeintel.Transformation, error)
	Inspect(codeintel.Inspection) (codeintel.Report, error)
	Close() error
}

func withCodeIntelligence(inner toolcontract.Tool, intelligence CodeIntelligence, root *filesystemRoot, executor *fs.LocalExecutor) toolcontract.Tool {
	return decorateCall(inner, func(ctx context.Context, next toolcontract.Caller, invocation toolcontract.Invocation) (out toolcontract.Result, err error) {
		paths, err := mutationPaths(inner, invocation)
		if err != nil {
			return out, err
		}
		use, selected, acquireErr := intelligence.AcquireCodeIntelligence(ctx)
		if acquireErr != nil {
			out, err = next.Call(ctx, invocation)
			if err == nil {
				out.Output = appendCodeIntelligenceNotices(out.Output, []string{"Unavailable: " + acquireErr.Error()})
			}
			return out, err
		}
		if !selected {
			return next.Call(ctx, invocation)
		}
		review := codeReview{use: use, root: root, executor: executor}
		defer func() {
			if closeErr := use.Close(); closeErr != nil {
				if err != nil {
					err = errors.Join(err, closeErr)
					return
				}
				review.notices = append(review.notices, closeErr.Error())
			}
			if err == nil {
				out.Output = appendCodeIntelligenceNotices(out.Output, review.notices)
			}
		}()
		baseline := review.baseline(ctx, paths)
		out, err = next.Call(ctx, invocation)
		if err != nil {
			return out, err
		}
		review.inspectChanges(ctx, paths, baseline)
		return out, nil
	})
}

func appendCodeIntelligenceNotices(output toolcontract.Output, notices []string) toolcontract.Output {
	if len(notices) == 0 {
		return output
	}
	return appendToolOutputText(output, "\n\nCode intelligence:\n"+strings.Join(notices, "\n"))
}

type codeReview struct {
	use      CodeIntelligenceUse
	root     *filesystemRoot
	executor *fs.LocalExecutor
	notices  []string
}

func (r *codeReview) notice(format string, args ...any) {
	r.notices = append(r.notices, fmt.Sprintf(format, args...))
}

func (r *codeReview) baseline(ctx context.Context, paths []string) map[string][]string {
	baseline := make(map[string][]string, len(paths))
	for _, path := range paths {
		document, _, found, err := captureCodeDocument(ctx, r.root, r.executor, path)
		if err != nil {
			r.notice("%s baseline unavailable: %v", path, err)
			continue
		}
		if !found {
			continue
		}
		report, err := r.use.Inspect(codeintel.Inspection{Document: document})
		if err == nil {
			err = report.Validate(document)
		}
		if err != nil {
			r.notice("%s baseline unavailable: %v", path, err)
			continue
		}
		baseline[path] = report.Problems
	}
	return baseline
}

func (r *codeReview) inspectChanges(ctx context.Context, paths []string, baseline map[string][]string) {
	for _, path := range paths {
		if err := formatPath(ctx, r.use, r.root, r.executor, path); err != nil {
			r.notice("%s formatting unavailable: %v", path, err)
		}
		document, source, found, err := captureCodeDocument(ctx, r.root, r.executor, path)
		if err != nil {
			r.notice("%s diagnostics unavailable: %v", path, err)
			continue
		}
		if !found {
			continue
		}
		report, err := r.use.Inspect(codeintel.Inspection{Document: document, Baseline: baseline[path]})
		if err == nil {
			err = report.Validate(document)
		}
		if err == nil {
			err = verifyCodeSource(r.root, document.Path, source)
		}
		if err != nil {
			r.notice("%s diagnostics unavailable: %v", path, err)
			continue
		}
		if report.Summary != "" {
			r.notices = append(r.notices, report.Summary)
		}
	}
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

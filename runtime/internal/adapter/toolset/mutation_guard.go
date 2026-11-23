package toolset

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/packages/go-sdk/capture"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"
)

const readBufferBytes = 64 << 10

func withReadTracking(inner toolcontract.Tool, tr *readTracker, root *filesystemRoot) toolcontract.Tool {
	if tr == nil {
		return inner
	}
	return decorateCall(inner, func(ctx context.Context, next toolcontract.Caller, invocation toolcontract.Invocation) (toolcontract.Result, error) {
		request, decodeErr := decodeToolArguments[fs.ReadRequest](invocation)
		if decodeErr != nil {
			return toolcontract.Result{}, fmt.Errorf("track read arguments: %w", decodeErr)
		}
		path, pathErr := resolveRootPath(root, request.Path)
		if pathErr != nil {
			return toolcontract.Result{}, fmt.Errorf("track read path: %w", pathErr)
		}
		abs := filepath.Join(root.identity, path)
		before, existedBefore, err := observeFingerprintExistingFile(ctx, root, path)
		if err != nil {
			return toolcontract.Result{}, fmt.Errorf("track read %s: %w", request.Path, err)
		}
		out, err := next.Call(ctx, invocation)
		if err != nil {
			return out, err
		}
		after, existsAfter, err := observeFingerprintExistingFile(ctx, root, path)
		if err != nil {
			tr.forget(executionctx.SessionID(ctx), abs)
			return out, fmt.Errorf("track read %s: %w", request.Path, err)
		}
		if !existedBefore || !existsAfter || !sameFingerprintObservation(before, after) {
			tr.forget(executionctx.SessionID(ctx), abs)
			return out, fmt.Errorf("track read %s: file changed while reading; read it again", request.Path)
		}
		tr.record(executionctx.SessionID(ctx), abs, after.fingerprint)
		return out, nil
	})
}

func withMutationGuard(inner toolcontract.Tool, tr *readTracker, root *filesystemRoot) toolcontract.Tool {
	if tr == nil {
		return inner
	}
	return decorateCall(inner, func(ctx context.Context, next toolcontract.Caller, invocation toolcontract.Invocation) (toolcontract.Result, error) {
		paths, err := mutationPaths(inner, invocation)
		if err != nil {
			err = fmt.Errorf("inspect mutation paths before applying patch: %w", err)
			return toolcontract.Result{}, beforeCallFailure(ctx, err)
		}
		sessionID := executionctx.SessionID(ctx)
		targets, blocked, err := admitMutationPaths(ctx, tr, root, sessionID, paths)
		if err != nil {
			return toolcontract.Result{}, beforeCallFailure(ctx, err)
		}
		if blocked != "" {
			failure, err := toolcontract.NewFailure(toolcontract.FailureConfig{
				Kind: toolcontract.FailureKindRejected, Output: toolcontract.NewTextOutput(blocked),
			})
			if err != nil {
				return toolcontract.Result{}, beforeCallFailure(ctx, err)
			}
			return toolcontract.Result{}, beforeCallFailure(ctx, failure)
		}
		for _, target := range targets {
			tr.forget(sessionID, target)
		}
		return next.Call(ctx, invocation)
	})
}

func beforeCallFailure(ctx context.Context, cause error) error {
	if cancelled := errors.Join(ctx.Err(), context.Cause(ctx)); cancelled != nil {
		return errors.Join(cause, cancelled)
	}
	return pluginsdk.DefiniteToolFailure(cause)
}

func admitMutationPaths(ctx context.Context, tr *readTracker, root *filesystemRoot, sessionID identity.SessionID, paths []string) ([]string, string, error) {
	targets := make([]string, 0, len(paths))
	for _, path := range paths {
		relative, err := resolveRootPath(root, path)
		if err != nil {
			return nil, "", fmt.Errorf("resolve mutation path: %w", err)
		}
		target := filepath.Join(root.identity, relative)
		targets = append(targets, target)
		fingerprint, exists, err := fingerprintExistingFile(ctx, root, relative)
		if errors.Is(err, errRuntimeReadFileTooLarge) {
			return nil, unreadableMutationMessage(path), nil
		}
		if err != nil {
			return nil, "", fmt.Errorf("fingerprint mutation path %s: %w", path, err)
		}
		if !exists {
			continue
		}
		if verdict := tr.check(sessionID, target, fingerprint); !verdict.allowed() {
			return nil, mutationGuardMessage(verdict, path), nil
		}
	}
	return targets, "", nil
}

func mutationGuardMessage(verdict guardVerdict, path string) string {
	switch verdict {
	case readRequired:
		return fmt.Sprintf("You must read %s before modifying it. Use the read tool first.", path)
	case contentChanged:
		return path + " changed since you last read it (modified by the user or a tool). Read it again before modifying it."
	default:
		return fmt.Sprintf("Cannot modify %s until its current contents have been read.", path)
	}
}

func unreadableMutationMessage(path string) string {
	return fmt.Sprintf(
		"%s is larger than the %d MiB read limit, so its current contents cannot be confirmed before modifying it.",
		path, maxRuntimeReadFileBytes/filesystem.MiB,
	)
}

func fingerprintExistingFile(ctx context.Context, root *filesystemRoot, path string) (contentFingerprint, bool, error) {
	observation, exists, err := observeFingerprintExistingFile(ctx, root, path)
	return observation.fingerprint, exists, err
}

type fingerprintObservation struct {
	fingerprint contentFingerprint
	info        os.FileInfo
}

func observeFingerprintExistingFile(ctx context.Context, root *filesystemRoot, path string) (_ fingerprintObservation, exists bool, err error) {
	if cause := errors.Join(ctx.Err(), context.Cause(ctx)); cause != nil {
		return fingerprintObservation{}, false, cause
	}
	preflight, err := root.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fingerprintObservation{}, false, nil
	}
	if err != nil {
		return fingerprintObservation{}, false, err
	}
	if !preflight.Mode().IsRegular() {
		return fingerprintObservation{}, false, fmt.Errorf("unsupported file mode %s", preflight.Mode().Type())
	}
	if preflight.Size() > maxRuntimeReadFileBytes {
		return fingerprintObservation{}, false, fmt.Errorf("%w: file uses %d bytes", errRuntimeReadFileTooLarge, preflight.Size())
	}
	file, before, err := filesystem.OpenFileAtExpected(root.Root, path, preflight, maxRuntimeReadFileBytes)
	if err != nil {
		if errors.Is(err, filesystem.ErrFileTooLarge) {
			return fingerprintObservation{}, false, fmt.Errorf("%w: file grew while opening: %w", errRuntimeReadFileTooLarge, err)
		}
		return fingerprintObservation{}, false, err
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()
	fingerprint, err := fingerprintContent(ctx, file)
	if err != nil {
		return fingerprintObservation{}, false, err
	}
	after, err := file.Stat()
	if err != nil {
		return fingerprintObservation{}, false, err
	}
	current, err := root.Stat(path)
	if err != nil {
		return fingerprintObservation{}, false, fmt.Errorf("file changed while fingerprinting: %w", err)
	}
	if !filesystem.SameVersion(before, after) || !filesystem.SameVersion(after, current) {
		return fingerprintObservation{}, false, errors.New("file changed while fingerprinting")
	}
	return fingerprintObservation{fingerprint: fingerprint, info: current}, true, nil
}

func fingerprintContent(ctx context.Context, file io.Reader) (contentFingerprint, error) {
	hash := sha256.New()
	contextReader, err := capture.NewReader(ctx, file)
	if err != nil {
		return contentFingerprint{}, err
	}
	written, err := io.CopyBuffer(hash, io.LimitReader(contextReader, maxRuntimeReadFileBytes+1), make([]byte, readBufferBytes))
	if err != nil {
		return contentFingerprint{}, err
	}
	if written > maxRuntimeReadFileBytes {
		return contentFingerprint{}, fmt.Errorf("%w: file grew while fingerprinting", errRuntimeReadFileTooLarge)
	}
	var fingerprint contentFingerprint
	copy(fingerprint[:], hash.Sum(nil))
	return fingerprint, nil
}

func sameFingerprintObservation(left, right fingerprintObservation) bool {
	return left.fingerprint == right.fingerprint && filesystem.SameVersion(left.info, right.info)
}

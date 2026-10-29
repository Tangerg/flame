package toolset

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/qimu/cancellation"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"
)

const readBufferBytes = 64 << 10

// These decorators enforce the readTracker invariant at the model boundary.
// They resolve tool paths against the Run working directory, scope stamps to
// the active session, and return an actionable result when a mutation is stale.

// withReadTracking stamps a full-file fingerprint only when the same digest
// brackets the successful read. The read may return a range, but any concurrent
// whole-file change invalidates the call rather than authorizing unseen bytes.
func withReadTracking(inner toolcontract.Tool, tr *readTracker, root *filesystemRoot) toolcontract.Tool {
	if tr == nil {
		return inner
	}
	return decorateCall(inner, func(ctx context.Context, invocation toolcontract.Invocation) (toolcontract.Result, error) {
		request, decodeErr := decodeToolArguments[fs.ReadRequest](invocation)
		if decodeErr != nil {
			return toolcontract.Result{}, fmt.Errorf("track read arguments: %w", decodeErr)
		}
		path, pathErr := resolveRootPath(root, request.Path)
		if pathErr != nil {
			return toolcontract.Result{}, fmt.Errorf("track read path: %w", pathErr)
		}
		abs := filepath.Join(root.identity, path)
		before, existedBefore, err := observeFingerprintExistingFile(ctx, root, path, maxRuntimeReadFileBytes)
		if err != nil {
			return toolcontract.Result{}, fmt.Errorf("track read %s: %w", request.Path, err)
		}
		out, err := inner.Call(ctx, invocation)
		if err != nil {
			return out, err
		}
		after, existsAfter, err := observeFingerprintExistingFile(ctx, root, path, maxRuntimeReadFileBytes)
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

// Only a stable read may establish read evidence. An admitted mutation consumes
// it before dispatch: neither a write acknowledgement nor a later filesystem
// observation proves what the model has read, including after a partial failure.
func withMutationGuard(inner toolcontract.Tool, tr *readTracker, root *filesystemRoot) toolcontract.Tool {
	if tr == nil {
		return inner
	}
	return decorateCall(inner, func(ctx context.Context, invocation toolcontract.Invocation) (toolcontract.Result, error) {
		paths, err := mutationPaths(inner, invocation)
		if err != nil {
			return toolcontract.Result{}, fmt.Errorf("inspect mutation paths before applying patch: %w", err)
		}
		sessionID := executionctx.SessionID(ctx)
		targets, blocked, err := admitMutationPaths(ctx, tr, root, sessionID, paths)
		if err != nil {
			return toolcontract.Result{}, err
		}
		if blocked != "" {
			failure, err := toolcontract.NewFailure(toolcontract.FailureConfig{
				Kind: toolcontract.FailureKindRejected, Output: chat.NewTextToolOutput(blocked),
			})
			if err != nil {
				return toolcontract.Result{}, err
			}
			return toolcontract.Result{}, failure
		}
		for _, target := range targets {
			tr.forget(sessionID, target)
		}
		return inner.Call(ctx, invocation)
	})
}

func admitMutationPaths(ctx context.Context, tr *readTracker, root *filesystemRoot, sessionID string, paths []string) ([]string, string, error) {
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

// unreadableMutationMessage answers the one case where "read it first" is not
// an instruction the model can follow: the read tool refuses the same file.
func unreadableMutationMessage(path string) string {
	return fmt.Sprintf(
		"%s is larger than the %d MiB read limit, so its current contents cannot be confirmed before modifying it.",
		path, maxRuntimeReadFileBytes>>20,
	)
}

// fingerprintExistingFile stamps a mutation target under the same bound the
// read tool enforces. A larger file can never hold a read stamp, so hashing it
// without that bound would spend the whole file to produce a fingerprint
// nothing can match.
func fingerprintExistingFile(ctx context.Context, root *filesystemRoot, path string) (contentFingerprint, bool, error) {
	observation, exists, err := observeFingerprintExistingFile(ctx, root, path, maxRuntimeReadFileBytes)
	return observation.fingerprint, exists, err
}

type fingerprintObservation struct {
	fingerprint contentFingerprint
	info        os.FileInfo
}

func observeFingerprintExistingFile(ctx context.Context, root *filesystemRoot, path string, maxBytes int64) (_ fingerprintObservation, exists bool, err error) {
	if cause := cancellation.Err(ctx); cause != nil {
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
	if maxBytes > 0 && preflight.Size() > maxBytes {
		return fingerprintObservation{}, false, fmt.Errorf("%w: file uses %d bytes", errRuntimeReadFileTooLarge, preflight.Size())
	}
	file, before, err := filesystem.OpenFileAtExpected(root.Root, path, preflight, maxBytes)
	if err != nil {
		if errors.Is(err, filesystem.ErrFileTooLarge) {
			return fingerprintObservation{}, false, fmt.Errorf("%w: file grew while opening: %w", errRuntimeReadFileTooLarge, err)
		}
		return fingerprintObservation{}, false, err
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()
	hash := sha256.New()
	buffer := make([]byte, readBufferBytes)
	var source io.Reader = cancellation.Reader(ctx, file)
	if maxBytes > 0 {
		source = io.LimitReader(source, maxBytes+1)
	}
	written, err := io.CopyBuffer(hash, source, buffer)
	if err != nil {
		return fingerprintObservation{}, false, err
	}
	if maxBytes > 0 && written > maxBytes {
		return fingerprintObservation{}, false, fmt.Errorf("%w: file grew while fingerprinting", errRuntimeReadFileTooLarge)
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
	var fingerprint contentFingerprint
	copy(fingerprint[:], hash.Sum(nil))
	return fingerprintObservation{fingerprint: fingerprint, info: current}, true, nil
}

func sameFingerprintObservation(left, right fingerprintObservation) bool {
	return left.fingerprint == right.fingerprint && filesystem.SameVersion(left.info, right.info)
}

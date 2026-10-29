package toolset

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/codeintel"
	"github.com/Tangerg/flame/qimu/cancellation"
	"github.com/Tangerg/scope/tools/fs"
)

const maxAutoFormatFileBytes = codeintel.MaxDocumentBytes

var errAutoFormatFileTooLarge = fmt.Errorf("auto-format: file exceeds the %d MiB limit", maxAutoFormatFileBytes>>20)

type autoFormatSource struct {
	content string
	info    os.FileInfo
}

func readAutoFormatFile(ctx context.Context, root *filesystemRoot, executor *fs.LocalExecutor, path string) (autoFormatSource, error) {
	if err := cancellation.Err(ctx); err != nil {
		return autoFormatSource{}, err
	}
	before, err := root.Lstat(path)
	if err != nil {
		return autoFormatSource{}, err
	}
	if err := validateAutoFormatSource(before); err != nil {
		return autoFormatSource{}, err
	}
	result, err := executor.Read(ctx, fs.ReadInput{
		Path: path, MaxInputBytes: maxAutoFormatFileBytes,
		MaxLineBytes: int(maxAutoFormatFileBytes), MaxOutputBytes: int(maxAutoFormatFileBytes),
	})
	if err != nil {
		return autoFormatSource{}, errors.Join(err, cancellation.Err(ctx))
	}
	if result.Truncated {
		return autoFormatSource{}, errors.Join(errAutoFormatFileTooLarge, cancellation.Err(ctx))
	}
	after, err := root.Lstat(path)
	if err != nil || !filesystem.SameVersion(before, after) {
		return autoFormatSource{}, errors.Join(errors.New("file changed while reading for formatting"), err, cancellation.Err(ctx))
	}
	if err := cancellation.Err(ctx); err != nil {
		return autoFormatSource{}, err
	}
	return autoFormatSource{content: result.Content, info: after}, nil
}

func validateAutoFormatSource(info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported file mode %s", info.Mode().Type())
	}
	if info.Size() > maxAutoFormatFileBytes {
		return fmt.Errorf("%w: file uses %d bytes", errAutoFormatFileTooLarge, info.Size())
	}
	return nil
}

// Scope owns atomic replacement, parent-directory handles, and preservation of
// BOM, line endings, and mode. Formatting supplies only an exact-text edit of
// the complete, bounded source it observed.
func applyFormattedFile(ctx context.Context, root *filesystemRoot, executor *fs.LocalExecutor, path string, data []byte, source autoFormatSource) error {
	if err := cancellation.Err(ctx); err != nil {
		return err
	}
	current, err := root.Lstat(path)
	if err != nil || !filesystem.SameVersion(source.info, current) {
		return errors.Join(errors.New("file changed while formatting"), err, cancellation.Err(ctx))
	}
	_, err = executor.Edit(ctx, fs.EditRequest{Path: path, OldString: source.content, NewString: string(data)})
	if err != nil {
		return errors.Join(err, cancellation.Err(ctx))
	}
	return nil
}

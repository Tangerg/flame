package git

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
)

const (
	rawChangeFields = 5
	numstatFields   = 3
)

type FileChange struct {
	Path         string
	Status       Status
	PreviousPath string
	Added        int
	Removed      int
	Binary       bool
}

func ListChanges(ctx context.Context, dir string, maxChanges int) ([]FileChange, error) {
	_, revision, err := diffBaseline(ctx, dir, Worktree)
	if err != nil {
		return nil, err
	}
	comparison := worktreeComparison{revision: revision, includeUntracked: true}
	output, err := comparison.read(ctx, dir, "--raw", "--numstat", "-z")
	if err != nil {
		return nil, err
	}
	return parseChangeSummary(output, maxChanges)
}

type changeSummaryReader struct{ unread []byte }

func (r *changeSummaryReader) field() string {
	field, rest, _ := bytes.Cut(r.unread, []byte{0})
	r.unread = rest
	return string(field)
}

func parseChangeSummary(output worktreeComparisonOutput, maxChanges int) ([]FileChange, error) {
	reader := changeSummaryReader{unread: output.data}
	var files []FileChange
	// Git writes raw records and then numstat records from the same diff queue.
	for len(reader.unread) != 0 && reader.unread[0] == ':' {
		if len(files) >= maxChanges {
			return nil, fmt.Errorf("%w: more than %d workspace changes", ErrResultTooLarge, maxChanges)
		}
		file, err := reader.change()
		if err != nil {
			return nil, err
		}
		file.Status = output.status(file.Path, file.Status)
		files = append(files, file)
	}
	for index := range files {
		if err := reader.lineCounts(&files[index]); err != nil {
			return nil, err
		}
	}
	if len(reader.unread) != 0 {
		return nil, fmt.Errorf("git: numstat contains additional file records")
	}
	return files, nil
}

func (r *changeSummaryReader) change() (FileChange, error) {
	header := strings.Fields(r.field())
	if len(header) != rawChangeFields {
		return FileChange{}, fmt.Errorf("git: malformed raw change header %q", header)
	}
	file := FileChange{Path: r.field()}
	status := header[rawChangeFields-1]
	switch status[0] {
	case 'A':
		file.Status = StatusAdded
	case 'D':
		file.Status = StatusDeleted
	case 'M', 'T', 'U':
		file.Status = StatusModified
	case 'R':
		file.Status = StatusRenamed
		file.PreviousPath, file.Path = file.Path, r.field()
	default:
		return FileChange{}, fmt.Errorf("git: unsupported raw change status %q", status)
	}
	if file.Path == "" || file.Status == StatusRenamed && file.PreviousPath == "" {
		return FileChange{}, fmt.Errorf("git: raw change has a missing path")
	}
	return file, nil
}

func (r *changeSummaryReader) lineCounts(file *FileChange) error {
	record := r.field()
	parts := strings.SplitN(record, "\t", numstatFields)
	if len(parts) != numstatFields {
		return fmt.Errorf("git: malformed numstat record %q for %q", record, file.Path)
	}
	path := parts[2]
	if file.Status == StatusRenamed && path == "" {
		previous := r.field()
		path = r.field()
		if previous != file.PreviousPath {
			return fmt.Errorf("git: numstat rename source %q differs from %q", previous, file.PreviousPath)
		}
	}
	if path != file.Path {
		return fmt.Errorf("git: numstat path %q differs from %q", path, file.Path)
	}
	if parts[0] == "-" && parts[1] == "-" {
		file.Binary = true
		return nil
	}
	added, addErr := strconv.Atoi(parts[0])
	removed, removeErr := strconv.Atoi(parts[1])
	if addErr != nil || removeErr != nil || added < 0 || removed < 0 {
		return fmt.Errorf("git: invalid numstat counts in %q", record)
	}
	file.Added, file.Removed = added, removed
	return nil
}

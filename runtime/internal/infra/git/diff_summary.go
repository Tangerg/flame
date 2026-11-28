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
	comparison, _, err := resolveComparison(ctx, dir, "", Worktree)
	if err != nil {
		return nil, err
	}
	output, err := comparison.read(ctx, dir, changeSummaryFlags()...)
	if err != nil {
		return nil, err
	}
	summary, err := output.summarize(maxChanges)
	if err != nil {
		return nil, err
	}
	if summary.truncated {
		return nil, fmt.Errorf("%w: more than %d workspace changes", ErrResultTooLarge, maxChanges)
	}
	files := make([]FileChange, 0, len(summary.files))
	for _, file := range summary.files {
		files = append(files, file.FileChange)
	}
	return files, nil
}

func changeSummaryFlags() []string { return []string{"--raw", "--numstat", "-z"} }

type comparisonFile struct {
	FileChange
	typeChanged bool
}

type comparisonSummary struct {
	files     []comparisonFile
	patch     []byte
	truncated bool
}

type changeSummaryReader struct{ unread []byte }

func (r *changeSummaryReader) field() string {
	field, rest, _ := bytes.Cut(r.unread, []byte{0})
	r.unread = rest
	return string(field)
}

func (output worktreeComparisonOutput) summarize(maxChanges int) (comparisonSummary, error) {
	// Git uses a blank NUL-delimited record to separate metadata from the patch.
	metadata, patch, _ := bytes.Cut(output.data, []byte{0, 0})
	summary := comparisonSummary{patch: patch}
	reader := changeSummaryReader{unread: metadata}
	for len(reader.unread) != 0 && reader.unread[0] == ':' {
		file, err := reader.change()
		if err != nil {
			return comparisonSummary{}, err
		}
		if len(summary.files) >= maxChanges {
			summary.truncated = true
			continue
		}
		file.Status = output.status(file.Path, file.Status)
		summary.files = append(summary.files, file)
	}
	// Raw and numstat records have the same order because Git emits both from one diff queue.
	for index := range summary.files {
		if err := reader.lineCounts(&summary.files[index].FileChange); err != nil {
			return comparisonSummary{}, err
		}
	}
	if !summary.truncated && len(reader.unread) != 0 {
		return comparisonSummary{}, fmt.Errorf("git: numstat contains additional file records")
	}
	return summary, nil
}

func (r *changeSummaryReader) change() (comparisonFile, error) {
	header := strings.Fields(r.field())
	if len(header) != rawChangeFields {
		return comparisonFile{}, fmt.Errorf("git: malformed raw change header %q", header)
	}
	file := comparisonFile{FileChange: FileChange{Path: r.field()}}
	status := header[rawChangeFields-1]
	switch status[0] {
	case 'A':
		file.Status = StatusAdded
	case 'D':
		file.Status = StatusDeleted
	case 'M', 'U':
		file.Status = StatusModified
	case 'T':
		file.Status = StatusModified
		file.typeChanged = true
	case 'R':
		file.Status = StatusRenamed
		file.PreviousPath, file.Path = file.Path, r.field()
	default:
		return comparisonFile{}, fmt.Errorf("git: unsupported raw change status %q", status)
	}
	if file.Path == "" || file.Status == StatusRenamed && file.PreviousPath == "" {
		return comparisonFile{}, fmt.Errorf("git: raw change has a missing path")
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

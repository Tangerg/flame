package workspace

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type Change struct {
	Path         string
	Status       protocol.FileStatus
	PreviousPath string
	Added        *int
	Removed      *int
	Binary       bool
}

func (c Change) Stat() string {
	if c.Binary {
		return "binary"
	}
	parts := make([]string, 0, 2)
	if c.Added != nil {
		parts = append(parts, fmt.Sprintf("+%d", *c.Added))
	}
	if c.Removed != nil {
		parts = append(parts, fmt.Sprintf("-%d", *c.Removed))
	}
	return strings.Join(parts, " ")
}

type DiffRequest struct {
	Workspace string
	Path      string
	Mode      protocol.DiffMode
	Format    protocol.DiffFormat
	RowLimit  DiffRowLimit
}

// DiffRowLimit is an optional positive row budget. Zero leaves the default
// with Runtime; construction validates an explicit request once.
type DiffRowLimit struct{ rows int }

func DefaultDiffRowLimit() DiffRowLimit { return DiffRowLimit{} }

func NewDiffRowLimit(rows int) (DiffRowLimit, error) {
	if rows <= 0 {
		return DiffRowLimit{}, errors.New("workspace diff row limit must be positive")
	}
	return DiffRowLimit{rows: rows}, nil
}

func (l DiffRowLimit) Rows() int { return l.rows }

func (d DiffRequest) Validate() error {
	if strings.TrimSpace(d.Workspace) == "" {
		return errors.New("workspace diff workspace is empty")
	}
	var limit *int
	if rows := d.RowLimit.Rows(); rows > 0 {
		limit = &rows
	}
	if err := (protocol.GetDiffRequest{
		Workspace: protocol.WorkspaceRef{Path: d.Workspace}, Path: d.Path,
		Mode: d.Mode, Format: d.Format, Limit: limit,
	}).ValidateWire(); err != nil {
		return fmt.Errorf("workspace diff: %w", err)
	}
	return nil
}

type FileDiff struct {
	Change
	Rows []protocol.DiffRow
}

type Diff struct {
	Baseline  protocol.DiffBaseline
	Files     []FileDiff
	Patch     string
	Truncated bool
}

func (d Diff) Validate() error {
	if err := protocol.ValidateWireTree(d.Baseline); err != nil {
		return fmt.Errorf("workspace diff baseline: %w", err)
	}
	if d.Patch != "" && len(d.Files) != 0 {
		return errors.New("workspace diff mixes raw and structured representations")
	}
	paths := make(map[string]struct{}, len(d.Files))
	for index, file := range d.Files {
		if _, duplicate := paths[file.Path]; duplicate {
			return fmt.Errorf("file diff %d repeats path %q", index, file.Path)
		}
		paths[file.Path] = struct{}{}
		if file.Binary && len(file.Rows) != 0 {
			return fmt.Errorf("file diff %d: binary file has text rows", index)
		}
	}
	return nil
}

// Text returns the raw patch when available and otherwise renders the complete
// structured rows without inventing line content.
func (d Diff) Text() string {
	if d.Patch != "" {
		return d.Patch
	}
	var output strings.Builder
	for index, file := range d.Files {
		if index > 0 {
			output.WriteByte('\n')
		}
		fmt.Fprintf(&output, "diff -- %s (%s)\n", file.Path, file.Status)
		for _, row := range file.Rows {
			switch row.Type {
			case protocol.DiffRowHunk:
				output.WriteString(row.Text)
			case protocol.DiffRowAdded:
				output.WriteByte('+')
				output.WriteString(*row.Code)
			case protocol.DiffRowDeleted:
				output.WriteByte('-')
				output.WriteString(*row.Code)
			case protocol.DiffRowContext:
				output.WriteByte(' ')
				output.WriteString(*row.Code)
			}
			output.WriteByte('\n')
		}
	}
	return strings.TrimSuffix(output.String(), "\n")
}

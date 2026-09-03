package workspace

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/protocol"
)

type Change struct {
	Path         string
	Status       protocol.FileStatus
	PreviousPath string
	Added        *int
	Removed      *int
	Binary       bool
}

func (c Change) Validate() error {
	wire := protocol.WorkspaceFileChange{
		Path: c.Path, Status: c.Status, PreviousPath: c.PreviousPath,
		Added: c.Added, Removed: c.Removed, Binary: c.Binary,
	}
	switch {
	case strings.TrimSpace(c.Path) == "":
		return errors.New("changed file path is empty")
	}
	if err := wire.ValidateWire(); err != nil {
		return fmt.Errorf("changed file: %w", err)
	}
	return nil
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

// DiffRowLimit is the optional positive row budget for a structured diff. Its
// zero value is invalid; callers explicitly choose an absent default or a
// present positive budget.
type DiffRowLimit struct {
	kind requestLimitKind
	rows int
}

func DefaultDiffRowLimit() DiffRowLimit { return DiffRowLimit{kind: defaultRequestLimit} }

func NewDiffRowLimit(rows int) (DiffRowLimit, error) {
	if rows <= 0 {
		return DiffRowLimit{}, errors.New("workspace diff row limit must be positive")
	}
	return DiffRowLimit{kind: explicitRequestLimit, rows: rows}, nil
}

func (l DiffRowLimit) Rows() (int, bool, error) {
	if err := l.Validate(); err != nil {
		return 0, false, err
	}
	return l.rows, l.kind == explicitRequestLimit, nil
}

func (l DiffRowLimit) Validate() error {
	switch l.kind {
	case explicitRequestLimit:
		if l.rows <= 0 {
			return errors.New("workspace diff row limit must be positive")
		}
		return nil
	case defaultRequestLimit:
		if l.rows != 0 {
			return errors.New("workspace diff default row limit carries a value")
		}
		return nil
	default:
		return errors.New("workspace diff row limit kind is unknown")
	}
}

func (d DiffRequest) Validate() error {
	if strings.TrimSpace(d.Workspace) == "" {
		return errors.New("workspace diff workspace is empty")
	}
	if err := d.RowLimit.Validate(); err != nil {
		return err
	}
	rows, explicit, _ := d.RowLimit.Rows()
	var limit *int
	if explicit {
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

type DiffRow struct {
	Type      protocol.DiffRowType
	Text      string
	LeftLine  int
	RightLine int
	Code      string
}

func (d DiffRow) Validate() error {
	if err := (protocol.DiffRow{
		Type: d.Type, Text: d.Text, LeftLine: d.LeftLine, RightLine: d.RightLine, Code: d.Code,
	}).ValidateWire(); err != nil {
		return fmt.Errorf("diff row: %w", err)
	}
	return nil
}

type FileDiff struct {
	Change
	Rows []DiffRow
}

type Diff struct {
	Files     []FileDiff
	Patch     string
	Truncated bool
}

func (d Diff) Validate() error {
	if d.Patch != "" && len(d.Files) != 0 {
		return errors.New("workspace diff mixes raw and structured representations")
	}
	paths := make(map[string]struct{}, len(d.Files))
	for index, file := range d.Files {
		if err := file.Validate(); err != nil {
			return fmt.Errorf("file diff %d: %w", index, err)
		}
		if _, duplicate := paths[file.Path]; duplicate {
			return fmt.Errorf("file diff %d repeats path %q", index, file.Path)
		}
		paths[file.Path] = struct{}{}
		if file.Binary && len(file.Rows) != 0 {
			return fmt.Errorf("file diff %d: binary file has text rows", index)
		}
		for rowIndex, row := range file.Rows {
			if err := row.Validate(); err != nil {
				return fmt.Errorf("file diff %d row %d: %w", index, rowIndex, err)
			}
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
				output.WriteString(row.Code)
			case protocol.DiffRowDeleted:
				output.WriteByte('-')
				output.WriteString(row.Code)
			case protocol.DiffRowContext:
				output.WriteByte(' ')
				output.WriteString(row.Code)
			}
			output.WriteByte('\n')
		}
	}
	return strings.TrimSuffix(output.String(), "\n")
}

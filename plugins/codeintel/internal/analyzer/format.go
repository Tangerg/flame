package analyzer

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/plugins/codeintel/internal/lsp"
)

func toPosition(line, column int) lsp.Position {
	if line < 1 {
		line = 1
	}
	if column < 1 {
		column = 1
	}
	return lsp.Position{Line: line - 1, Character: column - 1}
}

func diagnosticsSection(file string, diags []lsp.Diagnostic) string {
	var lines []string
	for _, d := range diags {
		if !d.Severity.IsProblem() {
			continue
		}
		sev := d.SeverityName()
		if sev == "" {
			sev = "error" // unset severity → treat as error per the LSP default
		}
		line := fmt.Sprintf("%s %s:%d:%d: %s", sev, file, d.Range.Start.Line+1, d.Range.Start.Character+1, d.Message)
		if d.Source != "" {
			line += fmt.Sprintf(" [%s]", d.Source)
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}
	return fmt.Sprintf("Language server flagged %d new problem(s) in %s after this edit:\n%s", len(lines), file, strings.Join(lines, "\n"))
}

func relPath(root, abs string) string {
	if rel, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return abs
}

func formatLocations(root string, locs []lsp.Location, kind string) string {
	if len(locs) == 0 {
		return fmt.Sprintf("No %s found.", kind)
	}
	var b strings.Builder
	for _, l := range locs {
		fmt.Fprintf(&b, "%s:%d:%d\n", relPath(root, l.Path()), l.Range.Start.Line+1, l.Range.Start.Character+1)
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatSymbols(root string, syms []lsp.Symbol) string {
	if len(syms) == 0 {
		return "No symbols found."
	}
	var b strings.Builder
	for _, s := range syms {
		fmt.Fprintf(&b, "%s %s", s.Kind, s.Name)
		if s.Detail != "" {
			fmt.Fprintf(&b, " %s", s.Detail)
		}
		if s.Container != "" {
			fmt.Fprintf(&b, " (in %s)", s.Container)
		}
		fmt.Fprintf(&b, " — %s:%d:%d\n", relPath(root, s.Location.Path()), s.Location.Range.Start.Line+1, s.Location.Range.Start.Character+1)
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatCalls(root string, syms []lsp.Symbol, kind string) string {
	if len(syms) == 0 {
		return fmt.Sprintf("No %s found.", kind)
	}
	return formatSymbols(root, syms)
}

func formatDiagnostics(file string, diags []lsp.Diagnostic) string {
	if len(diags) == 0 {
		return fmt.Sprintf("No diagnostics for %s.", file)
	}
	var b strings.Builder
	for _, d := range diags {
		sev := d.SeverityName()
		if sev == "" {
			sev = "note"
		}
		fmt.Fprintf(&b, "%s %s:%d:%d: %s", sev, file, d.Range.Start.Line+1, d.Range.Start.Character+1, d.Message)
		if d.Source != "" {
			fmt.Fprintf(&b, " [%s]", d.Source)
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

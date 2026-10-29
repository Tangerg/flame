package analyzer

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/codeintel"
	"github.com/Tangerg/flame/plugins/codeintel/internal/lsp"
)

func (a *Analyzer) Inspect(ctx context.Context, input codeintel.Inspection) (codeintel.Report, error) {
	if err := input.Validate(); err != nil {
		return codeintel.Report{}, err
	}
	document := input.Document
	report := codeintel.Report{Version: document.Version}
	if !a.Supported(document.Path) {
		return report, nil
	}
	diagnostics, err := a.servers.InspectDocument(ctx, document.Workspace, document.Path, []byte(document.Content))
	if err != nil {
		return report, err
	}
	remaining := make(map[string]int, len(input.Baseline))
	for _, key := range input.Baseline {
		remaining[key]++
	}
	var introduced []lsp.Diagnostic
	for _, diagnostic := range diagnostics {
		if !diagnostic.Severity.IsProblem() {
			continue
		}
		key := problemKey(diagnostic)
		report.Problems = append(report.Problems, key)
		if remaining[key] > 0 {
			remaining[key]--
			continue
		}
		introduced = append(introduced, diagnostic)
	}
	report.Summary = diagnosticsSection(document.Path, introduced)
	return report, report.Validate(document)
}

func problemKey(d lsp.Diagnostic) string {
	code := fmt.Sprint(d.Code)
	identity := fmt.Sprintf("%d:%d:%s%d:%s%d:%s", d.Severity, len(d.Source), d.Source, len(code), code, len(d.Message), d.Message)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
}

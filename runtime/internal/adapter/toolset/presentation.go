package toolset

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/builtin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/infra/git"
	"github.com/Tangerg/scope/tools/content"
)

// Presenter owns the client-facing projection of concrete tool schemas. Its
// zero value is ready for use. Unknown tools retain their canonical result and
// let the execution adapter supply generic activity text.
type Presenter struct{}

// PresentationContract describes one built-in tool result after Presenter has
// projected it into the transcript. ResultType is the exact root object carried
// by ToolInvocation.result; EnumValues supplies the closed string vocabularies
// reflection cannot discover.
type PresentationContract struct {
	ToolName   string
	ResultType reflect.Type
	EnumValues map[reflect.Type][]string
}

func PresentationContracts() []PresentationContract {
	var contracts []PresentationContract
	for name, descriptor := range descriptors() {
		if descriptor.result.project == nil {
			continue
		}
		contract := PresentationContract{ToolName: string(name), ResultType: descriptor.result.resultType}
		if descriptor.result.enums != nil {
			contract.EnumValues = descriptor.result.enums()
		}
		contracts = append(contracts, contract)
	}
	return contracts
}

// Activity returns concise progress text for a known concrete tool.
func (Presenter) Activity(ref tool.Ref, arguments tool.Arguments) string {
	descriptor, ok := descriptorFor(ref)
	if !ok {
		return ""
	}
	if descriptor.activity != nil {
		return descriptor.activity(arguments)
	}
	return descriptor.activityText
}

func shellActivity(arguments tool.Arguments) string {
	if args, ok := decodeArguments[builtincontract.ShellArguments](arguments, "description"); ok &&
		isConciseActivityText(args.Description, 120) {
		return args.Description
	}
	return "Running command"
}

func delegationActivity(arguments tool.Arguments) string {
	if args, ok := decodeArguments[delegationActivityArguments](arguments, "summary"); ok &&
		isConciseActivityText(args.Summary, 80) {
		return "Delegating: " + args.Summary
	}
	return "Delegating to a sub-agent"
}

func loadSkillActivity(arguments tool.Arguments) string {
	if args, ok := decodeArguments[skillActivityArguments](arguments, "name"); ok &&
		isConciseActivityText(args.Name, 80) {
		return "Loading Skill: " + args.Name
	}
	return "Loading a Skill"
}

func proposeSkillActivity(arguments tool.Arguments) string {
	if args, ok := decodeArguments[skillActivityArguments](arguments, "name"); ok &&
		isConciseActivityText(args.Name, 64) {
		return "Proposing Skill: " + args.Name
	}
	return "Proposing a Skill"
}

func lspActivity(arguments tool.Arguments) string {
	if args, ok := decodeArguments[lspActivityArguments](arguments, "operation"); ok {
		if activity := lspOperationActivity(args.Operation); activity != "" {
			return activity
		}
	}
	return "Querying the language server"
}

func httpActivity(arguments tool.Arguments) string {
	if args, ok := decodeArguments[httpActivityArguments](arguments, "url"); ok {
		if method := httpMethod(args.Method); method != "" {
			return "Sending " + method + " request"
		}
	}
	return "Sending an HTTP request"
}

type delegationActivityArguments struct {
	Summary string `json:"summary"`
}

type skillActivityArguments struct {
	Name string `json:"name"`
}

type lspActivityArguments struct {
	Operation string `json:"operation"`
}

type httpActivityArguments struct {
	URL    string `json:"url"`
	Method string `json:"method"`
}

func isConciseActivityText(value string, maxRunes int) bool {
	return value != "" && value == strings.TrimSpace(value) && utf8.RuneCountInString(value) <= maxRunes
}

// lspOperationActivity phrases the operation the tool declares. The vocabulary
// belongs to the tool, so name its values rather than respelling them: a
// renamed operation must break here rather than silently fall through to the
// generic label.
func lspOperationActivity(operation string) string {
	switch builtin.LSPOperation(operation) {
	case builtin.LSPDefinition:
		return "Finding a symbol definition"
	case builtin.LSPReferences:
		return "Finding symbol references"
	case builtin.LSPImplementation:
		return "Finding symbol implementations"
	case builtin.LSPHover:
		return "Inspecting a symbol"
	case builtin.LSPIncomingCalls:
		return "Finding incoming calls"
	case builtin.LSPOutgoingCalls:
		return "Finding outgoing calls"
	case builtin.LSPDocumentSymbols:
		return "Listing document symbols"
	case builtin.LSPWorkspaceSymbols:
		return "Searching workspace symbols"
	case builtin.LSPDiagnostics:
		return "Checking file diagnostics"
	default:
		return ""
	}
}

func httpMethod(method string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return "GET"
	}
	switch method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE":
		return method
	default:
		return ""
	}
}

// Present projects a known tool's canonical arguments and result into the
// client transcript shape. The second result is optional plain output for
// clients that render command text separately.
func (Presenter) Present(ref tool.Ref, arguments tool.Arguments, result tool.Result) (tool.Result, string) {
	descriptor, ok := descriptorFor(ref)
	if !ok || descriptor.result.project == nil {
		return result, ""
	}
	return descriptor.result.project(arguments, result)
}

func presentCommand(_ tool.Arguments, result tool.Result) (tool.Result, string) {
	return presentCommandResult(result)
}

func presentSearch(_ tool.Arguments, result tool.Result) (tool.Result, string) {
	return presentSearchResult(result), ""
}

func presentApplyPatch(arguments tool.Arguments, result tool.Result) (tool.Result, string) {
	return presentApplyPatchResult(arguments, result), ""
}

func commandResultContract() resultProjectionContract {
	return resultProjectionContract{project: presentCommand, resultType: reflect.TypeFor[builtincontract.CommandResult]()}
}

// commandExecutionResult is the shell tool's own result shape. The shell
// captures one interleaved stream, so there is no separate stderr to carry.
type commandExecutionResult struct {
	Stdout   content.Content `json:"stdout"`
	ExitCode *int            `json:"exit_code"`
}

func presentCommandResult(result tool.Result) (tool.Result, string) {
	if existing, ok := decodeResult[builtincontract.CommandResult](result, "output"); ok {
		return result, existing.Output
	}
	raw, ok := decodeResult[commandExecutionResult](result, "stdout", "exit_code")
	if !ok {
		return result, ""
	}
	output, text := raw.Stdout.Text()
	if !text {
		encoded, err := json.Marshal(raw.Stdout)
		if err != nil {
			return result, ""
		}
		output = string(encoded)
	}
	presentation := builtincontract.CommandResult{Output: output, ExitCode: raw.ExitCode}
	return projectResult(result, presentation), output
}

func searchResultContract() resultProjectionContract {
	return resultProjectionContract{project: presentSearch, resultType: reflect.TypeFor[builtincontract.SearchResult]()}
}

type localSearchExecutionResult struct {
	Matches []localSearchMatch `json:"matches"`
	Files   []string           `json:"files"`
	Paths   []string           `json:"paths"`
	Counts  []localSearchCount `json:"counts"`
}

type localSearchMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type localSearchCount struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

func presentSearchResult(result tool.Result) tool.Result {
	if _, ok := decodeResult[builtincontract.SearchResult](result, "hits"); ok {
		return result
	}
	raw, ok := decodeResult[localSearchExecutionResult](result, "matches", "files", "paths", "counts")
	if !ok {
		return result
	}
	hits := make([]builtincontract.SearchHit, 0, len(raw.Matches)+len(raw.Files)+len(raw.Paths)+len(raw.Counts))
	for _, match := range raw.Matches {
		hits = append(hits, builtincontract.SearchHit{Path: match.Path, Snippet: match.Text, LineNumber: match.Line})
	}
	for _, path := range raw.Files {
		hits = append(hits, builtincontract.SearchHit{Path: path})
	}
	for _, path := range raw.Paths {
		hits = append(hits, builtincontract.SearchHit{Path: path})
	}
	for _, count := range raw.Counts {
		hits = append(hits, builtincontract.SearchHit{Path: count.Path, Snippet: strconv.Itoa(count.Count) + " matches"})
	}
	return projectResult(result, builtincontract.SearchResult{Hits: hits})
}

func patchResultContract() resultProjectionContract {
	return resultProjectionContract{
		project: presentApplyPatch, resultType: reflect.TypeFor[builtincontract.PatchResult](), enums: patchResultEnums,
	}
}

func patchResultEnums() map[reflect.Type][]string {
	return map[reflect.Type][]string{
		reflect.TypeFor[builtincontract.ChangeStatus](): builtincontract.ChangeStatus("").Values(),
	}
}

type patchExecutionResult struct {
	Files []struct {
		Path      string `json:"path"`
		Created   bool   `json:"created"`
		Deleted   bool   `json:"deleted"`
		MovedFrom string `json:"moved_from"`
	} `json:"files"`
}

func presentApplyPatchResult(arguments tool.Arguments, result tool.Result) tool.Result {
	if _, ok := decodeResult[builtincontract.PatchResult](result, "changes"); ok {
		return result
	}
	decoded, ok := decodeResult[patchExecutionResult](result, "files")
	if !ok || len(decoded.Files) == 0 {
		return result
	}
	// The executor reports the paths it actually touched; the patch argument
	// carries the line counts the executor does not. Joining by path keeps the
	// applied set authoritative while projecting a diffstat for each file.
	counts := patchLineCounts(arguments)
	changes := make([]builtincontract.AppliedChange, 0, len(decoded.Files))
	for _, file := range decoded.Files {
		status := builtincontract.Modified
		switch {
		case file.MovedFrom != "":
			status = builtincontract.Moved
		case file.Created:
			status = builtincontract.Added
		case file.Deleted:
			status = builtincontract.Deleted
		}
		count := counts[file.Path]
		changes = append(changes, builtincontract.AppliedChange{
			Path: file.Path, Status: status, From: file.MovedFrom,
			Added: count.added, Removed: count.removed,
		})
	}
	return projectResult(result, builtincontract.PatchResult{Changes: changes})
}

type patchLineCount struct{ added, removed int }

// patchLineCounts projects the per-file added/removed line counts of the patch
// argument, keyed by the file path the patch names. A missing or unparseable
// argument yields no counts rather than a guess.
func patchLineCounts(arguments tool.Arguments) map[string]patchLineCount {
	args, ok := decodeArguments[applyPatchArguments](arguments, "patch")
	if !ok || args.Patch == "" {
		return nil
	}
	files, err := git.SummarizePatch([]byte(args.Patch))
	if err != nil {
		return nil
	}
	counts := make(map[string]patchLineCount, len(files))
	for _, file := range files {
		counts[file.Path] = patchLineCount{added: file.Added, removed: file.Removed}
	}
	return counts
}

type applyPatchArguments struct {
	Patch string `json:"patch"`
}

func decodeResult[T any](result tool.Result, knownFields ...string) (T, bool) {
	data, err := json.Marshal(result)
	if err != nil {
		return *new(T), false
	}
	return decodePresentation[T](data, knownFields...)
}

func decodeArguments[T any](arguments tool.Arguments, knownFields ...string) (T, bool) {
	return decodePresentation[T]([]byte(arguments.Canonical()), knownFields...)
}

func decodePresentation[T any](data []byte, knownFields ...string) (T, bool) {
	var decoded T
	var fields map[string]jsontext.Value
	if err := json.Unmarshal(data, &fields); err != nil {
		return decoded, false
	}
	for _, field := range knownFields {
		if _, ok := fields[field]; !ok {
			continue
		}
		if json.Unmarshal(data, &decoded) == nil {
			return decoded, true
		}
		return decoded, false
	}
	return decoded, false
}

type presentationValue interface {
	builtincontract.CommandResult | builtincontract.SearchResult | builtincontract.PatchResult
}

func projectResult[T presentationValue](original tool.Result, value T) tool.Result {
	projected, err := tool.NewResult(value)
	if err != nil {
		return original
	}
	return projected
}

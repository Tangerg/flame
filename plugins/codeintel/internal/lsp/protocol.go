package lsp

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

func (l Location) Path() string { return uriToPath(l.URI) }

type DiagnosticSeverity uint8

const (
	diagnosticSeverityUnspecified DiagnosticSeverity = iota
	diagnosticSeverityError
	diagnosticSeverityWarning
	diagnosticSeverityInformation
	diagnosticSeverityHint
)

func (d DiagnosticSeverity) IsProblem() bool {
	return d <= diagnosticSeverityWarning
}

func (d DiagnosticSeverity) name() string {
	switch d {
	case diagnosticSeverityError:
		return "error"
	case diagnosticSeverityWarning:
		return "warning"
	case diagnosticSeverityInformation:
		return "info"
	case diagnosticSeverityHint:
		return "hint"
	default:
		return ""
	}
}

type Diagnostic struct {
	Range    Range              `json:"range"`
	Severity DiagnosticSeverity `json:"severity"`
	Code     any                `json:"code,omitempty"`
	Source   string             `json:"source,omitempty"`
	Message  string             `json:"message"`
}

func (d Diagnostic) SeverityName() string {
	return d.Severity.name()
}

// Values follow the LSP SymbolKind enumeration, which numbers kinds from 1.
type SymbolKind int

const (
	symbolFile SymbolKind = iota + 1
	symbolModule
	symbolNamespace
	symbolPackage
	symbolClass
	symbolMethod
	symbolProperty
	symbolField
	symbolConstructor
	symbolEnum
	symbolInterface
	symbolFunction
	symbolVariable
	symbolConstant
	symbolString
	symbolNumber
	symbolBoolean
	symbolArray
	symbolObject
	symbolKey
	symbolNull
	symbolEnumMember
	symbolStruct
	symbolEvent
	symbolOperator
	symbolTypeParameter
)

var symbolKindNames = [...]string{
	symbolFile: "file", symbolModule: "module", symbolNamespace: "namespace", symbolPackage: "package",
	symbolClass: "class", symbolMethod: "method", symbolProperty: "property", symbolField: "field",
	symbolConstructor: "constructor", symbolEnum: "enum", symbolInterface: "interface", symbolFunction: "function",
	symbolVariable: "variable", symbolConstant: "constant", symbolString: "string", symbolNumber: "number",
	symbolBoolean: "boolean", symbolArray: "array", symbolObject: "object", symbolKey: "key", symbolNull: "null",
	symbolEnumMember: "enum-member", symbolStruct: "struct", symbolEvent: "event", symbolOperator: "operator",
	symbolTypeParameter: "type-parameter",
}

// Servers may report kinds newer than this enumeration; they render generically.
func (k SymbolKind) String() string {
	if k < symbolFile || int(k) >= len(symbolKindNames) {
		return "symbol"
	}
	return symbolKindNames[k]
}

type Symbol struct {
	Name      string
	Kind      SymbolKind
	Location  Location
	Container string
	Detail    string
}

type initializeParams struct {
	ProcessID        int                `json:"processId"`
	RootURI          string             `json:"rootUri"`
	Capabilities     clientCapabilities `json:"capabilities"`
	WorkspaceFolders []workspaceFolder  `json:"workspaceFolders,omitempty"`
}

type workspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

type textDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

type versionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type didOpenParams struct {
	TextDocument textDocumentItem `json:"textDocument"`
}

type contentChange struct {
	Range *Range `json:"range,omitzero"`
	Text  string `json:"text"`
}

type didCloseParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type didChangeParams struct {
	TextDocument   versionedTextDocumentIdentifier `json:"textDocument"`
	ContentChanges []contentChange                 `json:"contentChanges"`
}

type positionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type referenceParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	Context      referenceContext       `json:"context"`
}

type referenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

type didSaveParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Text         *string                `json:"text,omitzero"`
}

type documentSymbolParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type workspaceSymbolParams struct {
	Query string `json:"query"`
}

type configurationItem struct {
	ScopeURI string `json:"scopeUri,omitempty"`
	Section  string `json:"section,omitempty"`
}

type callHierarchyItem struct {
	Name           string         `json:"name"`
	Kind           SymbolKind     `json:"kind"`
	URI            string         `json:"uri"`
	Range          Range          `json:"range"`
	SelectionRange Range          `json:"selectionRange"`
	Detail         string         `json:"detail,omitempty"`
	Data           jsontext.Value `json:"data,omitzero"`
}

func (c callHierarchyItem) symbol() Symbol {
	return Symbol{
		Name:     c.Name,
		Kind:     c.Kind,
		Detail:   c.Detail,
		Location: Location{URI: c.URI, Range: c.SelectionRange},
	}
}

type callHierarchyItemParams struct {
	Item jsontext.Value `json:"item"`
}

type callHierarchyIncomingCall struct {
	From callHierarchyItem `json:"from"`
}

type callHierarchyOutgoingCall struct {
	To callHierarchyItem `json:"to"`
}

type publishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Version     int          `json:"version"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

func (p publishDiagnosticsParams) validate() error {
	if p.Version < 0 {
		return errors.New("diagnostics version is negative")
	}
	if strings.TrimSpace(p.URI) == "" {
		return errors.New("uri is empty")
	}
	if p.Diagnostics == nil {
		return errors.New("diagnostics must be an array")
	}
	for index, diagnostic := range p.Diagnostics {
		if err := diagnostic.validate(); err != nil {
			return fmt.Errorf("diagnostic %d: %w", index, err)
		}
	}
	return nil
}

func (d Diagnostic) validate() error {
	if d.Severity > diagnosticSeverityHint {
		return fmt.Errorf("unknown severity %d", d.Severity)
	}
	if err := d.Range.validate(); err != nil {
		return fmt.Errorf("range: %w", err)
	}
	return nil
}

func (r Range) validate() error {
	if err := r.Start.validate(); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	if err := r.End.validate(); err != nil {
		return fmt.Errorf("end: %w", err)
	}
	if r.End.Line < r.Start.Line || (r.End.Line == r.Start.Line && r.End.Character < r.Start.Character) {
		return errors.New("end precedes start")
	}
	return nil
}

func (p Position) validate() error {
	if p.Line < 0 {
		return errors.New("line is negative")
	}
	if p.Character < 0 {
		return errors.New("character is negative")
	}
	return nil
}

type markupKind string

const (
	markupMarkdown  markupKind = "markdown"
	markupPlaintext markupKind = "plaintext"
)

func pathToURI(p string) string {
	p = filepath.ToSlash(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	u := url.URL{Scheme: "file", Path: p}
	return u.String()
}

func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	p := u.Path
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

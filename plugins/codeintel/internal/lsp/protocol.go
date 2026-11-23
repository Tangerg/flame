package lsp

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
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

type saveSynchronization struct {
	Enabled     bool
	IncludeText bool
}

type syncKind int

const (
	changeNone syncKind = iota
	changeFull
	changeIncremental
)

type documentSynchronization struct {
	OpenClose bool                `json:"openClose"`
	Change    syncKind            `json:"change"`
	Save      saveSynchronization `json:"-"`
}

func negotiateDocumentSync(raw jsontext.Value) (documentSynchronization, error) {
	var policy documentSynchronization
	var err error
	switch {
	case len(raw) == 0 || raw.Kind() == 'n':
		return policy, nil
	case raw.Kind() == '{':
		if err = json.Unmarshal(raw, &policy); err == nil {
			policy.Save, err = negotiatedSave(raw)
		}
	default:
		err = json.Unmarshal(raw, &policy.Change)
		policy.OpenClose = policy.Change != changeNone
	}
	if err != nil || policy.Change < changeNone || policy.Change > changeIncremental {
		return documentSynchronization{}, errors.Join(errors.New("lsp: invalid document synchronization capability"), err)
	}
	return policy, nil
}

func negotiatedSave(sync jsontext.Value) (saveSynchronization, error) {
	var options struct {
		Save jsontext.Value `json:"save"`
	}
	if len(sync) == 0 || sync.Kind() != '{' {
		return saveSynchronization{}, nil
	}
	if err := json.Unmarshal(sync, &options); err != nil {
		return saveSynchronization{}, err
	}
	if len(options.Save) == 0 {
		return saveSynchronization{}, nil
	}
	switch options.Save.Kind() {
	case 't':
		return saveSynchronization{Enabled: true}, nil
	case 'f', 'n':
		return saveSynchronization{}, nil
	case '{':
		var detail struct {
			IncludeText bool `json:"includeText"`
		}
		if err := json.Unmarshal(options.Save, &detail); err != nil {
			return saveSynchronization{}, err
		}
		return saveSynchronization{Enabled: true, IncludeText: detail.IncludeText}, nil
	default:
		return saveSynchronization{}, errors.New("invalid server save synchronization option")
	}
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

type symbolInformation struct {
	Name          string     `json:"name"`
	Kind          SymbolKind `json:"kind"`
	Location      *Location  `json:"location"`
	ContainerName string     `json:"containerName"`
}

type locationLink struct {
	TargetURI            *string `json:"targetUri"`
	TargetRange          *Range  `json:"targetRange"`
	TargetSelectionRange *Range  `json:"targetSelectionRange"`
}

type documentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail"`
	Kind           SymbolKind       `json:"kind"`
	Range          *Range           `json:"range"`
	SelectionRange *Range           `json:"selectionRange"`
	Children       []documentSymbol `json:"children"`
}

type markupKind string

const (
	markupMarkdown  markupKind = "markdown"
	markupPlaintext markupKind = "plaintext"
)

type supportedCapability struct{}

type diagnosticClientCapabilities struct {
	VersionSupport bool `json:"versionSupport"`
}

type clientCapabilities struct {
	TextDocument textDocumentClientCapabilities `json:"textDocument"`
	Workspace    workspaceClientCapabilities    `json:"workspace"`
	General      generalClientCapabilities      `json:"general"`
}

const positionEncodingUTF16 = "utf-16"

type generalClientCapabilities struct {
	PositionEncodings []string `json:"positionEncodings"`
}

type textDocumentClientCapabilities struct {
	Synchronization    synchronizationClientCapabilities `json:"synchronization"`
	Definition         supportedCapability               `json:"definition"`
	References         supportedCapability               `json:"references"`
	Implementation     supportedCapability               `json:"implementation"`
	Hover              hoverClientCapabilities           `json:"hover"`
	DocumentSymbol     documentSymbolClientCapabilities  `json:"documentSymbol"`
	CallHierarchy      supportedCapability               `json:"callHierarchy"`
	PublishDiagnostics diagnosticClientCapabilities      `json:"publishDiagnostics"`
}

type synchronizationClientCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration"`
	DidSave             bool `json:"didSave"`
}

type hoverClientCapabilities struct {
	ContentFormat []markupKind `json:"contentFormat"`
}

type documentSymbolClientCapabilities struct {
	HierarchicalDocumentSymbolSupport bool `json:"hierarchicalDocumentSymbolSupport"`
}

type workspaceClientCapabilities struct {
	Symbol           supportedCapability `json:"symbol"`
	Configuration    bool                `json:"configuration"`
	WorkspaceFolders bool                `json:"workspaceFolders"`
}

// HierarchicalDocumentSymbolSupport stays false so documentSymbol returns flat
// SymbolInformation with locations. DidSave is true because a server that
// registers handlers from client capabilities drops notifications the client never
// claimed to send.
func defaultCapabilities() clientCapabilities {
	return clientCapabilities{
		TextDocument: textDocumentClientCapabilities{
			Synchronization:    synchronizationClientCapabilities{DidSave: true},
			PublishDiagnostics: diagnosticClientCapabilities{VersionSupport: true},
			Hover: hoverClientCapabilities{
				ContentFormat: []markupKind{markupMarkdown, markupPlaintext},
			},
		},
		Workspace: workspaceClientCapabilities{
			Configuration:    true,
			WorkspaceFolders: true,
		},
		General: generalClientCapabilities{PositionEncodings: []string{positionEncodingUTF16}},
	}
}

func parseLocations(raw jsontext.Value) ([]Location, error) {
	value := normalizedJSON(raw)
	if len(value) == 0 {
		return nil, errors.New("decode locations: response is empty")
	}
	if isJSONNull(value) {
		return nil, nil
	}
	switch value[0] {
	case '{':
		location, err := parseLocation(value)
		if err != nil {
			return nil, err
		}
		return []Location{location}, nil
	case '[':
		var items []jsontext.Value
		if err := json.Unmarshal(value, &items); err != nil {
			return nil, fmt.Errorf("decode location list: %w", err)
		}
		locations := make([]Location, 0, len(items))
		for index, item := range items {
			location, err := parseLocationItem(item)
			if err != nil {
				return nil, fmt.Errorf("decode location list item %d: %w", index, err)
			}
			locations = append(locations, location)
		}
		return locations, nil
	default:
		return nil, fmt.Errorf("decode locations: expected object, array, or null, got %s", jsonKind(value))
	}
}

func parseLocationItem(raw jsontext.Value) (Location, error) {
	var shape struct {
		URI       *string `json:"uri"`
		TargetURI *string `json:"targetUri"`
	}
	if err := json.Unmarshal(raw, &shape); err != nil {
		return Location{}, err
	}
	switch {
	case shape.URI != nil && shape.TargetURI == nil:
		return parseLocation(raw)
	case shape.URI == nil && shape.TargetURI != nil:
		var link locationLink
		if err := json.Unmarshal(raw, &link); err != nil {
			return Location{}, err
		}
		if link.TargetRange == nil {
			return Location{}, errors.New("location link targetRange is missing or null")
		}
		if link.TargetSelectionRange == nil {
			return Location{}, errors.New("location link targetSelectionRange is missing or null")
		}
		location := Location{URI: *link.TargetURI, Range: *link.TargetSelectionRange}
		if err := validateLocation(location); err != nil {
			return Location{}, fmt.Errorf("location link: %w", err)
		}
		return location, nil
	case shape.URI != nil && shape.TargetURI != nil:
		return Location{}, errors.New("ambiguous location object contains both uri and targetUri")
	default:
		return Location{}, errors.New("location object contains neither uri nor targetUri")
	}
}

func parseLocation(raw jsontext.Value) (Location, error) {
	var wire struct {
		URI   *string `json:"uri"`
		Range *Range  `json:"range"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return Location{}, fmt.Errorf("decode location: %w", err)
	}
	if wire.URI == nil {
		return Location{}, errors.New("location uri is missing or null")
	}
	if wire.Range == nil {
		return Location{}, errors.New("location range is missing or null")
	}
	location := Location{URI: *wire.URI, Range: *wire.Range}
	if err := validateLocation(location); err != nil {
		return Location{}, err
	}
	return location, nil
}

func validateLocation(location Location) error {
	if strings.TrimSpace(location.URI) == "" {
		return errors.New("location uri is empty")
	}
	return nil
}

func parseSymbols(raw jsontext.Value, docURI string) ([]Symbol, error) {
	value := normalizedJSON(raw)
	if len(value) == 0 {
		return nil, errors.New("decode symbols: response is empty")
	}
	if isJSONNull(value) {
		return nil, nil
	}
	if value[0] != '[' {
		return nil, fmt.Errorf("decode symbols: expected array or null, got %s", jsonKind(value))
	}
	var items []jsontext.Value
	if err := json.Unmarshal(value, &items); err != nil {
		return nil, fmt.Errorf("decode symbol list: %w", err)
	}
	if len(items) == 0 {
		return nil, nil
	}
	var first struct {
		Location jsontext.Value `json:"location"`
	}
	if err := json.Unmarshal(items[0], &first); err != nil {
		return nil, fmt.Errorf("decode first symbol shape: %w", err)
	}
	if len(first.Location) != 0 {
		return parseSymbolInformation(value)
	}
	if strings.TrimSpace(docURI) == "" {
		return nil, errors.New("decode DocumentSymbol list: document uri is empty")
	}
	var tree []documentSymbol
	if err := json.Unmarshal(value, &tree); err != nil {
		return nil, fmt.Errorf("decode DocumentSymbol list: %w", err)
	}
	var out []Symbol
	if err := appendDocumentSymbols(&out, "", docURI, tree); err != nil {
		return nil, err
	}
	return out, nil
}

func parseSymbolInformation(value jsontext.Value) ([]Symbol, error) {
	var infos []symbolInformation
	if err := json.Unmarshal(value, &infos); err != nil {
		return nil, fmt.Errorf("decode SymbolInformation list: %w", err)
	}
	out := make([]Symbol, 0, len(infos))
	for index, symbol := range infos {
		if strings.TrimSpace(symbol.Name) == "" {
			return nil, fmt.Errorf("decode SymbolInformation item %d: name is empty", index)
		}
		if symbol.Location == nil {
			return nil, fmt.Errorf("decode SymbolInformation item %d: location is missing or null", index)
		}
		if err := validateLocation(*symbol.Location); err != nil {
			return nil, fmt.Errorf("decode SymbolInformation item %d: %w", index, err)
		}
		out = append(out, Symbol{Name: symbol.Name, Kind: symbol.Kind, Location: *symbol.Location, Container: symbol.ContainerName})
	}
	return out, nil
}

func appendDocumentSymbols(out *[]Symbol, parent, docURI string, symbols []documentSymbol) error {
	for index, symbol := range symbols {
		if strings.TrimSpace(symbol.Name) == "" {
			return fmt.Errorf("decode DocumentSymbol item %d under %q: name is empty", index, parent)
		}
		if symbol.Range == nil {
			return fmt.Errorf("decode DocumentSymbol item %d under %q: range is missing or null", index, parent)
		}
		if symbol.SelectionRange == nil {
			return fmt.Errorf("decode DocumentSymbol item %d under %q: selectionRange is missing or null", index, parent)
		}
		*out = append(*out, Symbol{
			Name:      symbol.Name,
			Kind:      symbol.Kind,
			Detail:    symbol.Detail,
			Location:  Location{URI: docURI, Range: *symbol.SelectionRange},
			Container: parent,
		})
		if err := appendDocumentSymbols(out, symbol.Name, docURI, symbol.Children); err != nil {
			return err
		}
	}
	return nil
}

func parseHover(raw jsontext.Value) (string, error) {
	value := normalizedJSON(raw)
	if len(value) == 0 {
		return "", errors.New("decode hover: response is empty")
	}
	if isJSONNull(value) {
		return "", nil
	}
	if value[0] != '{' {
		return "", fmt.Errorf("decode hover: expected object or null, got %s", jsonKind(value))
	}
	var hover struct {
		Contents jsontext.Value `json:"contents"`
	}
	if err := json.Unmarshal(value, &hover); err != nil {
		return "", fmt.Errorf("decode hover object: %w", err)
	}
	if len(hover.Contents) == 0 {
		return "", errors.New("decode hover object: contents is missing")
	}
	return hoverText(hover.Contents)
}

func hoverText(raw jsontext.Value) (string, error) {
	value := normalizedJSON(raw)
	if len(value) == 0 {
		return "", errors.New("decode hover contents: value is empty")
	}
	switch value[0] {
	case '"':
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			return "", fmt.Errorf("decode hover string: %w", err)
		}
		return strings.TrimSpace(text), nil
	case '{':
		var content struct {
			Value *string `json:"value"`
		}
		if err := json.Unmarshal(value, &content); err != nil {
			return "", fmt.Errorf("decode hover object: %w", err)
		}
		if content.Value == nil {
			return "", errors.New("decode hover object: value is missing or null")
		}
		return strings.TrimSpace(*content.Value), nil
	case '[':
		var items []jsontext.Value
		if err := json.Unmarshal(value, &items); err != nil {
			return "", fmt.Errorf("decode hover list: %w", err)
		}
		parts := make([]string, 0, len(items))
		for index, item := range items {
			text, err := hoverText(item)
			if err != nil {
				return "", fmt.Errorf("decode hover list item %d: %w", index, err)
			}
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n\n"), nil
	default:
		return "", fmt.Errorf("decode hover contents: expected string, object, or array, got %s", jsonKind(value))
	}
}

func normalizedJSON(raw jsontext.Value) []byte {
	return bytes.TrimSpace(raw)
}

func jsonKind(value []byte) string {
	if len(value) == 0 {
		return "empty input"
	}
	switch value[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case 'n':
		return "null"
	case 't', 'f':
		return "boolean"
	case '"':
		return "string"
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '-':
		return "number"
	default:
		return fmt.Sprintf("token %q", value[0])
	}
}

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

func isJSONNull(value []byte) bool { return bytes.Equal(value, []byte("null")) }

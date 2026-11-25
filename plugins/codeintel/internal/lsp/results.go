package lsp

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"
)

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

func isJSONNull(value []byte) bool { return bytes.Equal(value, []byte("null")) }

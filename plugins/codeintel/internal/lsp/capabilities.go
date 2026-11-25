package lsp

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
)

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

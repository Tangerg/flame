package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

// These fields are the consumed language-binding projection. Every protocol
// value is compiled with the same SDK as the schemas; source paths are not authority.
type backendProtocolConstants struct {
	MaximumFrameBytes   int    `json:"maximumFrameBytes"`
	DeclarationPattern  string `json:"declarationPattern"`
	InstallationPattern string `json:"installationPattern"`
	NodeTarget          string `json:"nodeTarget"`
	ToolCommandPrefix   string `json:"toolCommandPrefix"`
}

func runBackendProtocol(args []string) error {
	flags := flag.NewFlagSet("flame-sdk backend-protocol", flag.ContinueOnError)
	nodeRelease := flags.String("node-release", "", "managed Node release manifest")
	out := flags.String("out", "", "protocol JSON output file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *nodeRelease == "" || *out == "" {
		return fmt.Errorf("node-release and out are required")
	}
	return generateBackendProtocol(*nodeRelease, *out)
}

func generateBackendProtocol(nodeReleasePath, output string) error {
	nodeRelease, err := os.ReadFile(nodeReleasePath)
	if err != nil {
		return err
	}
	var node struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(nodeRelease, &node); err != nil {
		return err
	}
	if node.Version == "" {
		return fmt.Errorf("node distribution version is required")
	}
	constants := backendProtocolConstants{
		MaximumFrameBytes:   sdk.DefaultMaxLineLength,
		DeclarationPattern:  identity.DeclarationIDPattern,
		InstallationPattern: identity.InstallationIDPattern(),
		NodeTarget:          "node" + strings.Split(node.Version, ".")[0],
		ToolCommandPrefix:   pluginsdk.ToolCommandPrefix,
	}
	toolReceipt, err := pluginsdk.ToolReceiptQuery()
	if err != nil {
		return err
	}
	toolReceiptJSON, err := jsonvalue.Encode(toolReceipt.Descriptor())
	if err != nil {
		return err
	}
	data, err := json.Marshal(struct {
		Constants   backendProtocolConstants `json:"constants"`
		ToolReceipt string                   `json:"toolReceipt"`
	}{constants, toolReceiptJSON.String()}, jsontext.WithIndent("  "), json.Deterministic(true))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	return os.WriteFile(output, append(data, '\n'), 0644)
}

package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
)

// These fields are the consumed language-binding projection. Every protocol
// value is compiled with the same SDK as the schemas; source paths are not authority.
type backendProtocolConstants struct {
	Protocol              string `json:"protocol"`
	DeclarationPattern    string `json:"declarationPattern"`
	InstallationPattern   string `json:"installationPattern"`
	NodeTarget            string `json:"nodeTarget"`
	InvokeMethod          string `json:"invokeMethod"`
	DescribeMethod        string `json:"describeMethod"`
	ValidateMethod        string `json:"validateMethod"`
	SetupMethod           string `json:"setupMethod"`
	ActivateMethod        string `json:"activateMethod"`
	CallbackMetadataKey   string `json:"callbackMetadataKey"`
	StreamMetadataKey     string `json:"streamMetadataKey"`
	ToolCommandPrefix     string `json:"toolCommandPrefix"`
	InvocationHeader      string `json:"InvocationHeader"`
	InvocationMetadataKey string `json:"InvocationMetadataKey"`
	ReceiptMethod         string `json:"ReceiptMethod"`
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
		Protocol:              extension.Protocol,
		DeclarationPattern:    identity.DeclarationIDPattern,
		InstallationPattern:   identity.InstallationIDPattern(),
		NodeTarget:            "node" + strings.Split(node.Version, ".")[0],
		InvokeMethod:          pluginsdk.InvokeMethod,
		DescribeMethod:        pluginsdk.DescribeMethod,
		ValidateMethod:        pluginsdk.ValidateMethod,
		SetupMethod:           pluginsdk.SetupMethod,
		ActivateMethod:        pluginsdk.ActivateMethod,
		CallbackMetadataKey:   pluginsdk.CallbackMetadataKey,
		StreamMetadataKey:     pluginsdk.StreamMetadataKey,
		ToolCommandPrefix:     pluginsdk.ToolCommandPrefix,
		InvocationHeader:      pluginsdk.InvocationHeader,
		InvocationMetadataKey: pluginsdk.InvocationMetadataKey,
		ReceiptMethod:         pluginsdk.ReceiptMethod,
	}
	receipt, err := pluginsdk.ReceiptQuery()
	if err != nil {
		return err
	}
	receiptJSON, err := jsonvalue.Encode(receipt.Descriptor())
	if err != nil {
		return err
	}
	toolReceipt, err := pluginsdk.ToolReceiptQuery()
	if err != nil {
		return err
	}
	toolReceiptJSON, err := jsonvalue.Encode(toolReceipt.Descriptor())
	if err != nil {
		return err
	}
	delivery, err := pluginsdk.StreamDeliveryContract()
	if err != nil {
		return err
	}
	deliveryJSON, err := jsonvalue.Encode(delivery)
	if err != nil {
		return err
	}
	type inputVector struct {
		Source    string `json:"source"`
		Canonical string `json:"canonical"`
		Digest    string `json:"digest"`
	}
	var vectors []inputVector
	for _, source := range []string{`{"z":1.0,"a":9007199254740993}`, `{"a":1e+20,"n":-0,"s":"\u0061"}`, `{"\ud83d\ude00":1,"\uffff":2,"a":[false,null]}`} {
		input, err := jsonvalue.Parse([]byte(source))
		if err != nil {
			return err
		}
		vectors = append(vectors, inputVector{Source: source, Canonical: input.String(), Digest: input.Digest()})
	}
	astral, bmp := receipt.Descriptor(), receipt.Descriptor()
	astral.Name, bmp.Name = "\U0001f600", "\uffff"
	contract, err := extension.NewCapability(extension.CapabilityConfig{Reference: "\ufeffcontract", Composition: extension.Collection, Scope: "installation"}, astral, bmp)
	if err != nil {
		return err
	}
	contractJSON, err := jsonvalue.Encode(contract.Descriptor())
	if err != nil {
		return err
	}
	data, err := json.Marshal(struct {
		Constants          backendProtocolConstants `json:"constants"`
		Receipt            string                   `json:"receipt"`
		ReceiptIdentity    string                   `json:"receiptIdentity"`
		ToolReceipt        string                   `json:"toolReceipt"`
		StreamDelivery     string                   `json:"streamDelivery"`
		Inputs             []inputVector            `json:"inputs"`
		Capability         string                   `json:"capability"`
		CapabilityIdentity string                   `json:"capabilityIdentity"`
	}{constants, receiptJSON.String(), receiptContract(receipt.Descriptor()), toolReceiptJSON.String(), deliveryJSON.String(), vectors, contractJSON.String(), contract.Identity().Digest()}, jsontext.WithIndent("  "), json.Deterministic(true))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	return os.WriteFile(output, append(data, '\n'), 0644)
}

func receiptContract(descriptor extension.MethodDescriptor) string {
	method, err := extension.AdmitMethod(descriptor)
	if err != nil {
		panic(err)
	}
	return method.Identity().Digest()
}

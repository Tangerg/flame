package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
)

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
	_, source, _, ok := runtime.Caller(0)
	if !ok || !filepath.IsAbs(source) {
		return fmt.Errorf("backend protocol generation requires its matching SDK sources; run go run without -trimpath")
	}
	return generateBackendProtocol(filepath.Join(filepath.Dir(source), "..", "..", "plugin"), *nodeRelease, *out)
}

func generateBackendProtocol(sourceDirectory, nodeReleasePath, output string) error {
	constants := map[string]string{"protocol": extension.Protocol}
	constants["declarationPattern"] = identity.DeclarationIDPattern
	constants["installationPattern"] = identity.InstallationIDPattern()
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
	constants["nodeTarget"] = "node" + strings.Split(node.Version, ".")[0]
	// The command reads the package from the SDK sources that built it. An arbitrary
	// second checkout could publish constants from a different protocol version.
	files, err := os.ReadDir(sourceDirectory)
	if err != nil {
		return err
	}
	for _, source := range files {
		if source.IsDir() || !strings.HasSuffix(source.Name(), ".go") || strings.HasSuffix(source.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(sourceDirectory, source.Name()), nil, 0)
		if err != nil {
			return err
		}
		if file.Name.Name != "plugin" {
			continue
		}
		for _, declaration := range file.Decls {
			constantsDeclaration, ok := declaration.(*ast.GenDecl)
			if !ok || constantsDeclaration.Tok != token.CONST {
				continue
			}
			for _, specification := range constantsDeclaration.Specs {
				value := specification.(*ast.ValueSpec)
				for index, expression := range value.Values {
					literal, ok := expression.(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						continue
					}
					text, err := strconv.Unquote(literal.Value)
					if err != nil {
						return err
					}
					constants[value.Names[index].Name] = text
				}
			}
		}
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
		Constants          map[string]string `json:"constants"`
		Receipt            string            `json:"receipt"`
		ReceiptIdentity    string            `json:"receiptIdentity"`
		ToolReceipt        string            `json:"toolReceipt"`
		StreamDelivery     string            `json:"streamDelivery"`
		Inputs             []inputVector     `json:"inputs"`
		Capability         string            `json:"capability"`
		CapabilityIdentity string            `json:"capabilityIdentity"`
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

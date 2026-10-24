package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/identity"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/flame/runtime/resourceid"
)

func main() {
	if err := generate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate() error {
	constants := map[string]string{"protocol": capability.Protocol}
	constants["declarationPattern"] = identity.DeclarationIDPattern
	constants["installationPattern"] = resourceid.InstallationIDPattern()
	nodeRelease, err := os.ReadFile("../local-runtime/node-release.json")
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
	for _, source := range []string{"backend.go", "setup.go", "callbacks.go", "backend_stream.go", "invocation.go", "tools.go", "receipt.go", "tool_receipt.go"} {
		file, err := parser.ParseFile(token.NewFileSet(), source, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			declaration, ok := node.(*ast.GenDecl)
			if !ok || declaration.Tok != token.CONST {
				return true
			}
			for _, spec := range declaration.Specs {
				value := spec.(*ast.ValueSpec)
				for i, name := range value.Names {
					if i >= len(value.Values) {
						continue
					}
					literal, ok := value.Values[i].(*ast.BasicLit)
					if ok && literal.Kind == token.STRING {
						constants[name.Name], _ = strconv.Unquote(literal.Value)
					}
				}
			}
			return false
		})
	}
	receipt, err := pluginsdk.NewMethod[pluginsdk.ReceiptRequest, pluginsdk.Receipt](pluginsdk.MethodConfig{Name: pluginsdk.ReceiptMethod, Kind: capability.Query, Target: "installation", Description: pluginsdk.ReceiptMethod})
	if err != nil {
		return err
	}
	receiptJSON, err := jsoninput.Encode(receipt.Descriptor())
	if err != nil {
		return err
	}
	toolReceipt, err := pluginsdk.NewMethod[pluginsdk.ReceiptRequest, pluginsdk.ToolReceipt](pluginsdk.MethodConfig{Name: pluginsdk.ToolReceiptMethod, Kind: capability.Query, Target: "installation", Description: pluginsdk.ToolReceiptMethod})
	if err != nil {
		return err
	}
	toolReceiptJSON, err := jsoninput.Encode(toolReceipt.Descriptor())
	if err != nil {
		return err
	}
	delivery, err := pluginsdk.StreamDeliveryContract()
	if err != nil {
		return err
	}
	deliveryJSON, err := jsoninput.Encode(delivery)
	if err != nil {
		return err
	}
	var vectors []map[string]string
	for _, source := range []string{`{"z":1.0,"a":9007199254740993}`, `{"a":1e+20,"n":-0,"s":"\u0061"}`, `{"\ud83d\ude00":1,"\uffff":2,"a":[false,null]}`} {
		input, err := jsoninput.Parse([]byte(source))
		if err != nil {
			return err
		}
		vectors = append(vectors, map[string]string{"source": source, "canonical": input.String(), "digest": input.Digest()})
	}
	data, err := json.Marshal(struct {
		Constants       map[string]string   `json:"constants"`
		Receipt         string              `json:"receipt"`
		ReceiptIdentity string              `json:"receiptIdentity"`
		ToolReceipt     string              `json:"toolReceipt"`
		StreamDelivery  string              `json:"streamDelivery"`
		Inputs          []map[string]string `json:"inputs"`
	}{constants, receiptJSON.String(), receiptContract(receipt.Descriptor()), toolReceiptJSON.String(), deliveryJSON.String(), vectors}, jsontext.WithIndent("  "), json.Deterministic(true))
	if err != nil {
		return err
	}
	if err := os.MkdirAll("typescript/src", 0755); err != nil {
		return err
	}
	return os.WriteFile("typescript/src/protocol.generated.json", append(data, '\n'), 0644)
}

func receiptContract(descriptor capability.MethodDescriptor) string {
	method, err := capability.AdmitMethod(descriptor)
	if err != nil {
		panic(err)
	}
	return method.Identity().Digest()
}

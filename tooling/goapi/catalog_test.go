package main

import (
	json "encoding/json/v2"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"path/filepath"
)

func TestPublicGoAPICapturesExactRuntimeBinding(t *testing.T) {
	repository, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	api, err := loadPublicGoAPI(repository)
	if err != nil {
		t.Fatalf("load public Go API: %v", err)
	}
	methods, err := manifestGoNames(filepath.Join(repository, "runtime/contract/manifest.json"))
	if err != nil {
		t.Fatal(err)
	}

	binding := publicGoPackageByPath(t, api, runtimeModulePath)
	if !slices.ContainsFunc(binding.Functions, func(function publicGoFunction) bool {
		return function.Name == "Open" && strings.Contains(function.Signature, "(*Runtime, error)")
	}) {
		t.Fatal("Runtime public API does not contain Open returning *Runtime")
	}
	if slices.ContainsFunc(binding.Imports, func(path string) bool { return strings.Contains(path, "/internal/") }) {
		t.Fatalf("public API exposes private packages: %v", binding.Imports)
	}

	for _, typeName := range []string{"Runtime", "Client"} {
		owner := binding
		if typeName == "Client" {
			owner = publicGoPackageByPath(t, api, "github.com/Tangerg/flame/packages/go-sdk/client")
		}
		runtimeType := publicGoTypeByName(t, owner, typeName)
		if got, want := len(runtimeType.Methods), len(methods)+1; got != want {
			t.Fatalf("Runtime methods = %d, want %d operations plus Close", got, want)
		}
		for goName := range methods {
			if !slices.ContainsFunc(runtimeType.Methods, func(method publicGoFunction) bool { return method.Name == goName }) {
				t.Fatalf("%s binding lacks %s", typeName, goName)
			}
		}
		if !slices.ContainsFunc(runtimeType.Methods, func(method publicGoFunction) bool {
			return method.Name == "Close" && method.Signature == "func() error"
		}) {
			t.Fatal("Runtime public API does not contain Close")
		}

	}
	if !slices.ContainsFunc(publicGoPackageByPath(t, api, "github.com/Tangerg/flame/packages/go-sdk/client").Functions, func(function publicGoFunction) bool {
		return function.Name == "Connect" && strings.Contains(function.Signature, "(*Client, error)")
	}) {
		t.Fatal("Runtime public API does not contain Connect returning *Client")
	}
	if !slices.ContainsFunc(publicGoPackageByPath(t, api, "github.com/Tangerg/flame/packages/go-sdk/client").Functions, func(function publicGoFunction) bool {
		return function.Name == "ConnectCallback" && function.Signature == "func(ctx context.Context, ref mcp.CallbackRef) (*Client, error)"
	}) {
		t.Fatal("Runtime public API does not contain the typed callback Client entrance")
	}

	localRuntime := publicGoPackageByPath(t, api, "github.com/Tangerg/flame/packages/local-runtime")
	for _, name := range []string{"OpenToken", "ReadToken"} {
		if !slices.ContainsFunc(localRuntime.Functions, func(function publicGoFunction) bool {
			return function.Name == name && function.Signature == "func(path string) (*Token, error)"
		}) {
			t.Fatalf("localruntime public API does not contain %s", name)
		}
	}
	token := publicGoTypeByName(t, localRuntime, "Token")
	for _, name := range []string{"Path", "Value"} {
		if !slices.ContainsFunc(token.Methods, func(method publicGoFunction) bool {
			return method.Name == name && method.Signature == "func() string"
		}) {
			t.Fatalf("localruntime.Token public API does not contain %s", name)
		}
	}

	protocol := publicGoPackageByPath(t, api, "github.com/Tangerg/flame/packages/go-sdk/protocol")
	if len(protocol.Types) == 0 || len(protocol.Constants) == 0 {
		t.Fatal("protocol Go baseline is vacuous")
	}
	problemError := publicGoTypeByName(t, protocol, "ProblemError")
	if !slices.ContainsFunc(problemError.Methods, func(method publicGoFunction) bool {
		return method.Name == "Problem" && method.Signature == "func() ProblemData"
	}) {
		t.Fatal("protocol ProblemError method contract is missing")
	}
}

func publicGoPackageByPath(t *testing.T, api publicGoAPI, path string) publicGoPackage {
	t.Helper()
	for _, pkg := range api.Packages {
		if pkg.Path == path {
			return pkg
		}
	}
	t.Fatalf("public Go API has no package %s", path)
	return publicGoPackage{}
}

func publicGoTypeByName(t *testing.T, pkg publicGoPackage, name string) publicGoType {
	t.Helper()
	for _, declared := range pkg.Types {
		if declared.Name == name {
			return declared
		}
	}
	t.Fatalf("public Go package %s has no type %s", pkg.Path, name)
	return publicGoType{}
}

func manifestGoNames(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Methods []struct {
			Name   string `json:"name"`
			GoName string `json:"goName"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	methods := map[string]string{}
	names := map[string]bool{}
	for _, method := range manifest.Methods {
		if method.Name == "" || names[method.Name] {
			return nil, fmt.Errorf("manifest has empty or duplicate method %q", method.Name)
		}
		names[method.Name] = true
		if method.GoName == "" {
			continue
		}
		if _, exists := methods[method.GoName]; exists {
			return nil, fmt.Errorf("manifest has duplicate Go method %q", method.GoName)
		}
		methods[method.GoName] = method.Name
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("manifest has no Go method projections")
	}
	return methods, nil
}

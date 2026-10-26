package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/apps/desktop/internal/bridge"
	"golang.org/x/tools/go/packages"
)

func main() {
	source := flag.String("source", "", "Desktop package source directory")
	receiver := flag.String("receiver", "", "actual exported service receiver")
	output := flag.String("out", "", "generated Desktop TypeScript path")
	foundation := flag.String("foundation", "", "generated notification authorization path")
	flag.Parse()
	if *source == "" || *receiver == "" || *output == "" || *foundation == "" || flag.NArg() != 0 {
		fail(fmt.Errorf("source, receiver, out and foundation are required"))
	}
	contract, err := loadContract(*source, *receiver)
	if err != nil {
		fail(err)
	}
	desktop, neutral, err := render(contract)
	if err != nil {
		fail(err)
	}
	for path, body := range map[string][]byte{*output: desktop, *foundation: neutral} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fail(err)
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			fail(err)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "bridgegen:", err)
	os.Exit(1)
}

func loadContract(source, receiver string) (*contract, error) {
	loaded, err := packages.Load(&packages.Config{
		Dir: source,
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax |
			packages.NeedTypesInfo | packages.NeedImports | packages.NeedFiles,
	}, "./internal/bridge")
	if err != nil {
		return nil, err
	}
	if len(loaded) != 1 {
		return nil, fmt.Errorf("expected one pure bridge package, got %d", len(loaded))
	}
	pkg := loaded[0]
	if len(pkg.Errors) != 0 {
		return nil, fmt.Errorf("load bridge types: %v", pkg.Errors)
	}
	graph, events, err := newGraph(pkg.Types, pkg.Syntax)
	if err != nil {
		return nil, err
	}
	methods, err := readMethods(source, receiver, graph)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		if _, err := graph.named(event.payload); err != nil {
			return nil, err
		}
	}
	if err := graph.refine(bridge.Rules()); err != nil {
		return nil, err
	}
	return &contract{definitions: graph.definitions, methods: methods, events: events}, nil
}

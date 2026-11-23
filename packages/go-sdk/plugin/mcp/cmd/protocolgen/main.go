package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
)

func main() {
	output := flag.String("out", "", "protocol JSON projection")
	flag.Parse()
	if *output == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "protocolgen: out is required")
		os.Exit(1)
	}
	data, err := mcp.ProtocolJSON()
	if err == nil {
		err = os.MkdirAll(filepath.Dir(*output), filesystem.SharedDirectoryMode)
	}
	if err == nil {
		err = os.WriteFile(*output, data, filesystem.SharedFileMode)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "protocolgen:", err)
		os.Exit(1)
	}
}

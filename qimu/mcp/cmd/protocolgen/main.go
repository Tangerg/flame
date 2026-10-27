package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/qimu/mcp"
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
		err = os.MkdirAll(filepath.Dir(*output), 0755)
	}
	if err == nil {
		err = os.WriteFile(*output, data, 0644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "protocolgen:", err)
		os.Exit(1)
	}
}

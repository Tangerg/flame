package main

import "testing"

func TestCommandRequiresOneExplicitGenerationMode(t *testing.T) {
	for _, args := range [][]string{
		{}, {"--feature", "missing", "--model-contract", "context"},
		{"--model-contract", "context", "--out", "."},
		{"--model-contract", "input", "--go-prefix", "X"},
		{"--model-contract", "unknown"}, {"--model-contract", "selection", "extra"},
	} {
		if err := runContract(args); err == nil {
			t.Fatalf("accepted ambiguous command: %v", args)
		}
	}
}

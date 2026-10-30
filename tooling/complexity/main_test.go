package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRatchetRejectsNewGrownAndStaleHotspots(t *testing.T) {
	root := t.TempDir()
	branches := strings.Repeat("\tif x > 0 {\n\t\tx--\n\t}\n", threshold)
	source := "package p\n\nfunc hot(x int) int {\n" + branches + "\treturn x\n}\n\nfunc cool() {}\n"
	if err := os.WriteFile(filepath.Join(root, "p.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	baseline := filepath.Join(t.TempDir(), "baseline.txt")
	for _, test := range []struct {
		name, baseline string
		ok             bool
	}{
		{name: "unlisted hotspot", baseline: "", ok: false},
		{name: "exact entry", baseline: "21 p.go:hot\n", ok: true},
		{name: "growth", baseline: "20 p.go:hot\n", ok: false},
		{name: "improvement not recorded", baseline: "25 p.go:hot\n", ok: false},
		{name: "stale entry", baseline: "21 p.go:hot\n30 p.go:cool\n", ok: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(baseline, []byte(test.baseline), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := check(root, baseline, false); (err == nil) != test.ok {
				t.Fatalf("check = %v, want ok=%t", err, test.ok)
			}
		})
	}
}

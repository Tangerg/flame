package lsp

import (
	"io"
	"os"
	"os/exec"
	"testing"
)

const stdioHelperEnvironment = "FLAME_LSP_STDIO_HELPER"
const stdioHelperOutput = "last language-server response"

func TestStdioParentEndsOutliveProcessWait(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestStdioProcessHelper$")
	cmd.Env = append(os.Environ(), stdioHelperEnvironment+"=1")
	stream, closeChildCopies, err := openStdio(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stream.Close(); err != nil {
			t.Errorf("parent stream lost its sole close ownership: %v", err)
		}
	}()
	defer closeChildCopies()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := closeChildCopies(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	response, err := io.ReadAll(stream)
	if err != nil || string(response) != stdioHelperOutput {
		t.Fatalf("response after process exit = %q, %v", response, err)
	}
}

func TestStdioProcessHelper(t *testing.T) {
	if os.Getenv(stdioHelperEnvironment) != "1" {
		return
	}
	if _, err := io.WriteString(os.Stdout, stdioHelperOutput); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

package lsp

import (
	"bufio"

	"errors"
	"strings"
	"testing"
)

func TestReadLSPObjectRejectsOversizedHeaderBeforeTerminator(t *testing.T) {
	stream := bufio.NewReaderSize(strings.NewReader("X-Long: "+strings.Repeat("x", 64)+"\r\n\r\n"), 8)
	var destination any
	if err := readLSPObject(stream, &destination, 32, 64); !errors.Is(err, errLSPHeaderTooLarge) {
		t.Fatalf("oversized header error = %v, want errLSPHeaderTooLarge", err)
	}
}

func TestReadLSPObjectRejectsOversizedBodyBeforeReadingIt(t *testing.T) {
	stream := bufio.NewReader(strings.NewReader("Content-Length: 65\r\n\r\n"))
	var destination any
	if err := readLSPObject(stream, &destination, 64, 64); !errors.Is(err, errLSPMessageTooLarge) {
		t.Fatalf("oversized body error = %v, want errLSPMessageTooLarge", err)
	}
}

func TestReadLSPObjectRequiresOneUnambiguousExactFrame(t *testing.T) {
	for _, test := range []struct {
		name  string
		frame string
	}{
		{name: "missing length", frame: "Content-Type: application/vscode-jsonrpc\r\n\r\n{}"},
		{name: "duplicate length", frame: "Content-Length: 2\r\nContent-Length: 2\r\n\r\n{}"},
		{name: "truncated body", frame: "Content-Length: 3\r\n\r\n{}"},
		{name: "trailing JSON", frame: "Content-Length: 4\r\n\r\n{}{}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var destination any
			if err := readLSPObject(bufio.NewReader(strings.NewReader(test.frame)), &destination, 256, 256); err == nil {
				t.Fatal("malformed frame was accepted")
			}
		})
	}
}

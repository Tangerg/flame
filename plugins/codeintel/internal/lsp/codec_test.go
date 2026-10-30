package lsp

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestLSPObjectCodecRoundTripsOneFrame(t *testing.T) {
	var stream bytes.Buffer
	codec := lspObjectCodec{}
	want := struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Method  string `json:"method"`
	}{JSONRPC: "2.0", ID: 7, Method: "textDocument/hover"}
	if err := codec.WriteObject(&stream, want); err != nil {
		t.Fatal(err)
	}
	var got struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Method  string `json:"method"`
	}
	if err := codec.ReadObject(bufio.NewReader(&stream), &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("decoded frame = %+v, want %+v", got, want)
	}
}

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

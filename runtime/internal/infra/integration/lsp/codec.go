package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	maxLSPHeaderBytes  = 8 << 10
	maxLSPMessageBytes = 64 << 20
)

var (
	errLSPHeaderTooLarge  = errors.New("lsp: message header too large")
	errLSPMessageTooLarge = errors.New("lsp: message body too large")
)

// lspObjectCodec keeps the mature jsonrpc2 connection lifecycle while owning
// Flame's finite LSP framing policy. The upstream VS Code codec accepts an
// unbounded header line and content lengths up to 4 GiB.
type lspObjectCodec struct{}

func (lspObjectCodec) WriteObject(stream io.Writer, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(body) > maxLSPMessageBytes {
		return fmt.Errorf("%w: encoded body uses %d bytes; limit is %d", errLSPMessageTooLarge, len(body), maxLSPMessageBytes)
	}
	if _, err := fmt.Fprintf(stream, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = stream.Write(body)
	return err
}

func (lspObjectCodec) ReadObject(stream *bufio.Reader, value any) error {
	return readLSPObject(stream, value, maxLSPHeaderBytes, maxLSPMessageBytes)
}

func readLSPObject(stream *bufio.Reader, value any, headerLimit, messageLimit int) error {
	if stream == nil || value == nil || headerLimit <= 0 || messageLimit <= 0 {
		return errors.New("lsp: valid stream, destination, and framing limits are required")
	}
	remainingHeader := headerLimit
	contentLength := -1
	for {
		line, err := readLSPHeaderLine(stream, &remainingHeader, headerLimit)
		if err != nil {
			return err
		}
		if len(line) < 2 || line[len(line)-2] != '\r' {
			return errors.New("lsp: message header requires CRLF line endings")
		}
		line = line[:len(line)-2]
		if len(line) == 0 {
			break
		}
		name, rawValue, found := bytes.Cut(line, []byte{':'})
		if !found {
			return errors.New("lsp: malformed message header")
		}
		if !strings.EqualFold(strings.TrimSpace(string(name)), "Content-Length") {
			continue
		}
		if contentLength >= 0 {
			return errors.New("lsp: duplicate Content-Length header")
		}
		length, err := strconv.ParseUint(strings.TrimSpace(string(rawValue)), 10, 63)
		if err != nil || length == 0 {
			return errors.New("lsp: invalid Content-Length header")
		}
		if length > uint64(messageLimit) {
			return fmt.Errorf("%w: declared body uses %d bytes; limit is %d", errLSPMessageTooLarge, length, messageLimit)
		}
		contentLength = int(length)
	}
	if contentLength < 0 {
		return errors.New("lsp: missing Content-Length header")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(stream, body); err != nil {
		return fmt.Errorf("lsp: read %d-byte message body: %w", contentLength, err)
	}
	if err := json.Unmarshal(body, value); err != nil {
		return fmt.Errorf("lsp: decode message body: %w", err)
	}
	return nil
}

func readLSPHeaderLine(stream *bufio.Reader, remaining *int, limit int) ([]byte, error) {
	var line []byte
	for {
		if *remaining <= 0 {
			return nil, fmt.Errorf("%w: limit is %d bytes", errLSPHeaderTooLarge, limit)
		}
		fragment, err := stream.ReadSlice('\n')
		if len(fragment) > *remaining {
			return nil, fmt.Errorf("%w: limit is %d bytes", errLSPHeaderTooLarge, limit)
		}
		*remaining -= len(fragment)
		line = append(line, fragment...)
		switch {
		case err == nil:
			return line, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		default:
			return nil, err
		}
	}
}

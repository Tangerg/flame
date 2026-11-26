package main

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/packages/go-sdk/wiregen"
)

const protocolVersionMember = "protocolVersion"

func writeSamples(source, destination string) error {
	samples := contractSamples()
	retained := make(map[string]bool, len(samples))
	for _, sample := range samples {
		path := filepath.Join(source, sample.File)
		encoded, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, versioned := protocol.WireFieldNamed(sample.Type, protocolVersionMember); versioned {
			current, err := refreshSampleVersion(encoded)
			if err != nil {
				return fmt.Errorf("refresh %s: %w", path, err)
			}
			if !bytes.Equal(current, encoded) {
				if err := os.WriteFile(path, current, filesystem.SharedFileMode); err != nil {
					return err
				}
				encoded = current
			}
		}
		if err := os.WriteFile(filepath.Join(destination, sample.File), encoded, filesystem.SharedFileMode); err != nil {
			return err
		}
		retained[sample.File] = true
	}
	previous, err := filepath.Glob(filepath.Join(destination, "*.json"))
	if err != nil {
		return err
	}
	for _, path := range previous {
		if !retained[filepath.Base(path)] {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}
	return nil
}

func refreshSampleVersion(encoded []byte) ([]byte, error) {
	reader := jsontext.NewDecoder(bytes.NewReader(encoded))
	root, err := reader.ReadToken()
	if err != nil {
		return nil, err
	}
	if root.Kind() != jsontext.KindBeginObject {
		return nil, errors.New("expected a JSON object")
	}
	var start, end int
	for {
		name, err := reader.ReadToken()
		if err != nil {
			return nil, err
		}
		if name.Kind() == jsontext.KindEndObject {
			break
		}
		versioned := name.String() == protocolVersionMember
		value, err := reader.ReadValue()
		if err != nil {
			return nil, err
		}
		if versioned {
			if value.Kind() != jsontext.KindString {
				return nil, fmt.Errorf("%s must be a string", protocolVersionMember)
			}
			end = int(reader.InputOffset())
			start = end - len(value)
		}
	}
	if _, err := reader.ReadToken(); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("unexpected JSON after the sample")
	}
	if start == end {
		return encoded, nil
	}
	version := []byte(wiregen.Literal(protocol.Version))
	return slices.Concat(encoded[:start], version, encoded[end:]), nil
}

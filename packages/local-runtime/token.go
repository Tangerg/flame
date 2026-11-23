package localruntime

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// The token authenticates the local Runtime, so only its user may read it or its directory.
const (
	tokenDirectoryMode = 0o700
	tokenFileMode      = 0o600
)

const (
	rawTokenBytes         = 32
	tokenCandidatePattern = ".local-token-*"
)

var encodedTokenBytes = base64.RawURLEncoding.EncodedLen(rawTokenBytes)

var ErrInvalidToken = errors.New("invalid local Runtime token")

type Token struct {
	value string
	path  string
}

// An empty string would mean no credential, which disables the gate.
func (t *Token) Value() string { return t.value }

func (t *Token) Path() string { return t.path }

func OpenToken(path string) (*Token, error) {
	path, err := cleanTokenPath(path)
	if err != nil {
		return nil, err
	}
	token, err := ReadToken(path)
	if err == nil {
		return token, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	parent := filepath.Dir(path)
	if mkdirErr := os.MkdirAll(parent, tokenDirectoryMode); mkdirErr != nil {
		return nil, fmt.Errorf("local Runtime token: create parent: %w", mkdirErr)
	}
	value := newTokenValue()
	temporary, err := os.CreateTemp(parent, tokenCandidatePattern)
	if err != nil {
		return nil, fmt.Errorf("local Runtime token: create candidate: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(tokenFileMode); err != nil {
		_ = temporary.Close()
		return nil, fmt.Errorf("local Runtime token: protect candidate: %w", err)
	}
	if _, err := temporary.WriteString(value); err != nil {
		_ = temporary.Close()
		return nil, fmt.Errorf("local Runtime token: write candidate: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return nil, fmt.Errorf("local Runtime token: sync candidate: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return nil, fmt.Errorf("local Runtime token: close candidate: %w", err)
	}
	if err := os.Link(temporaryPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ReadToken(path)
		}
		return nil, fmt.Errorf("local Runtime token: publish candidate: %w", err)
	}
	// Link is the credential's logical commit point. A directory-sync refusal
	// cannot be reported as failed publication after trusted clients can already
	// observe the token; strengthen crash durability where supported instead.
	syncCommittedDirectory(parent)
	return &Token{value: value, path: path}, nil
}

func ReadToken(path string) (*Token, error) {
	path, err := cleanTokenPath(path)
	if err != nil {
		return nil, err
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if validationErr := validateTokenFile(pathInfo); validationErr != nil {
		return nil, validationErr
	}
	return readTokenFile(path, pathInfo)
}

func readTokenFile(path string, pathInfo os.FileInfo) (*Token, error) {
	file, err := openReadOnlyPath(path)
	if err != nil {
		return nil, fmt.Errorf("local Runtime token: open: %w", err)
	}
	defer func() { _ = file.Close() }()
	return readOpenedTokenFile(path, file, pathInfo)
}

func readOpenedTokenFile(path string, file *os.File, pathInfo os.FileInfo) (*Token, error) {
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("local Runtime token: inspect opened file: %w", err)
	}
	if !sameTokenFileVersion(pathInfo, openedInfo) {
		return nil, invalidToken("path changed while opening")
	}
	if validationErr := validateTokenFile(openedInfo); validationErr != nil {
		return nil, validationErr
	}

	encoded, err := readTokenBytes(file)
	if err != nil {
		return nil, err
	}
	if err := confirmTokenFileUnchanged(path, file, openedInfo); err != nil {
		return nil, err
	}
	var decoded [rawTokenBytes]byte
	decodedBytes, err := base64.RawURLEncoding.Decode(decoded[:], encoded)
	if err != nil || decodedBytes != rawTokenBytes || base64.RawURLEncoding.EncodeToString(decoded[:]) != string(encoded) {
		return nil, invalidToken("file does not contain one canonical 32-byte token")
	}
	return &Token{value: string(encoded), path: path}, nil
}

func readTokenBytes(file io.Reader) ([]byte, error) {
	encoded := make([]byte, encodedTokenBytes+1)
	n, err := io.ReadFull(file, encoded)
	if n == encodedTokenBytes && errors.Is(err, io.ErrUnexpectedEOF) {
		return encoded[:encodedTokenBytes], nil
	}
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("local Runtime token: read: %w", err)
	}
	return nil, invalidToken(fmt.Sprintf("file must contain exactly %d bytes", encodedTokenBytes))
}

func confirmTokenFileUnchanged(path string, file *os.File, opened os.FileInfo) error {
	after, err := file.Stat()
	if err != nil {
		return fmt.Errorf("local Runtime token: inspect file after reading: %w", err)
	}
	current, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return invalidToken("path changed while reading")
	}
	if err != nil {
		return fmt.Errorf("local Runtime token: inspect path after reading: %w", err)
	}
	if !sameTokenFileVersion(opened, after) || !sameTokenFileVersion(after, current) {
		return invalidToken("path changed while reading")
	}
	return nil
}

func sameTokenFileVersion(left, right os.FileInfo) bool {
	return left != nil && right != nil && os.SameFile(left, right) &&
		left.Size() == right.Size() && left.Mode() == right.Mode() &&
		left.ModTime().Equal(right.ModTime())
}

func cleanTokenPath(path string) (string, error) {
	if path == "" {
		return "", invalidToken("path is required")
	}
	if !filepath.IsAbs(path) {
		return "", invalidToken("path must be absolute")
	}
	return filepath.Clean(path), nil
}

func validateTokenFile(info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return invalidToken("path must be a regular file")
	}
	if permissions := info.Mode().Perm(); permissions != tokenFileMode {
		return invalidToken(fmt.Sprintf("permissions are %04o, want 0600", permissions))
	}
	if info.Size() != int64(encodedTokenBytes) {
		return invalidToken(fmt.Sprintf("file must contain exactly %d bytes", encodedTokenBytes))
	}
	return nil
}

func newTokenValue() string {
	var raw [rawTokenBytes]byte
	rand.Read(raw[:])
	return base64.RawURLEncoding.EncodeToString(raw[:])
}

func invalidToken(reason string) error {
	return fmt.Errorf("local Runtime token: %s: %w", reason, ErrInvalidToken)
}

func syncCommittedDirectory(path string) {
	_ = syncDirectory(path)
}

func syncDirectory(path string) error {
	directory, err := openReadOnlyPath(path)
	if err != nil {
		return err
	}
	info, err := directory.Stat()
	if err != nil {
		return errors.Join(err, directory.Close())
	}
	if !info.IsDir() {
		return errors.Join(errors.New("local Runtime token: sync target is not a directory"), directory.Close())
	}
	return errors.Join(directory.Sync(), directory.Close())
}

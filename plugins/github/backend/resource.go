package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var errResource = errors.New("github: repository or file path is invalid")

var repositoryPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

type repository struct{ owner, name string }

func newRepository(value string) (repository, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 || len(value) > 256 {
		return repository{}, errResource
	}
	for _, part := range parts {
		if !repositoryPart.MatchString(part) || part == "." || part == ".." {
			return repository{}, errResource
		}
	}
	return repository{owner: strings.ToLower(parts[0]), name: strings.ToLower(parts[1])}, nil
}

func (r repository) String() string { return r.owner + "/" + r.name }
func (r repository) apiPath() string {
	return "/repos/" + url.PathEscape(r.owner) + "/" + url.PathEscape(r.name)
}

type filePath struct{ encoded string }

func newFilePath(value string) (filePath, error) {
	if !utf8.ValidString(value) || len(value) > 4096 || strings.ContainsAny(value, "\\\x00") {
		return filePath{}, errResource
	}
	parts := strings.Split(value, "/")
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return filePath{}, errResource
		}
		parts[i] = url.PathEscape(part)
	}
	return filePath{encoded: strings.Join(parts, "/")}, nil
}

func (p filePath) apiPath() string { return p.encoded }

type credential struct{ token string }

func newCredential(value *string) (credential, error) {
	if value == nil {
		return credential{}, nil
	}
	if *value == "" || len(*value) > 4096 || strings.ContainsFunc(*value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) || r > unicode.MaxASCII }) {
		return credential{}, errors.New("github: configured token must be nonempty printable ASCII without whitespace")
	}
	return credential{token: *value}, nil
}

func (c credential) Format(state fmt.State, _ rune) { _, _ = state.Write([]byte("[REDACTED]")) }
func (c credential) configured() bool               { return c.token != "" }
func (c credential) authorization() string {
	if !c.configured() {
		return ""
	}
	return "Bearer " + c.token
}
func (c credential) cacheKey(resource string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(c.token+"\x00"+resource)))
}

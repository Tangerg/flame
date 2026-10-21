package resource

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

var ErrResource = errors.New("github: repository or file path is invalid")

var repositoryPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

type Repository struct{ owner, name string }

func NewRepository(value string) (Repository, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 || len(value) > 256 {
		return Repository{}, ErrResource
	}
	for _, part := range parts {
		if !repositoryPart.MatchString(part) || part == "." || part == ".." {
			return Repository{}, ErrResource
		}
	}
	return Repository{owner: strings.ToLower(parts[0]), name: strings.ToLower(parts[1])}, nil
}

func (r Repository) String() string { return r.owner + "/" + r.name }
func (r Repository) APIPath() string {
	return "/repos/" + url.PathEscape(r.owner) + "/" + url.PathEscape(r.name)
}

type FilePath struct{ encoded string }

func NewFilePath(value string) (FilePath, error) {
	if !utf8.ValidString(value) || len(value) > 4096 || strings.ContainsAny(value, "\\\x00") {
		return FilePath{}, ErrResource
	}
	parts := strings.Split(value, "/")
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return FilePath{}, ErrResource
		}
		parts[i] = url.PathEscape(part)
	}
	return FilePath{encoded: strings.Join(parts, "/")}, nil
}

func (p FilePath) APIPath() string { return p.encoded }

type Credential struct{ token string }

func NewCredential(value *string) (Credential, error) {
	if value == nil {
		return Credential{}, nil
	}
	if *value == "" || len(*value) > 4096 || strings.ContainsFunc(*value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) || r > unicode.MaxASCII }) {
		return Credential{}, errors.New("github: configured token must be nonempty printable ASCII without whitespace")
	}
	return Credential{token: *value}, nil
}

func (c Credential) Format(state fmt.State, _ rune) { _, _ = state.Write([]byte("[REDACTED]")) }
func (c Credential) Configured() bool               { return c.token != "" }
func (c Credential) Authorization() string {
	if !c.Configured() {
		return ""
	}
	return "Bearer " + c.token
}
func (c Credential) CacheKey(resource string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(c.token+"\x00"+resource)))
}

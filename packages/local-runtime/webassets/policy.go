package webassets

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"path"
	"strings"
)

const (
	policyFile    = ".vite/flame-web-policy.json"
	policyVersion = 1
	entryDocument = "index.html"
)

type documentKind string

const (
	mainDocument       documentKind = "main"
	sameOriginDocument documentKind = "same-origin"
	sandboxDocument    documentKind = "sandbox"
)

type fingerprintKind string

const (
	unfingerprinted  fingerprintKind = ""
	contentAddressed fingerprintKind = "sha256"
	bundlerAddressed fingerprintKind = "bundler"
)

type artifact struct {
	SHA256      string          `json:"sha256"`
	Fingerprint fingerprintKind `json:"fingerprint,omitempty"`
	Document    documentKind    `json:"document,omitempty"`
}

func (a artifact) validate(name, sandbox string) error {
	hash, err := hex.DecodeString(a.SHA256)
	if !publicPath(name) || err != nil || len(hash) != sha256.Size || hex.EncodeToString(hash) != a.SHA256 {
		return errors.New("webassets: invalid artifact identity")
	}
	switch a.Fingerprint {
	case unfingerprinted, contentAddressed, bundlerAddressed:
	default:
		return errors.New("webassets: invalid artifact fingerprint")
	}
	if a.Fingerprint == contentAddressed && path.Base(name) != a.SHA256+path.Ext(name) {
		return errors.New("webassets: invalid content-addressed asset")
	}
	if path.Ext(name) != ".html" {
		if a.Document != "" {
			return errors.New("webassets: document kind on a non-document artifact")
		}
		return nil
	}
	switch a.Document {
	case mainDocument, sameOriginDocument, sandboxDocument:
	default:
		return errors.New("webassets: invalid document kind")
	}
	if (a.Document == sandboxDocument) != (name == sandbox) {
		return errors.New("webassets: invalid sandbox identity")
	}
	return nil
}

func (a artifact) matches(data []byte) bool {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]) == a.SHA256
}

type policy struct {
	Version          int    `json:"version"`
	NoncePlaceholder string `json:"noncePlaceholder"`
	DocumentPolicy   string `json:"documentPolicy"`
	EmbeddedPolicy   string `json:"embeddedPolicy"`
	Sandbox          struct {
		File   string `json:"file"`
		Policy string `json:"policy"`
	} `json:"sandbox"`
	Files map[string]artifact `json:"files"`
}

func parsePolicy(data []byte) (policy, error) {
	var p policy
	if err := json.Unmarshal(data, &p, json.RejectUnknownMembers(true)); err != nil {
		return policy{}, fmt.Errorf("webassets: decode artifact policy: %w", err)
	}
	if err := p.validate(); err != nil {
		return policy{}, err
	}
	return p, nil
}

func (p policy) validate() error {
	if p.Version != policyVersion || p.NoncePlaceholder == "" || strings.ContainsAny(p.NoncePlaceholder, "\r\n'\"<>&") {
		return errors.New("webassets: invalid artifact policy")
	}
	if !publicPath(p.Sandbox.File) || path.Ext(p.Sandbox.File) != ".html" || p.Sandbox.Policy == "" || strings.ContainsAny(p.Sandbox.Policy, "\r\n") {
		return errors.New("webassets: invalid sandbox policy")
	}
	for _, csp := range []string{p.DocumentPolicy, p.EmbeddedPolicy} {
		if !strings.Contains(csp, "'nonce-"+p.NoncePlaceholder+"'") || strings.ContainsAny(csp, "\r\n") {
			return errors.New("webassets: invalid document policy")
		}
	}
	for name, file := range p.Files {
		if err := file.validate(name, p.Sandbox.File); err != nil {
			return err
		}
	}
	if p.Files[entryDocument].Document != mainDocument || p.Files[p.Sandbox.File].Document != sandboxDocument {
		return errors.New("webassets: missing required document identity")
	}
	return nil
}

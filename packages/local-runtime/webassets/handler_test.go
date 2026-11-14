package webassets

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

type unreadableArtifact struct{ fs.File }

func (unreadableArtifact) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type unclosableArtifact struct{ fs.File }

func (f unclosableArtifact) Close() error { return errors.Join(f.File.Close(), fs.ErrPermission) }

func TestUnpublishedPathsConcealSourceRefusals(t *testing.T) {
	files := distribution(t)
	h, err := New(func(name string) (fs.File, error) {
		if name == "unpublished.txt" {
			return nil, errors.New("private source refusal")
		}
		return files.Open(name)
	})
	if err != nil {
		t.Fatal(err)
	}
	response := request(h, http.MethodGet, "/unpublished.txt", nil)
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "private source refusal") {
		t.Fatalf("unpublished source refusal = %d %q", response.Code, response.Body.String())
	}
}

func TestArtifactReadFailuresRemainUnavailableAndCannotBecomeNotFound(t *testing.T) {
	for _, test := range []struct {
		name string
		open func(fstest.MapFS) (fs.File, error)
	}{
		{"open", func(fstest.MapFS) (fs.File, error) { return nil, fs.ErrPermission }},
		{"read", func(files fstest.MapFS) (fs.File, error) {
			file, err := files.Open("assets/app.js")
			return unreadableArtifact{file}, err
		}},
		{"close", func(files fstest.MapFS) (fs.File, error) {
			file, err := files.Open("assets/app.js")
			return unclosableArtifact{file}, err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			files := distribution(t)
			h, err := New(func(name string) (fs.File, error) {
				if name == "assets/app.js" {
					return test.open(files)
				}
				return files.Open(name)
			})
			if err != nil {
				t.Fatal(err)
			}
			response := request(h, http.MethodGet, "/assets/app.js", nil)
			if response.Code != http.StatusServiceUnavailable || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("artifact failure = %d, cache = %q", response.Code, response.Header().Get("Cache-Control"))
			}
			if missing := request(h, http.MethodGet, "/missing.js", nil); missing.Code != http.StatusNotFound {
				t.Fatalf("missing artifact status = %d", missing.Code)
			}
			if directory := request(h, http.MethodGet, "/assets", map[string]string{"Accept": "text/html"}); directory.Code != http.StatusNotFound {
				t.Fatalf("directory status = %d", directory.Code)
			}
			if route := request(h, http.MethodGet, "/session/record", map[string]string{"Accept": "text/html"}); route.Code != http.StatusOK {
				t.Fatalf("application route status = %d", route.Code)
			}
		})
	}
}

func TestPolicyRefusesDuplicateDeclarationsAndNoncanonicalDigests(t *testing.T) {
	t.Run("duplicate version", func(t *testing.T) {
		files := distribution(t)
		files[policyFile].Data = bytes.Replace(files[policyFile].Data, []byte(`"version":1`), []byte(`"version":0,"version":1`), 1)
		if _, err := New(files.Open); err == nil {
			t.Fatal("accepted competing policy version declarations")
		}
	})
	t.Run("noncanonical digest", func(t *testing.T) {
		files := distribution(t)
		var p policy
		if err := json.Unmarshal(files[policyFile].Data, &p); err != nil {
			t.Fatal(err)
		}
		file := p.Files["assets/app.js"]
		file.SHA256 = strings.ToUpper(file.SHA256)
		p.Files["assets/app.js"] = file
		data, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		files[policyFile].Data = data
		if _, err := New(files.Open); err == nil {
			t.Fatal("accepted an identity that cannot match the served bytes")
		}
	})
}

func distribution(t *testing.T) fstest.MapFS {
	t.Helper()
	files := fstest.MapFS{
		"index.html":       {Data: []byte(`<!doctype html><script nonce="RESPONSE_NONCE" src="/assets/app.js"></script><meta property="csp-nonce" nonce="RESPONSE_NONCE">`)},
		"sandbox.html":     {Data: []byte(`<!doctype html><p>opaque program wrapper</p>`)},
		"preview.html":     {Data: []byte(`<!doctype html><script nonce="RESPONSE_NONCE" src="/assets/app.js"></script>`)},
		"assets/app.js":    {Data: []byte(`export const captured = true`)},
		"appearance.js":    {Data: []byte(`document.documentElement.dataset.theme = 'light'`)},
		"fonts/mono.woff2": {Data: []byte("public font")},
		".private":         {Data: []byte("not public")},
	}
	p := policy{
		Version:          1,
		NoncePlaceholder: "RESPONSE_NONCE",
		DocumentPolicy:   "script-src 'nonce-RESPONSE_NONCE' 'strict-dynamic'; frame-src 'self' https://www.youtube-nocookie.com; frame-ancestors 'none'",
		EmbeddedPolicy:   "script-src 'nonce-RESPONSE_NONCE' 'strict-dynamic'; frame-ancestors 'self'",
		Files:            make(map[string]artifact),
	}
	p.Sandbox.File = "sandbox.html"
	p.Sandbox.Policy = "default-src 'none'; script-src 'unsafe-inline'; frame-src 'none'"
	for name, file := range files {
		if !publicPath(name) {
			continue
		}
		hash := sha256.Sum256(file.Data)
		identity := artifact{SHA256: hex.EncodeToString(hash[:])}
		switch name {
		case "index.html":
			identity.Document = "main"
		case "sandbox.html":
			identity.Document = "sandbox"
		case "preview.html":
			identity.Document = "same-origin"
		}
		p.Files[name] = identity
	}
	app := p.Files["assets/app.js"]
	app.Fingerprint = "bundler"
	p.Files["assets/app.js"] = app
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	files[policyFile] = &fstest.MapFile{Data: data}
	return files
}

func request(h http.Handler, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, nil)
	for key, value := range headers {
		r.Header.Set(key, value)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestDocumentPairsFreshNonceWithUnconditionalBody(t *testing.T) {
	h, err := New(distribution(t).Open)
	if err != nil {
		t.Fatal(err)
	}
	first := request(h, http.MethodGet, "/", nil)
	second := request(h, http.MethodGet, "/", map[string]string{
		"If-None-Match": "*", "If-Modified-Since": "Wed, 31 Dec 2099 23:59:59 GMT",
	})
	for _, response := range []*httptest.ResponseRecorder{first, second} {
		if response.Code != http.StatusOK {
			t.Fatalf("document status = %d", response.Code)
		}
		if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Frame-Options") != "DENY" {
			t.Fatal("main document lacks its delivery policy")
		}
		_, rest, found := strings.Cut(response.Header().Get("Content-Security-Policy"), "'nonce-")
		nonce, _, ended := strings.Cut(rest, "'")
		if !found || !ended || nonce == "" || strings.Contains(response.Body.String(), "RESPONSE_NONCE") ||
			strings.Count(response.Body.String(), `nonce="`+nonce+`"`) != 2 {
			t.Fatal("document and header do not share one fresh nonce")
		}
	}
	if first.Header().Get("Content-Security-Policy") == second.Header().Get("Content-Security-Policy") {
		t.Fatal("document requests reused a nonce")
	}
	head := request(h, http.MethodHead, "/", nil)
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("head lost the document policy or wrote a body")
	}
}

func TestWrapperHasIndependentPolicyAndCanBeEmbedded(t *testing.T) {
	h, err := New(distribution(t).Open)
	if err != nil {
		t.Fatal(err)
	}
	response := request(h, http.MethodGet, "/sandbox.html", nil)
	if response.Code != http.StatusOK || response.Header().Get("X-Frame-Options") != "" ||
		strings.Contains(response.Header().Get("Content-Security-Policy"), "frame-ancestors") ||
		!strings.Contains(response.Header().Get("Content-Security-Policy"), "frame-src 'none'") {
		t.Fatal("program wrapper acquired the main document's embedding or script policy")
	}
	preview := request(h, http.MethodGet, "/preview.html", nil)
	if preview.Code != http.StatusOK || preview.Header().Get("X-Frame-Options") != "SAMEORIGIN" ||
		!strings.Contains(preview.Header().Get("Content-Security-Policy"), "frame-ancestors 'self'") ||
		!strings.Contains(preview.Header().Get("Content-Security-Policy"), "'strict-dynamic'") {
		t.Fatal("trusted preview lost its same-origin embedding or strict script policy")
	}
}

func TestImmutableReplyRequiresCurrentVerifiedArtifact(t *testing.T) {
	files := distribution(t)
	h, err := New(files.Open)
	if err != nil {
		t.Fatal(err)
	}
	response := request(h, http.MethodGet, "/assets/app.js", nil)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatal("fingerprinted artifact is not immutable")
	}
	cached := request(h, http.MethodGet, "/assets/app.js", map[string]string{"If-None-Match": response.Header().Get("ETag")})
	if cached.Code != http.StatusNotModified {
		t.Fatalf("verified cache status = %d", cached.Code)
	}
	files["assets/app.js"].Data = []byte("changed after publication")
	changed := request(h, http.MethodGet, "/assets/app.js", nil)
	if changed.Code != http.StatusServiceUnavailable || changed.Header().Get("Cache-Control") != "no-store" || strings.Contains(changed.Body.String(), "changed after") {
		t.Fatal("mutable bytes were published under an immutable identity")
	}
}

func TestPublicUnfingerprintedFilesRemainUpdateableAndPrivatePathsStayHidden(t *testing.T) {
	h, err := New(distribution(t).Open)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/appearance.js", "/fonts/mono.woff2"} {
		response := request(h, http.MethodGet, target, nil)
		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-cache" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("unfingerprinted policy changed for %s", target)
		}
	}
	for _, target := range []string{"/.private", "/.vite/flame-web-policy.json", "/../index.html", "/..%5cindex.html"} {
		if response := request(h, http.MethodGet, target, nil); response.Code != http.StatusNotFound {
			t.Fatalf("private path status = %d for %s", response.Code, target)
		}
	}
}

func TestDistributionRequiresCurrentCompletePolicyAndOriginalDocumentBytes(t *testing.T) {
	for _, change := range []func(fstest.MapFS){
		func(files fstest.MapFS) { delete(files, policyFile) },
		func(files fstest.MapFS) { files[policyFile].Data = []byte(`{"version":0}`) },
		func(files fstest.MapFS) { files["index.html"].Data = []byte("replacement document") },
		func(files fstest.MapFS) { delete(files, "sandbox.html") },
	} {
		files := distribution(t)
		change(files)
		if _, err := New(files.Open); err == nil {
			t.Fatal("accepted an incomplete or changed distribution")
		}
	}
	if _, err := New(func(string) (fs.File, error) { return nil, fs.ErrPermission }); err == nil {
		t.Fatal("accepted an unreadable artifact source")
	}
}

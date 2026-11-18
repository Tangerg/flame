package main

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	localruntime "github.com/Tangerg/flame/packages/local-runtime"
)

func mustDesktopHost(t *testing.T, home string) *DesktopHost {
	t.Helper()
	root, err := localruntime.ResolveProductRoot(home, "")
	if err != nil {
		t.Fatal(err)
	}
	host, err := newDesktopHost(root, localruntime.DefaultOrigin)
	if err != nil {
		t.Fatal(err)
	}
	return host
}

func mustDesktopDataDirectory(t *testing.T, home string) localruntime.DataDirectory {
	t.Helper()
	root, err := localruntime.ResolveProductRoot(home, "")
	if err != nil {
		t.Fatal(err)
	}
	directory, err := localruntime.DataDirectoryUnder(root)
	if err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestDesktopHostBootstrapObservesCredentialsCreatedAfterStartup(t *testing.T) {
	home := t.TempDir()
	host := mustDesktopHost(t, home)
	initial, err := host.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if initial.LocalRuntime.LocalToken != "" {
		t.Fatal("bootstrap returned a credential before Runtime created it")
	}
	directory := mustDesktopDataDirectory(t, home)
	if err := os.MkdirAll(directory.Path(), 0o700); err != nil {
		t.Fatal(err)
	}
	token, err := localruntime.OpenToken(directory.LocalTokenPath())
	if err != nil {
		t.Fatal(err)
	}
	observed, err := host.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if observed.LocalRuntime.LocalToken != token.Value() {
		t.Fatal("running Desktop host did not observe the new Runtime credential")
	}
	if err := os.WriteFile(directory.LocalTokenPath(), []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := host.Bootstrap(); err == nil {
		t.Fatal("bootstrap accepted an invalid replacement credential")
	}
	replacement := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	if err := os.WriteFile(directory.LocalTokenPath(), []byte(replacement), 0o600); err != nil {
		t.Fatal(err)
	}
	observed, err = host.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if observed.LocalRuntime.LocalToken != replacement {
		t.Fatal("running Desktop host did not recover after credential replacement")
	}
}

func TestDesktopBootstrapPairsConfiguredEndpointWithProductRootToken(t *testing.T) {
	tests := map[string]struct {
		configured string
		want       string
	}{
		"default":            {configured: "", want: localruntime.DefaultOrigin},
		"alternate loopback": {configured: "http://127.0.0.1:17172", want: "http://127.0.0.1:17172"},
		"root slash":         {configured: "http://127.0.0.1:17172/", want: "http://127.0.0.1:17172"},
		"https":              {configured: "https://127.0.0.1:17172", want: "https://127.0.0.1:17172"},
		"ipv6 loopback":      {configured: "http://[::1]:17172/", want: "http://[::1]:17172"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "product")
			t.Setenv("HOME", t.TempDir())
			t.Setenv("FLAME_HOME", root)
			t.Setenv("FLAME_LOCAL_RUNTIME_URL", test.configured)
			directory, err := localruntime.DataDirectoryUnder(root)
			if err != nil {
				t.Fatal(err)
			}
			token, err := localruntime.OpenToken(directory.LocalTokenPath())
			if err != nil {
				t.Fatal(err)
			}

			host, err := defaultDesktopHost()
			if err != nil {
				t.Fatal(err)
			}
			bootstrap, err := host.Bootstrap()
			if err != nil {
				t.Fatal(err)
			}
			if bootstrap.LocalRuntime.Endpoint != test.want {
				t.Fatalf("endpoint = %q, want %q", bootstrap.LocalRuntime.Endpoint, test.want)
			}
			if bootstrap.LocalRuntime.LocalToken != token.Value() {
				t.Fatal("Bootstrap did not pair the endpoint with the configured product root token")
			}
		})
	}
}

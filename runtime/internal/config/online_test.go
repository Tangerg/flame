package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadOnline_EnvOverridesYAML(t *testing.T) {
	t.Setenv("FLAME_HTTP_ALLOWED_HOSTS", "api.github.com, *.example.com ")

	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "config.yaml"), []byte(`
provider: anthropic
online:
  httpAllowedHosts: ["yaml.example.com"]
`), 0o600); err != nil {
		t.Fatalf("read config: %v", err)
	}

	t.Setenv("FLAME_PROVIDER", "")
	settings, err := Load([]string{directory})
	if err != nil {
		t.Fatal(err)
	}
	got := settings.Online
	want := Online{
		HTTPAllowedHosts: []string{"api.github.com", "*.example.com"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loadOnline = %+v, want %+v", got, want)
	}
}

func TestRemovedWebCredentialsCannotReenterCoreConfiguration(t *testing.T) {
	directory := t.TempDir()
	secret := "private-legacy-web-fixture"
	if err := os.WriteFile(filepath.Join(directory, "config.yaml"), []byte("online:\n  jinaApiKey: "+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load([]string{directory})
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("legacy web credentials were accepted or disclosed: %v", err)
	}
}

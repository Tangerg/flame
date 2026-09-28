package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadOnline_EnvOverridesYAML(t *testing.T) {
	t.Setenv("FLAME_JINA_API_KEY", "jina-env")
	t.Setenv("FLAME_TAVILY_API_KEY", "tavily-env")
	t.Setenv("FLAME_HTTP_ALLOWED_HOSTS", "api.github.com, *.example.com ")

	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "config.yaml"), []byte(`
provider: anthropic
online:
  jinaApiKey: jina-yaml
  tavilyApiKey: tavily-yaml
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
		JinaAPIKey:       "jina-env",
		TavilyAPIKey:     "tavily-env",
		HTTPAllowedHosts: []string{"api.github.com", "*.example.com"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loadOnline = %+v, want %+v", got, want)
	}
}

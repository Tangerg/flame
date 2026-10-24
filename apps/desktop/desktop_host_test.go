package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

type workingDirectoryPickerFunc func() (string, error)

func (w workingDirectoryPickerFunc) ChooseWorkingDirectory() (string, error) {
	return w()
}

type imageSaverFunc func(suggestedFilename string, contents []byte) (bool, error)

func (i imageSaverFunc) SaveImage(suggestedFilename string, contents []byte) (bool, error) {
	return i(suggestedFilename, contents)
}

type textFileSaverFunc func(suggestedFilename string, contents []byte) (bool, error)

func (s textFileSaverFunc) SaveTextFile(suggestedFilename string, contents []byte) (bool, error) {
	return s(suggestedFilename, contents)
}

func TestDesktopHostBootstrap(t *testing.T) {
	home := t.TempDir()
	host := mustDesktopHost(t, home)
	directory := mustDesktopDataDirectory(t, home)
	if err := os.MkdirAll(directory.Path(), 0o700); err != nil {
		t.Fatal(err)
	}
	value := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	if err := os.WriteFile(directory.LocalTokenPath(), []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := host.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.LocalRuntime.Endpoint != localruntime.DefaultOrigin || bootstrap.LocalRuntime.LocalToken != value {
		t.Fatalf("local runtime = %#v", bootstrap.LocalRuntime)
	}
}

func TestDesktopHostBootstrapRejectsInvalidDurableCredentials(t *testing.T) {
	value := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	tests := map[string]struct {
		contents []byte
		mode     os.FileMode
		link     bool
	}{
		"arbitrary text":     {contents: []byte("token"), mode: 0o600},
		"whitespace wrapped": {contents: []byte("\n" + value + "\n"), mode: 0o600},
		"oversized":          {contents: bytes.Repeat([]byte("x"), 1<<20), mode: 0o600},
		"public permissions": {contents: []byte(value), mode: 0o644},
		"symbolic link":      {contents: []byte(value), mode: 0o600, link: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			directory := mustDesktopDataDirectory(t, home)
			root := directory.Path()
			if err := os.MkdirAll(root, 0o700); err != nil {
				t.Fatal(err)
			}
			tokenPath := directory.LocalTokenPath()
			writePath := tokenPath
			if test.link {
				writePath = filepath.Join(root, "actual-token")
			}
			if err := os.WriteFile(writePath, test.contents, test.mode); err != nil {
				t.Fatal(err)
			}
			if test.link {
				if err := os.Symlink(writePath, tokenPath); err != nil {
					if runtime.GOOS == "windows" {
						t.Skipf("symlink creation is unavailable: %v", err)
					}
					t.Fatal(err)
				}
			}

			if _, err := mustDesktopHost(t, home).Bootstrap(); err == nil {
				t.Fatal("Bootstrap() accepted an invalid durable credential")
			}
		})
	}
}

func TestDesktopHostBootstrapAllowsMissingState(t *testing.T) {
	bootstrap, err := mustDesktopHost(t, t.TempDir()).Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.LocalRuntime.LocalToken != "" {
		t.Fatalf("local token = %q, want empty", bootstrap.LocalRuntime.LocalToken)
	}
}

func TestDesktopBootstrapUsesConfiguredProductRoot(t *testing.T) {
	for _, state := range []string{"present", "missing", "invalid"} {
		t.Run(state, func(t *testing.T) {
			home := t.TempDir()
			root := filepath.Join(t.TempDir(), " product root ")
			t.Setenv("HOME", home)
			t.Setenv("FLAME_HOME", root)
			t.Setenv("FLAME_LOCAL_RUNTIME_URL", "")
			defaultDirectory := mustDesktopDataDirectory(t, home)
			if _, err := localruntime.OpenToken(defaultDirectory.LocalTokenPath()); err != nil {
				t.Fatal(err)
			}
			directory, err := localruntime.DataDirectoryUnder(root)
			if err != nil {
				t.Fatal(err)
			}
			var expected string
			switch state {
			case "present":
				token, err := localruntime.OpenToken(directory.LocalTokenPath())
				if err != nil {
					t.Fatal(err)
				}
				expected = token.Value()
			case "invalid":
				if err := os.MkdirAll(directory.Path(), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(directory.LocalTokenPath(), []byte("invalid"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			host, err := defaultDesktopHost()
			if err != nil {
				t.Fatal(err)
			}
			bootstrap, err := host.Bootstrap()
			if state == "invalid" {
				if !errors.Is(err, localruntime.ErrInvalidToken) {
					t.Fatalf("Bootstrap error = %v, want invalid configured credential", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if bootstrap.LocalRuntime.Endpoint != localruntime.DefaultOrigin || bootstrap.LocalRuntime.LocalToken != expected {
				t.Fatal("Bootstrap did not use only the configured product root")
			}
		})
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

func TestDesktopHostRejectsInvalidLocalRuntimeEndpoint(t *testing.T) {
	tests := []string{
		"127.0.0.1:17172",
		"//127.0.0.1:17172",
		"ftp://127.0.0.1:17172",
		"ws://127.0.0.1:17172",
		"http:127.0.0.1:17172",
		"http://user:secret@127.0.0.1:17172",
		"http://secret@127.0.0.1:17172",
		"http://user:secret@127.0.0.1:invalid",
		"http://192.168.1.10:17172",
		"http://0.0.0.0:17172",
		"http://localhost:17172",
		"http://flame.example:17172",
		"http://[::1%25lo0]:17172",
		"http://127.0.0.1:17172/v2",
		"http://127.0.0.1:17172//",
		"http://127.0.0.1:17172?token=secret",
		"http://127.0.0.1:17172/?",
		"http://127.0.0.1:17172#secret",
		"http://127.0.0.1:17172#",
		"http://127.0.0.1",
		"http://127.0.0.1:",
		"http://127.0.0.1:0",
		"http://127.0.0.1:65536",
		" http://127.0.0.1:17172",
	}

	for _, configured := range tests {
		t.Run(configured, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("FLAME_HOME", filepath.Join(t.TempDir(), "product"))
			t.Setenv("FLAME_LOCAL_RUNTIME_URL", configured)

			_, err := defaultDesktopHost()
			if err == nil {
				t.Fatal("desktop accepted an invalid local runtime endpoint")
			}
			if strings.Contains(err.Error(), configured) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("error disclosed the rejected endpoint: %v", err)
			}
		})
	}
}

func TestDesktopBootstrapRejectsRelativeProductRoot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FLAME_HOME", "relative")
	if _, err := defaultDesktopHost(); err == nil {
		t.Fatal("desktop accepted a relative FLAME_HOME")
	}
}

func TestDesktopHostChooseWorkingDirectory(t *testing.T) {
	directory := t.TempDir()
	host := mustDesktopHost(t, t.TempDir())
	host.useWorkingDirectoryPicker(workingDirectoryPickerFunc(func() (string, error) {
		return directory, nil
	}))

	selected, err := host.ChooseWorkingDirectory()
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(directory)
	if err != nil {
		t.Fatal(err)
	}
	if selected != want {
		t.Fatalf("selected directory = %q, want %q", selected, want)
	}
}

func TestDesktopHostChooseWorkingDirectoryPreservesCancellation(t *testing.T) {
	host := mustDesktopHost(t, t.TempDir())
	host.useWorkingDirectoryPicker(workingDirectoryPickerFunc(func() (string, error) {
		return "", nil
	}))

	selected, err := host.ChooseWorkingDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if selected != "" {
		t.Fatalf("canceled selection = %q, want empty", selected)
	}
}

func TestDesktopHostChooseWorkingDirectoryRequiresPicker(t *testing.T) {
	host := mustDesktopHost(t, t.TempDir())

	if selected, err := host.ChooseWorkingDirectory(); err == nil {
		t.Fatalf("ChooseWorkingDirectory() = %q, nil; want unconfigured picker error", selected)
	}
}

func TestDesktopHostChooseWorkingDirectoryRejectsInvalidSelections(t *testing.T) {
	tests := map[string]struct {
		selection func(t *testing.T) string
		pickErr   error
	}{
		"picker failure": {
			selection: func(t *testing.T) string { return "" },
			pickErr:   errors.New("dialog failed"),
		},
		"missing path": {
			selection: func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing") },
		},
		"regular file": {
			selection: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "file")
				if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
					t.Fatal(err)
				}
				return path
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			host := mustDesktopHost(t, t.TempDir())
			host.useWorkingDirectoryPicker(workingDirectoryPickerFunc(func() (string, error) {
				return test.selection(t), test.pickErr
			}))

			if selected, err := host.ChooseWorkingDirectory(); err == nil {
				t.Fatalf("ChooseWorkingDirectory() = %q, nil; want error", selected)
			}
		})
	}
}

func TestDesktopHostSaveImageDecodesInlineMaterial(t *testing.T) {
	host := mustDesktopHost(t, t.TempDir())
	var filename string
	var contents []byte
	host.useImageSaver(imageSaverFunc(func(suggestedFilename string, material []byte) (bool, error) {
		filename = suggestedFilename
		contents = append([]byte(nil), material...)
		return true, nil
	}))

	saved, err := host.SaveImage("data:image/png;base64,aW1hZ2U=")
	if err != nil {
		t.Fatal(err)
	}
	if !saved {
		t.Fatal("SaveImage() = false, want completed save")
	}
	if !bytes.Equal(contents, []byte("image")) {
		t.Fatalf("saved material = %q; want image bytes", contents)
	}
	if !strings.HasPrefix(filename, "Flame Image ") || !strings.HasSuffix(filename, ".png") {
		t.Fatalf("suggested filename = %q, want Flame image PNG name", filename)
	}
}

func TestDesktopHostSaveImagePreservesCancellation(t *testing.T) {
	host := mustDesktopHost(t, t.TempDir())
	host.useImageSaver(imageSaverFunc(func(string, []byte) (bool, error) {
		return false, nil
	}))

	if saved, err := host.SaveImage("data:image/svg+xml,%3Csvg%3E%2B%3C%2Fsvg%3E"); err != nil {
		t.Fatal(err)
	} else if saved {
		t.Fatal("canceled SaveImage() = true, want false")
	}
}

func TestDesktopHostSaveImageRejectsInvalidSourcesBeforeOpeningDialog(t *testing.T) {
	tests := []string{
		"https://example.com/image.png",
		"data:text/plain;base64,aW1hZ2U=",
		"data:image/png;base64,%%broken%%",
		"data:image/png;base64,",
	}
	for _, source := range tests {
		t.Run(source, func(t *testing.T) {
			called := false
			host := mustDesktopHost(t, t.TempDir())
			host.useImageSaver(imageSaverFunc(func(string, []byte) (bool, error) {
				called = true
				return true, nil
			}))

			if saved, err := host.SaveImage(source); err == nil {
				t.Fatalf("SaveImage(%q) = %v, nil; want validation error", source, saved)
			}
			if called {
				t.Fatal("native image saver opened for invalid source")
			}
		})
	}
}

func TestDesktopHostSaveImageRequiresAndPropagatesNativeOwner(t *testing.T) {
	if saved, err := mustDesktopHost(t, t.TempDir()).SaveImage("data:image/png;base64,aW1hZ2U="); err == nil {
		t.Fatalf("SaveImage() = %v, nil; want unconfigured saver error", saved)
	}

	host := mustDesktopHost(t, t.TempDir())
	host.useImageSaver(imageSaverFunc(func(string, []byte) (bool, error) {
		return false, errors.New("disk full")
	}))
	if saved, err := host.SaveImage("data:image/png;base64,aW1hZ2U="); err == nil {
		t.Fatalf("SaveImage() = %v, nil; want native save error", saved)
	}
}

func TestDesktopHostSaveTextFileHandsExactNameAndTextToTheNativeOwner(t *testing.T) {
	const filename = "flame-session 1-2026-10-24T09-30-00.md"
	const content = "# Café 火焰 🔥\r\n\tline\x00 end\n"
	destination := filepath.Join(t.TempDir(), "chosen.md")
	host := mustDesktopHost(t, t.TempDir())
	var suggested string
	host.useTextFileSaver(textFileSaverFunc(func(suggestedFilename string, contents []byte) (bool, error) {
		suggested = suggestedFilename
		return true, os.WriteFile(destination, contents, 0o666)
	}))

	saved, err := host.SaveTextFile(filename, content)
	if err != nil {
		t.Fatal(err)
	}
	if !saved {
		t.Fatal("SaveTextFile() = false, want completed save")
	}
	if suggested != filename {
		t.Fatalf("suggested filename = %q, want %q unchanged", suggested, filename)
	}
	written, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(written, []byte(content)) {
		t.Fatalf("written text = %q, want %q", written, content)
	}
}

func TestDesktopHostSaveTextFilePreservesCancellation(t *testing.T) {
	host := mustDesktopHost(t, t.TempDir())
	host.useTextFileSaver(textFileSaverFunc(func(string, []byte) (bool, error) {
		return false, nil
	}))

	if saved, err := host.SaveTextFile("flame-trajectory.json", "{}"); err != nil {
		t.Fatal(err)
	} else if saved {
		t.Fatal("canceled SaveTextFile() = true, want false")
	}
}

func TestDesktopHostSaveTextFileRejectsInvalidNamesBeforeOpeningDialog(t *testing.T) {
	tests := []string{
		"",
		".md",
		".json",
		"export.txt",
		"export",
		"export.MD",
		"export.md.exe",
		"../export.md",
		"dir/export.md",
		`dir\export.md`,
		"/tmp/export.md",
		`C:export.md`,
		"export\n.md",
		"export\x00.md",
		"export\u0085.md",
		"export\xff.md",
	}
	for _, filename := range tests {
		t.Run(filename, func(t *testing.T) {
			called := false
			host := mustDesktopHost(t, t.TempDir())
			host.useTextFileSaver(textFileSaverFunc(func(string, []byte) (bool, error) {
				called = true
				return true, nil
			}))

			if saved, err := host.SaveTextFile(filename, "text"); err == nil {
				t.Fatalf("SaveTextFile(%q) = %v, nil; want validation error", filename, saved)
			}
			if called {
				t.Fatal("native text file saver opened for invalid filename")
			}
		})
	}
}

func TestDesktopHostSaveTextFileRequiresAndPropagatesNativeOwner(t *testing.T) {
	if saved, err := mustDesktopHost(t, t.TempDir()).SaveTextFile("export.md", "text"); err == nil {
		t.Fatalf("SaveTextFile() = %v, nil; want unconfigured saver error", saved)
	}

	diskFull := errors.New("disk full")
	host := mustDesktopHost(t, t.TempDir())
	host.useTextFileSaver(textFileSaverFunc(func(string, []byte) (bool, error) {
		return false, diskFull
	}))
	saved, err := host.SaveTextFile("export.json", "{}")
	if !errors.Is(err, diskFull) {
		t.Fatalf("SaveTextFile() error = %v, want wrapped %v", err, diskFull)
	}
	if saved {
		t.Fatal("failed SaveTextFile() = true, want false")
	}
}

type recordingRevealer struct{ revealed string }

func (r *recordingRevealer) OpenFileManager(path string, selectFile bool) error {
	r.revealed = path
	return nil
}

func TestRevealPathOnlyHandsAnExistingAbsolutePathToTheFileManager(t *testing.T) {
	dir := t.TempDir()
	host := mustDesktopHost(t, dir)
	revealer := &recordingRevealer{}
	host.usePathRevealer(revealer)

	if err := host.RevealPath("relative/file.txt"); err == nil {
		t.Fatal("RevealPath accepted a relative path")
	}
	if err := host.RevealPath(filepath.Join(dir, "missing.txt")); err == nil {
		t.Fatal("RevealPath accepted a path that does not exist")
	}
	if revealer.revealed != "" {
		t.Fatalf("a rejected path reached the file manager: %q", revealer.revealed)
	}
	if err := host.RevealPath(dir + string(filepath.Separator) + "."); err != nil {
		t.Fatalf("RevealPath(existing dir) = %v", err)
	}
	if revealer.revealed != dir {
		t.Fatalf("revealed %q, want the cleaned %q", revealer.revealed, dir)
	}
}

type recordingOpener struct{ opened string }

func (o *recordingOpener) OpenFile(path string) error {
	o.opened = path
	return nil
}

func TestOpenPathOnlyHandsAnExistingAbsolutePathToTheDefaultApplication(t *testing.T) {
	dir := t.TempDir()
	host := mustDesktopHost(t, dir)
	opener := &recordingOpener{}
	host.usePathOpener(opener)
	file := filepath.Join(dir, "diagram.png")
	if err := os.WriteFile(file, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := host.OpenPath("relative/diagram.png"); err == nil {
		t.Fatal("OpenPath accepted a relative path")
	}
	if err := host.OpenPath(filepath.Join(dir, "missing.png")); err == nil {
		t.Fatal("OpenPath accepted a path that does not exist")
	}
	if opener.opened != "" {
		t.Fatalf("a rejected path reached the default application: %q", opener.opened)
	}
	if err := host.OpenPath(file); err != nil {
		t.Fatalf("OpenPath(existing file) = %v", err)
	}
	if opener.opened != file {
		t.Fatalf("opened %q, want %q", opener.opened, file)
	}
}

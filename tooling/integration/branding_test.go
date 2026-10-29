package integration

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var retiredProductBrandTokens = [][]byte{
	[]byte("LY" + "RA"),
	[]byte("Ly" + "ra"),
	[]byte("ly" + "ra"),
}

const (
	brandScanChunkBytes       = 64 << 10
	maxRetiredBrandTokenBytes = len("ly" + "ra")
)

// TestAppHasNoRetiredProductBrand keeps the breaking product-identity cutover
// complete across every shipped Flame module. Generated dependencies and build
// outputs are not repository product sources.
func TestAppHasNoRetiredProductBrand(t *testing.T) {
	appRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	sources, err := maintainedProductFiles(appRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, relative := range sources {
		path := filepath.Join(appRoot, relative)
		for _, token := range retiredProductBrandTokens {
			if bytes.Contains([]byte(relative), token) {
				t.Errorf("retired product brand remains in path %s", relative)
			}
		}
		if !productBrandTextFile(filepath.Base(relative)) {
			continue
		}
		retiredToken, err := retiredBrandInFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if retiredToken != "" {
			t.Errorf("retired product brand %q remains in %s", retiredToken, relative)
		}
	}
}

// Git owns the maintained-source boundary, including ignored embed staging and
// build outputs. New output locations must not create another exclusion policy.
func maintainedProductFiles(root string) ([]string, error) {
	command := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	command.Dir = root
	data, err := command.Output()
	if err != nil {
		return nil, err
	}
	paths := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
	return slices.DeleteFunc(paths, func(path string) bool { return path == "" }), nil
}

func retiredBrandInFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	var window [brandScanChunkBytes + maxRetiredBrandTokenBytes - 1]byte
	overlap := 0
	var position, skipUntil int64
	for {
		n, readErr := file.Read(window[overlap:])
		chunk := window[:overlap+n]
		start := position - int64(overlap)
		for from := int(min(int64(len(chunk)), max(0, skipUntil-start))); from < len(chunk); {
			found := -1
			var token []byte
			for _, candidate := range retiredProductBrandTokens {
				if at := bytes.Index(chunk[from:], candidate); at >= 0 && (found < 0 || from+at < found) {
					found, token = from+at, candidate
				}
			}
			if found < 0 {
				break
			}
			end, err := inlinePNGEnd(file, start+int64(found))
			if err != nil {
				return "", err
			}
			if end == 0 {
				return string(token), nil
			}
			skipUntil = end
			from = int(min(int64(len(chunk)), end-start))
		}
		position += int64(n)
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return "", nil
			}
			return "", readErr
		}
		overlap = min(len(chunk), maxRetiredBrandTokenBytes-1)
		copy(window[:overlap], chunk[len(chunk)-overlap:])
	}
}

// Encoded artwork remains binary content when embedded in a source string.
// ReadAt leaves the streaming scanner's file position unchanged.
func inlinePNGEnd(file *os.File, tokenOffset int64) (int64, error) {
	var window [brandScanChunkBytes]byte
	for before := tokenOffset; before > 0; {
		n := int(min(before, int64(len(window))))
		before -= int64(n)
		if _, err := file.ReadAt(window[:n], before); err != nil {
			return 0, err
		}
		quoteAt := bytes.LastIndexAny(window[:n], "\"'`")
		if quoteAt < 0 {
			continue
		}
		quote := window[quoteAt]
		start := before + int64(quoteAt) + 1
		const prefix = "data:image/png;base64,"
		var header [len(prefix)]byte
		if _, err := file.ReadAt(header[:], start); err != nil {
			if errors.Is(err, io.EOF) {
				return 0, nil
			}
			return 0, err
		}
		if string(header[:]) != prefix {
			return 0, nil
		}
		info, err := file.Stat()
		if err != nil {
			return 0, err
		}
		bodyStart := start + int64(len(prefix))
		reader := bufio.NewReader(io.NewSectionReader(file, bodyStart, info.Size()-bodyStart))
		end := bodyStart
		for {
			part, err := reader.ReadSlice(quote)
			end += int64(len(part))
			if errors.Is(err, bufio.ErrBufferFull) {
				continue
			}
			if errors.Is(err, io.EOF) {
				return 0, nil
			}
			if err != nil {
				return 0, err
			}
			end--
			decoded := base64.NewDecoder(base64.StdEncoding, io.NewSectionReader(file, bodyStart, end-bodyStart))
			_, err = png.DecodeConfig(decoded)
			if err == nil {
				_, err = io.Copy(io.Discard, decoded)
			}
			if err != nil {
				var corrupt base64.CorruptInputError
				var malformed png.FormatError
				var unsupported png.UnsupportedError
				if errors.As(err, &corrupt) || errors.As(err, &malformed) || errors.As(err, &unsupported) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
					return 0, nil
				}
				return 0, err
			}
			return end, nil
		}
	}
	return 0, nil
}

func TestProductBrandScanDistinguishesInlinePNGFromSourceText(t *testing.T) {
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	payload := imageBytes.Bytes()
	for len(payload)%3 != 0 {
		payload = append(payload, 0)
	}
	encodedToken := string(retiredProductBrandTokens[0])
	noise, err := base64.StdEncoding.DecodeString(encodedToken)
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, bytes.Repeat(noise, brandScanChunkBytes/len(noise)+1)...)
	url := "data:image/png;base64," + base64.StdEncoding.EncodeToString(payload)
	source := `const image = "` + url + `";`
	visibleToken := string(retiredProductBrandTokens[2])
	for _, test := range []struct{ name, source, token string }{
		{"embedded image", source, ""},
		{"single quotes", "const image = '" + url + "';", ""},
		{"template string", "const image = `" + url + "`;", ""},
		{"prefix across chunks", strings.Repeat(" ", brandScanChunkBytes-30) + source, ""},
		{"visible before image", `const name = "` + visibleToken + `";` + source, visibleToken},
		{"visible after image", source + `const name = "` + visibleToken + `";`, visibleToken},
		{"wrong media type", strings.Replace(source, "image/png", "text/plain", 1), encodedToken},
		{"invalid image", strings.Replace(source, "base64,", "base64,AAAAAAAA", 1), encodedToken},
		{"invalid encoding", `const image = "` + url + `!";`, encodedToken},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "component.js")
			if err := os.WriteFile(path, []byte(test.source), 0600); err != nil {
				t.Fatal(err)
			}
			actual, err := retiredBrandInFile(path)
			if err != nil || actual != test.token {
				t.Fatalf("retired brand = %q, %v; want %q", actual, err, test.token)
			}
		})
	}
}

func TestProductBrandScanFollowsTrackedAndUntrackedSource(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		command := exec.CommandContext(t.Context(), "git", args...)
		command.Dir = root
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	write := func(name, data string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "--quiet")
	write(".gitignore", "out/\napps/desktop/assets/web/\nnode_modules/\n**/config.yaml\n")
	write("assets/brand/README.md", "Flame")
	git("add", "--", ".gitignore", "assets/brand/README.md")
	write("apps/web/src/new.ts", "export const product = 'Flame'")
	write("apps/desktop/build/config.yml", "name: Flame")
	for _, name := range []string{"out/generated.js", "apps/desktop/assets/web/vendor.js", "apps/web/node_modules/vendor/index.js", "runtime/config/config.yaml"} {
		write(name, "private or generated")
	}
	actual, err := maintainedProductFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(actual)
	expected := []string{".gitignore", "apps/desktop/build/config.yml", "apps/web/src/new.ts", "assets/brand/README.md"}
	if !slices.Equal(actual, expected) {
		t.Fatalf("maintained product sources = %v, want %v", actual, expected)
	}
}

func productBrandTextFile(name string) bool {
	if name == "Makefile" || strings.HasPrefix(name, ".git") {
		return true
	}
	switch filepath.Ext(name) {
	case ".cjs", ".css", ".go", ".html", ".js", ".json", ".jsx", ".md", ".mjs",
		".plist", ".ts", ".tsx", ".txt", ".xml", ".yaml", ".yml":
		return true
	default:
		return false
	}
}

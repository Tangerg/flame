package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"unsafe"

	"github.com/Tangerg/flame/runtime/localruntime"
)

const localRuntimeEndpoint = "http://127.0.0.1:17171"

type LocalRuntimeConnection struct {
	Endpoint   string `json:"endpoint"`
	LocalToken string `json:"localToken,omitempty"`
}

type DesktopBootstrap struct {
	LocalRuntime LocalRuntimeConnection `json:"localRuntime"`
}

// Geometry uses CSS pixels from the window top-left. Measured zeroes mean fullscreen;
// Measured false means the platform supplied no geometry, so CSS keeps its defaults.
type WindowChrome struct {
	ControlsCentreY   float64 `json:"controlsCentreY"`
	ControlsInlineEnd float64 `json:"controlsInlineEnd"`
	Measured          bool    `json:"measured"`
}

type nativeWindow interface {
	NativeWindow() unsafe.Pointer
}

type workingDirectoryPicker interface {
	ChooseWorkingDirectory() (string, error)
}

// The native picker owns the write destination; IPC callers supply only image bytes.
type imageSaver interface {
	SaveImage(suggestedFilename string, contents []byte) (bool, error)
}

type pathRevealer interface {
	OpenFileManager(path string, selectFile bool) error
}

type pathOpener interface {
	OpenFile(path string) error
}

// Every exported method becomes a Wails IPC entry point. Keep composition setters
// unexported; TestDesktopHostBinds checks the exposed method set.
type DesktopHost struct {
	pluginPages        map[string]*nativePluginPage
	pluginPagePublish  func(string, json.RawMessage)
	pluginPageSequence uint64
	pluginPagesClosed  bool

	localTokenPath         string
	workingDirectoryPicker workingDirectoryPicker
	imageSaver             imageSaver
	window                 nativeWindow
	reveal                 func()
	pathRevealer           pathRevealer
	pathOpener             pathOpener
	// Started in ServiceStartup; notificationsReady flips once, before the frontend
	// can call, and is read from IPC goroutines afterwards.
	notifications      systemNotifications
	notificationsReady atomic.Bool
}

func newDesktopHost(productRoot string) (*DesktopHost, error) {
	dataDirectory, err := localruntime.DataDirectoryUnder(productRoot)
	if err != nil {
		return nil, fmt.Errorf("desktop host: resolve data directory: %w", err)
	}
	return &DesktopHost{
		localTokenPath: dataDirectory.LocalTokenPath(),
		pluginPages:    make(map[string]*nativePluginPage),
	}, nil
}

func (d *DesktopHost) useWindow(window nativeWindow) {
	d.window = window
}

func (d *DesktopHost) useRevealer(reveal func()) {
	d.reveal = reveal
}

func (d *DesktopHost) usePathRevealer(revealer pathRevealer) {
	d.pathRevealer = revealer
}

func (d *DesktopHost) usePathOpener(opener pathOpener) {
	d.pathOpener = opener
}

func (d *DesktopHost) useWorkingDirectoryPicker(picker workingDirectoryPicker) {
	d.workingDirectoryPicker = picker
}

func (d *DesktopHost) useImageSaver(saver imageSaver) {
	d.imageSaver = saver
}

func defaultDesktopHost() (*DesktopHost, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("desktop host: resolve user home: %w", err)
	}
	root, err := localruntime.ResolveProductRoot(home, os.Getenv("FLAME_HOME"))
	if err != nil {
		return nil, fmt.Errorf("desktop host: resolve flame home: %w", err)
	}
	return newDesktopHost(root)
}

// Fullscreen transitions rebuild the native titlebar, so cached geometry would be stale.
func (d *DesktopHost) WindowChrome() WindowChrome {
	if d.window == nil {
		return WindowChrome{}
	}
	controlsCentreY, controlsInlineEnd, measured := nativeWindowChrome(d.window.NativeWindow())
	if !measured {
		return WindowChrome{}
	}
	return WindowChrome{
		ControlsCentreY:   controlsCentreY,
		ControlsInlineEnd: controlsInlineEnd,
		Measured:          true,
	}
}

func (d *DesktopHost) RevealWindow() error {
	if d.reveal == nil {
		return errors.New("desktop host: window is not attached")
	}
	d.reveal()
	return nil
}

func (d *DesktopHost) RevealPath(path string) error {
	if d.pathRevealer == nil {
		return errors.New("desktop host: file manager is not attached")
	}
	clean, err := existingAbsolutePath(path)
	if err != nil {
		return fmt.Errorf("desktop host: reveal path: %w", err)
	}
	if err := d.pathRevealer.OpenFileManager(clean, true); err != nil {
		return fmt.Errorf("desktop host: reveal path: %w", err)
	}
	return nil
}

func (d *DesktopHost) OpenPath(path string) error {
	if d.pathOpener == nil {
		return errors.New("desktop host: default application launcher is not attached")
	}
	clean, err := existingAbsolutePath(path)
	if err != nil {
		return fmt.Errorf("desktop host: open path: %w", err)
	}
	if err := d.pathOpener.OpenFile(clean); err != nil {
		return fmt.Errorf("desktop host: open path: %w", err)
	}
	return nil
}

func existingAbsolutePath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%q is not absolute", path)
	}
	clean := filepath.Clean(path)
	if _, err := os.Stat(clean); err != nil {
		return "", err
	}
	return clean, nil
}

// An empty selection means cancellation and must not become the process working directory.
func (d *DesktopHost) ChooseWorkingDirectory() (string, error) {
	if d.workingDirectoryPicker == nil {
		return "", errors.New("desktop host: working directory picker is not configured")
	}
	selected, err := d.workingDirectoryPicker.ChooseWorkingDirectory()
	if err != nil {
		return "", fmt.Errorf("desktop host: choose working directory: %w", err)
	}
	if selected == "" {
		return "", nil
	}
	absolute, err := filepath.Abs(selected)
	if err != nil {
		return "", fmt.Errorf("desktop host: resolve working directory: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("desktop host: inspect working directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("desktop host: working directory %q is not a directory", absolute)
	}
	return filepath.Clean(absolute), nil
}

// The bool distinguishes a completed write from user cancellation.
func (d *DesktopHost) SaveImage(source string) (bool, error) {
	if d.imageSaver == nil {
		return false, errors.New("desktop host: image saver is not configured")
	}
	extension, contents, err := decodeInlineImage(source)
	if err != nil {
		return false, fmt.Errorf("desktop host: save image: %w", err)
	}
	saved, err := d.imageSaver.SaveImage(suggestedImageFilename(extension), contents)
	if err != nil {
		return false, fmt.Errorf("desktop host: save image: %w", err)
	}
	return saved, nil
}

func (d *DesktopHost) Bootstrap() (DesktopBootstrap, error) {
	token, err := d.localToken()
	if err != nil {
		return DesktopBootstrap{}, err
	}
	return DesktopBootstrap{
		LocalRuntime: LocalRuntimeConnection{Endpoint: localRuntimeEndpoint, LocalToken: token},
	}, nil
}

func (d *DesktopHost) localToken() (string, error) {
	token, err := localruntime.ReadToken(d.localTokenPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("desktop host: read local runtime token: %w", err)
	}
	return token.Value(), nil
}

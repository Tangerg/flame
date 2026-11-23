package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"unsafe"

	"github.com/Tangerg/flame/apps/desktop/internal/bridge"
	"github.com/Tangerg/flame/packages/go-sdk/httporigin"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
)

type nativeWindow interface {
	NativeWindow() unsafe.Pointer
}

type workingDirectoryPicker interface {
	ChooseWorkingDirectory() (string, error)
}

type fileSaver interface {
	SaveFile(suggestedFilename string, contents []byte) (bool, error)
	exportDestinationPicker
}

type pathRevealer interface {
	OpenFileManager(path string, selectFile bool) error
}

type pathOpener interface {
	OpenFile(path string) error
}

type nativeShell struct {
	window        nativeWindow
	reveal        func()
	pathRevealer  pathRevealer
	pathOpener    pathOpener
	directories   workingDirectoryPicker
	files         fileSaver
	notifications systemNotifications
}

type DesktopHost struct {
	transfer             exportTransfer
	localRuntimeEndpoint string
	localTokenPath       string
	shell                *nativeShell
	notificationsReady   atomic.Bool
}

var errShellUnbound = errors.New("desktop host: native shell is not bound")

func newDesktopHost(productRoot, localRuntimeEndpoint string) (*DesktopHost, error) {
	endpoint, err := loopbackRuntimeOrigin(localRuntimeEndpoint)
	if err != nil {
		return nil, fmt.Errorf("desktop host: local runtime endpoint: %w", err)
	}
	dataDirectory, err := localruntime.DataDirectoryUnder(productRoot)
	if err != nil {
		return nil, fmt.Errorf("desktop host: resolve data directory: %w", err)
	}
	return &DesktopHost{
		localRuntimeEndpoint: endpoint,
		localTokenPath:       dataDirectory.LocalTokenPath(),
	}, nil
}

// The frontend grants native filesystem access to exactly this origin, so only a
// loopback IP literal qualifies and host names are never resolved. Errors omit the
// input because it may carry credentials.
func loopbackRuntimeOrigin(raw string) (string, error) {
	endpoint, err := httporigin.ParseEndpoint(raw)
	if err != nil {
		return "", errors.New("must be an absolute http or https URL without credentials")
	}
	if !endpoint.IsOrigin() {
		return "", errors.New("must be an origin without path, query, or fragment")
	}
	if !endpoint.LoopbackIP() {
		return "", errors.New("host must be a loopback IP address")
	}
	if endpoint.URL().Port() == "" {
		return "", errors.New("port must be explicit")
	}
	return endpoint.Origin().String(), nil
}

func (d *DesktopHost) bind(shell nativeShell) error {
	if d.shell != nil {
		return errors.New("desktop host: native shell is already bound")
	}
	if shell.window == nil || shell.reveal == nil || shell.pathRevealer == nil || shell.pathOpener == nil ||
		shell.directories == nil || shell.files == nil || shell.notifications == nil {
		return errors.New("desktop host: native shell is incomplete")
	}
	d.shell = &shell
	return nil
}

func defaultDesktopHost() (*DesktopHost, error) {
	root, err := localruntime.ProductRoot()
	if err != nil {
		return nil, fmt.Errorf("desktop host: %w", err)
	}
	endpoint := os.Getenv("FLAME_LOCAL_RUNTIME_URL")
	if endpoint == "" {
		endpoint = localruntime.DefaultOrigin
	}
	return newDesktopHost(root, endpoint)
}

// Fullscreen transitions rebuild the native titlebar, so cached geometry would be stale.
func (d *DesktopHost) WindowChrome() bridge.WindowChrome {
	if d.shell == nil {
		return bridge.WindowChrome{}
	}
	controlsCentreY, controlsInlineEnd, measured := nativeWindowChrome(d.shell.window.NativeWindow())
	if !measured {
		return bridge.WindowChrome{}
	}
	return bridge.WindowChrome{
		ControlsCentreY:   controlsCentreY,
		ControlsInlineEnd: controlsInlineEnd,
		Measured:          true,
	}
}

func (d *DesktopHost) RevealWindow() error {
	if d.shell == nil {
		return errShellUnbound
	}
	d.shell.reveal()
	return nil
}

func (d *DesktopHost) RevealPath(path string) error {
	if d.shell == nil {
		return errShellUnbound
	}
	clean, err := existingAbsolutePath(path)
	if err != nil {
		return fmt.Errorf("desktop host: reveal path: %w", err)
	}
	if err := d.shell.pathRevealer.OpenFileManager(clean, true); err != nil {
		return fmt.Errorf("desktop host: reveal path: %w", err)
	}
	return nil
}

func (d *DesktopHost) OpenPath(path string) error {
	if d.shell == nil {
		return errShellUnbound
	}
	clean, err := existingAbsolutePath(path)
	if err != nil {
		return fmt.Errorf("desktop host: open path: %w", err)
	}
	if err := d.shell.pathOpener.OpenFile(clean); err != nil {
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
	if d.shell == nil {
		return "", errShellUnbound
	}
	selected, err := d.shell.directories.ChooseWorkingDirectory()
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

func (d *DesktopHost) SaveFile(suggestedFilename, contentBase64 string) (bool, error) {
	contents, err := bridge.DecodeFileSave(suggestedFilename, contentBase64)
	if err != nil {
		return false, fmt.Errorf("desktop host: save file: %w", err)
	}
	if d.shell == nil {
		return false, errShellUnbound
	}
	saved, err := d.shell.files.SaveFile(suggestedFilename, contents)
	if err != nil {
		return false, fmt.Errorf("desktop host: save file: %w", err)
	}
	return saved, nil
}

func (d *DesktopHost) Bootstrap() (bridge.DesktopBootstrap, error) {
	token, err := d.localToken()
	if err != nil {
		return bridge.DesktopBootstrap{}, err
	}
	return bridge.DesktopBootstrap{
		LocalRuntime: bridge.LocalRuntimeConnection{Endpoint: d.localRuntimeEndpoint, LocalToken: token},
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

func (d *DesktopHost) BeginFileSave(name string, size int) (string, error) {
	if d.shell == nil {
		return "", errShellUnbound
	}
	return d.transfer.begin(name, size, d.shell.files)
}

func (d *DesktopHost) AppendFileSave(id string, offset int, encoded string) error {
	return d.transfer.appendChunk(id, offset, encoded)
}

func (d *DesktopHost) FinishFileSave(id string) error { return d.transfer.finish(id) }
func (d *DesktopHost) CancelFileSave(id string) error { return d.transfer.cancel(id) }

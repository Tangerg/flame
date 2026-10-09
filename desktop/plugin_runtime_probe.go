//go:build pluginruntimeprobe && darwin

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type RuntimeGateFixture struct {
	Endpoint       string `json:"endpoint"`
	LocalToken     string `json:"localToken"`
	InstallationID string `json:"installationId"`
	Digest         string `json:"digest"`
	ViewID         string `json:"viewId"`
	SessionID      string `json:"sessionId"`
	WorkspacePath  string `json:"workspacePath"`
	HTMLSHA256     string `json:"htmlSHA256"`
}

type NativeRuntimeGate struct {
	fixture   RuntimeGateFixture
	host      *DesktopHost
	calls     atomic.Uint64
	ready     atomic.Uint64
	connected atomic.Uint64
	finish    func(error)
}

func (g *NativeRuntimeGate) ActivePageCount() (count int) {
	application.InvokeSync(func() { count = len(g.host.pluginPages) })
	return count
}

func (g *NativeRuntimeGate) Configuration() RuntimeGateFixture {
	g.calls.Add(1)
	return g.fixture
}

func (g *NativeRuntimeGate) Complete(failure string) {
	var err error
	if failure != "" {
		err = errors.New(failure)
	}
	g.finish(err)
}

func main() {
	if err := runNativeRuntimeGate(); err != nil {
		log.Fatal(err)
	}
}

func runNativeRuntimeGate() (err error) {
	if len(os.Args) != 3 {
		return errors.New("native runtime gate requires assets and fixture paths")
	}
	body, err := os.ReadFile(os.Args[2])
	if err != nil {
		return err
	}
	gate := &NativeRuntimeGate{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&gate.fixture); err != nil {
		return errors.New("native runtime gate fixture is invalid")
	}
	root, err := os.MkdirTemp("", "flame-native-runtime-gate-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(root)) }()
	host, err := newDesktopHost(root)
	if err != nil {
		return err
	}
	gate.host = host
	options := desktopApplicationOptions(host)
	options.Name = "Flame native Runtime gate"
	options.Services = append(options.Services, application.NewService(gate))
	options.Assets = application.AssetOptions{Handler: application.AssetFileServerFS(os.DirFS(os.Args[1]))}
	app := application.New(options)
	window := app.Window.NewWithOptions(desktopWindowOptions())
	host.useWindow(window)
	host.pluginPagePublish = func(id string, message json.RawMessage) {
		var value struct{ Type string }
		if json.Unmarshal(message, &value) == nil {
			switch value.Type {
			case "ready":
				gate.ready.Add(1)
			case "connected":
				gate.connected.Add(1)
			}
		}
		app.Event.Emit("desktop:plugin-page", map[string]any{"id": id, "message": message})
	}
	completed := make(chan error, 1)
	var once sync.Once
	gate.finish = func(outcome error) {
		once.Do(func() {
			application.InvokeSync(func() {
				if len(host.pluginPages) != 0 {
					outcome = errors.Join(outcome, errors.New("native runtime gate left a page active"))
				}
			})
			completed <- outcome
			app.Quit()
		})
	}
	deadline := time.AfterFunc(30*time.Second, func() { gate.finish(errors.New("native runtime gate deadline exceeded")) })
	defer deadline.Stop()
	app.OnShutdown(func() {
		var outcome error
		select {
		case outcome = <-completed:
		default:
			outcome = errors.New("native runtime gate ended without a result")
		}
		report := struct {
			Calls, Ready, Connected uint64
			Error                   string `json:"error,omitempty"`
		}{Calls: gate.calls.Load(), Ready: gate.ready.Load(), Connected: gate.connected.Load()}
		if report.Calls != 1 || report.Ready != 2 || report.Connected != 2 {
			outcome = errors.Join(outcome, errors.New("native runtime gate did not connect two owned pages"))
		}
		if outcome != nil {
			report.Error = outcome.Error()
		}
		encoded, e := json.Marshal(report)
		if e != nil {
			log.Fatal(e)
		}
		if _, e := fmt.Fprintln(os.Stdout, string(encoded)); e != nil {
			log.Fatal(e)
		}
	})
	return app.Run()
}

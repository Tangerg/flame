package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed index.html frame.html workbench.html network-policy.txt
var assets embed.FS

// Probe records effects at the native owner, even when an attacking frame
// cannot read the response. It never opens a Runtime or touches user data.
type Probe struct {
	calls            atomic.Uint64
	ready            atomic.Uint64
	childMessages    atomic.Uint64
	resourceRequests atomic.Uint64
	peerPackets      atomic.Uint64
	peerEndpoint     string
	readySignal      chan struct{}
	baselineMu       sync.Mutex
	baseline         *effects
}

type effects struct {
	Calls            uint64 `json:"calls"`
	Ready            uint64 `json:"ready"`
	ChildMessages    uint64 `json:"childMessages"`
	ResourceRequests uint64 `json:"resourceRequests"`
	PeerPackets      uint64 `json:"peerPackets"`
	PeerEndpoint     string `json:"peerEndpoint"`
}

type report struct {
	Carrier   json.RawMessage  `json:"carrier"`
	Baseline  *effects         `json:"baseline,omitempty"`
	Effects   effects          `json:"effects"`
	Isolation *nativeIsolation `json:"isolation,omitempty"`
}

type nativeIsolation struct {
	LockdownEnabled  bool   `json:"lockdownEnabled"`
	RejectedMessages uint64 `json:"rejectedMessages"`
}

func newProbe() *Probe { return &Probe{readySignal: make(chan struct{})} }

func (p *Probe) noteReady() {
	if p.ready.Add(1) == 1 {
		close(p.readySignal)
	}
}

func (p *Probe) Begin(ctx context.Context) (effects, error) {
	select {
	case <-ctx.Done():
		return effects{}, ctx.Err()
	case <-p.readySignal:
	}
	if err := ctx.Err(); err != nil {
		return effects{}, err
	}
	p.baselineMu.Lock()
	defer p.baselineMu.Unlock()
	if p.baseline != nil {
		return effects{}, errors.New("native probe already began")
	}
	baseline := p.snapshot()
	p.baseline = &baseline
	return baseline, nil
}

func (p *Probe) encodeReport(payload string, cause error, isolation *nativeIsolation) ([]byte, error) {
	p.baselineMu.Lock()
	defer p.baselineMu.Unlock()
	if cause == nil && !json.Valid([]byte(payload)) {
		cause = errors.New("invalid carrier result")
	} else if cause == nil && p.baseline == nil {
		cause = errors.New("native probe did not begin")
	}
	if cause != nil {
		failure, err := json.Marshal(struct {
			Error string `json:"error"`
		}{cause.Error()})
		if err != nil {
			return nil, errors.Join(cause, err)
		}
		payload = string(failure)
	}
	return json.Marshal(report{Carrier: json.RawMessage(payload), Baseline: p.baseline, Effects: p.snapshot(), Isolation: isolation})
}

func (p *Probe) Touch() uint64 { return p.calls.Add(1) }

func (p *Probe) snapshot() effects {
	return effects{
		Calls:            p.calls.Load(),
		Ready:            p.ready.Load(),
		ChildMessages:    p.childMessages.Load(),
		ResourceRequests: p.resourceRequests.Load(),
		PeerPackets:      p.peerPackets.Load(),
		PeerEndpoint:     p.peerEndpoint,
	}
}

// The receiver observes packets outside the frame and its claimed CSP evidence.
func (p *Probe) listenPeer() (func() error, error) {
	socket, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p.peerEndpoint = "stun:" + socket.LocalAddr().String()
	joined := make(chan error, 1)
	go func() {
		buffer := make([]byte, 64<<10)
		for {
			if _, _, err := socket.ReadFrom(buffer); err != nil {
				if errors.Is(err, net.ErrClosed) {
					err = nil
				}
				joined <- err
				return
			}
			p.peerPackets.Add(1)
		}
	}()
	return func() error {
		closeErr := socket.Close()
		return errors.Join(closeErr, <-joined)
	}, nil
}

func (p *Probe) assetHandler(frameSource string) (http.Handler, error) {
	index, err := assets.ReadFile("index.html")
	if err != nil {
		return nil, err
	}
	policy, err := assets.ReadFile("network-policy.txt")
	if err != nil {
		return nil, err
	}
	host := strings.ReplaceAll(string(index), "__CARRIER_FRAME_SOURCE__", frameSource)
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/carrier-") {
			p.resourceRequests.Add(1)
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			http.ServeContent(w, r, "index.html", time.Time{}, strings.NewReader(host))
			return
		}
		if r.URL.Path == "/frame.html" {
			w.Header().Set("Connection-Allowlist", strings.TrimSpace(string(policy)))
		}
		files.ServeHTTP(w, r)
	}), nil
}

func (p *Probe) listenHTTP() (string, func() error, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	origin := "http://" + listener.Addr().String()
	handler, err := p.assetHandler(origin + "/frame.html")
	if err != nil {
		return "", nil, errors.Join(err, listener.Close())
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	joined := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		joined <- err
	}()
	entry := origin + "?carrier-stun=" + url.QueryEscape(p.peerEndpoint)
	return entry, func() error { return errors.Join(server.Close(), <-joined) }, nil
}

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "wails" && os.Args[1] != "isolated-webkit" && os.Args[1] != "isolated-webkit-control") {
		log.Fatal("expected wails, isolated-webkit or isolated-webkit-control carrier")
	}
	mode := os.Args[1]
	if err := runNativeProbe(mode); err != nil {
		log.Fatal(err)
	}
}

func runNativeProbe(mode string) (err error) {
	probe := newProbe()
	closePeer, err := probe.listenPeer()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closePeer()) }()
	handler, err := probe.assetHandler("wails://localhost/frame.html")
	if err != nil {
		return err
	}
	var app *application.App
	var isolated *isolatedCarrier
	var window *application.WebviewWindow
	var deadline *time.Timer
	entry := ""
	if mode != "wails" {
		var closeHTTP func() error
		entry, closeHTTP, err = probe.listenHTTP()
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, closeHTTP()) }()
	}
	retire := func() {
		application.InvokeSync(func() {
			if isolated != nil {
				isolated.Close()
				isolated = nil
			}
		})
	}
	finish := func(payload string, cause error, isolation *nativeIsolation) {
		deadline.Stop()
		retire()
		encoded, err := probe.encodeReport(payload, cause, isolation)
		if err != nil {
			log.Printf("encode carrier result: %v", err)
		} else {
			fmt.Printf("carrier-result:%s\n", encoded)
		}
		app.Quit()
	}
	app = application.New(application.Options{
		Name:       "Flame plugin carrier probe",
		Services:   []application.Service{application.NewService(probe)},
		Assets:     application.AssetOptions{Handler: handler},
		OnShutdown: func() { deadline.Stop(); retire() },
		RawMessageHandler: func(_ application.Window, message string, origin *application.OriginInfo) {
			if origin != nil && !origin.IsMainFrame && message == "carrier-witness" {
				probe.childMessages.Add(1)
				return
			}
			if origin == nil || !origin.IsMainFrame {
				return
			}
			if mode != "wails" && message == "carrier-workbench-ready" {
				application.InvokeSync(func() {
					if isolated != nil {
						finish("", errors.New("isolated carrier already started"), nil)
						return
					}
					carrier, createErr := newIsolatedCarrier(window.NativeWindow(), entry, mode == "isolated-webkit", finish)
					if createErr != nil {
						finish("", createErr, nil)
						return
					}
					isolated = carrier
				})
				return
			}
			payload, found := strings.CutPrefix(message, "carrier-result:")
			if !found {
				return
			}
			finish(payload, nil, nil)
		},
	})
	hostURL := "/"
	if mode != "wails" {
		hostURL = "/workbench.html"
	}
	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "Flame plugin carrier probe", Width: 640, Height: 480, URL: hostURL,
	})
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		probe.noteReady()
	})
	deadline = time.AfterFunc(20*time.Second, app.Quit)
	defer deadline.Stop()
	return app.Run()
}

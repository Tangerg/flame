package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed index.html frame.html network-policy.txt
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
	Carrier json.RawMessage `json:"carrier"`
	Effects effects         `json:"effects"`
}

func (p *Probe) Touch() uint64 { return p.calls.Add(1) }

func (p *Probe) Snapshot() effects {
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

func (p *Probe) assetHandler() (http.Handler, error) {
	index, err := assets.ReadFile("index.html")
	if err != nil {
		return nil, err
	}
	policy, err := assets.ReadFile("network-policy.txt")
	if err != nil {
		return nil, err
	}
	host := strings.ReplaceAll(string(index), "__CARRIER_FRAME_SOURCE__", "wails://localhost/frame.html")
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

func main() {
	probe := &Probe{}
	closePeer, err := probe.listenPeer()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := closePeer(); err != nil {
			log.Fatal(err)
		}
	}()
	handler, err := probe.assetHandler()
	if err != nil {
		log.Fatal(err)
	}
	var app *application.App
	app = application.New(application.Options{
		Name:     "Flame plugin carrier probe",
		Services: []application.Service{application.NewService(probe)},
		Assets:   application.AssetOptions{Handler: handler},
		RawMessageHandler: func(_ application.Window, message string, origin *application.OriginInfo) {
			if origin != nil && !origin.IsMainFrame && message == "carrier-witness" {
				probe.childMessages.Add(1)
				return
			}
			if origin == nil || !origin.IsMainFrame {
				return
			}
			payload, found := strings.CutPrefix(message, "carrier-result:")
			if !found {
				return
			}
			if !json.Valid([]byte(payload)) {
				log.Print("invalid carrier result")
				app.Quit()
				return
			}
			encoded, err := json.Marshal(report{Carrier: json.RawMessage(payload), Effects: probe.Snapshot()})
			if err != nil {
				log.Printf("encode carrier result: %v", err)
			} else {
				fmt.Printf("carrier-result:%s\n", encoded)
			}
			app.Quit()
		},
	})
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "Flame plugin carrier probe", Width: 640, Height: 480, URL: "/",
	})
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		probe.ready.Add(1)
	})
	deadline := time.AfterFunc(20*time.Second, app.Quit)
	defer deadline.Stop()
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

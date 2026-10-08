package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed index.html
var assets embed.FS

// Probe records effects at the native owner, even when an attacking frame
// cannot read the response. It never opens a Runtime or touches user data.
type Probe struct {
	calls         atomic.Uint64
	ready         atomic.Uint64
	childMessages atomic.Uint64
}

func (p *Probe) Touch() uint64 { return p.calls.Add(1) }

func (p *Probe) Snapshot() map[string]uint64 {
	return map[string]uint64{"calls": p.calls.Load(), "ready": p.ready.Load(), "childMessages": p.childMessages.Load()}
}

func main() {
	probe := &Probe{}
	var app *application.App
	app = application.New(application.Options{
		Name:     "Flame plugin carrier probe",
		Services: []application.Service{application.NewService(probe)},
		Assets:   application.AssetOptions{Handler: http.FileServer(http.FS(assets))},
		RawMessageHandler: func(_ application.Window, message string, origin *application.OriginInfo) {
			if origin != nil && !origin.IsMainFrame && message == "carrier-witness" {
				probe.childMessages.Add(1)
				return
			}
			if origin == nil || !origin.IsMainFrame || !strings.HasPrefix(message, "carrier-result:") {
				return
			}
			var result map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(message, "carrier-result:")), &result); err != nil {
				log.Printf("invalid carrier result: %v", err)
				app.Quit()
				return
			}
			result["nativeFinal"] = probe.Snapshot()
			encoded, err := json.Marshal(result)
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

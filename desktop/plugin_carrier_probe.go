//go:build plugincarrierprobe

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
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed testdata/plugin-view/workbench.html
var gateAssets embed.FS

type probeEvent struct{ ID, Kind string }
type NativeGate struct {
	calls    atomic.Uint64
	observed chan probeEvent
}

func (g *NativeGate) Touch()                  { g.calls.Add(1) }
func (g *NativeGate) Observe(id, kind string) { g.observed <- probeEvent{ID: id, Kind: kind} }
func main() {
	if err := runPluginCarrierGate(); err != nil {
		log.Fatal(err)
	}
}
func runPluginCarrierGate() (err error) {
	root, err := os.MkdirTemp("", "flame-native-page-gate-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(root)) }()
	host, err := newDesktopHost(root)
	if err != nil {
		return err
	}
	peer, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	var packets atomic.Uint64
	peerDone := make(chan error, 1)
	go func() {
		body := make([]byte, 2048)
		for {
			_, _, e := peer.ReadFrom(body)
			if errors.Is(e, net.ErrClosed) {
				peerDone <- nil
				return
			}
			if e != nil {
				peerDone <- e
				return
			}
			packets.Add(1)
		}
	}()
	var peerOnce sync.Once
	var peerErr error
	closePeer := func() error { peerOnce.Do(func() { peerErr = errors.Join(peer.Close(), <-peerDone) }); return peerErr }
	defer func() { err = errors.Join(err, closePeer()) }()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	var requests atomic.Uint64
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(404) }), ReadHeaderTimeout: 5 * time.Second}
	serveDone := make(chan error, 1)
	go func() {
		e := server.Serve(listener)
		if errors.Is(e, http.ErrServerClosed) {
			e = nil
		}
		serveDone <- e
	}()
	var serverOnce sync.Once
	var serverErr error
	closeServer := func() error {
		serverOnce.Do(func() { serverErr = errors.Join(server.Close(), <-serveDone) })
		return serverErr
	}
	defer func() { err = errors.Join(err, closeServer()) }()
	gate := &NativeGate{observed: make(chan probeEvent, 16)}
	var readiness atomic.Uint64
	messages := make(chan json.RawMessage, 16)
	completed := make(chan error, 1)
	options := desktopApplicationOptions(host)
	options.Services = append(options.Services, application.NewService(gate))
	options.Assets = application.AssetOptions{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, e := gateAssets.ReadFile("testdata/plugin-view/workbench.html")
		if e != nil {
			http.Error(w, "fixture unavailable", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(body)
	})}
	var app *application.App
	var finishOnce sync.Once
	finish := func(result error) { finishOnce.Do(func() { completed <- result; app.Quit() }) }
	var start atomic.Bool
	options.RawMessageHandler = func(window application.Window, body string, source *application.OriginInfo) {
		if body != "gate-start" || source == nil || !source.IsMainFrame || !start.CompareAndSwap(false, true) {
			return
		}
		go func() {
			finish(verifyNativePluginPages(host, messages, gate.observed, peer.LocalAddr().String(), listener.Addr().String()))
		}()
	}
	app = application.New(options)
	window := app.Window.NewWithOptions(desktopWindowOptions())
	host.useWindow(window)
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) { readiness.Add(1) })
	host.pluginPagePublish = func(id string, message json.RawMessage) {
		app.Event.Emit("desktop:plugin-page", map[string]any{"id": id, "message": message})
		select {
		case messages <- message:
		default:
			finish(errors.New("native gate message capacity exceeded"))
		}
	}
	timeout := time.AfterFunc(20*time.Second, func() { finish(errors.New("native page gate deadline exceeded")) })
	defer timeout.Stop()
	optionsShutdown := func() {
		var outcome error
		select {
		case outcome = <-completed:
		default:
			outcome = errors.New("native gate ended without a result")
		}
		outcome = errors.Join(outcome, closeServer(), closePeer(), os.RemoveAll(root))
		report := struct {
			Calls, Ready, Packets, Requests uint64
			Error                           string `json:"error,omitempty"`
		}{Calls: gate.calls.Load(), Ready: readiness.Load(), Packets: packets.Load(), Requests: requests.Load()}
		if report.Calls != 1 || report.Ready != 1 || report.Packets != 0 || report.Requests != 0 {
			outcome = errors.Join(outcome, errors.New("native effects escaped"))
		}
		if outcome != nil {
			report.Error = outcome.Error()
		}
		encoded, e := json.Marshal(report)
		if e != nil {
			log.Fatal(e)
		}
		fmt.Println(string(encoded))
	}
	app.OnShutdown(optionsShutdown)
	return app.Run()
}
func verifyNativePluginPages(host *DesktopHost, messages <-chan json.RawMessage, observed <-chan probeEvent, peer, endpoint string) error {
	workbenchResponder, err := focusProbe(host, 20, 100)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	next := func() (map[string]json.RawMessage, error) {
		select {
		case body := <-messages:
			var value map[string]json.RawMessage
			if err := json.Unmarshal(body, &value); err != nil {
				return nil, err
			}
			return value, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	open := func() (string, error) {
		id, err := host.OpenPluginPage(PluginPageBounds{X: 100, Y: 100, Width: 640, Height: 480})
		if err != nil {
			return "", err
		}
		message, err := next()
		if err != nil {
			return "", err
		}
		if string(message["type"]) != `"ready"` {
			return "", fmt.Errorf("native initialization: %s", message)
		}
		for {
			select {
			case event := <-observed:
				if event.ID == id && event.Kind == "ready" {
					return id, nil
				}
			case <-ctx.Done():
				return "", errors.New("Wails SDK did not receive the native instance event")
			}
		}
	}
	id, err := open()
	if err != nil {
		return err
	}
	html := fmt.Sprintf(`<!doctype html><script>
addEventListener('message', async event=>{
 if(event.source!==parent || event.data?.type!=='flame.view.connect.v1')return;
 const port=event.ports[0];addEventListener('resize',()=>port.postMessage({type:'read',geometry:{width:innerWidth,height:innerHeight}}),{once:true});const evidence={geometry:innerWidth!==640||innerHeight!==480,peer:typeof RTCPeerConnection==='function',native:!!window.webkit?.messageHandlers?.external,dom:false,storage:false};
 try{parent.document.body;evidence.dom=true}catch{}
 try{localStorage.length;evidence.storage=true}catch{}
 try{window.webkit.messageHandlers.external.postMessage('wails:runtime:ready')}catch{}
 try{window.webkit.messageHandlers.carrier.postMessage(JSON.stringify({type:'request',request:{cursor:'forged-native'}}))}catch{}
 const endpoint='http://%s/leak';new Image().src=endpoint;
 try{await fetch(endpoint);evidence.fetch=true}catch{evidence.fetch=false}
 const image=new Image();await new Promise(resolve=>{image.onload=image.onerror=resolve;image.src='data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGOYuOs/AAQqAktocgm/AAAAAElFTkSuQmCC';document.body.append(image)});
 if(evidence.peer){const p=new RTCPeerConnection({iceServers:[{urls:'stun:%s'}]});p.createDataChannel('escape');await p.setLocalDescription();setTimeout(()=>p.close(),500)}
 port.postMessage({type:'read',evidence,imageLoaded:image.naturalWidth===1});
});parent.postMessage('flame.view.ready.v1','*');
</script>`, endpoint, peer)
	body, err := json.Marshal(map[string]any{"type": "boot", "html": html, "initial": map[string]any{"data": []any{}}, "scheme": "light"})
	if err != nil {
		return err
	}
	if err = host.SendPluginPage(id, string(body)); err != nil {
		return err
	}
	for {
		message, e := next()
		if e != nil {
			return e
		}
		if string(message["type"]) == `"connected"` {
			continue
		}
		var request struct {
			Evidence    map[string]bool `json:"evidence"`
			Cursor      string          `json:"cursor"`
			ImageLoaded bool            `json:"imageLoaded"`
		}
		if e = json.Unmarshal(message["request"], &request); e != nil {
			return e
		}
		if request.Cursor != "" || len(request.Evidence) != 6 {
			return fmt.Errorf("unexpected native message: %s", message)
		}
		if !request.ImageLoaded {
			return errors.New("native carrier did not render the inline image")
		}
		for name, value := range request.Evidence {
			if value {
				return fmt.Errorf("guest escaped %s", name)
			}
		}
		break
	}
	if err = host.PositionPluginPage(id, PluginPageBounds{X: 0, Y: 0, Width: 512, Height: 420}); err != nil {
		return err
	}
	message, err := next()
	if err != nil {
		return err
	}
	var resized struct{ Geometry struct{ Width, Height int } }
	if err = json.Unmarshal(message["request"], &resized); err != nil {
		return err
	}
	if resized.Geometry.Width != 512 || resized.Geometry.Height != 420 {
		return fmt.Errorf("native viewport geometry = %+v", resized.Geometry)
	}
	pageResponder, err := focusProbe(host, 100, 100)
	if err != nil {
		return err
	}
	if pageResponder == workbenchResponder {
		return errors.New("native page did not receive independent focus")
	}
	if err = host.PositionPluginPage(id, PluginPageBounds{}); err != nil {
		return err
	}
	if err = verifyProbeResponder(host, workbenchResponder); err != nil {
		return err
	}
	revealed, err := focusProbe(host, 100, 100)
	if err != nil || revealed != workbenchResponder {
		return errors.Join(err, errors.New("hidden page still covered the workbench"))
	}
	if err = host.PositionPluginPage(id, PluginPageBounds{X: 0, Y: 0, Width: 512, Height: 420}); err != nil {
		return err
	}
	if _, err = focusProbe(host, 100, 100); err != nil {
		return err
	}
	if err = host.ClosePluginPage(id); err != nil {
		return err
	}
	if err = verifyProbeResponder(host, workbenchResponder); err != nil {
		return err
	}

	successor, err := open()
	if err != nil {
		return err
	}
	if err = host.SendPluginPage(id, `{"type":"boot"}`); err == nil {
		return errors.New("retired page accepted publication")
	}
	if err = host.ClosePluginPage(id); err != nil {
		return err
	}
	if err = host.SendPluginPage(successor, string(body)); err != nil {
		return err
	}
	for {
		message, e := next()
		if e != nil {
			return e
		}
		if string(message["type"]) == `"request"` {
			break
		}
	}
	if _, err = focusProbe(host, 150, 150); err != nil {
		return err
	}
	overlapping, err := open()
	if err != nil {
		return err
	}
	if _, err = focusProbe(host, 150, 150); err != nil {
		return err
	}
	if err = host.ClosePluginPage(successor); err != nil {
		return err
	}
	if err = host.ClosePluginPage(overlapping); err != nil {
		return err
	}
	if err = verifyProbeResponder(host, workbenchResponder); err != nil {
		return err
	}
	time.Sleep(600 * time.Millisecond)
	return nil
}

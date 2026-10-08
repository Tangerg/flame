package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed frontend/public/plugin-carrier.html
var pluginCarrierAssets embed.FS

const (
	maxPluginMessageBytes = 4 << 20
	maxNativePluginPages  = 16
)

type PluginPageBounds struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

func (b PluginPageBounds) validate() error {
	for _, value := range []float64{b.X, b.Y, b.Width, b.Height} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return errors.New("plugin carrier: invalid bounds")
		}
	}
	return nil
}

// Native resources belong to the main-thread actor; the frontend authorizes reads.
func (d *DesktopHost) OpenPluginPage(bounds PluginPageBounds) (id string, err error) {
	if err = bounds.validate(); err != nil {
		return "", err
	}
	if d.window == nil || d.pluginPagePublish == nil {
		return "", errors.New("plugin carrier: native host unavailable")
	}
	body, err := pluginCarrierAssets.ReadFile("frontend/public/plugin-carrier.html")
	if err != nil {
		return "", err
	}
	application.InvokeSync(func() {
		if d.pluginPagesClosed {
			err = errors.New("plugin carrier: host retired")
			return
		}
		if len(d.pluginPages) >= maxNativePluginPages {
			err = errors.New("plugin carrier: native page capacity exceeded")
			return
		}
		d.pluginPageSequence++
		id = fmt.Sprintf("page-%d", d.pluginPageSequence)
		var page *nativePluginPage
		page, err = createNativePluginPage(d.window.NativeWindow(), string(body), bounds, func(message json.RawMessage) {
			if page != nil && d.pluginPages[id] == page {
				d.pluginPagePublish(id, message)
			}
		})
		if err == nil {
			d.pluginPages[id] = page
		}
	})
	return id, err
}

func (d *DesktopHost) SendPluginPage(id, message string) (err error) {
	if len(message) > maxPluginMessageBytes || !json.Valid([]byte(message)) {
		return errors.New("plugin carrier: invalid message")
	}
	application.InvokeSync(func() {
		page := d.pluginPages[id]
		if page == nil {
			err = errors.New("plugin carrier: page retired")
			return
		}
		page.send(message)
	})
	return err
}

func (d *DesktopHost) PositionPluginPage(id string, bounds PluginPageBounds) (err error) {
	if err = bounds.validate(); err != nil {
		return err
	}
	application.InvokeSync(func() {
		if page := d.pluginPages[id]; page != nil {
			page.position(bounds)
		}
	})
	return nil
}

func (d *DesktopHost) ClosePluginPage(id string) error {
	application.InvokeSync(func() { d.closePluginPage(id) })
	return nil
}

func (d *DesktopHost) closePluginPage(id string) {
	page := d.pluginPages[id]
	if page == nil {
		return
	}
	delete(d.pluginPages, id)
	page.close()
}

func (d *DesktopHost) retirePluginPages() {
	d.pluginPagesClosed = true
	for id := range d.pluginPages {
		d.closePluginPage(id)
	}
}

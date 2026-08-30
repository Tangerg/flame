package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

const (
	productName         = "Flame"
	productDescription  = "Agent client for the Flame Runtime"
	defaultWindowWidth  = 1440
	defaultWindowHeight = 900
	minimumWindowWidth  = 1120
	minimumWindowHeight = 720
)

// The application, which owns the process: its name, what it serves, and what the
// frontend may call. Geometry and window chrome are NOT here — in v3 a window is a
// separate object with its own options, which is the honest shape: an application can
// hold several windows, and none of their sizes is a property of the process.
func desktopApplicationOptions(host *DesktopHost) application.Options {
	return application.Options{
		Name:        productName,
		Description: productDescription,
		// The Wails-owned boundary, and the whole of it: one service, whose methods are
		// the only Go the frontend can reach.
		Services: []application.Service{application.NewService(host)},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			// One window, and closing it means quitting. Without this the process outlives
			// its own window and the dock icon stays lit with nothing behind it.
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	}
}

// A real titled window with a transparent, empty title bar and content running full height
// underneath it. The platform draws its own three controls over that content; the app
// reserves a gutter for them from measured geometry (DesktopHost.WindowChrome).
//
// UseToolbar is here for its HEIGHT, not for a toolbar: it is what makes the titlebar
// taller than 32pt, and the compact style then pins it at 40pt so the frame buttons land
// within a pixel of a 42pt header's center line. An empty toolbar does NOT take clicks in
// that band — verified by hit-testing the frame view at 8/16/24/36/44pt.
//
// `Hide` stays false deliberately: setting it drops NSWindowStyleMaskTitled, which removes
// the buttons and the window frame with them, leaving square corners and no shadow.
func desktopWindowOptions() application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Title:            productName,
		Width:            defaultWindowWidth,
		Height:           defaultWindowHeight,
		MinWidth:         minimumWindowWidth,
		MinHeight:        minimumWindowHeight,
		URL:              "/",
		BackgroundColour: application.NewRGB(255, 255, 255),
		Mac: application.MacWindow{
			TitleBar: application.MacTitleBar{
				AppearsTransparent:   true,
				HideTitle:            true,
				FullSizeContent:      true,
				UseToolbar:           true,
				HideToolbarSeparator: true,
				// Automatic style resolves a transparent-titlebar window to a 66pt titlebar,
				// placing the controls below the app header. Declare compact style at creation.
				ToolbarStyle: application.MacToolbarStyleUnifiedCompact,
			},
			Appearance: application.NSAppearanceNameAqua,
		},
	}
}

func main() {
	host, err := defaultDesktopHost()
	if err != nil {
		log.Fatal(err)
	}
	app := application.New(desktopApplicationOptions(host))
	// The service is registered before any window exists, so the window it measures is
	// named here rather than found later. v3 is a multi-window framework: asking the
	// platform for "the app's window" is a guess that a sheet or a second window wins.
	window := app.Window.NewWithOptions(desktopWindowOptions())
	host.useWindow(window)
	host.useWorkingDirectoryPicker(wailsWorkingDirectoryPicker{
		dialogs: app.Dialog,
		window:  window,
	})
	host.useImageSaver(wailsImageSaver{
		dialogs: app.Dialog,
		window:  window,
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

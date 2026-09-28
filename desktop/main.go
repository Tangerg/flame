package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
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

func desktopApplicationOptions(host *DesktopHost) application.Options {
	return application.Options{
		Name:        productName,
		Description: productDescription,
		Services:    []application.Service{application.NewService(host)},
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

// The WebView owns theme preferences. Until it loads, use the system default canvas;
// check-bootstrap verifies these colors against the HTML and stylesheet.
func desktopWindowBackground() application.RGBA {
	if !systemPrefersDarkAppearance() {
		return application.NewRGB(255, 255, 255)
	}
	return application.NewRGB(31, 31, 31)
}

// Hiding the native titlebar also removes macOS window controls, corners, and shadow.
func desktopWindowOptions() application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Title:            productName,
		Width:            defaultWindowWidth,
		Height:           defaultWindowHeight,
		MinWidth:         minimumWindowWidth,
		MinHeight:        minimumWindowHeight,
		URL:              "/",
		BackgroundColour: desktopWindowBackground(),
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
	window := app.Window.NewWithOptions(desktopWindowOptions())
	host.useWindow(window)
	host.usePathRevealer(app.Env)
	host.usePathOpener(app.Browser)
	host.useNotifications(wailsNotifications{
		service: notifications.New(),
		opened: func(target string) {
			app.Event.Emit(notificationOpenedEvent, target)
		},
	})
	host.useRevealer(func() {
		window.UnMinimise()
		window.Show()
		window.Focus()
	})
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

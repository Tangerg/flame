package main

import (
	"errors"
	"log"

	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/Tangerg/flame/apps/desktop/internal/bridge"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func main() {
	host, err := defaultDesktopHost()
	if err != nil {
		log.Fatal(err)
	}
	app := application.New(desktopApplicationOptions(host))
	window := app.Window.NewWithOptions(desktopWindowOptions())
	shell := nativeShell{
		window: window,
		reveal: func() {
			window.UnMinimise()
			window.Show()
			window.Focus()
		},
		pathRevealer: app.Env,
		pathOpener:   app.Browser,
		directories:  wailsWorkingDirectoryPicker{dialogs: app.Dialog, window: window},
		files:        wailsFileSaver{dialogs: app.Dialog, window: window},
		notifications: wailsNotifications{
			service: notifications.New(),
			opened: func(target bridge.NotificationTarget) {
				app.Event.Emit(bridge.NotificationOpenedEvent, target)
			},
		},
	}
	if err := host.bind(shell); err != nil {
		log.Fatal(err)
	}
	discardExport := func(*application.WindowEvent) {
		if err := host.transfer.abort(); err != nil {
			log.Printf("desktop host: discard export: %v", err)
		}
	}
	window.RegisterHook(events.Mac.WebViewDidCommitNavigation, discardExport)
	window.RegisterHook(events.Common.WindowClosing, discardExport)
	installNativeZoom(app)
	err = errors.Join(app.Run(), host.transfer.abort())
	if err != nil {
		log.Fatal(err)
	}
}

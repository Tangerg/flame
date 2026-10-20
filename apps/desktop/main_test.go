package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestDesktopWindowGeometry(t *testing.T) {
	window := desktopWindowOptions()

	if window.Width != 1440 || window.Height != 900 {
		t.Fatalf("default window size = %dx%d, want 1440x900", window.Width, window.Height)
	}
	if window.MinWidth != 1120 || window.MinHeight != 720 {
		t.Fatalf("minimum window size = %dx%d, want 1120x720", window.MinWidth, window.MinHeight)
	}
	if window.MinWidth > window.Width || window.MinHeight > window.Height {
		t.Fatalf(
			"minimum window size %dx%d exceeds default %dx%d",
			window.MinWidth,
			window.MinHeight,
			window.Width,
			window.Height,
		)
	}
}

func TestDesktopWindowPinsTheCompactToolbarStyle(t *testing.T) {
	titleBar := desktopWindowOptions().Mac.TitleBar

	if titleBar.ToolbarStyle != application.MacToolbarStyleUnifiedCompact {
		t.Fatalf("toolbar style = %v, want unified compact", titleBar.ToolbarStyle)
	}
	if !titleBar.UseToolbar {
		t.Fatal("a toolbar style with no toolbar is not applied; UseToolbar must stay true")
	}

	if titleBar.Hide {
		t.Fatal("the title bar is transparent, not hidden")
	}
}

func TestDesktopWindowOpensOnACanvasTheAppPaints(t *testing.T) {
	light := application.NewRGB(247, 244, 243)
	dark := application.NewRGB(17, 14, 13)

	want := light
	if systemPrefersDarkAppearance() {
		want = dark
	}
	if got := desktopWindowBackground(); got != want {
		t.Fatalf("window background = %v, want %v for this appearance", got, want)
	}
	if desktopWindowOptions().BackgroundColour != want {
		t.Fatal("the window options carry a colour the background function did not decide")
	}
	if light == dark {
		t.Fatal("the two canvases must differ, or the appearance read decides nothing")
	}
}

func TestDesktopApplicationBindsHostAsItsOnlyService(t *testing.T) {
	host := mustDesktopHost(t, t.TempDir())
	services := desktopApplicationOptions(host).Services

	want := []application.Service{application.NewService(host)}
	if !reflect.DeepEqual(services, want) {
		t.Fatalf("services = %#v, want exactly the configured host", services)
	}
}

func TestStagedWebIndexIsServedAtTheNativeRoot(t *testing.T) {
	host := mustDesktopHost(t, t.TempDir())
	response := httptest.NewRecorder()
	desktopApplicationOptions(host).Assets.Handler.ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
	expected, err := assets.ReadFile("assets/web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), expected) {
		t.Fatalf("native root does not serve the staged Web index: status=%d", response.Code)
	}
}

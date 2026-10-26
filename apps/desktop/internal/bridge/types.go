package bridge

type LocalRuntimeConnection struct {
	Endpoint   string `json:"endpoint"`
	LocalToken string `json:"localToken,omitempty"`
}

type DesktopBootstrap struct {
	LocalRuntime LocalRuntimeConnection `json:"localRuntime"`
}

// Geometry uses CSS pixels from the window top-left. Measured zeroes mean fullscreen;
// Measured false means the platform supplied no geometry, so CSS keeps its defaults.
type WindowChrome struct {
	ControlsCentreY   float64 `json:"controlsCentreY"`
	ControlsInlineEnd float64 `json:"controlsInlineEnd"`
	Measured          bool    `json:"measured"`
}

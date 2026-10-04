package plugin

type Transport string

const (
	Stdio          Transport = "stdio"
	StreamableHTTP Transport = "streamable-http"
)

type InputTarget string

const (
	Environment   InputTarget = "env"
	Header        InputTarget = "header"
	Authorization InputTarget = "authorization"
)

type Capability string

const (
	InvokeTools Capability = "tools.invoke"
)

type ThemeScheme string

const (
	Dark  ThemeScheme = "dark"
	Light ThemeScheme = "light"
)

package plugin

type InputTarget string

const (
	Environment   InputTarget = "env"
	Header        InputTarget = "header"
	Authorization InputTarget = "authorization"
)

type ThemeScheme string

const (
	Dark  ThemeScheme = "dark"
	Light ThemeScheme = "light"
)

package plugin

type InputTarget string

const (
	Environment   InputTarget = "env"
	Header        InputTarget = "header"
	Authorization InputTarget = "authorization"
)

type ThemeScheme string

const (
	ThemeDark  ThemeScheme = "dark"
	ThemeLight ThemeScheme = "light"
)

package bootstrap

type terminalResource interface {
	Close() error
}

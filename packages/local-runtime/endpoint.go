package localruntime

const (
	// DefaultListenAddress is shared by the local Runtime and its clients.
	DefaultListenAddress = "127.0.0.1:17171"
	// DefaultOrigin projects the default listener into the clients' HTTP address.
	DefaultOrigin = "http://" + DefaultListenAddress
)

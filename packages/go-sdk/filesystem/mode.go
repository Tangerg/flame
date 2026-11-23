package filesystem

import "io/fs"

// Flame keeps its own state private to the user; authored skills and generated
// artifacts stay readable by the user's other tools.
const (
	PrivateDirectoryMode  fs.FileMode = 0o700
	PrivateFileMode       fs.FileMode = 0o600
	PrivateExecutableMode fs.FileMode = 0o700
	SharedDirectoryMode   fs.FileMode = 0o755
	SharedFileMode        fs.FileMode = 0o644
	ReadOnlyFileMode      fs.FileMode = 0o400
	ExecutableBits        fs.FileMode = 0o111
)

// MiB is the unit byte limits are reported in.
const MiB = 1 << 20

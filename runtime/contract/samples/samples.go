package samples

import "embed"

// Files retains the independently authored wire examples byte for byte.
//
//go:embed *.json
var Files embed.FS

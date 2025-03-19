package manifests

import "embed"

//go:embed server
var ServerFiles embed.FS

//go:embed client
var ClientFiles embed.FS

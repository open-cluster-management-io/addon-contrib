package manifests

import "embed"

//go:embed flower/server
var FlowerServerFiles embed.FS

//go:embed flower/client
var FlowerClientFiles embed.FS

// go:embed openfl/server
var OpenFLServerFiles embed.FS

// go:embed openfl/client
var OpenFLClientFiles embed.FS

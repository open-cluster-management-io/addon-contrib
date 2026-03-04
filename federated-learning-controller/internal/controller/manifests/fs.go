package manifests

import "embed"

//go:embed flower/serverapp
var FlowerServerAppFiles embed.FS

//go:embed flower/clientapp
var FlowerClientAppFiles embed.FS

//go:embed openfl/server
var OpenFLServerFiles embed.FS

//go:embed openfl/client
var OpenFLClientFiles embed.FS

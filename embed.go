package labplatform

import "embed"

//go:embed web/static
var StaticFiles embed.FS

//go:embed scenarios
var ScenariosFS embed.FS

//go:embed vm
var VMFiles embed.FS

package web

import "embed"

//go:embed all:frontend/dist
var FrontendFS embed.FS

// Package web embeds the built Vue SPA (web/dist). Before the frontend is
// built, dist holds only .gitkeep and the console serves a 503 placeholder.
package web

import "embed"

//go:embed all:dist
var DistFS embed.FS

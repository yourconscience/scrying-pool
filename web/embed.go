// Package web holds the app's pages, styles and scripts, embedded into scry.
package web

import "embed"

//go:embed *.html *.css *.js
var Files embed.FS

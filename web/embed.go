// Package web is the embedded browser UI.
package web

import "embed"

// Files is the static UI served at /.
//
//go:embed index.html styles.css app.js
var Files embed.FS

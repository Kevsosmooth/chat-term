// Package guide holds the cheat sheet picture that .guide sends.
//
// cheatsheet.html is generated from the command table; a test in
// internal/bot fails when it is out of date. After updating it, render the
// picture and the printable copy:
//
//	go test ./internal/bot -run TestCheatsheet -update
//	shot-scraper internal/guide/cheatsheet.html -o internal/guide/cheatsheet.png --width 720 --retina
//	shot-scraper pdf internal/guide/cheatsheet.html -o docs/cheatsheet.pdf --print-background
package guide

import _ "embed"

//go:embed cheatsheet.png
var PNG []byte

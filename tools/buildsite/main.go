// buildsite builds the hosted site, the one published on GitHub Pages, into
// site/ (or -o DIR):
//
//	index.html, data/   the Profiles page with the profiles in site-data/ (scry site)
//	notes.html          the Match notes page with CSS and JS inlined
//	scry.wasm           the formatter, compiled to WebAssembly
//
// Run from the repo root: go run ./tools/buildsite
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "buildsite:", err)
		os.Exit(1)
	}
}

func run() error {
	out := flag.String("o", "site", "output folder")
	flag.Parse()
	profiles := exec.Command("go", "run", "./scry", "site", "-data", "site-data", "-o", *out)
	profiles.Stdout, profiles.Stderr = os.Stdout, os.Stderr
	if err := profiles.Run(); err != nil {
		return err
	}
	build := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(*out, "scry.wasm"), "./scry")
	build.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		return err
	}
	goroot, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return err
	}
	wasmExec, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(goroot)), "lib", "wasm", "wasm_exec.js"))
	if err != nil {
		return err
	}
	page, err := os.ReadFile("scry/web/notes.html")
	if err != nil {
		return err
	}
	css, err := os.ReadFile("scry/web/style.css")
	if err != nil {
		return err
	}
	app, err := os.ReadFile("scry/web/app.js")
	if err != nil {
		return err
	}
	for name, s := range map[string][]byte{"wasm_exec.js": wasmExec, "app.js": app, "style.css": css} {
		if strings.Contains(strings.ToLower(string(s)), "</script") || strings.Contains(strings.ToLower(string(s)), "</style") {
			return fmt.Errorf("%s contains a closing tag and can't be inlined", name)
		}
	}

	html := string(page)
	html = replaceOnce(html, `<link rel="stylesheet" href="style.css">`, "<style>\n"+string(css)+"</style>")
	html = replaceOnce(html, `<script src="app.js"></script>`,
		"<script>window.SCRY_STATIC = true;</script>\n<script>\n"+string(wasmExec)+"</script>\n<script>\n"+string(app)+"</script>")
	if err := os.WriteFile(filepath.Join(*out, "notes.html"), []byte(html), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s: notes.html, scry.wasm\n", *out)
	return nil
}

func replaceOnce(s, old, new string) string {
	if strings.Count(s, old) != 1 {
		panic(fmt.Sprintf("expected exactly one %q in scry/web/notes.html", old))
	}
	return strings.Replace(s, old, new, 1)
}

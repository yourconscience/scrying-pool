// buildsite builds the hosted, browser-only version of the web UI into site/:
//
//	site/index.html     full page with CSS and JS inlined (any static host)
//	site/artifact.html  the same page without the html/head/body wrapper
//	site/dombot.wasm    the formatter, compiled to WebAssembly
//
// Run from the repo root: go run ./cmd/buildsite
package main

import (
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
	if err := os.MkdirAll("site", 0o755); err != nil {
		return err
	}
	build := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", "site/dombot.wasm", ".")
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
	page, err := os.ReadFile("web/index.html")
	if err != nil {
		return err
	}
	css, err := os.ReadFile("web/style.css")
	if err != nil {
		return err
	}
	app, err := os.ReadFile("web/app.js")
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
		"<script>window.DOMBOT_STATIC = true;</script>\n<script>\n"+string(wasmExec)+"</script>\n<script>\n"+string(app)+"</script>")
	if err := os.WriteFile("site/index.html", []byte(html), 0o644); err != nil {
		return err
	}

	var kept []string
	for _, l := range strings.Split(html, "\n") {
		switch strings.TrimSpace(l) {
		case "<!doctype html>", `<html lang="en">`, "<head>", "</head>", "<body>", "</body>", "</html>",
			`<meta charset="utf-8">`, `<meta name="viewport" content="width=device-width, initial-scale=1">`:
			continue
		}
		kept = append(kept, l)
	}
	if err := os.WriteFile("site/artifact.html", []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		return err
	}
	fmt.Println("built site/index.html, site/artifact.html, site/dombot.wasm")
	return nil
}

func replaceOnce(s, old, new string) string {
	if strings.Count(s, old) != 1 {
		panic(fmt.Sprintf("expected exactly one %q in web/index.html", old))
	}
	return strings.Replace(s, old, new, 1)
}

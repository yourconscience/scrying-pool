//go:build !js

package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

//go:embed web
var webFiles embed.FS

func serve(addr string, timeout time.Duration) error {
	fmt.Printf("Scrying Pool web UI: http://%s\n", addr)
	return http.ListenAndServe(addr, newHandler(timeout))
}

func newHandler(timeout time.Duration) http.Handler {
	static, _ := fs.Sub(webFiles, "web")
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(static))
	mux.HandleFunc("GET /api/sample", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"chat": sampleChat, "dombot": sampleDomBot})
	})
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		_, err := token()
		writeJSON(w, 200, map[string]bool{"discord": err == nil, "art": true})
	})
	mux.HandleFunc("GET /art/{file}", func(w http.ResponseWriter, r *http.Request) {
		p, err := artPath(r.PathValue("file"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "max-age=604800")
		http.ServeFile(w, r, p)
	})
	mux.HandleFunc("POST /api/format", func(w http.ResponseWriter, r *http.Request) {
		// Only our own page may trigger this: it can send a DM from the user's account.
		if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
			writeJSON(w, 403, map[string]string{"error": "cross-origin request refused"})
			return
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeJSON(w, 415, map[string]string{"error": "expected JSON"})
			return
		}
		var req struct {
			Chat   string `json:"chat"`
			DomBot string `json:"dombot"`
			Ask    bool   `json:"ask"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&req); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad request: " + err.Error()})
			return
		}
		res, err := process(req.Chat, req.DomBot, req.Ask, timeout)
		if err != nil {
			writeJSON(w, 422, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, res)
	})
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

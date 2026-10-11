//go:build !js

package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/yourconscience/scrying-pool/cartographer"
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
	mux.HandleFunc("GET /card-image/{name}", func(w http.ResponseWriter, r *http.Request) {
		p, ok := cardImage(r.PathValue("name"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "max-age=604800")
		http.ServeFile(w, r, p)
	})
	mux.HandleFunc("GET /api/games", func(w http.ResponseWriter, r *http.Request) {
		dir, err := gamesDir("")
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		games, err := listGames(dir)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, games)
	})
	mux.HandleFunc("GET /api/games/{id}", func(w http.ResponseWriter, r *http.Request) {
		client, err := webClient()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		dir, err := gamesDir("")
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		v, err := loadGameView(client, dir, r.PathValue("id"))
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(w, 404, map[string]string{"error": "game " + r.PathValue("id") + " isn't saved yet"})
			return
		}
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, v)
	})
	// The "Scry game" bookmark posts the game it loaded from the dominion.games
	// tab, as text/plain so the browser sends it without a preflight.
	mux.HandleFunc("OPTIONS /api/games", func(w http.ResponseWriter, r *http.Request) {
		if allowGamePost(w, r) {
			w.Header().Set("Access-Control-Allow-Methods", "POST")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /api/games", func(w http.ResponseWriter, r *http.Request) {
		if !allowGamePost(w, r) {
			writeJSON(w, 403, map[string]string{"error": "cross-origin request refused"})
			return
		}
		var g exportedGame
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20)).Decode(&g); err != nil {
			writeJSON(w, 400, map[string]string{"error": "not a captured game: " + err.Error()})
			return
		}
		if !reGameID.MatchString(g.ID) {
			writeJSON(w, 400, map[string]string{"error": fmt.Sprintf("bad game id %q", g.ID)})
			return
		}
		client, err := webClient()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		dir, err := gamesDir("")
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		res, _, err := saveGame(client, dir, g, 0)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"id": g.ID, "problems": res.Problems})
	})
	mux.HandleFunc("GET /api/bookmarklet", func(w http.ResponseWriter, r *http.Request) {
		capture, _ := webFiles.ReadFile("web/capture.js")
		one, _ := webFiles.ReadFile("web/scry-game.js")
		src := "if (!window.scryCapture) " + stripComments(string(capture)) + "\n" + strings.ReplaceAll(string(one), "SCRY_ORIGIN", "http://"+r.Host)
		writeJSON(w, 200, map[string]string{"href": bookmarklet(src)})
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

// allowGamePost lets our own pages and the dominion.games tab post games.
func allowGamePost(w http.ResponseWriter, r *http.Request) bool {
	switch o := r.Header.Get("Origin"); o {
	case "", "http://" + r.Host:
		return true
	case "https://dominion.games":
		w.Header().Set("Access-Control-Allow-Origin", o)
		w.Header().Set("Vary", "Origin")
		return true
	}
	return false
}

var (
	clientMu     sync.Mutex
	cachedClient *cartographer.Client
)

// webClient loads the client tables once they exist.
func webClient() (*cartographer.Client, error) {
	clientMu.Lock()
	defer clientMu.Unlock()
	if cachedClient == nil {
		c, err := loadClient()
		if err != nil {
			return nil, err
		}
		cachedClient = c
	}
	return cachedClient, nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

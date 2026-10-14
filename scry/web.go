//go:build !js

package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/yourconscience/scrying-pool/scry/internal/advisor"
	"github.com/yourconscience/scrying-pool/scry/internal/cartographer"
)

//go:embed web
var webFiles embed.FS

func serve(addr string, timeout time.Duration) error {
	fmt.Printf("Scrying Pool web UI: http://%s\n", addr)
	go loadCorpus() // read the saved games now, so the Profiles page doesn't wait for them
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
	mux.HandleFunc("GET /api/players", func(w http.ResponseWriter, r *http.Request) {
		c, err := loadCorpus()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"default": c.account, "players": advisor.Players(c.features, minProfileGames)})
	})
	mux.HandleFunc("GET /api/insights", func(w http.ResponseWriter, r *http.Request) {
		in, err := insights(r.URL.Query().Get("player"))
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, in)
	})
	mux.HandleFunc("GET /api/recent", func(w http.ResponseWriter, r *http.Request) {
		c, err := loadCorpus()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		player := r.URL.Query().Get("player")
		if player == "" {
			player = c.account
		}
		writeJSON(w, 200, advisor.LatestGames(player, c.features, recentGames))
	})
	mux.HandleFunc("GET /api/features.csv", func(w http.ResponseWriter, r *http.Request) {
		in, err := insights(r.URL.Query().Get("player"))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="scrying-pool-games.csv"`)
		writeFeatures(w, in.Games)
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
	// The Recent games page passes on what your profile lists about games that
	// are saved already: when they were played and both levels.
	mux.HandleFunc("POST /api/profiles", func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
			writeJSON(w, 403, map[string]string{"error": "cross-origin request refused"})
			return
		}
		var in []struct {
			ID      string       `json:"id"`
			Profile *gameProfile `json:"profile"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		dir, err := gamesDir("")
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		n := 0
		for _, g := range in {
			if g.Profile != nil && addProfile(dir, g.ID, g.Profile) == nil {
				n++
			}
		}
		writeJSON(w, 200, map[string]int{"updated": n})
	})
	// The Scry game bookmark, or with ?for=recent the Recent games one: capture.js
	// (unless the tab has this version of it already) and the bookmark's own script.
	mux.HandleFunc("GET /api/bookmarklet", func(w http.ResponseWriter, r *http.Request) {
		name := "web/scry-game.js"
		if r.URL.Query().Get("for") == "recent" {
			name = "web/scry-recent.js"
		}
		capture, _ := webFiles.ReadFile("web/capture.js")
		one, _ := webFiles.ReadFile(name)
		version := reCaptureVersion.FindStringSubmatch(string(capture))[1]
		src := "if ((window.scryCapture && window.scryCapture.version) !== " + version + ") " + stripComments(string(capture)) + "\n" +
			strings.ReplaceAll(string(one), "SCRY_ORIGIN", "http://"+r.Host)
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

	corpusMu    sync.Mutex
	corpusCache *corpus
)

// corpus is every saved game's features, kept until a game is added.
type corpus struct {
	stamp    time.Time // the newest score file when it was read
	account  string
	features []*advisor.Features
	insights map[string]*advisor.Insights
}

// minProfileGames is how many saved games a player needs for a profile.
const minProfileGames = 20

var reCaptureVersion = regexp.MustCompile(`version: (\d+),`)

// recentGames is how many latest games the Recent games page shows: as many as
// a dominion.games profile lists.
const recentGames = 25

// loadCorpus reads the saved games, or returns the copy from the last read when
// no score file changed since. Reading 244 games takes about 3 seconds.
func loadCorpus() (*corpus, error) {
	client, err := webClient()
	if err != nil {
		return nil, err
	}
	dir, err := gamesDir("")
	if err != nil {
		return nil, err
	}
	var newest time.Time
	files, _ := filepath.Glob(filepath.Join(dir, "*.score.txt"))
	for _, f := range files {
		if st, err := os.Stat(f); err == nil && st.ModTime().After(newest) {
			newest = st.ModTime()
		}
	}
	corpusMu.Lock()
	defer corpusMu.Unlock()
	if c := corpusCache; c != nil && c.stamp.Equal(newest) {
		return c, nil
	}
	account, _, features, err := advisor.Corpus(client, dir)
	if err != nil {
		return nil, err
	}
	corpusCache = &corpus{stamp: newest, account: account, features: features, insights: map[string]*advisor.Insights{}}
	return corpusCache, nil
}

// insights summarizes one player's saved games (default: the account that
// loaded most of them).
func insights(player string) (*advisor.Insights, error) {
	c, err := loadCorpus()
	if err != nil {
		return nil, err
	}
	if player == "" {
		player = c.account
	}
	corpusMu.Lock()
	defer corpusMu.Unlock()
	if in, ok := c.insights[player]; ok {
		return in, nil
	}
	in := advisor.Summarize(player, advisor.Of(player, c.features))
	in.Images = cardImages(in, func(name, _ string) string { return "/card-image/" + url.PathEscape(name) })
	c.insights[player] = in
	return in, nil
}

// cardImages maps the profile's kingdom cards that have an image in
// card-images/ to a URL; the function gets the card and the image's path there.
func cardImages(in *advisor.Insights, link func(name, rel string) string) map[string]string {
	out := map[string]string{}
	for _, c := range in.Cards {
		if p, ok := cardImage(c.Card); ok {
			rel, err := filepath.Rel(cardImageDir(), p)
			if err != nil {
				continue
			}
			out[c.Card] = link(c.Card, filepath.ToSlash(rel))
		}
	}
	return out
}

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

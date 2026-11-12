//go:build !js

package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
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
			if g.Profile == nil {
				continue
			}
			switch err := addProfile(dir, g.ID, g.Profile); {
			case err == nil:
				n++
			case !errors.Is(err, fs.ErrNotExist): // a game that isn't saved yet is fine
				writeJSON(w, 500, map[string]string{"error": fmt.Sprintf("game %s: %v", g.ID, err)})
				return
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
		writeJSON(w, 200, map[string]string{"href": bookmarklet(bookmarkSource(name, "http://"+r.Host))})
	})
	// The Last games page: a report from the saved games, and a run of `scry games`
	// that loads the missing ones in the dominion.games tab.
	mux.HandleFunc("GET /api/report", localOnly(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		c, err := loadCorpus()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
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
		ask := gamesAsk{Me: q.Get("me"), Opponent: strings.TrimSpace(q.Get("opponent")), Last: reportLast(q.Get("last")), IDs: reNumber.FindAllString(q.Get("ids"), -1)}
		rep, err := buildGames(client, dir, c.account, c.profiles, c.features, ask)
		if err != nil {
			writeJSON(w, 422, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, rep)
	}))
	mux.HandleFunc("GET /api/report/load", localOnly(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, loadStatus())
	}))
	mux.HandleFunc("POST /api/report/load", localOnly(func(w http.ResponseWriter, r *http.Request) {
		// Only our own page may start a run: it drives the dominion.games tab.
		if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
			writeJSON(w, 403, map[string]string{"error": "cross-origin request refused"})
			return
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeJSON(w, 415, map[string]string{"error": "expected JSON"})
			return
		}
		var req struct {
			Last     string `json:"last"`
			Opponent string `json:"opponent"`
			IDs      string `json:"ids"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad request: " + err.Error()})
			return
		}
		if req.Opponent = strings.TrimSpace(req.Opponent); len(req.Opponent) > 64 {
			writeJSON(w, 400, map[string]string{"error": "that's too long for a player's name"})
			return
		}
		dir, err := gamesDir("")
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		if err := startLoad(dir, reportLast(req.Last), req.Opponent, reNumber.FindAllString(req.IDs, -1)); err != nil {
			writeJSON(w, 409, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 202, loadStatus())
	}))
	// The Scout page: the players scouted before and the latest run, a player's
	// report and battles from the saved games, and a run of `scry scout` that loads their
	// games in the dominion.games tab.
	mux.HandleFunc("GET /api/scouts", localOnly(func(w http.ResponseWriter, r *http.Request) {
		dir, err := gamesDir("")
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"players": scouted(dir), "run": scoutStatus()})
	}))
	mux.HandleFunc("GET /api/scout", localOnly(func(w http.ResponseWriter, r *http.Request) {
		dir, err := gamesDir("")
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		player := r.URL.Query().Get("player")
		md, err := scout(player, "", dir, scoutGames(r.URL.Query().Get("n")), false)
		if err != nil {
			writeJSON(w, 422, map[string]string{"error": err.Error()})
			return
		}
		c, err := loadCorpus()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		client, err := webClient()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		battles, err := battlesOf(client, dir, c.features, player)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"player": player, "markdown": md, "battles": battles})
	}))
	mux.HandleFunc("POST /api/scout", localOnly(func(w http.ResponseWriter, r *http.Request) {
		// Only our own page may start a run: it drives the dominion.games tab.
		if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
			writeJSON(w, 403, map[string]string{"error": "cross-origin request refused"})
			return
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeJSON(w, 415, map[string]string{"error": "expected JSON"})
			return
		}
		var req struct {
			Player string `json:"player"`
			N      string `json:"n"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad request: " + err.Error()})
			return
		}
		if req.Player = strings.TrimSpace(req.Player); req.Player == "" || len(req.Player) > 64 {
			writeJSON(w, 400, map[string]string{"error": "type the player's name as dominion.games shows it"})
			return
		}
		dir, err := gamesDir("")
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		if err := startScout(req.Player, dir, scoutGames(req.N)); err != nil {
			writeJSON(w, 409, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 202, scoutStatus())
	}))
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

// localOnly serves an API only to requests addressed to this machine by name
// (localhost) or by IP. A site could otherwise point a hostname of its own at
// 127.0.0.1 (DNS rebinding): its pages would then be same-origin with the app and
// pass the Origin check.
func localOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if host != "localhost" && net.ParseIP(strings.Trim(host, "[]")) == nil {
			writeJSON(w, 403, map[string]string{"error": "open the app at localhost or 127.0.0.1"})
			return
		}
		h(w, r)
	}
}

var reNumber = regexp.MustCompile(`\d+`)

// reportLast reads how many latest games the Last games page asks for: 10 unless
// it says, and no more than the server lists.
func reportLast(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 10
	}
	return min(n, 100)
}

// loadRun is the Last games page's run of `scry games`. There's one at a time, as
// it drives the one dominion.games tab.
type loadRun struct {
	Started string   `json:"started,omitempty"`
	Running bool     `json:"running"`
	Log     []string `json:"log"`
	Error   string   `json:"error,omitempty"`
}

var (
	loadMu  sync.Mutex
	loadNow = loadRun{Log: []string{}}
)

// startLoad runs `scry games` in a process of its own and keeps its output. The
// page reads the report from the saved games afterwards, so the report goes nowhere.
func startLoad(dir string, last int, opponent string, ids []string) error {
	startMu.Lock()
	defer startMu.Unlock()
	if s := scoutStatus(); s.Running {
		return fmt.Errorf("the Scout page is scouting %s", s.Player)
	}
	loadMu.Lock()
	defer loadMu.Unlock()
	if loadNow.Running {
		return errors.New("already loading games")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"games", "-dir", dir, "-o", os.DevNull, "-last", strconv.Itoa(last)}
	if len(ids) > 0 {
		args = append(args, "-ids", strings.Join(ids, ","))
	}
	args = append(args, "--")
	if opponent != "" {
		args = append(args, opponent)
	}
	// scry's own binary, without a shell; the name can't be read as a flag after "--"
	cmd := exec.Command(exe, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd.Stdout, cmd.Stderr = w, w
	err = cmd.Start()
	w.Close()
	if err != nil {
		r.Close()
		return err
	}
	loadNow = loadRun{Started: time.Now().Format(time.RFC3339), Running: true, Log: []string{}}
	go func() {
		defer r.Close()
		for sc := bufio.NewScanner(r); sc.Scan(); {
			loadMu.Lock()
			loadNow.Log = append(loadNow.Log, sc.Text())
			loadMu.Unlock()
		}
		err := cmd.Wait()
		loadMu.Lock()
		defer loadMu.Unlock()
		loadNow.Running = false
		if err != nil {
			loadNow.Error = err.Error()
			if k := len(loadNow.Log); k > 0 && strings.HasPrefix(loadNow.Log[k-1], "scry: ") {
				loadNow.Error = strings.TrimPrefix(loadNow.Log[k-1], "scry: ")
			}
		}
	}()
	return nil
}

func loadStatus() loadRun {
	loadMu.Lock()
	defer loadMu.Unlock()
	s := loadNow
	s.Log = slices.Clone(s.Log)
	return s
}

// scoutGames reads how many of a player's games the Scout page asks for: 25
// unless it says, and no more than the server lists.
func scoutGames(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 25
	}
	return min(n, scoutList)
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
	profiles map[string]*advisor.Profile
	features []*advisor.Features
	insights map[string]*advisor.Insights
}

// minProfileGames is how many saved games a player needs for a profile.
const minProfileGames = 20

var reCaptureVersion = regexp.MustCompile(`version: (\d+),`)

// bookmarkSource is a bookmark's script: capture.js (unless the tab has this
// version of it already) and the bookmark's own file, with SCRY_ORIGIN set to
// the address of the page it reports to.
func bookmarkSource(name, origin string) string {
	capture, _ := webFiles.ReadFile("web/capture.js")
	one, _ := webFiles.ReadFile(name)
	version := reCaptureVersion.FindStringSubmatch(string(capture))[1]
	return "if ((window.scryCapture && window.scryCapture.version) !== " + version + ") " + stripComments(string(capture)) + "\n" +
		strings.ReplaceAll(string(one), "SCRY_ORIGIN", origin)
}

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
	account, profiles, features, err := advisor.Corpus(client, dir)
	if err != nil {
		return nil, err
	}
	corpusCache = &corpus{stamp: newest, account: account, profiles: profiles, features: features, insights: map[string]*advisor.Insights{}}
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
	in.Ratings = cardRatings(in)
	c.insights[player] = in
	return in, nil
}

// cardImages maps the profile's kingdom cards that have an image in
// card-images/ to a URL; the function gets the card and the image's path there.
func cardImages(in *advisor.Insights, link func(name, rel string) string) map[string]string {
	// the page's filters list cards from fewer games than in.Cards does
	names := map[string]bool{}
	for _, c := range in.Cards {
		names[c.Card] = true
	}
	for _, g := range in.Games {
		for _, k := range g.Kingdom {
			names[k] = true
		}
	}
	out := map[string]string{}
	for name := range names {
		if p, ok := cardImage(name); ok {
			rel, err := filepath.Rel(cardImageDir(), p)
			if err != nil {
				continue
			}
			out[name] = link(name, filepath.ToSlash(rel))
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

//go:build !js

package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/yourconscience/scrying-pool/scry/internal/advisor"
)

// braveScript opens, finds and closes Brave tabs and runs JavaScript in them:
//
//	open URL | find URL-PREFIX (the tabs' ids, comma-separated) | close TAB-ID | js TAB-ID CODE
//
// Brave runs the code in an isolated world, which sees the page but not its
// variables; tab.page reaches those through a script element.
const braveScript = `on run argv
	set cmd to item 1 of argv
	tell application "Brave Browser"
		if cmd is "open" then
			if (count of windows) is 0 then make new window
			set t to make new tab at end of tabs of window 1 with properties {URL:item 2 of argv}
			return id of t as text
		end if
		set found to {}
		repeat with w in windows
			repeat with t in tabs of w
				if cmd is "find" then
					if URL of t starts with item 2 of argv then set end of found to (id of t as text)
				else if (id of t as text) is item 2 of argv then
					if cmd is "close" then
						close t
						return ""
					end if
					return execute t javascript (item 3 of argv)
				end if
			end repeat
		end repeat
	end tell
	if cmd is "find" then
		set AppleScript's text item delimiters to ","
		return found as text
	end if
	error "the dominion.games tab was closed"
end run`

// brave runs braveScript. A call that hangs is most likely waiting on macOS asking
// whether this program may control Brave.
func brave(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// the script and its arguments go in through stdin: arguments show in ps,
	// and the login's carry the password
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(a) + `"`
	}
	script := strings.Replace(strings.Replace(braveScript, "on run argv", "on main(argv)", 1), "end run", "end main", 1) +
		"\nmain({" + strings.Join(quoted, ", ") + "})"
	cmd := exec.CommandContext(ctx, "osascript")
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	s := strings.TrimSuffix(string(out), "\n")
	if err != nil {
		if strings.Contains(s, "Allow JavaScript from Apple Events") || strings.Contains(s, "JavaScript through AppleScript is turned off") {
			return "", fmt.Errorf("turn on View > Developer > Allow JavaScript from Apple Events in Brave")
		}
		return "", fmt.Errorf("osascript: %v: %s", err, s)
	}
	return s, nil
}

// tab is a dominion.games tab in Brave.
type tab struct{ id string }

// js runs code in the tab's isolated world and returns its value, which should be a string.
func (t tab) js(code string) (string, error) { return brave("js", t.id, code) }

// page runs code, the body of an async function, in the page's own world and
// returns its value as JSON. The value comes back through a data attribute.
func (t tab) page(code string, wait time.Duration) (json.RawMessage, error) {
	body := `(async () => { const d = document.documentElement; try { const v = await (async () => {` + code + `
})(); d.dataset.scryOut = JSON.stringify({ ok: v === undefined ? null : v }); } catch (e) { d.dataset.scryOut = JSON.stringify({ error: String((e && e.message) || e) }); } })();`
	src, _ := json.Marshal(body)
	start := `(() => { const d = document.documentElement; delete d.dataset.scryOut; const s = document.createElement("script"); s.textContent = ` + string(src) + `; d.append(s); s.remove(); return "started"; })()`
	if _, err := t.js(start); err != nil {
		return nil, err
	}
	for t0 := time.Now(); ; time.Sleep(500 * time.Millisecond) {
		out, err := t.js(`document.documentElement.dataset.scryOut || ""`)
		if err != nil {
			return nil, err
		}
		if out != "" {
			var r struct {
				OK    json.RawMessage `json:"ok"`
				Error string          `json:"error"`
			}
			if err := json.Unmarshal([]byte(out), &r); err != nil {
				return nil, fmt.Errorf("dominion.games answered %.80q: %w", out, err)
			}
			if r.Error != "" {
				return nil, fmt.Errorf("dominion.games: %s", r.Error)
			}
			return r.OK, nil
		}
		if time.Since(t0) > wait {
			return nil, fmt.Errorf("dominion.games didn't answer in %v", wait)
		}
	}
}

// lobby is what the page shows: whether you're logged in and whether you're at a table.
type lobby struct{ In, Game, Table bool }

func (t tab) lobby() (st lobby, err error) {
	out, err := t.js(lobbyState)
	if err == nil {
		err = json.Unmarshal([]byte(out), &st)
	}
	return st, err
}

const lobbyState = `(() => {
	const shown = (t) => [...document.querySelectorAll("*")].some((e) => e.offsetParent !== null && e.textContent.trim() === t && ![...e.children].some((c) => c.textContent.trim() === t));
	return JSON.stringify({ in: shown("Logout"), game: !!document.querySelector(".log-scroll-container"), table: shown("Leave Table") });
})()`

// latestGames lists the latest two-player games of the logged-in account: the
// server answers with at most 100. Each one's profile line becomes its meta.
const latestGames = `const inj = angular.element(document.body).injector(), me = inj.get("meta").model.me, C = window.scryCapture;
await C.load();
C.watchProfiles();
const mark = (C.seen || []).length;
inj.get("serverMessenger").requestPlayerResults(me.id, RatingTypes.RATINGS_2P, 100);
await C.waitFor(() => (C.seen || []).length > mark, 20000, "your latest games");
for (let n = -1; n !== C.seen.length; ) { n = C.seen.length; await C.sleep(1000); }
const ids = C.seen.slice(mark).filter((r) => (r.players || []).length === 2).map((r) => C.noteGame(r, me.name));
await C.saveState();
return { me: me.name, ids };`

// collect saves your latest finished games from dominion.games, or just the games
// in ids, from both seats. It drives the dominion.games tab in Brave on this Mac
// (opening one if there is none) with the capture script, and imports the games
// not saved in dir yet. When the tab isn't logged in, or you're at a table, it stops.
func collect(dir string, minTurns int, budget, since time.Duration, ids []string) (err error) {
	if dir, err = gamesDir(dir); err != nil {
		return err
	}
	t, done, err := captureTab("collect")
	if err != nil {
		return err
	}
	defer done()
	var todo []string
	rows := map[string]string{}
	if len(ids) > 0 {
		// just these games: unrated ones, such as league matches, are on no list
		// the server gives; a manifest that names one gives its row
		named, err := manifestGames(dir)
		if err != nil {
			return err
		}
		for _, g := range ids {
			if _, err := os.Stat(filepath.Join(dir, g+".log")); os.IsNotExist(err) && !slices.Contains(todo, g) {
				todo = append(todo, g)
			}
		}
		for _, g := range named {
			if slices.Contains(todo, g.ID) {
				rows[g.ID] = g.Date + " | " + g.Result
			}
		}
		fmt.Printf("%d of %d games not saved\n", len(todo), len(ids))
	} else {
		out, err := t.page(latestGames, time.Minute)
		if err != nil {
			return err
		}
		var latest struct {
			Me  string   `json:"me"`
			IDs []string `json:"ids"`
		}
		if err := json.Unmarshal(out, &latest); err != nil {
			return err
		}
		for _, g := range latest.IDs {
			if _, err := os.Stat(filepath.Join(dir, g+".log")); os.IsNotExist(err) {
				todo = append(todo, g)
			}
		}
		fmt.Printf("%s: %d latest games, %d not saved\n", latest.Me, len(latest.IDs), len(todo))
		// with since, also the games an older list names that were never loaded,
		// each with its row so it keeps its date and scores
		if since > 0 {
			old, err := neverLoaded(dir, time.Now().Add(-since))
			if err != nil {
				return err
			}
			for _, g := range old {
				if _, err := os.Stat(filepath.Join(dir, g.ID+".log")); os.IsNotExist(err) && !slices.Contains(todo, g.ID) {
					todo = append(todo, g.ID)
					rows[g.ID] = g.Date + " | " + g.Result
				}
			}
			fmt.Printf("%d older games never loaded since %s\n", len(rows), time.Now().Add(-since).Format("2006-01-02"))
		}
	}
	if len(todo) == 0 {
		return nil
	}
	if len(rows) > 0 {
		b, _ := json.Marshal(rows)
		if _, err := t.page(`const C = window.scryCapture;
for (const [id, row] of Object.entries(`+string(b)+`)) C.meta[id] = Object.assign(C.meta[id] || {}, { row });
await C.saveState();`, time.Minute); err != nil {
			return err
		}
	}

	opts := captureOpts{Mode: "both", Alone: true, Retry: len(ids) > 0} // a game you name is tried again
	if err := captureGames(t, todo, dir, minTurns, budget, opts); err != nil {
		return err
	}
	if len(ids) > 0 {
		var missing []string
		for _, g := range todo {
			if _, err := os.Stat(filepath.Join(dir, g+".log")); os.IsNotExist(err) {
				missing = append(missing, g)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("%d of %d games not saved: %s", len(missing), len(todo), strings.Join(missing, ", "))
		}
	}
	return nil
}

// collectOther loads saved games again from the second seat and merges the logs,
// so they name both players' cards: the games in ids, or your n latest saved games
// that lack it. A game the server can't replay from there is marked and left alone.
func collectOther(dir string, budget time.Duration, n int, ids []string) (err error) {
	if dir, err = gamesDir(dir); err != nil {
		return err
	}
	t, done, err := captureTab("collect")
	if err != nil {
		return err
	}
	defer done()
	todo := slices.Clone(ids)
	if len(todo) == 0 {
		out, err := t.page(`return angular.element(document.body).injector().get("meta").model.me.name;`, time.Minute)
		if err != nil {
			return err
		}
		var me string
		if err := json.Unmarshal(out, &me); err != nil {
			return err
		}
		if todo, err = lackOtherSeat(dir, me, n); err != nil {
			return err
		}
		fmt.Printf("%s: %d saved games to load from the second seat\n", me, len(todo))
	}
	if len(todo) == 0 {
		return nil
	}
	if err := captureGames(t, todo, dir, 1, budget, captureOpts{Mode: "other", Alone: true, Retry: len(ids) > 0}); err != nil {
		return err
	}
	b, _ := json.Marshal(todo)
	out, err := t.page("return "+string(b)+`.filter((id) => (window.scryCapture.results[id + "+other"] || {}).status === "server");`, time.Minute)
	if err != nil {
		return err
	}
	var refused []string
	if err := json.Unmarshal(out, &refused); err != nil {
		return err
	}
	for _, g := range refused {
		if err := replaceHeaders(dir, g, []string{"both seats"}, "# both seats: the server can't replay it from the second seat\n"); err != nil {
			return err
		}
	}
	return nil
}

// lackOtherSeat lists up to n of me's saved games, latest first, whose score files
// don't say the second seat was tried.
func lackOtherSeat(dir, me string, n int) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.score.txt"))
	if err != nil {
		return nil, err
	}
	id := func(f string) string { return strings.TrimSuffix(filepath.Base(f), ".score.txt") }
	slices.SortFunc(files, func(a, b string) int {
		return cmp.Or(cmp.Compare(len(id(b)), len(id(a))), cmp.Compare(id(b), id(a))) // same-length ids sort as numbers
	})
	var out []string
	for _, f := range files {
		if len(out) == n {
			break
		}
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		score := string(b)
		if advisor.Header(score, "both seats") == "" && slices.Contains(strings.Split(advisor.Header(score, "original players"), " / "), me) {
			out = append(out, id(f))
		}
	}
	return out, nil
}

// login fills the dominion.games login form in the tab with the account and
// password the macOS keychain keeps for "dominion.games", which you add once:
//
//	security add-generic-password -s dominion.games -a NAME -w
//
// It doesn't kick another session: when the server offers to, the tab stays
// logged out and the caller stops.
func (t tab) login() error {
	attrs, err := exec.Command("security", "find-generic-password", "-s", "dominion.games").CombinedOutput()
	m := regexp.MustCompile(`"acct"<blob>="([^"]*)"`).FindSubmatch(attrs)
	if err != nil || m == nil {
		return fmt.Errorf("dominion.games isn't logged in in Brave, and the keychain has no dominion.games password to log in with")
	}
	pw, err := exec.Command("security", "find-generic-password", "-s", "dominion.games", "-w").Output()
	if err != nil {
		return fmt.Errorf("reading the dominion.games password from the keychain: %v", err)
	}
	user, _ := json.Marshal(string(m[1]))
	pass, _ := json.Marshal(strings.TrimSuffix(string(pw), "\n"))
	code := `(() => { const u = document.querySelector("input[name=username]"), p = document.querySelector("input[name=password][type=password]"), b = document.querySelector("input[type=submit]");
if (!u || !p || !b) return "no form";
const set = (e, v) => { e.value = v; e.dispatchEvent(new Event("input", { bubbles: true })); e.dispatchEvent(new Event("change", { bubbles: true })); };
set(u, ` + string(user) + `); set(p, ` + string(pass) + `); b.click(); return "sent"; })()`
	// the form shows once the client has loaded
	for t0 := time.Now(); ; time.Sleep(3 * time.Second) {
		out, err := t.js(code)
		if err != nil {
			return err
		}
		if out == "sent" {
			return nil
		}
		if time.Since(t0) > 30*time.Second {
			return fmt.Errorf("dominion.games isn't logged in in Brave, and its login form didn't show")
		}
	}
}

// captureTab finds the dominion.games tab in Brave that's logged in and readies
// the capture script in it. With no such tab, whatever else is open, it opens one
// and logs in with the keychain's password (login); when that fails, or you're at
// a table, it fails. done closes a tab it opened. cmd names the command to run again.
func captureTab(cmd string) (t tab, done func(), err error) {
	done = func() {}
	defer func() {
		if err != nil { // close a tab it opened, or they pile up run after run
			done()
			done = func() {}
		}
	}()
	if runtime.GOOS != "darwin" {
		return t, done, fmt.Errorf("%s drives Brave through AppleScript, which needs macOS", cmd)
	}
	// the tab that's logged in; only one is, the others offer to kick it
	found, err := brave("find", "https://dominion.games")
	if err != nil {
		return t, done, err
	}
	var st lobby
	for _, id := range strings.Split(found, ",") {
		if id == "" {
			continue
		}
		if st, err = (tab{id}).lobby(); err != nil {
			return t, done, err
		}
		if st.In || st.Game || st.Table {
			t = tab{id}
			break
		}
	}
	if t.id == "" {
		id, err := brave("open", "https://dominion.games/")
		if err != nil {
			return t, done, err
		}
		done = func() { brave("close", id) }
		t = tab{id}
		for t0 := time.Now(); !st.In && !st.Game && time.Since(t0) < 90*time.Second; time.Sleep(3 * time.Second) {
			if st, err = t.lobby(); err != nil {
				return t, done, err
			}
		}
		if !st.In && !st.Game && !st.Table {
			if err := t.login(); err != nil {
				return t, done, err
			}
			for t0 := time.Now(); !st.In && !st.Game && time.Since(t0) < 60*time.Second; time.Sleep(3 * time.Second) {
				if st, err = t.lobby(); err != nil {
					return t, done, err
				}
			}
		}
	}
	switch {
	case st.Game:
		return t, done, fmt.Errorf("you're in a game on dominion.games; finish it and run %s again", cmd)
	case st.Table:
		return t, done, fmt.Errorf("you're at a table on dominion.games (it shows Leave Table); leave it and run %s again", cmd)
	case !st.In:
		return t, done, fmt.Errorf("dominion.games isn't logged in in Brave; log in there and run %s again", cmd)
	}
	capture, _ := webFiles.ReadFile("web/capture.js")
	out, err := t.page(string(capture)+"\nreturn window.scryCapture.running ? \"running\" : \"ready\";", time.Minute)
	if err != nil {
		return t, done, err
	}
	if string(out) == `"running"` {
		return t, done, fmt.Errorf("a capture is already running in the dominion.games tab")
	}
	return t, done, nil
}

// captureOpts are the options of the capture script's run.
type captureOpts struct {
	Mode  string `json:"mode,omitempty"` // "both" seats, or the "other" seat of a saved game
	Retry bool   `json:"retry,omitempty"`
	Alone bool   `json:"alone"`
}

// captureGames loads games in the tab with the capture script and imports the
// ones it saved into dir. Their metas (date, scores, levels) should be noted in
// the tab already.
func captureGames(t tab, todo []string, dir string, minTurns int, budget time.Duration, opts captureOpts) error {
	// run skips games the tab finished earlier (saved, AFK, or ones the server
	// can't load) and sets aside a queue left from an earlier run
	queue, _ := json.Marshal(todo)
	o, _ := json.Marshal(opts)
	if _, err := t.page("return await window.scryCapture.run("+string(queue)+", "+string(o)+");", time.Minute); err != nil {
		return err
	}
	keys := slices.Clone(todo) // what the tab keeps each game's result under
	if opts.Mode == "other" {
		for i := range keys {
			keys[i] += "+other"
		}
	}
	ids, _ := json.Marshal(keys)
	var status struct {
		Running bool   `json:"running"`
		Error   string `json:"lastError"`
	}
	for t0 := time.Now(); ; time.Sleep(20 * time.Second) {
		out, err := t.page("return window.scryCapture.status();", time.Minute)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(out, &status); err != nil {
			return err
		}
		if !status.Running {
			break
		}
		if time.Since(t0) > budget {
			t.page("return window.scryCapture.stop();", time.Minute)
			budget += 10 * time.Minute // the game it's on finishes
		}
	}
	if status.Error != "" {
		fmt.Println("the capture stopped:", status.Error)
	}

	// the captured games, a few at a time, into one file for importGames
	byStatus := `const C = window.scryCapture, by = {};
for (const id of ` + string(ids) + `) { const r = C.results[id] || { status: "not tried" }; (by[r.status] = by[r.status] || []).push(r.status !== "ok" && r.error ? id + " (" + r.error + ")" : id); }
return by;`
	out, err := t.page(byStatus, time.Minute)
	if err != nil {
		return err
	}
	var by map[string][]string
	if err := json.Unmarshal(out, &by); err != nil {
		return err
	}
	for s, gs := range by {
		if s != "ok" {
			fmt.Printf("%d %s: %s\n", len(gs), s, strings.Join(gs, ", "))
		}
	}
	f, err := os.CreateTemp("", "scry-collect-*.jsonl")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	for ok := by["ok"]; len(ok) > 0; {
		n := min(10, len(ok))
		part, _ := json.Marshal(ok[:n])
		ok = ok[n:]
		out, err := t.page("return "+string(part)+".map((id) => window.scryCapture.results[id].record);", time.Minute)
		if err != nil {
			return err
		}
		var recs []json.RawMessage
		if err := json.Unmarshal(out, &recs); err != nil {
			return err
		}
		for _, r := range recs {
			f.Write(append(r, '\n'))
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if len(by["ok"]) > 0 {
		if err := importGames(f.Name(), dir, minTurns); err != nil {
			return err
		}
	}
	return nil
}

// manifestGame is a game a manifest-*.json in the games folder lists: DomBot's
// !prior for the player, with what became of each game.
type manifestGame struct {
	ID     string `json:"id"`
	Date   string `json:"date"`   // "2026-Sep-30"
	Result string `json:"result"` // "gamrevinu: 28, Wtrrch: 24"
	Status string `json:"status"` // saved, not-tried, server-error, afk, capture-error
}

// neverLoaded lists the manifests' games that were never tried, played on or after from.
func neverLoaded(dir string, from time.Time) ([]manifestGame, error) {
	all, err := manifestGames(dir)
	if err != nil {
		return nil, err
	}
	var out []manifestGame
	for _, g := range all {
		if d, err := time.Parse("2006-Jan-02", g.Date); err == nil && g.Status == "not-tried" && !d.Before(from.Truncate(24*time.Hour)) {
			out = append(out, g)
		}
	}
	return out, nil
}

// manifestGames lists the games of the games folder's manifests.
func manifestGames(dir string) ([]manifestGame, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "manifest-*.json"))
	var out []manifestGame
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var m struct {
			Games []manifestGame `json:"games"`
		}
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		out = append(out, m.Games...)
	}
	return out, nil
}

// publish rebuilds site-data/ in repo, a clean checkout of this repository, from
// the saved games: on a new branch from origin/main, for the players its
// players.json lists. When anything changed, it pushes the branch and opens a
// pull request; with merge, it waits for the pull request's checks and merges it.
func publish(repo string, merge bool) error {
	git := func(args ...string) (string, error) {
		out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %s: %v: %s", args[0], err, out)
		}
		return strings.TrimSpace(string(out)), nil
	}
	if out, err := git("status", "--porcelain"); err != nil || out != "" {
		return fmt.Errorf("%s has uncommitted changes, or isn't a checkout: %v", repo, err)
	}
	if _, err := git("fetch", "-q", "origin", "main"); err != nil {
		return err
	}
	now := time.Now()
	day, branch := now.Format("2006-01-02"), "site-data-"+now.Format("2006-01-02-1504")
	if _, err := git("switch", "-q", "-c", branch, "origin/main"); err != nil {
		return err
	}
	data := filepath.Join(repo, "site-data")
	var before siteList
	b, err := os.ReadFile(filepath.Join(data, "players.json"))
	if err == nil {
		err = json.Unmarshal(b, &before)
	}
	if err != nil {
		return fmt.Errorf("reading the published players: %w", err)
	}
	var players []string
	for _, p := range before.Players {
		players = append(players, p.Name)
	}
	if _, err := makeProfiles(data, players); err != nil {
		return err
	}
	if out, err := git("status", "--porcelain", "site-data"); err != nil || out == "" {
		git("switch", "-q", "--detach", "origin/main")
		git("branch", "-q", "-D", branch)
		if err == nil {
			fmt.Println("site data unchanged")
		}
		return err
	}
	var after siteList
	b, _ = os.ReadFile(filepath.Join(data, "players.json"))
	json.Unmarshal(b, &after)
	title := "Site data to " + day
	if len(before.Players) > 0 && len(after.Players) > 0 {
		title = fmt.Sprintf("Site data: %d new games to %s", after.Players[0].Games-before.Players[0].Games, day)
	}
	for _, args := range [][]string{{"add", "site-data"}, {"commit", "-q", "-m", title}, {"push", "-q", "-u", "origin", branch}} {
		if _, err := git(args...); err != nil {
			return err
		}
	}
	body := "Profiles and Recent games rebuilt from the saved games by `scry collect -publish`."
	pr := exec.Command("gh", "pr", "create", "--base", "main", "--head", branch, "--title", title, "--body", body)
	pr.Dir = repo
	out, err := pr.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gh pr create: %v: %s", err, out)
	}
	fmt.Print(string(out))
	if !merge {
		return nil
	}
	gh := func(args ...string) (string, error) {
		c := exec.Command("gh", args...)
		c.Dir = repo
		out, err := c.CombinedOutput()
		return string(out), err
	}
	// the checks take a moment to show up on a new pull request
	for try := 0; ; try++ {
		out, err := gh("pr", "checks", branch, "--watch", "--fail-fast", "--interval", "30")
		if err == nil {
			break
		}
		if !strings.Contains(out, "no checks reported") || try == 20 {
			return fmt.Errorf("the checks of %s didn't pass: %s", branch, out)
		}
		time.Sleep(15 * time.Second)
	}
	if out, err := gh("pr", "merge", branch, "--squash"); err != nil {
		return fmt.Errorf("gh pr merge: %v: %s", err, out)
	}
	// main is checked out elsewhere, so gh can't delete the branch for us
	git("switch", "-q", "--detach", "origin/main")
	git("branch", "-q", "-D", branch)
	git("push", "-q", "origin", "--delete", branch)
	fmt.Println("merged", branch)
	return nil
}

// notify shows a macOS notification, for runs nobody watches.
func notify(msg string) {
	exec.Command("osascript", "-e", "on run argv\ndisplay notification (item 1 of argv) with title \"Scrying Pool\"\nend run", msg).Run()
}

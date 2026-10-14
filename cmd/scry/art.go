//go:build !js

package main

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Card art is fetched from the Dominion Strategy wiki on demand and kept on disk.
// It belongs to Rio Grande Games, so it is never committed or bundled; each
// person's copy stays on their own machine.

const wikiImages = "https://wiki.dominionstrategy.com/images/thumb/"
const artWidth = "240px"
const userAgent = "scrying-pool (+https://github.com/yourconscience/scrying-pool)"

func artDir() string {
	for _, env := range []string{"SCRY_ART_DIR", "DOMBOT_ART_DIR"} {
		if d := os.Getenv(env); d != "" {
			return d
		}
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "scrying-pool", "art")
	// move art downloaded under the old name instead of fetching it again
	if old := filepath.Join(base, "dombot", "art"); !exists(dir) && exists(old) {
		os.MkdirAll(filepath.Dir(dir), 0o755)
		os.Rename(old, dir)
	}
	return dir
}

// knownArt maps art file names to cards, so only real card art is ever fetched.
var knownArt = func() map[string]bool {
	m := map[string]bool{}
	for _, c := range cardIndex {
		m[c.Art] = true
	}
	return m
}()

var artHTTP = &http.Client{Timeout: 30 * time.Second}
var artMu sync.Mutex // one wiki request at a time

// artURL is MediaWiki's thumbnail path: /images/thumb/<h>/<hh>/<file>/<width>-<file>.
func artURL(file string) string {
	sum := md5.Sum([]byte(file))
	h := hex.EncodeToString(sum[:])
	return wikiImages + h[:1] + "/" + h[:2] + "/" + file + "/" + artWidth + "-" + file
}

var errNoArt = errors.New("no art on the wiki")

// artPath returns the local file for a card's art, downloading it first if needed.
func artPath(file string) (string, error) {
	if !knownArt[file] {
		return "", errNoArt
	}
	p := filepath.Join(artDir(), file)
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	if _, err := os.Stat(p + ".missing"); err == nil {
		return "", errNoArt
	}
	artMu.Lock()
	defer artMu.Unlock()
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	if err := os.MkdirAll(artDir(), 0o755); err != nil {
		return "", err
	}
	req, _ := http.NewRequest("GET", artURL(file), nil)
	req.Header.Set("User-Agent", userAgent)
	resp, err := artHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		os.WriteFile(p+".missing", nil, 0o644) // remember, so we don't ask again
		return "", errNoArt
	}
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/jpeg" {
		return "", fmt.Errorf("%s: %s %s", file, resp.Status, resp.Header.Get("Content-Type"))
	}
	tmp, err := os.CreateTemp(artDir(), ".dl-*")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	tmp.Close()
	return p, os.Rename(tmp.Name(), p)
}

// syncArt downloads art for every card that doesn't have it yet, politely.
func syncArt(kinds map[string]bool) error {
	var files []string
	for _, c := range cardIndex {
		if len(kinds) == 0 || kinds[c.Kind] || (kinds["landscape"] && c.Landscape()) {
			files = append(files, c.Art)
		}
	}
	sort.Strings(files)
	var got, missing, failed int
	for i, f := range files {
		_, statErr := os.Stat(filepath.Join(artDir(), f))
		_, err := artPath(f)
		switch {
		case errors.Is(err, errNoArt):
			missing++
		case err != nil:
			failed++
			fmt.Fprintln(os.Stderr, "  ", err)
		default:
			got++
		}
		if statErr != nil && err == nil {
			time.Sleep(300 * time.Millisecond) // only after a real download
		}
		if (i+1)%50 == 0 || i+1 == len(files) {
			fmt.Fprintf(os.Stderr, "%d/%d cards: %d with art, %d without, %d failed\n", i+1, len(files), got, missing, failed)
		}
	}
	fmt.Printf("art in %s\n", artDir())
	if failed > 0 {
		return fmt.Errorf("%d downloads failed; run again to retry", failed)
	}
	return nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

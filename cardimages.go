//go:build !js

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Card images are full card faces kept in card-images/ (or $SCRY_CARD_IMAGES),
// which git ignores: they belong to Rio Grande Games. They come from the
// KingdomCreator repository (github.com/blakevanlan/KingdomCreator, docs/img/cards),
// named "<set>_<name>.jpg", e.g. "plunder_buriedtreasure.jpg", landscapes with
// their type, e.g. "plunder_event_avoid.jpg", and second-edition art with "_2nd".

func cardImageDir() string {
	if d := os.Getenv("SCRY_CARD_IMAGES"); d != "" {
		return d
	}
	return "card-images"
}

var reNotAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// imageKey reduces a card name to the letters and digits the image files use:
// "Will-o'-Wisp" is "willowisp", "Way Of The Rat" is "wayoftherat".
func imageKey(name string) string { return reNotAlnum.ReplaceAllString(strings.ToLower(name), "") }

// imageTypes are the type words between the set and the name in landscape files.
var imageTypes = map[string]bool{
	"event": true, "ally": true, "landmark": true, "way": true, "project": true,
	"trait": true, "prophecy": true, "boon": true, "hex": true, "state": true, "artifact": true,
}

// imageAliases are cards whose image has another name: Harem was renamed Farm;
// the kingdom viewer names the rotating piles "Augur Pile" and so on; the Ruins
// pile, which has no image of its own, shows one of its cards.
var imageAliases = map[string]string{
	"farm": "harem", "ruins": "ruinedvillage",
	"augurpile": "augurs", "clashpile": "clashes", "fortpile": "forts",
	"odysseypile": "odysseys", "townsfolkpile": "townsfolk", "wizardpile": "wizards",
}

// secondEdition sets hold the current versions of cards that also appear in a first-edition set.
var secondEdition = map[string]bool{
	"baseset2": true, "baseset2add": true, "intrigue2": true, "intrigue2add": true, "seaside2add": true,
	"prosperity2add": true, "hinterlands2add": true, "guildscornucopia": true, "guildscornucopia2add": true,
}

var (
	imagesOnce sync.Once
	imageIndex map[string]string // imageKey -> file path
)

// cardImage finds a card's image file.
func cardImage(name string) (string, bool) {
	imagesOnce.Do(func() { imageIndex = indexCardImages(cardImageDir()) })
	key := imageKey(name)
	if a, ok := imageAliases[key]; ok {
		key = a
	}
	p, ok := imageIndex[key]
	return p, ok
}

// indexCardImages maps each card to its image, preferring second-edition sets and art.
func indexCardImages(dir string) map[string]string {
	type pick struct {
		path string
		rank int
	}
	best := map[string]pick{}
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
			return nil
		}
		parts := strings.Split(strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)), "_")
		if len(parts) < 2 {
			return nil
		}
		set, rest := parts[0], parts[1:]
		rank := 0
		if secondEdition[set] {
			rank += 2
		}
		if rest[len(rest)-1] == "2nd" {
			rest, rank = rest[:len(rest)-1], rank+1
		}
		if len(rest) > 1 && rest[0] == "other" { // tokens and mats
			return nil
		}
		if len(rest) > 1 && imageTypes[rest[0]] {
			rest = rest[1:]
		}
		key := imageKey(strings.Join(rest, ""))
		if b, ok := best[key]; !ok || rank > b.rank {
			best[key] = pick{p, rank}
		}
		return nil
	})
	out := make(map[string]string, len(best))
	for k, b := range best {
		out[k] = b.path
	}
	return out
}

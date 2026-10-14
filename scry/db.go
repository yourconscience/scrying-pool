//go:build !js

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	_ "modernc.org/sqlite"

	"github.com/yourconscience/scrying-pool/scry/internal/advisor"
	"github.com/yourconscience/scrying-pool/scry/internal/cartographer"
)

// The games database is built from the saved files, which stay the source of
// truth: `scry db` rewrites it from scratch. Lists are JSON text, so SQLite's
// json_each can unpack them, e.g. a player's win rate by kingdom card:
//
//	select k.value, count(*), round(avg(p.result), 2) from player_games p
//	join games g on g.id = p.game_id, json_each(g.kingdom) k
//	where p.player = 'gamrevinu' group by k.value having count(*) >= 8 order by 3
const schema = `
create table meta (key text primary key, value text);
create table games (
	id text primary key,
	date text,            -- "2026-Oct-09", from the row
	played text,          -- when it was played, if a profile said (ISO time)
	rated integer,        -- 1 if a profile said it was rated
	first_player text,    -- took the first turn
	second_player text,
	first_score integer,  -- final scores from the row (DomBot or the profile)
	second_score integer,
	levels text,          -- "a 55.50 / b 48.25", if a profile said
	ending text,          -- provinces | piles | resign
	complete integer,     -- 1 if the replay's VP equal the final scores
	kingdom text,         -- JSON array
	landscapes text,      -- JSON array
	loaded_by text        -- the account that loaded the game
);
create table player_games (
	game_id text references games(id),
	player text,
	opponent text,
	went_first integer,
	result real,          -- 1 win, 0.5 tie, 0 loss
	vp integer,           -- computed from the replay
	opponent_vp integer,
	turns integer,
	opening text,         -- "4/3"
	opening_buys text,    -- JSON array
	first_gold integer,   -- the player's turn, 0 if never
	first_province integer,
	roles text,           -- JSON object: role -> true, for the kingdom
	took text,            -- JSON array of kingdom cards the player owned at some point
	coins text,           -- JSON array: $ on each of the player's turns
	primary key (game_id, player)
);
create table cards (
	card text primary key,
	plays integer,
	actions real, buys real, cards real, coins real, trashes real, curses real,  -- per play
	roles text            -- JSON array
);
create view player_summary as
	select player, count(*) as games, round(avg(result), 3) as win_rate,
	       round(avg(went_first), 3) as first_share
	from player_games group by player;
`

func buildDB(path, dir string) error {
	client, err := loadClient()
	if err != nil {
		return err
	}
	saved, err := advisor.Load(client, dir)
	if err != nil {
		return err
	}
	games := make([]*cartographer.Game, len(saved))
	for i, s := range saved {
		games[i] = s.Game
	}
	profiles := advisor.Profiles(games)

	tmp := path + ".new"
	os.Remove(tmp)
	db, err := sql.Open("sqlite", tmp)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	js := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	nGames, nRows := 0, 0
	for _, s := range saved {
		fs := advisor.GameFeatures(client, s, profiles)
		if len(fs) != 2 {
			continue
		}
		first, second := fs[0], fs[1]
		if second.First {
			first, second = second, first
		}
		var s1, s2 any
		if a, ok := s.Scores[first.Me]; ok {
			s1 = a
		}
		if b, ok := s.Scores[second.Me]; ok {
			s2 = b
		}
		var rated any
		if v := advisor.Header(s.Score, "rated"); v != "" {
			rated = map[bool]int{true: 1, false: 0}[v == "true"]
		}
		if _, err := tx.Exec(`insert into games values (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			s.ID, first.Date, advisor.Header(s.Score, "played"), rated, first.Me, second.Me, s1, s2,
			advisor.Header(s.Score, "levels"), first.Ending, first.Complete && second.Complete,
			js(nonNil(s.Kingdom)), js(nonNil(s.Landscapes)), s.Account); err != nil {
			return err
		}
		nGames++
		for _, f := range fs {
			var took []string
			for k := range f.Took {
				took = append(took, k)
			}
			sort.Strings(took)
			if _, err := tx.Exec(`insert into player_games values (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				f.ID, f.Me, f.Opponent, f.First, f.Result, f.MyVP, f.OppVP, f.Turns, f.Opening, js(nonNil(f.OpeningBuys)),
				f.FirstGold, f.FirstProvince, js(f.Roles), js(nonNil(took)), js(f.Coins)); err != nil {
				return err
			}
			nRows++
		}
	}
	for name, p := range profiles {
		var types []string
		if c := client.Cards[name]; c != nil {
			types = c.Types
		}
		if _, err := tx.Exec(`insert into cards values (?,?,?,?,?,?,?,?,?)`, name, p.Plays, p.Actions, p.Buys, p.Cards, p.Coins,
			p.Trashes, p.Curses, js(nonNil(advisor.CardRoles(name, profiles, types)))); err != nil {
			return err
		}
	}
	for k, v := range map[string]string{"built": time.Now().UTC().Format(time.RFC3339), "games_dir": dir, "client": client.Version} {
		if _, err := tx.Exec(`insert into meta values (?,?)`, k, v); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	db.Close()
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	fmt.Printf("%d games, %d player rows, %d cards in %s\n", nGames, nRows, len(profiles), path)
	return nil
}

func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Fake Discord: one DM with a human, one with DomBot; DomBot answers after the 2nd poll.
func TestAskFlowAgainstFakeDiscord(t *testing.T) {
	polls, sent := 0, ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "tok" {
			http.Error(w, "unauthorized", 401)
			return
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/users/@me/channels":
			w.Write([]byte(`[{"id":"c1","type":1,"recipients":[{"id":"u1","username":"Lynet"}]},
			                {"id":"c2","type":1,"recipients":[{"id":"b1","username":"DomBot","bot":true}]}]`))
		case r.Method == "POST" && r.URL.Path == "/channels/c2/messages":
			var b map[string]string
			json.NewDecoder(r.Body).Decode(&b)
			sent = b["content"]
			w.Write([]byte(`{"id":"m1","author":{"id":"me"},"content":"` + sent + `"}`))
		case r.Method == "GET" && r.URL.Path == "/channels/c2/messages":
			if r.URL.Query().Get("after") != "m1" {
				t.Errorf("polled with after=%q", r.URL.Query().Get("after"))
			}
			polls++
			if polls < 2 {
				w.Write([]byte(`[]`))
				return
			}
			w.Write([]byte(`[{"id":"m3","author":{"id":"b1","username":"DomBot"},"embeds":[{"title":"gamrevinu v. Lynet","description":"4–2"}]}]`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "nope", 404)
		}
	}))
	defer srv.Close()
	api = srv.URL

	c := &client{token: "tok", http: srv.Client()}
	ch, bot, err := c.domBotDM()
	if err != nil || ch != "c2" || bot.ID != "b1" {
		t.Fatalf("domBotDM = %q %v %v", ch, bot, err)
	}
	m, err := c.send(ch, "!match gamrevinu, Lynet")
	if err != nil || sent != "!match gamrevinu, Lynet" {
		t.Fatalf("send: %v, server got %q", err, sent)
	}
	replies, err := c.awaitReplies(ch, m.ID, bot.ID, 30*time.Second)
	if err != nil || len(replies) != 1 {
		t.Fatalf("awaitReplies = %v, %v", replies, err)
	}
	if got := renderMessages(replies, time.UTC); !strings.HasPrefix(got, "gamrevinu v. Lynet\n4–2") {
		t.Fatalf("render = %q", got)
	}
}

func TestRefusesWithoutDomBotDM(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// a human named DomBot must not match
		w.Write([]byte(`[{"id":"c1","type":1,"recipients":[{"id":"u1","username":"DomBot","bot":false}]}]`))
	}))
	defer srv.Close()
	api = srv.URL
	if _, _, err := (&client{token: "tok", http: srv.Client()}).domBotDM(); err == nil {
		t.Fatal("expected error when no bot DomBot DM exists")
	}
}

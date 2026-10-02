package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

var api = "https://discord.com/api/v10"

type user struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Bot      bool   `json:"bot"`
}

type discordEmbed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Fields      []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"fields"`
	Footer *struct {
		Text string `json:"text"`
	} `json:"footer"`
}

type message struct {
	ID        string         `json:"id"`
	Author    user           `json:"author"`
	Content   string         `json:"content"`
	Embeds    []discordEmbed `json:"embeds"`
	Timestamp string         `json:"timestamp"`
}

type client struct {
	token string
	http  *http.Client
}

// token reads the Discord user token from $DISCORD_TOKEN, then the macOS Keychain
// (service "dombot", account "discord"). It is never printed or logged.
func token() (string, error) {
	if t := strings.TrimSpace(os.Getenv("DISCORD_TOKEN")); t != "" {
		return t, nil
	}
	b, err := exec.Command("security", "find-generic-password", "-s", "dombot", "-a", "discord", "-w").Output()
	if err != nil || strings.TrimSpace(string(b)) == "" {
		return "", errors.New("no Discord token: set DISCORD_TOKEN (or on macOS: security add-generic-password -s dombot -a discord -w)")
	}
	return strings.TrimSpace(string(b)), nil
}

// askDomBot sends one command to DomBot over DM and returns its replies.
func askDomBot(cmd string, timeout time.Duration) ([]message, error) {
	if !strings.HasPrefix(cmd, "!") {
		return nil, errors.New("DomBot commands start with '!'")
	}
	tok, err := token()
	if err != nil {
		return nil, err
	}
	c := &client{token: tok, http: &http.Client{Timeout: 20 * time.Second}}
	ch, bot, err := c.domBotDM()
	if err != nil {
		return nil, err
	}
	sent, err := c.send(ch, cmd)
	if err != nil {
		return nil, err
	}
	return c.awaitReplies(ch, sent.ID, bot.ID, timeout)
}

func (c *client) do(method, path string, body any, v any) error {
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	for attempt := 0; ; attempt++ {
		req, _ := http.NewRequest(method, api+path, bytes.NewReader(payload))
		req.Header.Set("Authorization", c.token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			var rl struct {
				RetryAfter float64 `json:"retry_after"`
			}
			json.Unmarshal(data, &rl)
			time.Sleep(time.Duration((rl.RetryAfter + 0.5) * float64(time.Second)))
			continue
		}
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, bytes.TrimSpace(data))
		}
		if v != nil {
			return json.Unmarshal(data, v)
		}
		return nil
	}
}

// domBotDM finds the existing DM channel with the DomBot bot account.
func (c *client) domBotDM() (string, user, error) {
	var chans []struct {
		ID         string `json:"id"`
		Type       int    `json:"type"`
		Recipients []user `json:"recipients"`
	}
	if err := c.do("GET", "/users/@me/channels", nil, &chans); err != nil {
		return "", user{}, err
	}
	for _, ch := range chans {
		if ch.Type == 1 && len(ch.Recipients) == 1 && ch.Recipients[0].Bot && ch.Recipients[0].Username == "DomBot" {
			return ch.ID, ch.Recipients[0], nil
		}
	}
	return "", user{}, errors.New("no DM channel with DomBot found; DM it once from the Discord app first")
}

func (c *client) send(ch, content string) (message, error) {
	var m message
	err := c.do("POST", "/channels/"+ch+"/messages", map[string]any{"content": content}, &m)
	return m, err
}

// awaitReplies polls until DomBot answers, then waits a few more seconds so
// multi-message replies and edits (e.g. a placeholder replaced by an embed) settle.
func (c *client) awaitReplies(ch, after, botID string, timeout time.Duration) ([]message, error) {
	deadline := time.Now().Add(timeout)
	var settleUntil time.Time
	for {
		var msgs []message
		if err := c.do("GET", "/channels/"+ch+"/messages?limit=20&after="+after, nil, &msgs); err != nil {
			return nil, err
		}
		var mine []message
		for i := len(msgs) - 1; i >= 0; i-- { // API returns newest first
			if msgs[i].Author.ID == botID {
				mine = append(mine, msgs[i])
			}
		}
		now := time.Now()
		if len(mine) > 0 && settleUntil.IsZero() {
			settleUntil = now.Add(4 * time.Second)
		}
		if !settleUntil.IsZero() && now.After(settleUntil) {
			return mine, nil
		}
		if now.After(deadline) {
			if len(mine) > 0 {
				return mine, nil
			}
			return nil, fmt.Errorf("no reply from DomBot within %s", timeout)
		}
		time.Sleep(2 * time.Second)
	}
}

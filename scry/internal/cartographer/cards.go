package cartographer

import (
	"encoding/json"
	"strconv"
	"strings"
)

// CardN is a number of copies of one card. Card is nil when the log hides
// which cards they were ("5 cards", "2 other cards").
type CardN struct {
	Card *Card
	N    int
}

func (c CardN) String() string {
	name := "card"
	if c.Card != nil {
		name = c.Card.Name
	}
	if c.N == 1 {
		return name
	}
	return strconv.Itoa(c.N) + "× " + name
}

type form struct {
	text   string // lower case: "a hostelry", "hostelries", "the wind's gift"
	card   *Card
	plural bool
}

func (c *Client) addForm(text string, card *Card, plural bool) {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return
	}
	first, _, _ := strings.Cut(text, " ")
	for _, f := range c.forms[first] {
		if f.text == text {
			return
		}
	}
	c.forms[first] = append(c.forms[first], form{text, card, plural})
}

// CardList reads a list such as "2 Coppers, a Silver, and an Estate". It returns
// false if any part isn't a card, so other arguments (numbers, players, piles)
// aren't mistaken for cards.
func (c *Client) CardList(s string) ([]CardN, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	var out []CardN
	low := strings.ToLower(s)
	for low != "" {
		low = strings.TrimLeft(low, " ,")
		if strings.HasPrefix(low, "and ") {
			low = low[4:]
			continue
		}
		if low == "" {
			break
		}
		n := 1
		if i := strings.IndexByte(low, ' '); i > 0 {
			if v, err := strconv.Atoi(low[:i]); err == nil {
				n, low = v, low[i+1:]
			}
		}
		low = strings.TrimPrefix(low, "other ") // "2 other cards"
		f, ok := c.longestForm(low)
		if !ok {
			return nil, false
		}
		var card *Card
		if f.card.Key != "Back" {
			card = f.card
		}
		out = append(out, CardN{card, n})
		low = low[len(f.text):]
		if low != "" && !strings.HasPrefix(low, ",") && !strings.HasPrefix(low, " ") {
			return nil, false // matched only part of a longer word
		}
	}
	return out, len(out) > 0
}

func (c *Client) longestForm(low string) (form, bool) {
	first, _, _ := strings.Cut(low, " ")
	first = strings.TrimRight(first, ",")
	for _, f := range c.forms[first] {
		if !strings.HasPrefix(low, f.text) {
			continue
		}
		if rest := low[len(f.text):]; rest == "" || rest[0] == ',' || rest[0] == ' ' {
			return f, true
		}
	}
	return form{}, false
}

// MarshalJSON writes {"card": "Gold", "n": 2}; card is "" when the log hides it.
func (c CardN) MarshalJSON() ([]byte, error) {
	name := ""
	if c.Card != nil {
		name = c.Card.Name
	}
	return json.Marshal(struct {
		Card string `json:"card"`
		N    int    `json:"n"`
	}{name, c.N})
}

package kitty

import (
	"bytes"
	"encoding/base64"
	"strings"
	"sync"
)

type ircV3caps struct {
	saslOn   bool
	saslUser string
	saslPass string
	caps     []string
	mu       sync.Mutex
	done     bool
}

func (c *ircV3caps) saslEnable() {
	c.mu.Lock()
	c.saslOn = true
	c.mu.Unlock()
}

func (c *ircV3caps) saslCreds(user, pass string) {
	c.mu.Lock()
	c.saslUser = user
	c.saslPass = pass
	c.mu.Unlock()
}

func (c *ircV3caps) reset() {
	c.mu.Lock()
	c.saslOn = false
	c.done = false
	c.caps = []string{}
	c.mu.Unlock()
}

func (c *ircV3caps) saslAuth(m *Message) bool {
	return m.Command == "AUTHENTICATE" && len(m.Params) == 1 && m.Params[0] == "+"
}

func (c *ircV3caps) capLS(m *Message) bool {
	return m.Command == "CAP" && len(m.Params) > 1 && m.Params[1] == "LS"
}

func (c *ircV3caps) capACK(m *Message) bool {
	return m.Command == "CAP" && len(m.Params) > 1 && m.Params[1] == "ACK"
}

func (c *ircV3caps) Handle(bot *Bot, m *Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		return
	}

	if c.capLS(m) {
		for _, cap := range strings.Split(m.Content, " ") {
			if _, ok := allowedCAPs[cap]; ok {
				c.caps = append(c.caps, cap)
			}
		}
		bot.Send("CAP REQ :" + strings.Join(c.caps, " "))
	}

	if c.capACK(m) {
		bot.Info("IRCV3", "CAPs", m.Content)
		if c.saslOn && strings.Contains(m.Content, "sasl") {
			bot.Debug("Recieved SASL ACK")
			bot.Send("AUTHENTICATE PLAIN")
		} else {
			if c.saslOn {
				bot.Error("SASL not supported")
			}
			bot.Send("CAP END")
			c.done = true
		}
	}

	if c.saslAuth(m) {
		bot.Debug("Got auth message!")
		out := bytes.Join([][]byte{[]byte(c.saslUser), []byte(c.saslUser), []byte(c.saslPass)}, []byte{0})
		encpass := base64.StdEncoding.EncodeToString(out)
		bot.Send("AUTHENTICATE " + encpass)
		bot.Send("CAP END")
		c.done = true
	}
}

// Capabilities we can deal with
// without doing crazy things in the library
var allowedCAPs = map[string]struct{}{
	"account-notify": {},
	"away-notify":    {},
	"extended-join":  {},
	"sasl":           {},
	"chghost":        {},
	"invite-notify":  {},
	"multi-prefix":   {},
	//"userhost-in-names": {}, // Not sure how this works toghether with multi-prefix
	// There are more, even more mad caps... TODO!!!!!!
}

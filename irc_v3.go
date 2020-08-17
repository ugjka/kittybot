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

func (h *ircV3caps) saslEnable() {
	h.mu.Lock()
	h.saslOn = true
	h.mu.Unlock()
}

func (h *ircV3caps) saslCreds(user, pass string) {
	h.mu.Lock()
	h.saslUser = user
	h.saslPass = pass
	h.mu.Unlock()
}

func (h *ircV3caps) saslAuth(m *Message) bool {
	return m.Command == "AUTHENTICATE" && len(m.Params) == 1 && m.Params[0] == "+"
}

func (h *ircV3caps) capLS(m *Message) bool {
	return m.Command == "CAP" && len(m.Params) > 1 && m.Params[1] == "LS"
}

func (h *ircV3caps) capACK(m *Message) bool {
	return m.Command == "CAP" && len(m.Params) > 1 && m.Params[1] == "ACK"
}

func (h *ircV3caps) Handle(bot *Bot, m *Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.done {
		return
	}

	if h.capLS(m) {
		for _, cap := range strings.Split(m.Content, " ") {
			if _, ok := allowedCAPs[cap]; ok {
				h.caps = append(h.caps, cap)
			}
		}
		bot.Send("CAP REQ :" + strings.Join(h.caps, " "))
	}

	if h.capACK(m) {
		bot.Info("IRCV3", "CAPs", m.Content)
		if h.saslOn && strings.Contains(m.Content, "sasl") {
			bot.Debug("Recieved SASL ACK")
			bot.Send("AUTHENTICATE PLAIN")
		} else {
			if h.saslOn {
				bot.Error("SASL not supported")
			}
			bot.Send("CAP END")
			h.done = true
		}
	}

	if h.saslAuth(m) {
		bot.Debug("Got auth message!")
		out := bytes.Join([][]byte{[]byte(h.saslUser), []byte(h.saslUser), []byte(h.saslPass)}, []byte{0})
		encpass := base64.StdEncoding.EncodeToString(out)
		bot.Send("AUTHENTICATE " + encpass)
		bot.Send("CAP END")
		h.done = true
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

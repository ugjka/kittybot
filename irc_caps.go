package kitty

import (
	"bytes"
	"encoding/base64"
	"strings"
	"sync"
)

type ircCaps struct {
	saslOn      bool
	saslUser    string
	saslPass    string
	caps        []string
	capsEnabled map[string]bool
	mu          sync.Mutex
	done        bool
}

func (c *ircCaps) saslEnable() {
	c.mu.Lock()
	c.saslOn = true
	c.mu.Unlock()
}

func (c *ircCaps) saslCreds(user, pass string) {
	c.mu.Lock()
	c.saslUser = user
	c.saslPass = pass
	c.mu.Unlock()
}

func (c *ircCaps) reset() {
	c.mu.Lock()
	c.saslOn = false
	c.done = false
	c.caps = []string{}
	c.capsEnabled = make(map[string]bool)
	c.mu.Unlock()
}

func (c *ircCaps) isSaslAuth(m *Message) bool {
	return (m.Command == "AUTHENTICATE" && m.Param(0) == "+")
}

func (c *ircCaps) isSaslFinished(m *Message) bool {
	return m.Command == "903" || m.Command == "904"
}

func (c *ircCaps) isCapLS(m *Message) bool {
	return m.Command == "CAP" && m.Param(1) == "LS"
}

func (c *ircCaps) isCapACK(m *Message) bool {
	return m.Command == "CAP" && m.Param(1) == "ACK"
}

func (c *ircCaps) Handle(bot *Bot, m *Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		return
	}

	if c.isCapLS(m) {
		for _, cap := range strings.Split(m.Content, " ") {
			if _, ok := allowedCAPs[cap]; ok {
				c.capsEnabled[cap] = true
				c.caps = append(c.caps, cap)
			} else {
				c.capsEnabled[cap] = false
			}
		}
		bot.Send("CAP REQ :" + strings.Join(c.caps, " "))
	}

	if c.isCapACK(m) {
		bot.Info("ircv3", "capabilities", m.Content)
		if c.saslOn && strings.Contains(m.Content, "sasl") {
			bot.Debug("recieved sasl ack")
			bot.Send("AUTHENTICATE PLAIN")
		} else {
			if c.saslOn {
				bot.Crit("sasl not supported")
			}
			bot.Send("CAP END")
			c.done = true
		}
	}

	if c.isSaslAuth(m) {
		bot.Debug("got auth message")
		out := bytes.Join([][]byte{[]byte(c.saslUser), []byte(c.saslUser), []byte(c.saslPass)}, []byte{0})
		encpass := base64.StdEncoding.EncodeToString(out)
		bot.Send("AUTHENTICATE " + encpass)
	}

	if c.isSaslFinished(m) {
		bot.Send("CAP END")
		c.done = true
	}
}

// Capabilities we can deal with
// without doing crazy things in the library
var allowedCAPs = map[string]struct{}{
	CapAccountNotify: {},
	CapAwayNotify:    {},
	CapExtendedJoin:  {},
	CapSASL:          {},
	CapChghost:       {},
	CapInviteNotify:  {},
	CapMultiPrefix:   {},
	CapCapNotify:     {},
	CapSetName:       {},
	CapServerTime:    {},
	CapAccountTag:    {},
	CapMessageTags:   {},
}

// CapAccountNotify is account-notify CAP
const CapAccountNotify = "account-notify"

// CapAwayNotify is away-notify CAP
const CapAwayNotify = "away-notify"

// CapExtendedJoin is extended-join CAP
const CapExtendedJoin = "extended-join"

// CapSASL is SASL CAP
const CapSASL = "sasl"

// CapChghost is chghost CAP
const CapChghost = "chghost"

// CapInviteNotify is invite-notify CAP
const CapInviteNotify = "invite-notify"

// CapMultiPrefix is multi-prefix CAP
const CapMultiPrefix = "multi-prefix"

// CapUserhostInNames is userhost-in-names CAP
const CapUserhostInNames = "userhost-in-names"

// CapCapNotify is cap-notify CAP
const CapCapNotify = "cap-notify"

// CapIdentifyMsg is identify-msg CAP
const CapIdentifyMsg = "identify-msg"

// CapTLS is tls CAP
const CapTLS = "tls"

// CapSetName is setname CAP
const CapSetName = "setname"

// CapServerTime is server-time CAP
const CapServerTime = "server-time"

// CapAccountTag is account-tag CAP
const CapAccountTag = "account-tag"

// CapMessageTags is message-tags CAP
const CapMessageTags = "message-tags"

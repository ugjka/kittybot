package kitty

import (
	"fmt"
	"strings"

	"gopkg.in/sorcix/irc.v2"
)

// A trigger to respond to the servers ping pong messages.
// If PingPong messages are not responded to, the server assumes the
// client has timed out and will close the connection.
// Note: this is automatically added in the IrcCon constructor.
var pingPong = Trigger{
	Condition: func(bot *Bot, m *Message) bool {
		return m.Command == "PING"
	},
	Action: func(bot *Bot, m *Message) {
		bot.Send("PONG :" + m.Content)
	},
}

var joinChannels = Trigger{
	Condition: func(bot *Bot, m *Message) bool {
		return m.Command == irc.RPL_WELCOME || m.Command == irc.RPL_ENDOFMOTD // 001 or 372
	},
	Action: func(bot *Bot, m *Message) {
		bot.mu.Lock()
		defer bot.mu.Unlock()
		bot.didJoinChannels.Do(func() {
			for _, channel := range bot.Channels {
				splitchan := strings.SplitN(channel, ":", 2)
				fmt.Println("splitchan is:", splitchan)
				if len(splitchan) == 2 {
					channel = splitchan[0]
					password := splitchan[1]
					bot.Send(fmt.Sprintf("JOIN %s %s", channel, password))
				} else {
					bot.Send(fmt.Sprintf("JOIN %s", channel))
				}
			}
		})
	},
}

// Get bot's prefix by catching its own join
var getPrefix = Trigger{
	Condition: func(bot *Bot, m *Message) bool {
		return m.Command == "JOIN" && m.Name == bot.getNick()
	},
	Action: func(bot *Bot, m *Message) {
		bot.prefixMu.Lock()
		bot.prefix = &irc.Prefix{
			Name: m.Prefix.Name,
			User: m.Prefix.User,
			Host: m.Prefix.Host,
		}
		bot.prefixMu.Unlock()
		bot.Debug("Got prefix", "prefix", bot.Prefix().String())
	},
}

// Track nick changes internally so we can adjust the bot's prefix
var setNick = Trigger{
	Condition: func(bot *Bot, m *Message) bool {
		return m.Command == "NICK" && m.From == bot.getNick()
	},
	Action: func(bot *Bot, m *Message) {
		bot.mu.Lock()
		bot.nick = m.To
		bot.mu.Unlock()
		bot.PrefixChange(m.To, "", "")
		bot.Info("nick changed successfully")
	},
}

// Throw errors on invalid nick changes
var nickError = Trigger{
	Condition: func(bot *Bot, m *Message) bool {
		return m.Command == "436" || m.Command == "433" ||
			m.Command == "432" || m.Command == "431" || m.Command == "400"
	},
	Action: func(bot *Bot, m *Message) {
		bot.Error("nick change error", m.Params[1], m.Content)
	},
}

// TODO: throw errors on SASL errors

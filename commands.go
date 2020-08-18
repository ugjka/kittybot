package kitty

import (
	"fmt"
	"strings"
)

// Reply sends a message to where the message came from (user or channel)
func (bot *Bot) Reply(m *Message, text string) {
	var target string
	if strings.Contains(m.To, "#") {
		target = m.To
	} else {
		target = m.From
	}
	bot.Msg(target, text)
}

// Msg sends a message to 'who' (user or channel)
func (bot *Bot) Msg(who, text string) {
	const command = "PRIVMSG"
	for _, line := range bot.splitText(text, command, who) {
		bot.Send(command + " " + who + " :" + line)
	}
}

// MsgMaxSize returns maximum number of bytes that fit into one message
func (bot *Bot) MsgMaxSize(who string) int {
	const command = "PRIVMSG"
	maxSize := bot.maxMsgSize(command, who)
	return maxSize
}

// Notice sends a NOTICE message to 'who' (user or channel)
func (bot *Bot) Notice(who, text string) {
	const command = "NOTICE"
	for _, line := range bot.splitText(text, command, who) {
		bot.Send(command + " " + who + " :" + line)
	}
}

// NoticeMaxSize returns maximum number of bytes that fit into one message
func (bot *Bot) NoticeMaxSize(who string) int {
	const command = "NOTICE"
	maxSize := bot.maxMsgSize(command, who)
	return maxSize
}

// Action sends an action to 'who' (user or channel)
func (bot *Bot) Action(who, text string) {
	msg := fmt.Sprintf("\u0001ACTION %s\u0001", text)
	bot.Msg(who, msg)
}

// Topic sets the channel 'c' topic (requires bot has proper permissions)
func (bot *Bot) Topic(c, topic string) {
	str := fmt.Sprintf("TOPIC %s :%s", c, topic)
	bot.Send(str)
}

// Send any command to the server
func (bot *Bot) Send(command string) {
	bot.outgoing <- command
}

// ChMode is used to change users modes in a channel
// operator = "+o" deop = "-o"
// ban = "+b"
func (bot *Bot) ChMode(user, channel, mode string) {
	bot.Send("MODE " + channel + " " + mode + " " + user)
}

// Join a channel
func (bot *Bot) Join(ch string) {
	bot.Send("JOIN " + ch)
}

// Part a channel
func (bot *Bot) Part(ch, msg string) {
	bot.Send("PART " + ch + " " + msg)
}

func (bot *Bot) isClosing() bool {
	bot.mu.Lock()
	defer bot.mu.Unlock()
	return bot.closing
}

// SetNick sets the bots nick on the irc server.
// This does not alter *Bot.Nick, so be vary of that
func (bot *Bot) SetNick(nick string) {
	bot.Send(fmt.Sprintf("NICK %s", nick))
}

// This is an example program showing the usage of KittyBot
package main

import (
	"flag"
	"fmt"
	"time"

	kitty "github.com/ugjka/kittybot"
	log "gopkg.in/inconshreveable/log15.v2"
)

var serv = flag.String("server", "irc.coldfront.net:6667", "hostname and port for irc server to connect to")
var nick = flag.String("nick", "kittybot", "nickname for the bot")

func main() {
	flag.Parse()

	hijackSession := func(b *kitty.Bot) {
		b.HijackSession = true
	}
	channels := func(b *kitty.Bot) {
		b.Channels = []string{"#test"}
	}
	b, err := kitty.NewBot(*serv, *nick, hijackSession, channels)
	if err != nil {
		panic(err)
	}

	b.AddTrigger(sayInfoMessage)
	b.AddTrigger(longTrigger)
	b.Logger.SetHandler(log.StdoutHandler)
	// logHandler := log.LvlFilterHandler(log.LvlInfo, log.StdoutHandler)
	// or
	// irc.Logger.SetHandler(logHandler)
	// or
	// irc.Logger.SetHandler(log.StreamHandler(os.Stdout, log.JsonFormat()))

	// Start up bot (this blocks until we disconnect)
	b.Run()
	fmt.Println("Bot shutting down.")
}

// This trigger replies Hello when you say hello
var sayInfoMessage = kitty.Trigger{
	Condition: func(b *kitty.Bot, m *kitty.Message) bool {
		return m.Command == "PRIVMSG" && m.Content == "-info"
	},
	Action: func(b *kitty.Bot, m *kitty.Message) {
		b.Reply(m, "Hello")
	},
}

// This trigger replies Hello when you say hello
var longTrigger = kitty.Trigger{
	Condition: func(b *kitty.Bot, m *kitty.Message) bool {
		return m.Command == "PRIVMSG" && m.Content == "-long"
	},
	Action: func(b *kitty.Bot, m *kitty.Message) {
		b.Reply(m, "This is the first message")
		time.Sleep(5 * time.Second)
		b.Reply(m, "This is the second message")
	},
}

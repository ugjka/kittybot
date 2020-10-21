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

	hijackSession := func(bot *kitty.Bot) {
		bot.HijackSession = true
	}
	channels := func(bot *kitty.Bot) {
		bot.Channels = []string{"#test"}
	}
	bot := kitty.NewBot(*serv, *nick, hijackSession, channels)

	bot.AddTrigger(sayInfoMessage)
	bot.AddTrigger(longTrigger)
	bot.Logger.SetHandler(log.StdoutHandler)
	// logHandler := log.LvlFilterHandler(log.LvlInfo, log.StdoutHandler)
	// or
	// irc.Logger.SetHandler(logHandler)
	// or
	// irc.Logger.SetHandler(log.StreamHandler(os.Stdout, log.JsonFormat()))

	// Start up bot (this blocks until we disconnect)
	bot.Run()
	fmt.Println("Bot shutting down.")
}

// This trigger replies Hello when you say hello
var sayInfoMessage = kitty.Trigger{
	Condition: func(bot *kitty.Bot, m *kitty.Message) bool {
		return m.Command == "PRIVMSG" && m.Content == "-info"
	},
	Action: func(bot *kitty.Bot, m *kitty.Message) {
		bot.Reply(m, "Hello")
	},
}

// This trigger replies Hello when you say hello
var longTrigger = kitty.Trigger{
	Condition: func(bot *kitty.Bot, m *kitty.Message) bool {
		return m.Command == "PRIVMSG" && m.Content == "-long"
	},
	Action: func(bot *kitty.Bot, m *kitty.Message) {
		bot.Reply(m, "This is the first message")
		time.Sleep(5 * time.Second)
		bot.Reply(m, "This is the second message")
	},
}

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	kitty "github.com/ugjka/kittybot"
)

// This trigger will op people in the given list who ask by saying "-opme"
var oplist = []string{"ugjka", "madcotto", "bagpuss"}
var opPeople = kitty.Trigger{
	Condition: func(bot *kitty.Bot, m *kitty.Message) bool {
		if m.Content == "-opme" {
			for _, s := range oplist {
				if m.From == s {
					return true
				}
			}
		}
		return false
	},
	Action: func(irc *kitty.Bot, m *kitty.Message) {
		irc.ChMode(m.To, m.From, "+o")
	},
}

// This trigger will say the contents of the file "info" when prompted
var sayInfoMessage = kitty.Trigger{
	Condition: func(bot *kitty.Bot, m *kitty.Message) bool {
		return m.Command == "PRIVMSG" && m.Content == "-info"
	},
	Action: func(irc *kitty.Bot, m *kitty.Message) {
		fi, err := os.Open("info")
		if err != nil {
			return
		}
		info, _ := ioutil.ReadAll(fi)

		irc.Send("PRIVMSG " + m.From + " : " + string(info))
	},
}

// This trigger will listen for -toggle, -next and -prev and then
// perform the mpc action of the same name to control an mpd server running
// on localhost
var mpc = kitty.Trigger{
	Condition: func(bot *kitty.Bot, m *kitty.Message) bool {
		return m.Command == "PRIVMSG" && (m.Content == "-toggle" || m.Content == "-next" || m.Content == "-prev")
	},
	Action: func(irc *kitty.Bot, m *kitty.Message) {
		var mpcCMD string
		switch m.Content {
		case "-toggle":
			mpcCMD = "toggle"
		case "-next":
			mpcCMD = "next"
		case "-prev":
			mpcCMD = "prev"
		default:
			fmt.Println("Invalid command.")
			return
		}
		cmd := exec.Command("/usr/bin/mpc", mpcCMD)
		err := cmd.Run()
		if err != nil {
			fmt.Printf("error: %s\n", err)
		}
	},
}

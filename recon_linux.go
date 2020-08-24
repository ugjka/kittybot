package kitty

import (
	"io"
	"io/ioutil"
	"net"

	"github.com/ftrvxmtrx/fd"
	"github.com/ugjka/ircmsg"
)

// startUnixListener starts up a unix domain socket listener for reconnects to
// be sent through
func (bot *Bot) startUnixListener() {
	defer bot.wg.Done()
	unaddr, err := net.ResolveUnixAddr("unix", bot.unixastr)
	if err != nil {
		panic(err)
	}

	list, err := net.ListenUnix("unix", unaddr)
	if err != nil {
		panic(err)
	}
	bot.mu.Lock()
	bot.unixlist = list
	bot.mu.Unlock()
	con, err := list.AcceptUnix()
	if err != nil {
		if !bot.isClosing() {
			bot.Error("unix listener", "error", err)
		}
		return
	}
	defer con.Close()
	list.Close()

	fi, err := bot.con.(*net.TCPConn).File()
	if err != nil {
		panic(err)
	}
	err = fd.Put(con, fi)
	if err != nil {
		panic(err)
	}

	// Send own prefix
	_, err = io.WriteString(con, bot.Prefix().String())
	if err != nil {
		panic(err)
	}
	if !bot.isClosing() {
		bot.setClosing(true)
		err := bot.con.Close()
		if err != nil {
			bot.Error("hijack closing bot", "error", err)
		}
		select {
		case bot.outgoing <- "DISCONNECT":
		default:
		}
	}
	bot.hijacked = true
}

// Attempt to hijack session previously running bot
func (bot *Bot) hijackSession() bool {
	con, err := net.Dial("unix", bot.unixastr)
	if err != nil {
		bot.Info("Couldnt restablish connection, no prior bot.", "err", err)
		return false
	}
	defer con.Close()
	ncon, err := fd.Get(con.(*net.UnixConn), 1, nil)
	if err != nil {
		panic(err)
	}
	defer ncon[0].Close()

	netcon, err := net.FileConn(ncon[0])
	if err != nil {
		panic(err)
	}

	// Read the reminder which should be our prefix
	prefix, err := ioutil.ReadAll(con)
	if err != nil {
		panic(err)
	}
	bot.prefixMu.Lock()
	bot.prefix = ircmsg.ParsePrefix(string(prefix))
	bot.prefixMu.Unlock()
	bot.reconnecting = true
	bot.con = netcon
	return true
}

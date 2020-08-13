// +build !linux,!freebsd,!openbsd,!dragonfly,!netbsd,!darwin

package kitty

func (bot *Bot) startUnixListener() {
	bot.wg.Done()
}

// Attempt to hijack session previously running bot
func (bot *Bot) hijackSession() bool {
	return false
}

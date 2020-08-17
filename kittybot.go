package kitty

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	log "gopkg.in/inconshreveable/log15.v2"
	logext "gopkg.in/inconshreveable/log15.v2/ext"
	"gopkg.in/sorcix/irc.v2"

	"crypto/tls"
)

// Bot implements an irc bot to be connected to a given server
type Bot struct {

	// This is set if we have hijacked a connection
	reconnecting bool
	// This is set if we have been hijacked
	hijacked bool
	// Channel to indicate shutdown
	closer   chan struct{}
	con      net.Conn
	outgoing chan string
	handlers []Handler
	// -race complained a lot, we are thread safe now
	mu sync.Mutex
	// When did we start? Used for uptime
	started time.Time
	// Unix domain abstract socket address for reconnects (linux only)
	unixastr string
	// Unix domain socket address for other Unixes
	unixsock string
	unixlist *net.UnixListener
	closing  bool
	// Log15 loggger
	log.Logger
	didJoinChannels sync.Once
	didAddSASLtrig  sync.Once
	wg              sync.WaitGroup
	// IRC V3 CAPS and
	// SASL credentials
	ircV3 *ircV3caps
	// Exported fields
	Host          string
	Password      string
	Channels      []string
	SSL           bool
	SASL          bool
	HijackSession bool
	// HijackAfterFunc executes in its own goroutine after a succesful session hijack
	// If you need to do something after a hijack
	// for example, to run some irc commands or to restore some state
	HijackAfterFunc func()

	// An optional function that connects to an IRC server over plaintext:
	Dial func(network, addr string) (net.Conn, error)
	// An optional function that connects to an IRC server over a secured connection:
	DialTLS func(network, addr string, tlsConf *tls.Config) (*tls.Conn, error)
	// This bots nick
	Nick string
	// Transient nick, that is used internally to track nick changes and calculate the prefix for the bot
	nick string
	// Duration to wait between sending of messages to avoid being
	// kicked by the server for flooding (default 200ms)
	ThrottleDelay time.Duration
	// Maxmimum time between incoming data
	PingTimeout time.Duration

	TLSConfig tls.Config
	// Bot's prefix
	prefix   *irc.Prefix
	prefixMu *sync.RWMutex
}

func (bot *Bot) String() string {
	return fmt.Sprintf("Server: %s, Channels: %v, Nick: %s", bot.Host, bot.Channels, bot.getNick())
}

// NewBot creates a new instance of Bot
func NewBot(host, nick string, options ...func(*Bot)) (*Bot, error) {
	// Defaults are set here
	bot := Bot{
		started:         time.Now(),
		unixastr:        fmt.Sprintf("@%s-%s/bot", host, nick),
		unixsock:        fmt.Sprintf("/tmp/%s-%s-bot.sock", host, nick),
		Host:            host,
		Nick:            nick,
		nick:            nick,
		ircV3:           &ircV3caps{},
		ThrottleDelay:   200 * time.Millisecond,
		PingTimeout:     300 * time.Second,
		HijackSession:   false,
		HijackAfterFunc: func() {},
		SSL:             false,
		SASL:            false,
		Channels:        []string{"#test"},
		Password:        "",
		// Somewhat sane default if for some reason we can't retrieve bot's prefix
		// for example, if the server doesn't advertise joins
		prefix: &irc.Prefix{
			Name: nick,
			User: nick,
			Host: strings.Repeat("*", 510-400-len(nick)*2),
		},
		prefixMu: &sync.RWMutex{},
	}
	for _, option := range options {
		option(&bot)
	}
	// Discard logs by default
	bot.Logger = log.New("id", logext.RandId(8), "host", bot.Host, "nick", log.Lazy{Fn: bot.getNick})

	bot.Logger.SetHandler(log.DiscardHandler())
	bot.AddTrigger(pingPong)
	bot.AddTrigger(joinChannels)
	bot.AddTrigger(getPrefix)
	bot.AddTrigger(setNick)
	bot.AddTrigger(nickError)
	bot.AddTrigger(bot.ircV3)
	return &bot, nil
}

// Prefix returns the bot's own prefix.
// Can be useful if for example you want to
// make an emoji wall that fits into one message perfectly
func (bot *Bot) Prefix() *irc.Prefix {
	bot.prefixMu.RLock()
	prefix := &irc.Prefix{
		Name: bot.prefix.Name,
		User: bot.prefix.User,
		Host: bot.prefix.Host,
	}
	bot.prefixMu.RUnlock()
	return prefix
}

// PrefixChange changes bot's prefix,
// use empty strings to make no change
func (bot *Bot) PrefixChange(name, user, host string) {
	bot.prefixMu.Lock()
	if name != "" {
		bot.prefix.Name = name
	}
	if user != "" {
		bot.prefix.User = user
	}
	if host != "" {
		bot.prefix.Host = host
	}
	bot.prefixMu.Unlock()
}

// Uptime returns the uptime of the bot
func (bot *Bot) Uptime() string {
	return fmt.Sprintf("Started: %s, Uptime: %s", bot.started, time.Since(bot.started))
}

func (bot *Bot) getNick() string {
	bot.mu.Lock()
	defer bot.mu.Unlock()
	return bot.nick
}

func (bot *Bot) connect(host string) (err error) {
	bot.Debug("Connecting")
	dial := bot.Dial
	if dial == nil {
		dial = net.Dial
	}
	dialTLS := bot.DialTLS
	if dialTLS == nil {
		dialTLS = tls.Dial
	}

	if bot.SSL {
		bot.con, err = dialTLS("tcp", host, &bot.TLSConfig)
	} else {
		bot.con, err = dial("tcp", host)
	}
	return err
}

// Incoming message gathering routine
func (bot *Bot) handleIncomingMessages() {
	defer bot.wg.Done()
	scan := bufio.NewScanner(bot.con)
	for scan.Scan() {
		// Disconnect if we have seen absolutely nothing for 300 seconds
		bot.con.SetDeadline(time.Now().Add(bot.PingTimeout))
		msg := parseMessage(scan.Text())
		bot.Debug("Incoming", "raw", scan.Text(), "msg.To", msg.To, "msg.From", msg.From, "msg.Params", msg.Params, "msg.Trailing", msg.Trailing())
		go func() {
			for _, h := range bot.handlers {
				go h.Handle(bot, msg)
			}
		}()
	}
	if !bot.isClosing() {
		bot.setClosing(true)
		bot.Error("read incoming", "error", scan.Err())
		if bot.unixlist != nil {
			err := bot.unixlist.Close()
			if err != nil {
				bot.Error("closing unix listener", "error", err)
			}
		}
		select {
		case bot.outgoing <- "DISCONNECT":
		default:
		}
	}
}

// Handles message speed throtling
func (bot *Bot) handleOutgoingMessages() {
	defer bot.wg.Done()
	for s := range bot.outgoing {
		bot.Debug("Outgoing", "data", s)
		_, err := fmt.Fprint(bot.con, s+"\r\n")
		if err != nil {
			if !bot.isClosing() {
				bot.setClosing(true)
				bot.Error("send outgoing", "error", err)
				if bot.unixlist != nil {
					err := bot.unixlist.Close()
					if err != nil {
						bot.Error("closing unix listener", "error", err)
					}
				}
			}
			return
		}
		time.Sleep(bot.ThrottleDelay)
	}
}

// saslAuthenticate performs SASL authentication
// ref: https://github.com/atheme/charybdis/blob/master/doc/sasl.txt
func (bot *Bot) saslAuthenticate(user, pass string) {
	bot.ircV3.saslEnable()
	bot.ircV3.saslCreds(user, pass)
	bot.Debug("Beginning SASL Authentication")
	bot.Send("CAP LS")
	bot.SetNick(bot.Nick)
	bot.sendUserCommand(bot.Nick, bot.Nick, "0")
}

// standardRegistration performs a basic set of registration commands
func (bot *Bot) standardRegistration() {
	bot.Send("CAP LS")
	//Server registration
	if bot.Password != "" {
		bot.Send("PASS " + bot.Password)
	}
	bot.Debug("Sending standard registration")
	bot.sendUserCommand(bot.Nick, bot.Nick, "0")
	bot.SetNick(bot.Nick)
}

// Set username, real name, and mode
func (bot *Bot) sendUserCommand(user, realname, mode string) {
	bot.Send(fmt.Sprintf("USER %s %s * :%s", user, mode, realname))
}

// SetNick sets the bots nick on the irc server.
// This does not alter *Bot.Nick, so be vary of that
func (bot *Bot) SetNick(nick string) {
	bot.Send(fmt.Sprintf("NICK %s", nick))
}

// Run starts the bot and connects to the server. Blocks until we disconnect from the server.
// Returns true if we have been hijacked (if you loop over Run it might be wise to break on hijack
// to avoid looping between 2 instances).
func (bot *Bot) Run() (hijacked bool) {
	bot.Debug("Starting bot goroutines")
	// Reset some things in case we re-run Run
	bot.reset()
	// Attempt reconnection
	var hijack bool
	if bot.HijackSession {
		if bot.SSL {
			bot.Crit("Can't Hijack a SSL connection")
			return
		}
		hijack = bot.hijackSession()
		bot.Debug("Hijack", "Did we?", hijack)
	}

	if !hijack {
		err := bot.connect(bot.Host)
		if err != nil {
			bot.Crit("bot.Connect error", "err", err.Error())
			return
		}
		bot.Info("Connected successfully!")
	}

	bot.wg.Add(1)
	go bot.handleIncomingMessages()
	bot.wg.Add(1)
	go bot.handleOutgoingMessages()

	if hijack {
		go bot.HijackAfterFunc()
	}
	bot.wg.Add(1)
	go bot.startUnixListener()

	// Only register on an initial connection
	if !bot.reconnecting {
		if bot.SASL {
			bot.saslAuthenticate(bot.Nick, bot.Password)
		} else {
			bot.standardRegistration()
		}
	}
	bot.wg.Wait()
	bot.Info("Disconnected")
	return bot.hijacked

}

func (bot *Bot) reset() {
	// These need to be reset on each run
	bot.closer = make(chan struct{})
	bot.outgoing = make(chan string, 16)
	bot.mu.Lock()
	bot.didJoinChannels = sync.Once{}
	bot.mu.Unlock()
	bot.wg = sync.WaitGroup{}
	bot.hijacked = false
	bot.reconnecting = false
	bot.setClosing(false)
	bot.ircV3.reset()
}

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
	for _, line := range splitText(text, command, who, bot.Prefix()) {
		bot.Send(command + " " + who + " :" + line)
	}
}

// Notice sends a NOTICE message to 'who' (user or channel)
func (bot *Bot) Notice(who, text string) {
	const command = "NOTICE"
	for _, line := range splitText(text, command, who, bot.Prefix()) {
		bot.Send(command + " " + who + " :" + line)
	}
}

// Splits a given string into a string slice, in chunks ending
// either with \n, or with \r\n, or splitting text to maximally allowed size.
func splitText(text, command, who string, prefix *irc.Prefix) []string {
	var ret []string

	// Maximum message size that fits into 512 bytes.
	// Carriage return and linefeed are not counted here as they
	// are added by handleOutgoingMessages()
	maxSize := 510 - len(fmt.Sprintf(":%s %s %s :", prefix, command, who))

	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := scanner.Text()
		for len(line) > maxSize {
			totalSize := 0
			runeSize := 0
			// utf-8 aware splitting
			for _, v := range line {
				runeSize = utf8.RuneLen(v)
				if totalSize+runeSize > maxSize {
					ret = append(ret, line[:totalSize])
					line = line[totalSize:]
					totalSize = runeSize
					continue
				}
				totalSize += runeSize
			}

		}
		ret = append(ret, line)
	}
	return ret
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

func (bot *Bot) setClosing(closing bool) {
	bot.mu.Lock()
	bot.closing = closing
	bot.mu.Unlock()
}

// Close closes the bot
func (bot *Bot) Close() error {
	if bot.isClosing() {
		return errors.New("kittybot is already closing")
	}
	bot.setClosing(true)
	if bot.unixlist != nil {
		err := bot.unixlist.Close()
		if err != nil {
			return err
		}
	}
	err := bot.con.Close()
	if err != nil {
		return err
	}
	select {
	case bot.outgoing <- "DISCONNECT":
	default:
	}
	return nil
}

// AddTrigger adds a trigger to the bot's handlers
func (bot *Bot) AddTrigger(h Handler) {
	bot.handlers = append(bot.handlers, h)
}

// Handler is used to subscribe and react to events on the bot Server
type Handler interface {
	Handle(*Bot, *Message)
}

// Trigger is a Handler which is guarded by a condition.
// DO NOT alter *Message in your triggers or you'll have strange things happen.
type Trigger struct {
	// Returns true if this trigger applies to the passed in message
	Condition func(*Bot, *Message) bool

	// The action to perform if Condition is true
	Action func(*Bot, *Message)
}

// Handle executes the trigger action if the condition is satisfied
func (t Trigger) Handle(b *Bot, m *Message) {
	if t.Condition(b, m) {
		t.Action(b, m)
	}
}

// SaslAuth enables SASL authentification
func SaslAuth(pass string) func(*Bot) {
	return func(b *Bot) {
		b.SASL = true
		b.Password = pass
	}
}

// ReconOpt enables session hijacking
func ReconOpt() func(*Bot) {
	return func(b *Bot) {
		b.HijackSession = true
	}
}

// Message represents a message received from the server
type Message struct {
	// irc.Message from sorcix
	*irc.Message
	// Content generally refers to the text of a PRIVMSG
	Content string

	//Time at which this message was recieved
	TimeStamp time.Time

	// Entity that this message was addressed to (channel or user)
	To string

	// Nick of the messages sender (equivalent to Prefix.Name)
	// Outdated, please use .Name
	From string
}

// parseMessage takes a string and attempts to create a Message struct.
// Returns nil if the Message is invalid.
func parseMessage(raw string) (m *Message) {
	m = new(Message)
	m.Message = irc.ParseMessage(raw)
	m.Content = m.Trailing()

	if len(m.Params) > 0 {
		m.To = m.Params[0]
	} else if m.Command == "JOIN" {
		m.To = m.Trailing()
	}
	if m.Prefix != nil {
		m.From = m.Prefix.Name
	}
	m.TimeStamp = time.Now()

	return m
}

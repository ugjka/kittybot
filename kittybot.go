// Package kitty is IRCv3 enabled framework for writing IRC bots
package kitty

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/ugjka/ircmsg"
	log "gopkg.in/inconshreveable/log15.v2"

	"crypto/tls"
)

// Bot implements an irc bot to be connected to a given server
type Bot struct {

	// This is set if we have hijacked a connection
	reconnecting bool
	// This is set if we have been hijacked
	hijacked bool
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
	// Log15 loggger
	log.Logger
	joinOnce  sync.Once
	closeOnce sync.Once
	wg        sync.WaitGroup
	// IRC CAPS and
	// SASL credentials
	capHandler *ircCaps
	// Exported fields
	Host          string
	Password      string
	Channels      []string
	SSL           bool
	SASL          bool
	HijackSession bool
	// Set it if long messages get truncated
	// on the receiving end
	MsgSafetyBuffer bool
	// HijackAfterFunc executes in its own goroutine after a succesful session hijack
	// If you need to do something after a hijack
	// for example, to run some irc commands or to restore some state
	HijackAfterFunc func()
	// Fires after joining the channels
	Joined chan struct{}
	// An optional function that connects to an IRC server over plaintext:
	Dial func(network, addr string) (net.Conn, error)
	// An optional function that connects to an IRC server over a secured connection:
	DialTLS func(network, addr string, tlsConf *tls.Config) (*tls.Conn, error)
	// This bots nick
	Nick string
	// Transient nick, that is used internally to track nick changes and calculate the prefix for the bot
	nick string
	// This bots realname
	Realname string
	// Duration to wait between sending of messages to avoid being
	// kicked by the server for flooding (default 250ms)
	ThrottleDelay time.Duration
	// Enable reply flood protection
	LimitReplies bool
	// Token bucket rate limiter constraints
	// default: 5 messages per 10 seconds
	ReplyMessageLimit int
	ReplyInterval     time.Duration
	// Maxmimum time between incoming data
	PingTimeout time.Duration

	TLSConfig tls.Config
	// Bot's prefix
	prefix   *ircmsg.Prefix
	prefixMu *sync.RWMutex
	// rate limiter
	limiter *rateLimiter
}

func (bot *Bot) String() string {
	return fmt.Sprintf("Server: %s, Channels: %v, Nick: %s", bot.Host, bot.Channels, bot.getNick())
}

// NewBot creates a new instance of Bot
func NewBot(host, nick string, options ...func(*Bot)) *Bot {

	// determine user for intial prefix
	user := func() string {
		if len(nick) > 9 {
			return nick[:9]
		}
		return nick
	}()

	// Defaults are set here
	bot := Bot{
		started:         time.Now(),
		unixastr:        fmt.Sprintf("@%s-%s/bot", host, nick),
		unixsock:        fmt.Sprintf("/tmp/%s-%s-bot.sock", host, nick),
		outgoing:        make(chan string, 16),
		Host:            host,
		Nick:            nick,
		nick:            nick,
		Realname:        nick,
		capHandler:      &ircCaps{},
		ThrottleDelay:   time.Millisecond * 300,
		PingTimeout:     300 * time.Second,
		HijackSession:   false,
		HijackAfterFunc: func() {},
		Joined:          make(chan struct{}),
		SSL:             false,
		SASL:            false,
		Channels:        []string{"#test"},
		Password:        "",
		// Somewhat sane default if for some reason we can't retrieve bot's prefix
		// for example, if the server doesn't advertise joins
		prefix: &ircmsg.Prefix{
			Name: nick,
			User: user,
			Host: strings.Repeat("*", 510-353-len(nick)-len(user)),
		},
		prefixMu:          &sync.RWMutex{},
		ReplyMessageLimit: 5,
		ReplyInterval:     time.Second * 10,
	}
	for _, option := range options {
		option(&bot)
	}
	// Discard logs by default
	bot.Logger = log.New()

	bot.Logger.SetHandler(log.DiscardHandler())
	bot.AddTrigger(pingPong)
	bot.AddTrigger(joinChannels)
	bot.AddTrigger(getPrefix)
	bot.AddTrigger(setNick)
	bot.AddTrigger(nickError)
	bot.AddTrigger(bot.capHandler)
	bot.AddTrigger(saslFail)
	bot.AddTrigger(saslSuccess)
	return &bot
}

// saslAuthenticate performs SASL authentication
// ref: https://github.com/atheme/charybdis/blob/master/doc/sasl.txt
func (bot *Bot) saslAuthenticate(user, pass string) {
	bot.capHandler.saslEnable()
	bot.capHandler.saslCreds(user, pass)
	bot.Debug("beginning sasl authentication")
	bot.Send("CAP LS")
	bot.SetNick(bot.Nick)
	bot.sendUserCommand(bot.Nick, bot.Realname, "0")
}

// standardRegistration performs a basic set of registration commands
func (bot *Bot) standardRegistration() {
	bot.Send("CAP LS")
	//Server registration
	if bot.Password != "" {
		bot.Send("PASS " + bot.Password)
	}
	bot.Debug("sending standard registration")
	bot.sendUserCommand(bot.Nick, bot.Realname, "0")
	bot.SetNick(bot.Nick)
}

// Set username, real name, and mode
func (bot *Bot) sendUserCommand(user, realname, mode string) {
	bot.Send(fmt.Sprintf("USER %s %s * :%s", user, mode, realname))
}

func (bot *Bot) getNick() string {
	bot.mu.Lock()
	defer bot.mu.Unlock()
	return bot.nick
}

func (bot *Bot) connect(host string) (err error) {
	bot.Debug("connecting")
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
		// Disconnect if we have seen absolutely nothing for defined amount of time
		bot.con.SetDeadline(time.Now().Add(bot.PingTimeout))

		msg := parseMessage(scan.Text())
		bot.Debug(fmt.Sprintf("[incoming]-[%s]", bot.Host), "raw", scan.Text())
		go func() {
			for _, h := range bot.handlers {
				go h.Handle(bot, msg)
			}
		}()
	}
	bot.close("incoming", scan.Err())
}

// Handles message speed throtling
func (bot *Bot) handleOutgoingMessages() {
	defer bot.wg.Done()
	for s := range bot.outgoing {
		bot.Debug(fmt.Sprintf("[outgoing]-[%s]", bot.Host), "raw", s)
		_, err := fmt.Fprint(bot.con, s+"\r\n")
		if err != nil {
			bot.close("outgoing", err)
			return
		}
		time.Sleep(bot.ThrottleDelay)
	}
}

// Run starts the bot and connects to the server. Blocks until we disconnect from the server.
// Returns true if we have been hijacked (if you loop over Run it might be wise to break on hijack
// to avoid looping between 2 instances).
func (bot *Bot) Run() (hijacked bool) {
	bot.Debug("starting bot goroutines")
	// Reset some things in case we re-run Run
	bot.reset()
	// Attempt reconnection
	var hijack bool
	if bot.HijackSession {
		if bot.SSL {
			bot.Crit("can't hijack an ssl connection")
			return
		}
		hijack = bot.hijackSession()
		bot.Debug("hijack", "did we?", hijack)
	}

	if !hijack {
		err := bot.connect(bot.Host)
		if err != nil {
			bot.Crit("connect error", "err", err.Error())
			return
		}
		bot.Info("connected successfully!")
	}

	// token bucket rate limiter for reply spam
	if bot.LimitReplies {
		bot.limiter = newRateLimiter(bot.ReplyMessageLimit, bot.ReplyInterval)
		bot.limiter.start()
	}

	bot.wg.Add(1)
	go bot.handleIncomingMessages()
	bot.wg.Add(1)
	go bot.handleOutgoingMessages()
	bot.wg.Add(1)
	go bot.startUnixListener()

	if hijack {
		go bot.HijackAfterFunc()
	}

	// Only register on an initial connection
	if !bot.reconnecting {
		if bot.SASL {
			bot.saslAuthenticate(bot.Nick, bot.Password)
		} else {
			bot.standardRegistration()
		}
	}
	bot.wg.Wait()
	if bot.limiter != nil {
		bot.limiter.kill()
	}
	bot.Info("disconnected")
	return bot.hijacked

}

// CapStatus returns whether the server capability is enabled and present
func (bot *Bot) CapStatus(cap string) (enabled, present bool) {
	bot.capHandler.mu.Lock()
	defer bot.capHandler.mu.Unlock()
	if v, ok := bot.capHandler.capsEnabled[cap]; ok {
		return v, true
	}
	return false, false
}

// internal closer
func (bot *Bot) close(fault string, err error) {
	bot.closeOnce.Do(func() {
		if err != nil {
			bot.Error(fault, "error", err)
		}
		if bot.unixlist != nil {
			bot.unixlist.Close()
		}
		bot.con.Close()
		select {
		case bot.outgoing <- "PING":
		default:
		}
	})
}

// Close closes the bot
func (bot *Bot) Close() {
	bot.close("", nil)
}

// Prefix returns the bot's own prefix.
// Can be useful if for example you want to
// make an emoji wall that fits into one message perfectly
func (bot *Bot) Prefix() *ircmsg.Prefix {
	bot.prefixMu.RLock()
	prefix := &ircmsg.Prefix{
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

// ReconOpt enables session hijacking
func ReconOpt() func(*Bot) {
	return func(bot *Bot) {
		bot.HijackSession = true
	}
}

// SaslAuth enables SASL authentification
func SaslAuth(pass string) func(*Bot) {
	return func(bot *Bot) {
		bot.SASL = true
		bot.Password = pass
	}
}

// Uptime returns the uptime of the bot
func (bot *Bot) Uptime() string {
	return fmt.Sprintf("Started: %s, Uptime: %s", bot.started, time.Since(bot.started))
}

func (bot *Bot) reset() {
	// These need to be reset on each run
	bot.mu.Lock()
	bot.joinOnce = sync.Once{}
	bot.closeOnce = sync.Once{}
	bot.mu.Unlock()
	bot.wg = sync.WaitGroup{}
	bot.hijacked = false
	bot.reconnecting = false
	bot.capHandler.reset()
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

// AddTrigger adds a trigger to the bot's handlers
func (bot *Bot) AddTrigger(h Handler) {
	bot.handlers = append(bot.handlers, h)
}

// Handle executes the trigger action if the condition is satisfied
func (t Trigger) Handle(bot *Bot, m *Message) {
	if t.Condition(bot, m) {
		t.Action(bot, m)
	}
}

// Message represents a message received from the server
type Message struct {
	// ircmsg.Message with extended data, like GetTag() for IRCv3 tags
	*ircmsg.Message
	// Content generally refers to the text of a PRIVMSG
	Content string

	// Raw contains the raw message
	Raw string

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
	m.Message = ircmsg.ParseMessage(raw)
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

	m.Raw = raw

	return m
}

# KittyBot

[![GoDoc](https://godoc.org/github.com/ugjka/kittybot?status.png)](https://godoc.org/github.com/ugjka/kittybot)

Hard fork of [github.com/whyrusleeping/hellabot](https://github.com/whyrusleeping/hellabot)

![kittybot](kitty.png?raw=true)

Kitten approved Internet Relay Chat (IRC) bot. KittyBot is an easily hackable event based IRC bot
framework with the ability to be updated without losing connection to the
server. To respond to an event, simply create a "Trigger" struct containing
two functions, one for the condition, and one for the action.

## Warning

We are at v0.0.X, API may change without warning!!!

## Example Trigger

```go
var myTrigger = kitty.Trigger{
    Condition: func(bot *kitty.Bot, m *kitty.Message) bool {
        return m.From == "ugjka"
    },
    Action: func(bot *kitty.Bot, m *kitty.Message) {
        bot.Reply(m, "ugjka said something")
    },
}
```

The trigger makes the bot announce to everyone that something was said in the current channel. Use the code snippet below to make the bot and add the trigger.

```go
bot, err := kitty.NewBot("irc.freenode.net:6667","kittybot")
if err != nil {
    panic(err)
}
bot.AddTrigger(MyTrigger)
bot.Run() // Blocks until exit
```

The 'To' field on the message object in triggers will refer to the channel that
a given message is in, unless it is a server message, or a user to user private
message. In such cases, the field will be the target user's name.

For more example triggers, check the examples directory.

## The Message struct

The message struct is primarily what you will be dealing with when building
triggers or reading off the Incoming channel.
This is mainly the sorcix.Message struct with some additions.
See [https://github.com/sorcix/irc/blob/master/message.go#L153](https://github.com/sorcix/irc/blob/master/message.go#L153)

```go
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
```

## Connection Passing

KittyBot is able to restart without dropping its connection to the server
(on Linux machines, and BSD flavours) by passing the TCP connection through a UNIX domain socket.
This allows you to update triggers and other addons without actually logging
your bot out of the IRC, avoiding the loss of op status and spamming the channel
with constant join/part messages. To do this, run the program again with
the same nick and without killing the first program (different nicks wont reuse
the same bot instance). The first program will shutdown, and the new one
will take over.

\***\*This does not work with SSL connections, because we can't hand over a SSL connections state.\*\***

## Security

KittyBot supports both SSL and SASL for secure connections to whichever server
you like. To enable SSL, pass the following option to the NewBot function.

```go
sslOptions := func(bot *kitty.Bot) {
    bot.SSL = true
}

bot, err := kitty.NewBot("irc.freenode.net:6667","kittybot",sslOptions)
// Handle err as you like

bot.Run() # Blocks until disconnect.
```

To use SASL to authenticate with the server:

```go
saslOption = func(bot *kitty.Bot) {
    bot.SASL = true
    bot.Password = "somepassword"
}

bot, err := kitty.NewBot("irc.freenode.net:6667", "kittybot", saslOption)
// Handle err as you like

bot.Run() # Blocks until disconnect.
```

Note: SASL does not require SSL but can be used in combination.

## Passwords

For servers that require passwords in the initial registration, simply set
the Password field of the Bot struct before calling its Start method.

## Debugging

Hellabot uses github.com/inconshreveable/log15 for logging.
See [http://godoc.org/github.com/inconshreveable/log15](http://godoc.org/github.com/inconshreveable/log15)

By default it discards all logs. In order to see any logs, give it a better handler.
Example: This would only show INFO level and above logs, logging to STDOUT

```go
import log "gopkg.in/inconshreveable/log15.v2"

logHandler := log.LvlFilterHandler(log.LvlInfo, log.StdoutHandler)
bot.Logger.SetHandler(logHandler)
```

Note: This might be revisited in the future.

## Why

What do you need an IRC bot for you ask? Well, I've gone through the trouble of
compiling a list of fun things for you! The following are some of the things KittyBot is
currently being used for:

- AutoOp Bot: ops you when you join the channel
- Stats counting bot: counts how often people talk in a channel
- Mock users you don't like by repeating what they say
- Fire a USB dart launcher on a given command
- Control an MPD radio stream based on chat commands
- Award praise to people for guessing a random number
- Scrape news sites for relevant articles and send them to a channel
- And many other 'fun' things!

## References

[Client Protocol, RFC 2812](http://tools.ietf.org/html/rfc2812)
[SASL Authentication Documentation](https://tools.ietf.org/html/draft-mitchell-irc-capabilities-01)

## Credits

[sorcix](http://github.com/sorcix) for his Message Parsing code

## Contributors before the hard fork

- @[whyrusleeping](https://github.com/whyrusleeping)
- @[flexd](https://github.com/flexd)
- @[icholy](https://github.com/icholy)
- @[bbriggs](https://github.com/bbriggs)
- @[Luzifer](https://github.com/Luzifer)
- @[mudler](https://github.com/mudler)
- @[jonreyna](https://github.com/jonreyna)
- @[miloprice](https://github.com/miloprice)
- @[m-242](https://github.com/m-242)
- @[antifuchs](https://github.com/antifuchs)
- @[JReyLBC](https://github.com/)
- @[ForrestWeston](https://github.com/ForrestWeston)
- @[affankingkhan](https://github.com/affankingkhan)
- @[Villawhatever](https://github.com/Villawhatever)

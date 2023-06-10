package kitty

import (
	"time"
)

// Token Bucket rate limiter
type rateLimiter struct {
	messageLimit  int
	interval      time.Duration
	tokens        chan struct{}
	tokenInterval time.Duration
	killchan      chan struct{}
}

func newRateLimiter(messageLimit int, interval time.Duration) *rateLimiter {
	return &rateLimiter{
		messageLimit:  messageLimit,
		interval:      interval,
		tokens:        make(chan struct{}, messageLimit),
		tokenInterval: interval / time.Duration(messageLimit),
		killchan:      make(chan struct{}),
	}
}

func (rl *rateLimiter) start() {
	go func() {
		timer := time.NewTimer(rl.tokenInterval)
		for {
			select {
			case <-rl.killchan:
				timer.Stop()
				close(rl.tokens)
				return
			case <-timer.C:
				select {
				case rl.tokens <- struct{}{}:
				default:
				}
				timer = time.NewTimer(rl.tokenInterval)
			}
		}
	}()
}

func (rl *rateLimiter) kill() {
	close(rl.killchan)
}

// Drop drains the token bucket rate limiter
// returns true when the bucket is empty
func (rl *rateLimiter) drop() bool {
	select {
	case <-rl.tokens:
		return false
	default:
		return true
	}
}

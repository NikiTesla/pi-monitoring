package monitor

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/NikiTesla/pi-monitoring/internal/telegram"
)

type sender interface {
	Send(context.Context, int64, string) error
}

type outgoing struct{ text string }

// Each configured destination owns a bounded mailbox and retry loop. Alerts
// coalesce by entity, so a recovered entity replaces its pending failure alert.
type mailbox struct {
	mu      sync.Mutex
	pending map[string]*outgoing
	wake    chan struct{}
	chat    int64
}

func newMailbox(chat int64) *mailbox {
	return &mailbox{pending: make(map[string]*outgoing), wake: make(chan struct{}, 1), chat: chat}
}

func (m *mailbox) put(key, message string) {
	m.mu.Lock()
	m.pending[key] = &outgoing{text: message}
	m.mu.Unlock()
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *mailbox) deliver(ctx context.Context, client sender, logger *log.Logger) {
	backoff := time.Second
	failed := false
	for {
		m.mu.Lock()
		var key string
		var item *outgoing
		for k, value := range m.pending {
			key, item = k, value
			break
		}
		m.mu.Unlock()
		if item == nil {
			select {
			case <-ctx.Done():
				return
			case <-m.wake:
				continue
			}
		}
		sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := client.Send(sendCtx, m.chat, item.text)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			if !failed {
				logger.Print("Telegram delivery unavailable; retaining latest pending state")
			}
			failed = true
			delay := backoff
			var limited *telegram.RetryAfter
			if errors.As(err, &limited) {
				delay = max(delay, limited.Delay)
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			backoff = min(30*time.Second, backoff*2)
			continue
		}
		if failed {
			logger.Print("Telegram delivery resumed")
		}
		failed, backoff = false, time.Second
		m.mu.Lock()
		if m.pending[key] == item {
			delete(m.pending, key)
		}
		m.mu.Unlock()
	}
}

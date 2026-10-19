package monitor

import (
	"context"
	"errors"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/NikiTesla/pi-monitoring/internal/config"
	"github.com/NikiTesla/pi-monitoring/internal/telegram"
)

type poller interface {
	GetMe(context.Context) (telegram.User, error)
	Updates(context.Context, int64) ([]telegram.Update, error)
}

func (r *runtime) poll(ctx context.Context, client poller, commands chan<- command, logger *log.Logger) {
	backoff := time.Second
	for ctx.Err() == nil {
		identityCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		identity, err := client.GetMe(identityCtx)
		cancel()
		if err == nil && identity.IsBot && identity.Username != "" {
			r.username = identity.Username
			break
		}
		if backoff == time.Second {
			logger.Print("Telegram bot identity unavailable; monitoring continues while retrying")
		}
		if !waitRetry(ctx, retryDelay(backoff, err)) {
			return
		}
		backoff = min(30*time.Second, backoff*2)
	}
	var offset int64
	failed := false
	backoff = time.Second
	for ctx.Err() == nil {
		updates, err := client.Updates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if !failed {
				logger.Print("Telegram command polling unavailable; retrying")
			}
			failed = true
			if !waitRetry(ctx, retryDelay(backoff, err)) {
				return
			}
			backoff = min(30*time.Second, backoff*2)
			continue
		}
		if failed {
			logger.Print("Telegram command polling resumed")
		}
		failed = false
		backoff = time.Second
		for _, update := range updates {
			if update.ID < offset {
				continue
			}
			offset = update.ID + 1
			cmd, ok := r.authorizedCommand(update.Message)
			if !ok {
				continue
			}
			select {
			case commands <- cmd:
			default:
				r.reply(cmd.chat, "⏳ Command queue busy\nPlease try again shortly.")
			}
		}
	}
}

func retryDelay(backoff time.Duration, err error) time.Duration {
	var limited *telegram.RetryAfter
	if errors.As(err, &limited) {
		return max(backoff, limited.Delay)
	}
	return backoff
}

func waitRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (r *runtime) authorizedCommand(message *telegram.Message) (command, bool) {
	if message == nil || message.From == nil || message.From.IsBot || message.SenderChat != nil || !slices.Contains(r.settings.Telegram.AllowedChatIDs, message.Chat.ID) || !slices.Contains(r.settings.Telegram.AllowedUserIDs, message.From.ID) {
		return command{}, false
	}
	fields := strings.Fields(message.Text)
	if len(fields) == 0 || len(fields) > 2 || !strings.HasPrefix(fields[0], "/") {
		return command{}, false
	}
	name, addressed, hasAddress := strings.Cut(strings.TrimPrefix(fields[0], "/"), "@")
	if hasAddress && !strings.EqualFold(addressed, r.username) {
		return command{}, false
	}
	cmd := command{chat: message.Chat.ID, user: message.From.ID, sentAt: time.Unix(message.Date, 0), name: name}
	if len(fields) == 2 {
		cmd.argument = fields[1]
	}
	return cmd, true
}

func (r *runtime) handleCommand(cmd command, jobs chan<- config.HTTPCheck, now time.Time) {
	switch cmd.name {
	case "help", "start":
		r.reply(cmd.chat, "📖 Monitor commands\n\n/status — server overview\n/host — latest host metrics\n/checks — HTTP check results\n/check <id> — run a check now\n/incidents — active problems\n/reboot — reboot this server (opt-in; confirmation required)\n/help — show this guide\n\nExample: /check example-api")
	case "reboot":
		r.handleReboot(cmd, now)
	case "host":
		r.reply(cmd.chat, r.hostSummary(now))
	case "status":
		r.reply(cmd.chat, "🖥 "+r.settings.Server.Name+"\n\n"+r.hostSummary(now)+"\n\n"+r.checksSummary(now)+"\n\n"+r.incidentsSummary(now))
	case "checks":
		r.reply(cmd.chat, r.checksSummary(now))
	case "incidents":
		r.reply(cmd.chat, r.incidentsSummary(now))
	case "check":
		for _, state := range r.checks {
			if state.check.ID != cmd.argument {
				continue
			}
			if state.running {
				r.reply(cmd.chat, "⏳ Check already running\nUse /checks for the latest result.")
				return
			}
			select {
			case jobs <- state.check:
				state.running, state.waitingChat = true, cmd.chat
				state.next = now.Add(state.check.Interval.Value())
				r.reply(cmd.chat, "⏳ Check started: "+state.check.ID+"\nThe result will appear here.")
			default:
				r.reply(cmd.chat, "⏳ All check workers are busy\nPlease try again shortly.")
			}
			return
		}
		r.reply(cmd.chat, "❔ Check not found\nUse /checks to see available IDs, then /check <id>.")
	default:
		r.reply(cmd.chat, "❔ Unknown command\nUse /help to see available commands.")
	}
}

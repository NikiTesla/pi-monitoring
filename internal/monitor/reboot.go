package monitor

import (
	"context"
	"crypto/rand"
	"log"
	"os/exec"
	"time"
)

const rebootConfirmationLifetime = time.Minute

type rebootConfirmation struct {
	chat, user int64
	token      string
	expires    time.Time
}

// Confirmation and request state belong to the runtime loop. The worker uses
// only the channels; no shell command or executable comes from Telegram/config.
type rebootControl struct {
	pending   *rebootConfirmation
	requested bool
	requests  chan int64
	results   chan rebootResult
}

type rebootResult struct {
	chat int64
	err  error
}

func (r *runtime) handleReboot(cmd command, now time.Time) {
	if !r.settings.Telegram.AllowReboot {
		r.reply(cmd.chat, "🔒 Reboot is disabled\nEnable telegram.allow_reboot and configure host permissions first.")
		return
	}
	// Delayed Telegram updates must not turn into a later, unexpected reboot.
	if cmd.sentAt.Unix() <= 0 || now.Sub(cmd.sentAt) >= rebootConfirmationLifetime || cmd.sentAt.After(now.Add(5*time.Second)) {
		r.reply(cmd.chat, "⌛ Reboot command is too old or has an invalid time\nSend /reboot again.")
		return
	}
	if r.reboot.requested {
		r.reply(cmd.chat, "⏳ Reboot already requested\nThe bot may go offline while the server restarts.")
		return
	}
	if cmd.argument == "" {
		confirmation := &rebootConfirmation{
			chat: cmd.chat, user: cmd.user, token: rand.Text(),
			expires: now.Add(rebootConfirmationLifetime),
		}
		r.reboot.pending = confirmation
		r.reply(cmd.chat, "⚠️ Reboot "+r.settings.Server.Name+"?\nThis restarts the whole server and interrupts its applications.\n\nTo confirm within 60 seconds, send:\n/reboot "+confirmation.token+"\n\nOnly you can confirm in this chat. Ignore this message to cancel.")
		return
	}
	confirmation := r.reboot.pending
	if confirmation == nil || !now.Before(confirmation.expires) || cmd.chat != confirmation.chat || cmd.user != confirmation.user || cmd.argument != confirmation.token {
		r.reply(cmd.chat, "⌛ Reboot confirmation is invalid or expired\nSend /reboot to request a new confirmation.")
		return
	}
	// Consume before queueing, including on failure. Redelivery cannot reuse it.
	r.reboot.pending = nil
	select {
	case r.reboot.requests <- cmd.chat:
		r.reboot.requested = true
		r.reply(cmd.chat, "⏳ Requesting server reboot\nThe bot may go offline before another reply arrives.")
	default:
		r.reply(cmd.chat, "⏳ Reboot worker unavailable\nSend /reboot to try again.")
	}
}

func (r *runtime) finishReboot(result rebootResult, logger *log.Logger) {
	if result.err != nil {
		r.reboot.requested = false
		logger.Printf("Host reboot request failed: %v", result.err)
		r.reply(result.chat, "❌ Reboot request failed or timed out\nCheck server state before trying again. Verify systemd/logind and the pi-monitor reboot policy; check the service journal for details.")
		return
	}
	logger.Print("Host reboot request accepted")
	// Keep requested set until process exit: systemd acceptance precedes reboot.
	r.reply(result.chat, "🔄 Reboot request accepted\nThe bot will be unavailable while the server restarts.")
}

func (r *rebootControl) run(ctx context.Context, reboot func(context.Context) error) {
	for {
		select {
		case <-ctx.Done():
			return
		case chat := <-r.requests:
			if ctx.Err() != nil {
				return
			}
			rebootCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := reboot(rebootCtx)
			cancel()
			select {
			case r.results <- rebootResult{chat: chat, err: err}:
			case <-ctx.Done():
				return
			}
		}
	}
}

func hostRebootCommand(ctx context.Context) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "/usr/bin/systemctl", "--no-ask-password", "--no-wall", "reboot")
	cmd.WaitDelay = time.Second
	return cmd
}

func requestHostReboot(ctx context.Context) error {
	return hostRebootCommand(ctx).Run()
}

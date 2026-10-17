package monitor

import (
	"context"
	"errors"
	"io"
	"log"
	"reflect"
	"strings"
	"testing"
	"time"

	"pi-monitoring/internal/config"
	"pi-monitoring/internal/telegram"
)

func rebootFixture() runtime {
	return runtime{
		settings: config.Config{Server: config.Server{Name: "test-server"}, Telegram: config.Telegram{
			AllowReboot: true, AllowedChatIDs: []int64{11, 12}, AllowedUserIDs: []int64{22, 23},
		}},
		username:  "test_bot",
		mailboxes: map[int64]*mailbox{11: newMailbox(11), 12: newMailbox(12)},
		reboot:    rebootControl{requests: make(chan int64, 1), results: make(chan rebootResult, 1)},
	}
}

func rebootMessage(now time.Time, argument string) telegram.Message {
	text := "/reboot@test_bot"
	if argument != "" {
		text += " " + argument
	}
	return telegram.Message{Date: now.Unix(), From: &telegram.User{ID: 22}, Chat: telegram.Chat{ID: 11}, Text: text}
}

func dispatchReboot(t *testing.T, r *runtime, message telegram.Message, now time.Time) {
	t.Helper()
	cmd, ok := r.authorizedCommand(&message)
	if !ok {
		t.Fatal("fixture command was not authorized")
	}
	r.handleCommand(cmd, nil, now)
}

func TestRebootRequiresOptIn(t *testing.T) {
	r := rebootFixture()
	r.settings.Telegram.AllowReboot = false
	now := time.Now()
	dispatchReboot(t, &r, rebootMessage(now, ""), now)
	if r.reboot.pending != nil || len(r.reboot.requests) != 0 || !strings.Contains(r.mailboxes[11].pending["reply"].text, "disabled") {
		t.Fatal("disabled reboot created a confirmation or request")
	}
}

func TestRebootConfirmationBoundaries(t *testing.T) {
	for _, name := range []string{"other user", "other chat", "wrong token", "expired", "old message", "future message", "missing date"} {
		t.Run(name, func(t *testing.T) {
			r := rebootFixture()
			now := time.Unix(1800000000, 0)
			dispatchReboot(t, &r, rebootMessage(now, ""), now)
			message := rebootMessage(now, r.reboot.pending.token)
			switch name {
			case "other user":
				message.From.ID = 23 // Also authorized, but not the requesting user.
			case "other chat":
				message.Chat.ID = 12 // Also authorized, but not the requesting chat.
			case "wrong token":
				message.Text = "/reboot confirm"
			case "expired":
				now = now.Add(time.Minute)
				message.Date = now.Unix()
			case "old message":
				message.Date = now.Add(-time.Minute).Unix()
			case "future message":
				message.Date = now.Add(6 * time.Second).Unix()
			case "missing date":
				message.Date = 0
			}
			dispatchReboot(t, &r, message, now)
			if len(r.reboot.requests) != 0 || r.reboot.requested {
				t.Fatal("invalid confirmation requested reboot")
			}
		})
	}
}

func TestRebootRejectsUnauthorizedSenders(t *testing.T) {
	for _, name := range []string{"user", "chat", "bot", "anonymous", "missing sender", "wrong bot"} {
		t.Run(name, func(t *testing.T) {
			r := rebootFixture()
			now := time.Now()
			dispatchReboot(t, &r, rebootMessage(now, ""), now)
			message := rebootMessage(now, r.reboot.pending.token)
			switch name {
			case "user":
				message.From.ID = 99
			case "chat":
				message.Chat.ID = 99
			case "bot":
				message.From.IsBot = true
			case "anonymous":
				message.SenderChat = &telegram.Chat{ID: 11}
			case "missing sender":
				message.From = nil
			case "wrong bot":
				message.Text = "/reboot@other_bot " + r.reboot.pending.token
			}
			if cmd, ok := r.authorizedCommand(&message); ok {
				r.handleCommand(cmd, nil, now)
				t.Fatal("unauthorized reboot confirmation accepted")
			}
			if len(r.reboot.requests) != 0 {
				t.Fatal("unauthorized message requested reboot")
			}
		})
	}
}

func TestRebootConfirmationIsSingleUse(t *testing.T) {
	r := rebootFixture()
	now := time.Now()
	dispatchReboot(t, &r, rebootMessage(now, ""), now)
	if len(r.reboot.requests) != 0 {
		t.Fatal("prompt requested reboot without confirmation")
	}
	token := r.reboot.pending.token
	if !strings.Contains(r.mailboxes[11].pending["reply"].text, "/reboot "+token) {
		t.Fatal("confirmation reply omitted the command")
	}
	message := rebootMessage(now, token)
	dispatchReboot(t, &r, message, now)
	if len(r.reboot.requests) != 1 || !r.reboot.requested || r.reboot.pending != nil {
		t.Fatal("valid confirmation did not submit exactly one request")
	}
	if chat := <-r.reboot.requests; chat != 11 {
		t.Fatalf("request chat = %d", chat)
	}
	dispatchReboot(t, &r, message, now)
	if len(r.reboot.requests) != 0 {
		t.Fatal("duplicate confirmation submitted another request")
	}
	logger := log.New(io.Discard, "", 0)
	r.finishReboot(rebootResult{chat: 11, err: errors.New("fixture failure")}, logger)
	dispatchReboot(t, &r, message, now)
	if len(r.reboot.requests) != 0 || r.reboot.requested {
		t.Fatal("failed request allowed reuse of the consumed token")
	}
	dispatchReboot(t, &r, rebootMessage(now, ""), now)
	if r.reboot.pending.token == token {
		t.Fatal("new prompt reused the token")
	}
	dispatchReboot(t, &r, rebootMessage(now, r.reboot.pending.token), now)
	<-r.reboot.requests
	r.finishReboot(rebootResult{chat: 11}, logger)
	dispatchReboot(t, &r, rebootMessage(now, ""), now)
	if !r.reboot.requested || r.reboot.pending != nil || len(r.reboot.requests) != 0 {
		t.Fatal("accepted reboot allowed another request before process exit")
	}
}

func TestRebootPromptReplacementAndRestartInvalidateTokens(t *testing.T) {
	r := rebootFixture()
	now := time.Now()
	dispatchReboot(t, &r, rebootMessage(now, ""), now)
	oldToken := r.reboot.pending.token
	dispatchReboot(t, &r, rebootMessage(now, ""), now)
	dispatchReboot(t, &r, rebootMessage(now, oldToken), now)
	if len(r.reboot.requests) != 0 {
		t.Fatal("replaced token requested reboot")
	}
	latestToken := r.reboot.pending.token
	r = rebootFixture() // A process restart deliberately loses confirmation state.
	dispatchReboot(t, &r, rebootMessage(now, latestToken), now)
	if len(r.reboot.requests) != 0 || r.reboot.pending != nil {
		t.Fatal("token from a previous process requested reboot")
	}
}

func TestRebootStalePromptAndBusyWorker(t *testing.T) {
	r := rebootFixture()
	now := time.Now()
	dispatchReboot(t, &r, rebootMessage(now.Add(-time.Minute), ""), now)
	if r.reboot.pending != nil {
		t.Fatal("stale prompt created a confirmation")
	}
	dispatchReboot(t, &r, rebootMessage(now, ""), now)
	message := rebootMessage(now, r.reboot.pending.token)
	r.reboot.requests <- 12
	dispatchReboot(t, &r, message, now)
	if r.reboot.pending != nil || r.reboot.requested || len(r.reboot.requests) != 1 {
		t.Fatal("busy worker did not decline and consume confirmation")
	}
	<-r.reboot.requests
	dispatchReboot(t, &r, message, now)
	if len(r.reboot.requests) != 0 {
		t.Fatal("busy worker left a reusable token")
	}
}

func TestRebootWorkerReportsResultWithDeadline(t *testing.T) {
	for _, failure := range []bool{false, true} {
		control := rebootControl{requests: make(chan int64, 1), results: make(chan rebootResult, 1)}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var wantErr error
		if failure {
			wantErr = errors.New("fixture permission failure")
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			control.run(ctx, func(ctx context.Context) error {
				if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 10*time.Second {
					t.Error("reboot request lacks bounded deadline")
				}
				return wantErr
			})
		}()
		control.requests <- 11
		select {
		case result := <-control.results:
			if result.chat != 11 || !errors.Is(result.err, wantErr) {
				t.Fatalf("reboot result = %+v", result)
			}
		case <-time.After(time.Second):
			t.Fatal("worker did not report result")
		}
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("worker did not stop")
		}
	}
}

func TestRebootWorkerCancelsActiveRequest(t *testing.T) {
	control := rebootControl{requests: make(chan int64, 1), results: make(chan rebootResult, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		control.run(ctx, func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	control.requests <- 11
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("active request ignored cancellation")
	}
}

func TestCanceledRebootWorkerDoesNotExecuteQueuedRequest(t *testing.T) {
	control := rebootControl{requests: make(chan int64, 1)}
	control.requests <- 11
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	control.run(ctx, func(context.Context) error {
		t.Fatal("canceled worker executed queued request")
		return nil
	})
}

func TestHostRebootCommandIsFixed(t *testing.T) {
	// Inspect only: this test must never start a real reboot command.
	cmd := hostRebootCommand(context.Background())
	want := []string{"/usr/bin/systemctl", "--no-ask-password", "--no-wall", "reboot"}
	if cmd.Path != want[0] || !reflect.DeepEqual(cmd.Args, want) || cmd.WaitDelay != time.Second {
		t.Fatalf("unexpected reboot command: path=%q args=%v wait=%v", cmd.Path, cmd.Args, cmd.WaitDelay)
	}
}

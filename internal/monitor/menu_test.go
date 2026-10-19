package monitor

import (
	"context"
	"errors"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/NikiTesla/pi-monitoring/internal/telegram"
)

type menuFixture struct {
	set func(context.Context, []telegram.BotCommand) error
}

func (f menuFixture) SetCommandMenu(ctx context.Context, commands []telegram.BotCommand) error {
	return f.set(ctx, commands)
}

func TestRegisterCommandMenu(t *testing.T) {
	var calls int
	client := menuFixture{set: func(ctx context.Context, commands []telegram.BotCommand) error {
		calls++
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 15*time.Second {
			t.Error("menu registration lacks a bounded deadline")
		}
		var names []string
		for _, cmd := range commands {
			names = append(names, cmd.Command)
			if cmd.Description == "" {
				t.Errorf("missing description for %s", cmd.Command)
			}
		}
		if strings.Join(names, ",") != "status,host,checks,check,incidents,reboot,help" {
			t.Errorf("menu commands = %v", names)
		}
		return nil
	}}
	registerCommandMenu(t.Context(), client, log.New(io.Discard, "", 0))
	if calls != 1 {
		t.Fatalf("successful registration repeated: %d calls", calls)
	}
}

func TestMenuRetryStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	called := make(chan struct{})
	client := menuFixture{set: func(context.Context, []telegram.BotCommand) error {
		close(called)
		return &telegram.RetryAfter{Delay: time.Hour}
	}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		registerCommandMenu(ctx, client, log.New(io.Discard, "", 0))
	}()
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("registration did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("menu registration did not stop on cancellation")
	}
}

func TestMenuRegistrationRetriesFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	calls := 0
	client := menuFixture{set: func(context.Context, []telegram.BotCommand) error {
		calls++
		if calls == 1 {
			return errors.New("fixture failure")
		}
		return nil
	}}
	var diagnostics strings.Builder
	registerCommandMenu(ctx, client, log.New(&diagnostics, "", 0))
	if calls != 2 || ctx.Err() != nil {
		t.Fatalf("registration did not recover: calls=%d, context=%v", calls, ctx.Err())
	}
	if !strings.Contains(diagnostics.String(), "retrying") || !strings.Contains(diagnostics.String(), "registered") {
		t.Fatalf("missing failure/recovery diagnostics: %s", diagnostics.String())
	}
}

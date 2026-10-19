package monitor

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NikiTesla/pi-monitoring/internal/config"
	"github.com/NikiTesla/pi-monitoring/internal/httpcheck"
	"github.com/NikiTesla/pi-monitoring/internal/telegram"
)

func TestHTTPIncidentStreaksAndUnknown(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	state := checkState{check: config.HTTPCheck{ID: "api", Interval: config.Duration(time.Minute), Timeout: config.Duration(time.Second), AlertPolicy: config.AlertPolicy{Failures: 3, RecoverySuccesses: 2}}}
	observe := func(success bool) string {
		now = now.Add(time.Second)
		return state.observe(httpcheck.Result{ID: "api", StartedAt: now, Success: success, Reason: "unexpected status"}, now, now.Add(-time.Hour))
	}
	if observe(false) != "" || observe(false) != "" {
		t.Fatal("opened too early")
	}
	state.missed()
	if observe(false) != "" || observe(false) != "" {
		t.Fatal("missed sample did not break failure streak")
	}
	if !strings.HasPrefix(observe(false), "🔴 Alert") || !state.incident {
		t.Fatal("third consecutive failure did not open")
	}
	if observe(true) != "" {
		t.Fatal("recovered too early")
	}
	state.missed()
	if observe(true) != "" {
		t.Fatal("unknown incorrectly counted toward recovery")
	}
	if !strings.HasPrefix(observe(true), "✅ Recovered") || state.incident {
		t.Fatal("two consecutive successes did not recover")
	}
	observe(false)
	observe(false)
	observe(false)
	observe(true)
	now = now.Add(10 * time.Minute)
	if observe(true) != "" || !state.incident {
		t.Fatal("observation gap falsely recovered incident")
	}
}

func TestStartupGraceAndStaleResult(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	state := checkState{check: config.HTTPCheck{ID: "api", Interval: config.Duration(time.Minute), Timeout: config.Duration(time.Second), AlertPolicy: config.AlertPolicy{Failures: 1, RecoverySuccesses: 1}}}
	result := httpcheck.Result{StartedAt: now}
	if state.observe(result, now, now.Add(time.Minute)) != "" {
		t.Fatal("alert during grace")
	}
	result.StartedAt = now.Add(2 * time.Minute)
	if state.observe(result, result.StartedAt, now.Add(time.Minute)) == "" {
		t.Fatal("failure after grace not alerted")
	}
	result.Success = true
	if state.observe(result, now.Add(10*time.Minute), now) != "" || !state.incident || !state.unknown {
		t.Fatal("stale success recovered incident")
	}
}

func TestCommandAuthorization(t *testing.T) {
	r := runtime{settings: config.Config{Telegram: config.Telegram{AllowedChatIDs: []int64{11}, AllowedUserIDs: []int64{22}}}, username: "test_bot"}
	message := telegram.Message{From: &telegram.User{ID: 22}, Chat: telegram.Chat{ID: 11}, Text: "/check@test_bot api"}
	if cmd, ok := r.authorizedCommand(&message); !ok || cmd.argument != "api" {
		t.Fatal("authorized addressed command rejected")
	}
	for _, mutate := range []func(*telegram.Message){
		func(m *telegram.Message) { m.From = nil },
		func(m *telegram.Message) { m.From = &telegram.User{ID: 22, IsBot: true} },
		func(m *telegram.Message) { m.From = &telegram.User{ID: 99} },
		func(m *telegram.Message) { m.Chat.ID = 99 },
		func(m *telegram.Message) { m.Text = "/status@other_bot" },
		func(m *telegram.Message) { m.SenderChat = &telegram.Chat{ID: 11} },
	} {
		copy := message
		mutate(&copy)
		if _, ok := r.authorizedCommand(&copy); ok {
			t.Fatalf("unauthorized message accepted: %+v", copy)
		}
	}
}

func TestOnDemandCheckRejectsOverlapAndSaturation(t *testing.T) {
	r := runtime{checks: []*checkState{{check: config.HTTPCheck{ID: "api", Interval: config.Duration(time.Minute)}}}, mailboxes: map[int64]*mailbox{11: newMailbox(11)}}
	jobs := make(chan config.HTTPCheck, 1)
	r.handleCommand(command{chat: 11, name: "check", argument: "api"}, jobs, time.Now())
	if len(jobs) != 1 || !r.checks[0].running {
		t.Fatal("on-demand check not submitted")
	}
	r.handleCommand(command{chat: 11, name: "check", argument: "api"}, jobs, time.Now())
	if len(jobs) != 1 {
		t.Fatal("overlapping check submitted")
	}
	r.checks[0].running = false
	r.handleCommand(command{chat: 11, name: "check", argument: "api"}, jobs, time.Now())
	if r.checks[0].running {
		t.Fatal("saturated queue marked check running")
	}
}

type recordingSender struct{ sent chan string }

func (s recordingSender) Send(ctx context.Context, _ int64, text string) error {
	select {
	case s.sent <- text:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestDeliveryCoalescesLatestStateAndStops(t *testing.T) {
	mail := newMailbox(11)
	mail.put("api", "failed")
	mail.put("api", "recovered")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	client := recordingSender{sent: make(chan string, 2)}
	done := make(chan struct{})
	go func() { defer close(done); mail.deliver(ctx, client, log.New(io.Discard, "", 0)) }()
	select {
	case text := <-client.sent:
		if text != "recovered" {
			t.Fatalf("sent stale state %q", text)
		}
	case <-time.After(time.Second):
		t.Fatal("delivery did not progress")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("delivery did not stop")
	}
}

func TestRunRejectsUnsupportedComponentsBeforeNetwork(t *testing.T) {
	settings := config.Defaults()
	settings.Docker.Enabled = true
	if err := Run(t.Context(), settings, io.Discard); err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("got %v", err)
	}
}

func TestRunSchedulesRepeatedChecksAndJoinsOnCancellation(t *testing.T) {
	var active, maximum atomic.Int32
	requests := make(chan struct{}, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := active.Add(1)
		defer active.Add(-1)
		for old := maximum.Load(); count > old; old = maximum.Load() {
			if maximum.CompareAndSwap(old, count) {
				break
			}
		}
		requests <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	settings := config.Defaults()
	settings.Host.Enabled = false
	settings.Dashboard.Enabled = false
	settings.HTTPChecks = []config.HTTPCheck{{ID: "api", URL: server.URL, Method: "GET", Interval: config.Duration(150 * time.Millisecond), Timeout: config.Duration(100 * time.Millisecond), MaxBodyBytes: 1024, Expectations: config.Expectations{Status: 200}, AlertPolicy: config.AlertPolicy{Failures: 1, RecoverySuccesses: 1}}}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, settings, io.Discard) }()
	for range 2 {
		select {
		case <-requests:
		case err := <-done:
			t.Fatalf("run ended early: %v", err)
		case <-time.After(3 * time.Second):
			t.Fatal("scheduled check did not run")
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime did not join workers")
	}
	if maximum.Load() != 1 {
		t.Fatalf("overlapping requests: max=%d", maximum.Load())
	}
}

func TestPollingRetriesAndSkipsDuplicateUpdates(t *testing.T) {
	if got := retryDelay(time.Second, &telegram.RetryAfter{Delay: time.Minute}); got != time.Minute {
		t.Fatalf("retry delay = %s", got)
	}
	r := runtime{settings: config.Config{Telegram: config.Telegram{AllowedChatIDs: []int64{11}, AllowedUserIDs: []int64{22}}}}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	client := &fakePoller{cancel: cancel}
	commands := make(chan command, 4)
	r.poll(ctx, client, commands, log.New(io.Discard, "", 0))
	if len(commands) != 1 {
		t.Fatalf("duplicate command accepted: %d", len(commands))
	}
	if client.offset != 2 {
		t.Fatalf("next offset = %d", client.offset)
	}
}

type fakePoller struct {
	calls  int
	offset int64
	cancel context.CancelFunc
}

func (p *fakePoller) GetMe(context.Context) (telegram.User, error) {
	return telegram.User{ID: 1, IsBot: true, Username: "test_bot"}, nil
}
func (p *fakePoller) Updates(_ context.Context, offset int64) ([]telegram.Update, error) {
	p.calls++
	if p.calls > 1 {
		p.offset = offset
		p.cancel()
		return nil, context.Canceled
	}
	message := &telegram.Message{From: &telegram.User{ID: 22}, Chat: telegram.Chat{ID: 11}, Text: "/status"}
	return []telegram.Update{{ID: 1, Message: message}, {ID: 1, Message: message}}, nil
}

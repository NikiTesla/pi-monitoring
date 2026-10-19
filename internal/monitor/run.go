package monitor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/NikiTesla/pi-monitoring/internal/config"
	"github.com/NikiTesla/pi-monitoring/internal/host"
	"github.com/NikiTesla/pi-monitoring/internal/httpcheck"
	"github.com/NikiTesla/pi-monitoring/internal/telegram"
)

const maxWorkers = 4

type command struct {
	chat           int64
	user           int64
	sentAt         time.Time
	name, argument string
}

type runtime struct {
	settings      config.Config
	checks        []*checkState
	mailboxes     map[int64]*mailbox
	hostSnapshot  host.Snapshot
	hostIncidents map[string]host.Transition
	username      string
	results       chan httpcheck.Result
	reboot        rebootControl
}

// Run requires validated configuration and stops all workers before returning.
// Operational state and pending notifications are intentionally memory-only.
func Run(ctx context.Context, settings config.Config, diagnostics io.Writer) error {
	if err := config.Validate(settings); err != nil {
		return err
	}
	if settings.Docker.Enabled || len(settings.Heartbeats) != 0 {
		return errors.New("Docker and heartbeat runtime monitoring are not implemented")
	}
	for _, check := range settings.HTTPChecks {
		for _, header := range check.Headers {
			if header.EnvVar != "" && os.Getenv(header.EnvVar) == "" {
				return fmt.Errorf("HTTP check %s: required header environment variable is missing", check.ID)
			}
			if header.EnvVar != "" {
				for _, b := range []byte(os.Getenv(header.EnvVar)) {
					if (b < 0x20 && b != '\t') || b == 0x7f {
						return fmt.Errorf("HTTP check %s: header environment value is invalid", check.ID)
					}
				}
			}
		}
	}
	if diagnostics == nil {
		diagnostics = io.Discard
	}
	logger := log.New(diagnostics, "", log.LstdFlags)
	if settings.Dashboard.Enabled {
		logger.Print("Dashboard is not implemented; no HTTP listener will be started")
	}
	logger.Print("Monitoring state and pending alerts are memory-only and reset on restart")
	r := &runtime{settings: settings, mailboxes: make(map[int64]*mailbox), hostIncidents: make(map[string]host.Transition), results: make(chan httpcheck.Result, maxWorkers)}
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	commands := make(chan command, 32)
	if settings.Telegram.Enabled {
		token := os.Getenv(settings.Telegram.TokenEnvVar)
		if !validToken(token) {
			return errors.New("Telegram token is missing or malformed; set the variable named by telegram.token_env in the environment or in secrets.env beside the configuration")
		}
		client := telegram.New(token)
		workers.Add(1)
		go func() { defer workers.Done(); registerCommandMenu(ctx, client, logger) }()
		for _, chat := range slices.Concat(settings.Telegram.AlertChatIDs, settings.Telegram.AllowedChatIDs) {
			if _, exists := r.mailboxes[chat]; exists {
				continue
			}
			mail := newMailbox(chat)
			r.mailboxes[chat] = mail
			workers.Add(1)
			go func() { defer workers.Done(); mail.deliver(ctx, client, logger) }()
		}
		workers.Add(1)
		go func() { defer workers.Done(); r.poll(ctx, client, commands, logger) }()
		if settings.Telegram.AllowReboot {
			r.reboot.requests = make(chan int64, 1)
			r.reboot.results = make(chan rebootResult, 1)
			workers.Add(1)
			go func() {
				defer workers.Done()
				r.reboot.run(ctx, requestHostReboot)
			}()
		}
	}
	jobs := make(chan config.HTTPCheck)
	for range maxWorkers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case check := <-jobs:
					result := httpcheck.Run(ctx, check)
					select {
					case r.results <- result:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}
	now := time.Now()
	graceEnd := now.Add(settings.Alerts.StartupGrace.Value())
	for i, check := range settings.HTTPChecks {
		r.checks = append(r.checks, &checkState{check: check, next: now.Add(time.Duration(i%10) * 50 * time.Millisecond)})
	}
	hostResults := make(chan host.Snapshot, 1)
	if settings.Host.Enabled {
		workers.Add(1)
		go func() {
			defer workers.Done()
			collector := host.New(settings.Host.Mounts)
			ticker := time.NewTicker(settings.Host.Interval.Value())
			defer ticker.Stop()
			for {
				select {
				case hostResults <- collector.Collect():
				case <-ctx.Done():
					return
				}
				select {
				case <-ticker.C:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	rules := host.NewRules(settings.HostRules)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	nextRepeat := now.Add(settings.Alerts.CriticalRepeatInterval.Value())
	scheduleStart := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		case result := <-r.results:
			for _, state := range r.checks {
				if state.check.ID != result.ID {
					continue
				}
				if alert := state.observe(result, time.Now(), graceEnd); alert != "" {
					r.alert("http:"+result.ID, alert)
				}
				if state.waitingChat != 0 {
					r.reply(state.waitingChat, state.summary(time.Now()))
					state.waitingChat = 0
				}
				break
			}
		case snapshot := <-hostResults:
			freshness := 2 * settings.Host.Interval.Value()
			if (!r.hostSnapshot.ObservedAt.IsZero() && snapshot.ObservedAt.Sub(r.hostSnapshot.ObservedAt) > freshness) || time.Since(snapshot.ObservedAt) > freshness {
				rules.Evaluate(host.Snapshot{})
			}
			r.hostSnapshot = snapshot
			if snapshot.ObservedAt.Before(graceEnd) || time.Since(snapshot.ObservedAt) > freshness {
				continue
			}
			for _, transition := range rules.Evaluate(snapshot) {
				title := "✅ Recovered · Host"
				if transition.Open {
					r.hostIncidents[transition.ID] = transition
					title = "🔴 Alert · Host"
				} else {
					delete(r.hostIncidents, transition.ID)
				}
				r.alert("host:"+transition.ID, title+"\n"+hostIncidentDetails(transition))
			}
		case cmd := <-commands:
			r.handleCommand(cmd, jobs, time.Now())
		case result := <-r.reboot.results:
			r.finishReboot(result, logger)
		case now := <-ticker.C:
			for i := range r.checks {
				state := r.checks[(scheduleStart+i)%len(r.checks)]
				if now.Before(state.next) {
					continue
				}
				state.next = now.Add(state.check.Interval.Value())
				if state.running {
					state.missed()
					continue
				}
				select {
				case jobs <- state.check:
					state.running = true
				default:
					state.missed()
				}
			}
			if len(r.checks) > 0 {
				scheduleStart = (scheduleStart + 1) % len(r.checks)
			}
			if settings.Alerts.CriticalRepeatInterval.Value() > 0 && !now.Before(nextRepeat) {
				for _, state := range r.checks {
					if state.incident {
						r.alert("http:"+state.check.ID, "🔴 Ongoing · HTTP\n"+state.summary(now)+"\nOpened: "+observationTime(state.opened))
					}
				}
				for id, incident := range r.hostIncidents {
					r.alert("host:"+id, "🔴 Ongoing · Host\n"+hostIncidentDetails(incident))
				}
				nextRepeat = now.Add(settings.Alerts.CriticalRepeatInterval.Value())
			}
		}
	}
}

func validToken(token string) bool {
	if len(token) > 256 {
		return false
	}
	parts := strings.Split(token, ":")
	if len(parts) != 2 || len(parts[0]) == 0 || len(parts[1]) == 0 {
		return false
	}
	for _, c := range parts[0] {
		if c < '0' || c > '9' {
			return false
		}
	}
	for _, c := range parts[1] {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func (r *runtime) alert(key, message string) {
	for _, chat := range r.settings.Telegram.AlertChatIDs {
		if mail := r.mailboxes[chat]; mail != nil {
			mail.put(key, "🖥 "+r.settings.Server.Name+"\n\n"+message)
		}
	}
}

func (r *runtime) reply(chat int64, message string) {
	if mail := r.mailboxes[chat]; mail != nil {
		mail.put("reply", message)
	}
}

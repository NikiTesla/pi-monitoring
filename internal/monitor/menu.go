package monitor

import (
	"context"
	"log"
	"time"

	"pi-monitoring/internal/telegram"
)

type commandMenuSetter interface {
	SetCommandMenu(context.Context, []telegram.BotCommand) error
}

// Menu registration has its own lifetime so a Bot API failure cannot delay
// collection, delivery or command polling. The runtime joins it during shutdown.
func registerCommandMenu(ctx context.Context, client commandMenuSetter, logger *log.Logger) {
	commands := []telegram.BotCommand{
		{Command: "status", Description: "Server overview"},
		{Command: "host", Description: "Latest host metrics"},
		{Command: "checks", Description: "HTTP check results and IDs"},
		{Command: "check", Description: "Run one HTTP check: /check <id>"},
		{Command: "incidents", Description: "Active problems"},
		{Command: "reboot", Description: "Reboot server (opt-in; confirmation required)"},
		{Command: "help", Description: "Show command help"},
	}
	backoff := time.Second
	for ctx.Err() == nil {
		menuCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := client.SetCommandMenu(menuCtx, commands)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			logger.Print("Telegram command menu registered")
			return
		}
		if backoff == time.Second {
			logger.Print("Telegram command menu unavailable; monitoring continues while retrying")
		}
		if !waitRetry(ctx, retryDelay(backoff, err)) {
			return
		}
		backoff = min(30*time.Second, backoff*2)
	}
}

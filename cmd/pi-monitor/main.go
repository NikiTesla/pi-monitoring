package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/NikiTesla/pi-monitoring/internal/config"
	"github.com/NikiTesla/pi-monitoring/internal/httpcheck"
	"github.com/NikiTesla/pi-monitoring/internal/monitor"
	"github.com/NikiTesla/pi-monitoring/internal/setup"
)

var version = "dev"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, out, diagnostics io.Writer) int {
	if len(args) == 0 {
		usage(diagnostics)
		return 2
	}
	switch args[0] {
	case "help", "-h", "--help":
		usage(out)
		return 0
	case "version":
		if len(args) != 1 {
			fmt.Fprintln(diagnostics, "version accepts no arguments")
			return 2
		}
		fmt.Fprintln(out, "pi-monitor", version)
		return 0
	case "init":
		return runInit(args[1:], out, diagnostics)
	case "validate":
		return runValidate(args[1:], out, diagnostics)
	case "check":
		return runCheck(ctx, args[1:], out, diagnostics)
	case "run":
		return runMonitor(ctx, args[1:], diagnostics)
	default:
		fmt.Fprintf(diagnostics, "Unknown command %q. Use pi-monitor help.\n", args[0])
		return 2
	}
}

func runMonitor(ctx context.Context, args []string, diagnostics io.Writer) int {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	path := flags.String("config", "/etc/pi-monitor/config.yaml", "YAML configuration path")
	if err := flags.Parse(args); err != nil {
		return flagExitCode(err)
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(diagnostics, "run accepts flags only")
		return 2
	}
	settings, err := config.Load(*path)
	if err != nil {
		fmt.Fprintln(diagnostics, "run:", err)
		return 2
	}
	var credentialNames []string
	if settings.Telegram.Enabled {
		credentialNames = append(credentialNames, settings.Telegram.TokenEnvVar)
	}
	for _, check := range settings.HTTPChecks {
		credentialNames = append(credentialNames, headerEnvironmentNames(check)...)
	}
	if err := loadSecrets(*path, credentialNames); err != nil {
		fmt.Fprintln(diagnostics, "run:", err)
		return 2
	}
	if err := monitor.Run(ctx, settings, diagnostics); err != nil {
		fmt.Fprintln(diagnostics, "run:", err)
		return 1
	}
	return 0
}

func runCheck(ctx context.Context, args []string, out, diagnostics io.Writer) int {
	// Accept both `check ID --config PATH` and `check --config PATH ID`.
	var checkID string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		checkID, args = args[0], args[1:]
	}
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	path := flags.String("config", "/etc/pi-monitor/config.yaml", "YAML configuration path")
	if err := flags.Parse(args); err != nil {
		return flagExitCode(err)
	}
	if checkID == "" && flags.NArg() == 1 {
		checkID = flags.Arg(0)
	} else if flags.NArg() != 0 {
		fmt.Fprintln(diagnostics, "check requires exactly one check ID")
		return 2
	}
	if checkID == "" {
		fmt.Fprintln(diagnostics, "check requires an HTTP check ID")
		return 2
	}
	settings, err := config.Load(*path)
	if err != nil {
		fmt.Fprintln(diagnostics, "check:", err)
		return 2
	}
	for _, check := range settings.HTTPChecks {
		if check.ID != checkID {
			continue
		}
		if err := loadSecrets(*path, headerEnvironmentNames(check)); err != nil {
			fmt.Fprintln(diagnostics, "check:", err)
			return 2
		}
		result := httpcheck.Run(ctx, check)
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			fmt.Fprintln(diagnostics, "check: could not write result")
			return 1
		}
		if !result.Success {
			if result.Category == "configuration" {
				return 2
			}
			return 1
		}
		return 0
	}
	fmt.Fprintf(diagnostics, "HTTP check %q is not configured\n", checkID)
	return 2
}

func headerEnvironmentNames(check config.HTTPCheck) []string {
	var names []string
	for _, header := range check.Headers {
		if header.EnvVar != "" {
			names = append(names, header.EnvVar)
		}
	}
	return names
}

func flagExitCode(err error) int {
	if err == flag.ErrHelp {
		return 0
	}
	return 2
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `pi-monitor — personal server monitoring

Commands:
  init [--output PATH] [--docker] [--no-dashboard] [--dry-run]
  init --install --dry-run [--docker] [--no-dashboard]
  validate --config PATH
  check ID --config PATH
  run --config PATH
  version

Exit codes: 0 success, 1 failed check or operation, 2 invalid command/configuration.

Run supports scheduled HTTP checks, Linux host metrics and Telegram alerts/commands.
Run and check load private secrets.env beside the configuration; process environment
variables take precedence. Validate does not read secrets or contact endpoints.
Dashboard, Docker, heartbeats, persistent history and systemd installation remain
unavailable. See docs/usage.md for runtime limitations.`)
}

func runInit(args []string, out, diagnostics io.Writer) int {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	var options setup.InitOptions
	flags.StringVar(&options.OutputPath, "output", "", "configuration output path (default: ./config.yaml)")
	flags.BoolVar(&options.Install, "install", false, "plan systemd installation; real installation is not implemented")
	flags.BoolVar(&options.Start, "start", false, "start after installation (not available in this milestone)")
	flags.BoolVar(&options.DryRun, "dry-run", false, "print proposed actions without changing files")
	flags.BoolVar(&options.EnableDocker, "docker", false, "enable Docker monitoring in generated configuration")
	flags.BoolVar(&options.DisableDashboard, "no-dashboard", false, "disable the dashboard in generated configuration")
	if err := flags.Parse(args); err != nil {
		return flagExitCode(err)
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(diagnostics, "init accepts flags only")
		return 2
	}
	if options.Start && !options.Install {
		fmt.Fprintln(diagnostics, "init: --start requires --install")
		return 2
	}
	if options.Install && options.OutputPath != "" {
		fmt.Fprintln(diagnostics, "init: --output cannot be combined with --install")
		return 2
	}
	if err := setup.Init(options, out); err != nil {
		fmt.Fprintln(diagnostics, "init:", err)
		return 1
	}
	return 0
}

func runValidate(args []string, out, diagnostics io.Writer) int {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	path := flags.String("config", "/etc/pi-monitor/config.yaml", "YAML configuration path")
	if err := flags.Parse(args); err != nil {
		return flagExitCode(err)
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(diagnostics, "validate accepts flags only")
		return 2
	}
	if _, err := config.Load(*path); err != nil {
		fmt.Fprintln(diagnostics, "validate:", err)
		return 2
	}
	fmt.Fprintln(out, "Configuration is valid. No network requests were made; runtime features are not started.")
	return 0
}

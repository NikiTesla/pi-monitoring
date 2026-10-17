// Package setup creates initial configuration without activating monitoring.
package setup

import (
	"fmt"
	"io"
	"os"

	"pi-monitoring/internal/config"
)

type InitOptions struct {
	OutputPath       string
	Install          bool
	Start            bool
	DryRun           bool
	EnableDocker     bool
	DisableDashboard bool
}

// Init generates a configuration or describes future system installation.
// Service installation remains gated until its deployment path is implemented.
func Init(options InitOptions, out io.Writer) error {
	if options.Start && !options.Install {
		return fmt.Errorf("--start requires --install")
	}
	if options.Install && options.OutputPath != "" {
		return fmt.Errorf("--output cannot be combined with --install")
	}
	if options.Install && !options.DryRun {
		return fmt.Errorf("system installation is not available: service installation is not implemented; use --install --dry-run to inspect the future plan")
	}
	data, err := config.Generate(config.GenerationOptions{
		DockerEnabled:    options.EnableDocker,
		DashboardEnabled: !options.DisableDashboard,
		SystemPaths:      options.Install,
	})
	if err != nil {
		return fmt.Errorf("generate configuration: %w", err)
	}
	if options.Install {
		return installationPlan(options, out)
	}
	path := options.OutputPath
	if path == "" {
		path = "config.yaml"
	}
	if options.DryRun {
		_, err := fmt.Fprintf(out, "Would exclusively create %s (mode 0600); existing files are preserved.\nDocker enabled: %t; dashboard enabled: %t.\nNo files or services changed. Some generated settings describe future runtime features.\n", path, options.EnableDocker, !options.DisableDashboard)
		return err
	}
	// O_EXCL rejects existing files, including symlinks, instead of following or
	// truncating them. Parent directories must already exist.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create configuration %q (existing files are preserved): %w", path, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(path)
		return fmt.Errorf("write configuration %q: %w", path, err)
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return fmt.Errorf("close configuration %q: %w", path, err)
	}
	_, err = fmt.Fprintf(out, "Created %s (mode 0600). No service installed or started.\nUse validate, check or run; see docs/usage.md for supported runtime features.\n", path)
	return err
}

func installationPlan(options InitOptions, out io.Writer) error {
	_, err := fmt.Fprintf(out, `Future installation plan only: service installation is not implemented.
No files, permissions, users, or services will change.

Prerequisites before future installation: root, Linux/systemd, a daemon-capable binary,
writable deployment paths, and no conflicting configuration, secrets, binary or unit.
Create dedicated service identity: pi-monitor.
Install binary: /usr/local/bin/pi-monitor (root-owned, mode 0755).
Create /etc/pi-monitor (mode 0750); configuration mode 0640, root:pi-monitor.
Create /etc/pi-monitor/config.yaml; preserve any existing file.
Create /etc/pi-monitor/secrets.env (root-owned, mode 0600); no credentials generated.
Create /var/lib/pi-monitor (pi-monitor-owned, mode 0750) for bounded history.
Create /etc/systemd/system/pi-monitor.service (root-owned, mode 0644).
The future unit must use the stable binary and supported daemon command.
Only after service installation support and preflight checks: reload systemd and enable the unit.
Requested immediate start after future installation: %t.
Docker enabled: %t; dashboard enabled: %t (loopback when enabled).
`, options.Start, options.EnableDocker, !options.DisableDashboard)
	if err != nil {
		return err
	}
	if options.EnableDocker {
		_, err = fmt.Fprintln(out, "Docker prerequisite: an existing /var/run/docker.sock socket and explicitly granted service access.\nWARNING: Docker socket access grants substantial host privileges; it is not read-only authorization.")
		if err != nil {
			return err
		}
		info, statErr := os.Stat("/var/run/docker.sock")
		switch {
		case statErr != nil:
			_, err = fmt.Fprintln(out, "Docker prerequisite is unavailable on this host; this plan is not runnable. No Docker installation or permission change attempted.")
		case info.Mode()&os.ModeSocket == 0:
			_, err = fmt.Fprintln(out, "Docker prerequisite is not a socket; this plan is not runnable.")
		default:
			_, err = fmt.Fprintln(out, "Docker socket exists; daemon availability and future service permissions remain unverified.")
		}
	}
	return err
}

.DEFAULT_GOAL := help

GO ?= go
CONFIG ?= ./config.yaml
CHECK_ID ?=

.PHONY: help build init validate start run check version test vet race fmt install-service start-service install-reboot-policy

help:
	@printf '%s\n' \
	  'make build                  Build bin/pi-monitor' \
	  'make init                   Generate CONFIG without overwriting existing files' \
	  'make validate               Validate CONFIG without network requests' \
	  'make start (or make run)    Run monitoring in the foreground; Ctrl+C stops it' \
	  'make check CHECK_ID=ID       Run one HTTP check (may contact its endpoint)' \
	  'make version                Print the binary version' \
	  'make install-service        Install systemd service and enable startup at boot (uses sudo)' \
	  'make start-service          Start the installed service now (uses sudo)' \
	  'make install-reboot-policy  Allow pi-monitor to request host reboots (optional; uses sudo)' \
	  'make test                   Run tests with local fixtures' \
	  'make vet                    Run go vet' \
	  'make race                   Run tests with the race detector' \
	  'make fmt                    Format Go source' \
	  'Override CONFIG=./config.yaml or GO=go as needed.' \
	  'Application targets build first. Source review is required before verification.'

build:
	$(GO) build -trimpath -o bin/pi-monitor ./cmd/pi-monitor

init: build
	./bin/pi-monitor init --output "$(CONFIG)"

validate: build
	./bin/pi-monitor validate --config "$(CONFIG)"

start run: build
	./bin/pi-monitor run --config "$(CONFIG)"

check: build
	@test -n "$(CHECK_ID)" || { printf '%s\n' 'Usage: make check CHECK_ID=example-api [CONFIG=./config.yaml]' >&2; exit 2; }
	./bin/pi-monitor check --config "$(CONFIG)" "$(CHECK_ID)"

version: build
	./bin/pi-monitor version

# Keep deployment separate from starting monitoring, which can contact Telegram.
install-service: build
	@test -d /run/systemd/system || { printf '%s\n' 'A Linux host running systemd is required.' >&2; exit 1; }
	sudo test ! -L /etc/pi-monitor
	sudo test ! -L /etc/pi-monitor/config.yaml
	sudo test ! -L /etc/pi-monitor/secrets.env
	sudo test ! -L /etc/systemd/system/pi-monitor.service
	@if sudo test -e /etc/pi-monitor/config.yaml; then sudo test -f /etc/pi-monitor/config.yaml; fi
	@if sudo test -e /etc/pi-monitor/secrets.env; then sudo test -f /etc/pi-monitor/secrets.env; fi
	@if sudo test -e /etc/systemd/system/pi-monitor.service; then sudo test -f /etc/systemd/system/pi-monitor.service; fi
	@if ! sudo test -f /etc/pi-monitor/config.yaml; then ./bin/pi-monitor validate --config "$(CONFIG)"; fi
	@if ! id -u pi-monitor >/dev/null 2>&1; then sudo useradd --system --user-group --no-create-home --shell /usr/sbin/nologin pi-monitor; fi
	sudo install -d -o root -g pi-monitor -m 0750 /etc/pi-monitor
	@if ! sudo test -e /etc/pi-monitor/config.yaml; then sudo install -o root -g pi-monitor -m 0640 "$(CONFIG)" /etc/pi-monitor/config.yaml; fi
	sudo chown root:pi-monitor /etc/pi-monitor/config.yaml
	sudo chmod 0640 /etc/pi-monitor/config.yaml
	@secrets_path=$$(dirname "$(CONFIG)")/secrets.env; \
	if ! sudo test -e /etc/pi-monitor/secrets.env && test -e "$$secrets_path"; then \
	  test -f "$$secrets_path" && test ! -L "$$secrets_path" && \
	  sudo install -o pi-monitor -g pi-monitor -m 0600 "$$secrets_path" /etc/pi-monitor/secrets.env; \
	fi
	@if sudo test -e /etc/pi-monitor/secrets.env; then sudo chown pi-monitor:pi-monitor /etc/pi-monitor/secrets.env && sudo chmod 0600 /etc/pi-monitor/secrets.env; fi
	sudo install -o root -g root -m 0755 bin/pi-monitor /usr/local/bin/pi-monitor
	sudo -u pi-monitor /usr/local/bin/pi-monitor validate --config /etc/pi-monitor/config.yaml
	sudo install -o root -g root -m 0644 deploy/pi-monitor.service /etc/systemd/system/pi-monitor.service
	sudo systemctl daemon-reload
	sudo systemctl enable pi-monitor.service
	@printf '%s\n' 'Service installed and enabled for boot. Run make start-service to start monitoring now.'

start-service:
	sudo systemctl start pi-monitor.service

# Explicit opt-in; the ordinary service installation grants no reboot policy.
install-reboot-policy:
	@test -d /run/systemd/system && test -x /usr/bin/systemctl || { printf '%s\n' 'A Linux host running systemd is required.' >&2; exit 1; }
	@id -u pi-monitor >/dev/null 2>&1 || { printf '%s\n' 'Install the pi-monitor service account first.' >&2; exit 1; }
	sudo test -d /etc/polkit-1/rules.d
	sudo test ! -L /etc/polkit-1/rules.d
	sudo test ! -L /etc/polkit-1/rules.d/50-pi-monitor-reboot.rules
	@if sudo test -e /etc/polkit-1/rules.d/50-pi-monitor-reboot.rules; then sudo test -f /etc/polkit-1/rules.d/50-pi-monitor-reboot.rules; fi
	sudo install -o root -g root -m 0644 deploy/50-pi-monitor-reboot.rules /etc/polkit-1/rules.d/50-pi-monitor-reboot.rules
	@printf '%s\n' 'Reboot policy installed. Enable telegram.allow_reboot in the deployed config and restart monitoring to use /reboot.'

test:
	$(GO) test -timeout 30s ./...

vet:
	$(GO) vet ./...

race:
	$(GO) test -race -timeout 30s ./...

fmt:
	gofmt -w cmd internal

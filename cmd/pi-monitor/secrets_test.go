package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func unsetCredential(t *testing.T, name string) {
	t.Helper()
	// Register restoration even when the variable was initially absent.
	t.Setenv(name, "")
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
}

func writeSecrets(t *testing.T, dir, contents string) string {
	t.Helper()
	path := filepath.Join(dir, "secrets.env")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSecretsParserLiteralAssignments(t *testing.T) {
	values, err := parseSecrets([]byte(`# comment
TOKEN=123:fixture-token
export AUTH = 'Bearer fixture-value'
DOUBLE="value=with#punctuation"
EMPTY=
LITERAL='$(touch /should-not-exist) ${HOME} ` + "`command`" + `'
`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"TOKEN": "123:fixture-token", "AUTH": "Bearer fixture-value",
		"DOUBLE": "value=with#punctuation", "EMPTY": "",
		"LITERAL": "$(touch /should-not-exist) ${HOME} `command`",
	}
	if len(values) != len(want) {
		t.Fatalf("got %d assignments, want %d", len(values), len(want))
	}
	for name, value := range want {
		if values[name] != value {
			t.Fatalf("literal value changed for %s", name)
		}
	}
	values, err = parseSecrets([]byte("# Windows line endings\r\nTOKEN='fixture'\r\n"))
	if err != nil || values["TOKEN"] != "fixture" {
		t.Fatalf("CRLF assignments failed: %v", err)
	}
}

func TestSecretsParserRejectsInvalidAssignmentsWithoutEchoingThem(t *testing.T) {
	for name, contents := range map[string]string{
		"missing equals": "private-fixture-value",
		"invalid name":   "private-fixture-value=secret",
		"duplicate":      "TOKEN=private-fixture-value\nTOKEN=other",
		"open quote":     "TOKEN='private-fixture-value",
		"trailing text":  "TOKEN='private-fixture-value' extra",
		"extra quote":    "TOKEN='private-fixture-value' 'other'",
		"unquoted space": "TOKEN=private-fixture-value extra",
		"NUL":            "TOKEN=private-fixture-value\x00",
		"control":        "TOKEN='private-fixture-value\t'",
		"invalid UTF8":   "TOKEN=private-fixture-value\xff",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseSecrets([]byte(contents))
			if err == nil || strings.Contains(err.Error(), "private-fixture-value") {
				t.Fatalf("expected redacted syntax error, got %v", err)
			}
		})
	}
}

func TestLoadSecretsUsesConfigDirectoryAndOnlyReferencedVariables(t *testing.T) {
	dir := t.TempDir()
	const tokenName = "PI_MONITOR_TEST_FILE_TOKEN"
	const unusedName = "PI_MONITOR_TEST_UNUSED_TOKEN"
	unsetCredential(t, tokenName)
	unsetCredential(t, unusedName)
	writeSecrets(t, dir, tokenName+"='123:fixture-token'\n"+unusedName+"=unused\n")
	// A conflicting file in the working directory must never be considered.
	t.Chdir(t.TempDir())
	writeSecrets(t, ".", tokenName+"=wrong-directory\n")
	if err := loadSecrets(filepath.Join(dir, "config.yaml"), []string{tokenName}); err != nil {
		t.Fatal(err)
	}
	if os.Getenv(tokenName) != "123:fixture-token" {
		t.Fatal("token was not loaded from the configuration directory")
	}
	if _, exists := os.LookupEnv(unusedName); exists {
		t.Fatal("unreferenced variable was imported")
	}
}

func TestLoadSecretsPreservesExplicitEnvironmentIncludingEmpty(t *testing.T) {
	const name = "PI_MONITOR_TEST_OVERRIDE_TOKEN"
	for _, value := range []string{"environment-value", ""} {
		t.Run(fmt.Sprintf("empty=%t", value == ""), func(t *testing.T) {
			t.Setenv(name, value)
			dir := t.TempDir()
			writeSecrets(t, dir, name+"=file-value\n")
			if err := loadSecrets(filepath.Join(dir, "config.yaml"), []string{name}); err != nil {
				t.Fatal(err)
			}
			if got, exists := os.LookupEnv(name); !exists || got != value {
				t.Fatal("explicit process environment was replaced")
			}
		})
	}
}

func TestLoadSecretsOptionalFileAndRestart(t *testing.T) {
	const name = "PI_MONITOR_TEST_RESTART_TOKEN"
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	unsetCredential(t, name)
	if err := loadSecrets(configPath, []string{name}); err != nil {
		t.Fatalf("absent optional secrets file: %v", err)
	}
	writeSecrets(t, dir, name+"=persistent-value\n")
	for range 2 {
		unsetCredential(t, name) // Each new process starts without the variable.
		if err := loadSecrets(configPath, []string{name}); err != nil {
			t.Fatal(err)
		}
		if os.Getenv(name) != "persistent-value" {
			t.Fatal("persistent credential was not loaded")
		}
	}
}

func TestLoadSecretsRejectsUnsafeFilesAndParsesBeforeImport(t *testing.T) {
	const name = "PI_MONITOR_TEST_ATOMIC_TOKEN"
	for _, kind := range []string{"directory", "symlink", "dangling symlink", "public", "oversized", "syntax"} {
		t.Run(kind, func(t *testing.T) {
			unsetCredential(t, name)
			dir := t.TempDir()
			path := filepath.Join(dir, "secrets.env")
			switch kind {
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink", "dangling symlink":
				target := filepath.Join(dir, "target.env")
				if kind == "symlink" {
					if err := os.WriteFile(target, []byte(name+"=private-fixture-value\n"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "public":
				writeSecrets(t, dir, name+"=private-fixture-value\n")
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				writeSecrets(t, dir, name+"="+strings.Repeat("x", maxSecretsBytes)+"\n")
			case "syntax":
				writeSecrets(t, dir, name+"=private-fixture-value\ninvalid assignment\n")
			}
			err := loadSecrets(filepath.Join(dir, "config.yaml"), []string{name})
			if err == nil || strings.Contains(err.Error(), "private-fixture-value") {
				t.Fatalf("expected redacted error, got %v", err)
			}
			if _, exists := os.LookupEnv(name); exists {
				t.Fatal("imported a credential before file validation completed")
			}
		})
	}
}

func TestCLISecretsForRunAndCheck(t *testing.T) {
	const name = "PI_MONITOR_TEST_HTTP_FILE_AUTH"
	for _, command := range []string{"run", "check"} {
		t.Run(command, func(t *testing.T) {
			unsetCredential(t, name)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Header.Get("Authorization") != "Bearer fixture-secret" {
					t.Error("request did not use the file credential")
				}
				w.WriteHeader(http.StatusOK)
				if command == "run" {
					cancel()
				}
			}))
			defer server.Close()
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			contents := fmt.Sprintf(`host:
  enabled: false
dashboard:
  enabled: false
http_checks:
  - id: api
    url: %q
    interval: "30s"
    headers:
      Authorization: {env: %q}
`, server.URL, name)
			if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
			writeSecrets(t, dir, name+"='Bearer fixture-secret'\n")
			args := []string{command, "--config", path}
			if command == "check" {
				args = append(args, "api")
			}
			var out, diagnostics bytes.Buffer
			if code := run(ctx, args, &out, &diagnostics); code != 0 {
				t.Fatalf("%s: code %d: %s", command, code, &diagnostics)
			}
			if requests.Load() == 0 {
				t.Fatal("no local check was made")
			}
			if strings.Contains(out.String()+diagnostics.String(), "fixture-secret") {
				t.Fatal("credential exposed in command output")
			}
		})
	}
}

func TestCLILoadsTelegramTokenBeforeRuntimeWithoutContactingTelegram(t *testing.T) {
	const name = "PI_MONITOR_TEST_TELEGRAM_FILE_TOKEN"
	unsetCredential(t, name)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// The unsupported feature guard returns before constructing a Telegram client.
	contents := "docker:\n  enabled: true\ntelegram:\n  enabled: true\n  token_env: " + name + "\n  allowed_chat_ids: [12]\n  allowed_user_ids: [34]\n  alert_chat_ids: [12]\n"
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	writeSecrets(t, dir, name+"='123:fixture-secret'\n")
	var out, diagnostics bytes.Buffer
	code := run(context.Background(), []string{"run", "--config", path}, &out, &diagnostics)
	if code != 1 || !strings.Contains(diagnostics.String(), "not implemented") {
		t.Fatalf("expected unsupported runtime rejection, got %d: %s", code, &diagnostics)
	}
	if os.Getenv(name) != "123:fixture-secret" {
		t.Fatal("Telegram token was not imported")
	}
	if strings.Contains(out.String()+diagnostics.String(), "fixture-secret") {
		t.Fatal("Telegram token exposed in output")
	}
}

func TestCLIInvalidSecretsFailBeforeRequestsButValidationStaysOffline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request made with invalid secrets file")
	}))
	defer server.Close()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := fmt.Sprintf("http_checks:\n  - id: api\n    url: %q\n    interval: 30s\n", server.URL)
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	writeSecrets(t, dir, "private-fixture-value\n")
	for _, command := range []string{"run", "check", "validate"} {
		args := []string{command, "--config", path}
		if command == "check" {
			args = append(args, "api")
		}
		var out, diagnostics bytes.Buffer
		code := run(context.Background(), args, &out, &diagnostics)
		want := 2
		if command == "validate" {
			want = 0
		}
		if code != want {
			t.Fatalf("%s: got %d, want %d: %s", command, code, want, &diagnostics)
		}
		if strings.Contains(out.String()+diagnostics.String(), "private-fixture-value") {
			t.Fatal("secrets contents exposed in output")
		}
	}
}

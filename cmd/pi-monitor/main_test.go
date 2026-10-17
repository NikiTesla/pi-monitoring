package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedConfigurationCanBeValidated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	var out, diagnostics bytes.Buffer
	if code := run(context.Background(), []string{"init", "--output", path, "--no-dashboard"}, &out, &diagnostics); code != 0 {
		t.Fatalf("init: code %d: %s", code, &diagnostics)
	}
	out.Reset()
	if code := run(context.Background(), []string{"validate", "--config", path}, &out, &diagnostics); code != 0 {
		t.Fatalf("validate: code %d: %s", code, &diagnostics)
	}
	if !strings.Contains(out.String(), "valid") {
		t.Fatal("missing validation confirmation")
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if code := run(context.Background(), []string{"init", "--output", path}, &out, &diagnostics); code == 0 {
		t.Fatal("second init unexpectedly overwrote configuration")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("existing configuration changed")
	}
}

func TestCheckExitAndSecretRedaction(t *testing.T) {
	const secret = "private-expected-value"
	t.Setenv("TEST_HTTP_TOKEN", secret)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != secret {
			t.Error("missing configured header")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ready": false}`)
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := fmt.Sprintf(`schema_version: 1
http_checks:
  - id: api
    url: %q
    interval: "30s"
    headers:
      Authorization: {env: "TEST_HTTP_TOKEN"}
    expect:
      status: 200
      json:
        - pointer: "/ready"
          operator: "eq"
          value: true
`, srv.URL)
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"check", "api", "--config", path}, {"check", "--config", path, "api"}} {
		var out, diagnostics bytes.Buffer
		if code := run(context.Background(), args, &out, &diagnostics); code != 1 {
			t.Fatalf("expected failed check, got %d: %s %s", code, &out, &diagnostics)
		}
		if strings.Contains(out.String()+diagnostics.String(), secret) {
			t.Fatal("credential exposed")
		}
		if out.Len() == 0 {
			t.Fatal("missing structured check result")
		}
	}
}

func TestInvalidInitFlagsAreUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"init", "--start"},
		{"init", "--start", "--dry-run"},
		{"init", "--install", "--output", "unused.yaml", "--dry-run"},
	} {
		var out, diagnostics bytes.Buffer
		if code := run(context.Background(), args, &out, &diagnostics); code != 2 {
			t.Fatalf("%v: expected usage error, got %d: %s", args, code, &diagnostics)
		}
		if out.Len() != 0 {
			t.Fatalf("reported actions for invalid request: %s", &out)
		}
	}
}

func TestMissingHeaderCredentialsAreConfigurationErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request made without required credentials")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := fmt.Sprintf(`http_checks:
  - id: api
    url: %q
    interval: "30s"
    headers:
      Authorization: {env: "TEST_PI_MONITOR_REQUIRED_HEADER"}
`, srv.URL)
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprintf("unset=%t", missing), func(t *testing.T) {
			t.Setenv("TEST_PI_MONITOR_REQUIRED_HEADER", "")
			if missing {
				if err := os.Unsetenv("TEST_PI_MONITOR_REQUIRED_HEADER"); err != nil {
					t.Fatal(err)
				}
			}
			var out, diagnostics bytes.Buffer
			if code := run(context.Background(), []string{"check", "api", "--config", path}, &out, &diagnostics); code != 2 {
				t.Fatalf("expected configuration exit 2, got %d: %s %s", code, &out, &diagnostics)
			}
			var result struct {
				Category string `json:"category"`
				Success  bool   `json:"success"`
			}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.Category != "configuration" || result.Success {
				t.Fatalf("expected structured configuration failure, got %s: %v", &out, err)
			}
		})
	}
}

func TestRunRejectsUnsupportedFeaturesBeforeNetwork(t *testing.T) {
	for _, feature := range []string{
		"docker:\n  enabled: true\n",
		"heartbeats:\n  - id: job\n    max_age: 1h\n    initial_grace: 1h\n    token_env: JOB_TOKEN\n",
	} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(feature), 0600); err != nil {
			t.Fatal(err)
		}
		var out, diagnostics bytes.Buffer
		if code := run(context.Background(), []string{"run", "--config", path}, &out, &diagnostics); code != 1 {
			t.Fatalf("expected runtime rejection, got %d: %s", code, &diagnostics)
		}
		if !strings.Contains(diagnostics.String(), "not implemented") {
			t.Fatalf("missing feature diagnostic: %s", &diagnostics)
		}
	}
}

func TestRunStopsOnCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("host:\n  enabled: false\ndashboard:\n  enabled: false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, diagnostics bytes.Buffer
	if code := run(ctx, []string{"run", "--config", path}, &out, &diagnostics); code != 0 {
		t.Fatalf("expected graceful stop, got %d: %s", code, &diagnostics)
	}
}

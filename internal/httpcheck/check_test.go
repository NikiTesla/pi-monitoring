package httpcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NikiTesla/pi-monitoring/internal/config"
	"gopkg.in/yaml.v3"
)

func assertion(t *testing.T, path, operator, value string) config.JSONAssertion {
	t.Helper()
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(value), &node); err != nil {
		t.Fatal(err)
	}
	return config.JSONAssertion{Pointer: path, Operator: operator, Value: *node.Content[0]}
}
func checkConfig(url string) config.HTTPCheck {
	return config.HTTPCheck{ID: "test", URL: url, Timeout: config.Duration(time.Second), Expectations: config.Expectations{Status: 200}}
}

func TestTypedAssertions(t *testing.T) {
	tests := []struct {
		name, body, path, op, value string
		success                     bool
		category                    string
	}{
		{"large exact", "{\"n\":9007199254740993}", "/n", "eq", "9007199254740993", true, "ok"},
		{"large differs", "{\"n\":9007199254740993}", "/n", "eq", "9007199254740992", false, "value"},
		{"decimal exponent", "{\"n\":1e-3}", "/n", "eq", "0.001", true, "ok"},
		{"numeric equality", "{\"n\":1}", "/n", "eq", "1.0", true, "ok"},
		{"numeric ordering", "{\"n\":10}", "/n", "gte", "9.99", true, "ok"},
		{"string differs", "{\"n\":\"true\"}", "/n", "eq", "true", false, "type"},
		{"null exists", "{\"n\":null}", "/n", "exists", "true", true, "ok"},
		{"null equals", "{\"n\":null}", "/n", "eq", "null", true, "ok"},
		{"missing ne", "{}", "/n", "ne", "1", false, "missing"},
		{"missing absent", "{}", "/n", "exists", "false", true, "ok"},
		{"escaped key", "{\"a/b\":{\"~n\":[true]}}", "/a~1b/~0n/0", "eq", "true", true, "ok"},
		{"bad escape", "{}", "/~2", "eq", "true", false, "configuration"},
		{"array leading zero", "[true]", "/00", "eq", "true", false, "missing"},
		{"duplicate", "{\"n\":1,\"n\":2}", "/n", "eq", "2", false, "body"},
		{"nested duplicate", "{\"n\":{\"x\":1,\"x\":2}}", "/n/x", "eq", "2", false, "body"},
		{"invalid UTF-8 value", "{\"n\":true,\"other\":\"\xff\"}", "/n", "eq", "true", false, "body"},
		{"invalid UTF-8 key", "{\"n\":true,\"\xff\":0}", "/n", "eq", "true", false, "body"},
		{"valid Unicode", "{\"n\":\"healthy \u2713 \ufffd\"}", "/n", "eq", "\"healthy \u2713 \ufffd\"", true, "ok"},
		{"trailing", "{\"n\":1} {}", "/n", "eq", "1", false, "body"},
		{"huge exponent", "{\"n\":1e999999999}", "/n", "eq", "1", false, "body"},
		{"deep JSON", strings.Repeat("[", 130) + "true" + strings.Repeat("]", 130), "", "exists", "true", false, "body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tt.body) }))
			defer server.Close()
			c := checkConfig(server.URL)
			c.Expectations.JSON = []config.JSONAssertion{assertion(t, tt.path, tt.op, tt.value)}
			result := Run(t.Context(), c)
			if result.Success != tt.success || result.Category != tt.category {
				t.Fatalf("unexpected result: %+v", result)
			}
		})
	}
}

func TestTimeoutAndBodyLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(time.Second):
				return
			}
		}
		fmt.Fprint(w, "12345")
	}))
	defer server.Close()
	c := checkConfig(server.URL + "/slow")
	c.Timeout = config.Duration(20 * time.Millisecond)
	if r := Run(t.Context(), c); r.Success || r.Category != "timeout" {
		t.Fatalf("timeout result: %+v", r)
	}
	c = checkConfig(server.URL)
	c.MaxBodyBytes = 4
	c.Expectations.Text = []config.TextAssertion{{Operator: "contains", Value: "1"}}
	if r := Run(t.Context(), c); r.Success || r.Category != "body" {
		t.Fatalf("limit result: %+v", r)
	}
	c.Expectations.Text = nil
	if r := Run(t.Context(), c); !r.Success {
		t.Fatalf("status-only unexpectedly read body: %+v", r)
	}
}

func TestRedirectSecrets(t *testing.T) {
	const secret = "do-not-disclose-this"
	var mu sync.Mutex
	var gotHeader, gotReferer string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotHeader = r.Header.Get("X-Custom-Secret")
		gotReferer = r.Header.Get("Referer")
		mu.Unlock()
		fmt.Fprint(w, "ok")
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer origin.Close()
	c := checkConfig(origin.URL + "/?token=" + secret)
	c.Headers = map[string]config.RequestHeader{"X-Custom-Secret": {Value: secret}}
	if r := Run(t.Context(), c); r.Success || r.StatusCode != 302 {
		t.Fatalf("default redirect behavior: %+v", r)
	}
	c.FollowRedirects = true
	if r := Run(t.Context(), c); !r.Success {
		t.Fatalf("follow redirect failed: %+v", r)
	}
	mu.Lock()
	leaked := gotHeader != "" || gotReferer != ""
	mu.Unlock()
	if leaked {
		t.Fatal("custom secret or Referer crossed origin")
	}
	c = checkConfig("http://127.0.0.1:1/path?token=" + secret)
	r := Run(t.Context(), c)
	serialized, _ := json.Marshal(r)
	if strings.Contains(string(serialized), secret) || strings.Contains(string(serialized), "127.0.0.1") {
		t.Fatal("request details leaked")
	}
	c = checkConfig(origin.URL)
	c.Headers = map[string]config.RequestHeader{"X-Token": {EnvVar: "PI_MONITOR_TEST_UNSET_ENV"}}
	t.Setenv("PI_MONITOR_TEST_UNSET_ENV", secret)
	if r := Run(t.Context(), c); r.Category == "configuration" {
		t.Fatal("set variable rejected")
	}
	t.Setenv("PI_MONITOR_TEST_UNSET_ENV", "")
	if r := Run(t.Context(), c); r.Category != "configuration" {
		t.Fatal("empty credential environment variable accepted")
	}
	c.Headers = map[string]config.RequestHeader{"X-Token": {EnvVar: "PI_MONITOR_TEST_NEVER_DEFINED"}}
	if r := Run(t.Context(), c); r.Category != "configuration" {
		t.Fatal("missing environment variable accepted")
	}
}

func TestTLSVerificationAndCancellation(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") }))
	defer server.Close()
	c := checkConfig(server.URL)
	// Allow cold system trust-root loading on busy runners; this tests rejection,
	// while request deadlines have their own short-timeout fixture.
	c.Timeout = config.Duration(5 * time.Second)
	if r := Run(t.Context(), c); r.Success || r.Category != "connection" {
		t.Fatalf("untrusted TLS request = %+v, want connection failure", r)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if r := Run(ctx, c); r.Success || r.Category != "canceled" {
		t.Fatalf("canceled context not honored: %+v", r)
	}
}

func TestInvalidHeaderCredentialsDoNotSendRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	for _, value := range []string{"secret\rvalue", "secret\nvalue", "secret\x01value", "secret\x7fvalue"} {
		t.Setenv("PI_MONITOR_TEST_HEADER", value)
		check := checkConfig(server.URL)
		check.Headers = map[string]config.RequestHeader{"Authorization": {EnvVar: "PI_MONITOR_TEST_HEADER"}}
		result := Run(t.Context(), check)
		if result.Success || result.Category != "configuration" {
			t.Fatalf("invalid credential result: %+v", result)
		}
		serialized, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(serialized), "secret") {
			t.Fatal("credential leaked into result")
		}
	}
	if requests.Load() != 0 {
		t.Fatal("invalid credentials sent a request")
	}
}

func TestTextAndStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503); fmt.Fprint(w, "ready\n") }))
	defer server.Close()
	c := checkConfig(server.URL)
	c.Expectations.Text = []config.TextAssertion{{Operator: "eq", Value: "ready"}}
	r := Run(t.Context(), c)
	if r.Success || r.Category != "status" || len(r.Assertions) != 2 || r.Assertions[1].Success {
		t.Fatalf("unexpected result: %+v", r)
	}
	c.Expectations.Status = 503
	c.Expectations.Text[0] = config.TextAssertion{Operator: "contains", Value: "ready"}
	if r := Run(t.Context(), c); !r.Success {
		t.Fatalf("contains failed: %+v", r)
	}
}

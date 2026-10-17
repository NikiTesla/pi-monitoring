package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

func fixtureClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := New("test-token")
	client.baseURL = server.URL
	return client
}

func TestUpdatesDecodeAndPolling(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/bottest-token/getUpdates" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var input struct {
			Offset         int64
			Timeout, Limit int
			AllowedUpdates []string `json:"allowed_updates"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		if input.Offset != 13 || input.Timeout != 25 || input.Limit != 100 || len(input.AllowedUpdates) != 1 || input.AllowedUpdates[0] != "message" {
			t.Errorf("poll parameters: %+v", input)
		}
		io.WriteString(w, `{"ok":true,"result":[{"update_id":13,"message":{"message_id":5,"date":1800000000,"from":{"id":7,"is_bot":false},"chat":{"id":-9},"text":"/status"}},{"update_id":14}]}`)
	})
	updates, err := client.Updates(context.Background(), 13)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 2 || updates[0].ID != 13 || updates[0].Message.ID != 5 || updates[0].Message.Date != 1800000000 || updates[0].Message.From.ID != 7 || updates[0].Message.Chat.ID != -9 || updates[0].Message.Text != "/status" || updates[1].Message != nil {
		t.Fatalf("updates: %+v", updates)
	}
}

func TestGetMe(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottest-token/getMe" {
			t.Errorf("path = %s", r.URL.Path)
		}
		io.WriteString(w, `{"ok":true,"result":{"id":7,"is_bot":true,"username":"fixture_bot"}}`)
	})
	user, err := client.GetMe(context.Background())
	if err != nil || user.ID != 7 || user.Username != "fixture_bot" {
		t.Fatalf("user = %+v, error = %v", user, err)
	}
}

func TestSetCommandMenu(t *testing.T) {
	var methods []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.URL.Path)
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request method or content type")
		}
		var input map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		switch r.URL.Path {
		case "/bottest-token/setMyCommands":
			if len(input) != 1 || string(input["commands"]) != `[{"command":"status","description":"Server overview"}]` {
				t.Errorf("command registration: %s", input)
			}
		case "/bottest-token/setChatMenuButton":
			if len(input) != 1 || string(input["menu_button"]) != `{"type":"commands"}` {
				t.Errorf("menu registration: %s", input)
			}
		default:
			t.Errorf("unexpected API method: %s", r.URL.Path)
		}
		io.WriteString(w, `{"ok":true,"result":true}`)
	})
	err := client.SetCommandMenu(context.Background(), []BotCommand{{Command: "status", Description: "Server overview"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(methods, ",") != "/bottest-token/setMyCommands,/bottest-token/setChatMenuButton" {
		t.Fatalf("registration sequence = %v", methods)
	}
}

func TestCommandMenuFailures(t *testing.T) {
	for _, method := range []string{"setMyCommands", "setChatMenuButton"} {
		for _, body := range []string{
			`{"ok":true,"result":false}`,
			`{"ok":true}`,
			`{"ok":false,"description":"test-token private detail"}`,
			`{"ok":false,"error_code":429,"parameters":{"retry_after":17}}`,
		} {
			t.Run(method+body, func(t *testing.T) {
				var calls int
				client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.URL.Path == "/bottest-token/"+method {
						io.WriteString(w, body)
						return
					}
					io.WriteString(w, `{"ok":true,"result":true}`)
				})
				err := client.SetCommandMenu(context.Background(), []BotCommand{{Command: "help", Description: "Help"}})
				if err == nil || strings.Contains(err.Error(), "test-token") || strings.Contains(err.Error(), "private detail") {
					t.Fatalf("unsafe or missing error: %v", err)
				}
				if strings.Contains(body, "429") {
					var limited *RetryAfter
					if !errors.As(err, &limited) || limited.Delay != 17*time.Second {
						t.Fatalf("rate limit = %v", err)
					}
				}
				if method == "setMyCommands" && calls != 1 {
					t.Fatalf("continued after failed command registration: %d calls", calls)
				}
			})
		}
	}
}

func TestSendPlainBoundedText(t *testing.T) {
	for _, text := range []string{"<b>plain & text</b>", strings.Repeat("界", 5000), strings.Repeat("x", 20000), "invalid\xffUTF8"} {
		t.Run(text[:min(len(text), 16)], func(t *testing.T) {
			client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				var input map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
				}
				var message string
				json.Unmarshal(input["text"], &message)
				if !utf8.ValidString(message) || utf8.RuneCountInString(message) > 4096 {
					t.Errorf("invalid message length %d", utf8.RuneCountInString(message))
				}
				if len(text) > 16000 && !strings.HasSuffix(message, "…") {
					t.Error("truncated message lacks ellipsis")
				}
				if text == "<b>plain & text</b>" && message != text {
					t.Errorf("plain text changed: %q", message)
				}
				if _, ok := input["parse_mode"]; ok {
					t.Error("unexpected parse mode")
				}
				if string(input["link_preview_options"]) != `{"is_disabled":true}` {
					t.Error("link previews enabled")
				}
				io.WriteString(w, `{"ok":true,"result":{"message_id":1}}`)
			})
			if err := client.Send(context.Background(), 7, text); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestResponseFailuresDoNotExposeSecrets(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{"API rejection", 200, `{"ok":false,"description":"test-token private response"}`},
		{"HTTP error", 500, `{"ok":false,"description":"test-token"}`},
		{"invalid JSON", 200, "test-token"},
		{"missing result", 200, `{"ok":true}`},
		{"null result", 200, `{"ok":true,"result":null}`},
		{"wrong result type", 200, `{"ok":true,"result":{}}`},
		{"oversized", 200, strings.Repeat("x", maxResponseBytes+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(test.status); io.WriteString(w, test.body) })
			_, err := client.Updates(context.Background(), 0)
			if err == nil || strings.Contains(err.Error(), "test-token") || strings.Contains(err.Error(), "private response") {
				t.Fatalf("unsafe or missing error: %v", err)
			}
		})
	}
}

func TestRateLimit(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"ok":false,"parameters":{"retry_after":17}}`)
	})
	err := client.Send(context.Background(), 7, "fixture")
	var rateLimit *RetryAfter
	if !errors.As(err, &rateLimit) || rateLimit.Delay != 17*time.Second {
		t.Fatalf("rate limit = %v", err)
	}
}

func TestRedirectIsNotFollowed(t *testing.T) {
	var followed atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed.Store(true) }))
	defer target.Close()
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	})
	if err := client.Send(context.Background(), 7, "fixture"); err == nil {
		t.Fatal("accepted redirect")
	}
	if followed.Load() {
		t.Fatal("followed redirect")
	}
}

func TestCancellationAndTransportRedaction(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Consume the request so the server can observe a disconnected client.
		io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := client.Updates(ctx, 0); finished <- err }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not finish")
	}
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	client.baseURL = server.URL
	server.Close()
	_, err := client.GetMe(context.Background())
	if err == nil || strings.Contains(err.Error(), "test-token") {
		t.Fatalf("transport error = %v", err)
	}
}

func TestInvalidRequestsStayLocal(t *testing.T) {
	client := New("test-token")
	client.baseURL = ":invalid"
	if _, err := client.Updates(context.Background(), -1); err == nil {
		t.Error("accepted negative offset")
	}
	if err := client.Send(context.Background(), 0, "fixture"); err == nil {
		t.Error("accepted missing destination")
	}
	if err := client.Send(context.Background(), 7, " "); err == nil {
		t.Error("accepted blank message")
	}
	client.token = "token/other-path"
	if _, err := client.GetMe(context.Background()); err == nil || strings.Contains(err.Error(), client.token) {
		t.Fatalf("unsafe credential error: %v", err)
	}
}

// Package telegram provides the small Bot API surface used by the monitor.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const maxResponseBytes = 2 << 20

type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

// New creates a client for Telegram's public HTTPS API. It does not make requests.
func New(token string) *Client {
	return &Client{
		token:   token,
		baseURL: "https://api.telegram.org",
		http: &http.Client{
			Timeout:       35 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

type Update struct {
	ID      int64    `json:"update_id"`
	Message *Message `json:"message"`
}

type Message struct {
	SenderChat *Chat  `json:"sender_chat"`
	ID         int64  `json:"message_id"`
	Date       int64  `json:"date"`
	From       *User  `json:"from"`
	Chat       Chat   `json:"chat"`
	Text       string `json:"text"`
}

type User struct {
	ID       int64  `json:"id"`
	IsBot    bool   `json:"is_bot"`
	Username string `json:"username"`
}

type Chat struct {
	ID int64 `json:"id"`
}

// RetryAfter reports Telegram's requested delay after a rate limit.
type RetryAfter struct {
	Delay time.Duration
}

func (e *RetryAfter) Error() string { return "telegram: rate limited" }

// GetMe identifies the bot, including its username for addressed commands.
func (c *Client) GetMe(ctx context.Context) (User, error) {
	var user User
	err := c.call(ctx, "getMe", struct{}{}, &user)
	if err == nil && (user.ID <= 0 || !user.IsBot || user.Username == "") {
		err = errors.New("telegram: invalid bot identity")
	}
	return user, err
}

type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// SetCommandMenu replaces the default command list and makes the default private
// chat menu button open that list. Chat/language-specific overrides are preserved.
func (c *Client) SetCommandMenu(ctx context.Context, commands []BotCommand) error {
	request := struct {
		Commands []BotCommand `json:"commands"`
	}{Commands: commands}
	var accepted bool
	if err := c.call(ctx, "setMyCommands", request, &accepted); err != nil {
		return err
	}
	if !accepted {
		return errors.New("telegram: command list was not accepted")
	}
	menu := struct {
		MenuButton struct {
			Type string `json:"type"`
		} `json:"menu_button"`
	}{}
	menu.MenuButton.Type = "commands"
	if err := c.call(ctx, "setChatMenuButton", menu, &accepted); err != nil {
		return err
	}
	if !accepted {
		return errors.New("telegram: command menu button was not accepted")
	}
	return nil
}

// Updates waits up to 25 seconds for at most 100 new message updates. The caller
// owns the offset and must advance it only after handling the returned updates.
func (c *Client) Updates(ctx context.Context, offset int64) ([]Update, error) {
	if offset < 0 {
		return nil, errors.New("telegram: negative update offset")
	}
	request := struct {
		Offset         int64    `json:"offset"`
		Timeout        int      `json:"timeout"`
		Limit          int      `json:"limit"`
		AllowedUpdates []string `json:"allowed_updates"`
	}{offset, 25, 100, []string{"message"}}
	var updates []Update
	if err := c.call(ctx, "getUpdates", request, &updates); err != nil {
		return nil, err
	}
	if len(updates) > 100 {
		return nil, errors.New("telegram: too many updates")
	}
	return updates, nil
}

// Send sends plain text without link previews. Longer text is truncated to
// Telegram's 4096-character limit; an ellipsis marks the omitted suffix.
func (c *Client) Send(ctx context.Context, chatID int64, text string) error {
	// No valid 4096-character message needs more than this many UTF-8 bytes.
	if len(text) > 4096*utf8.UTFMax {
		text = text[:4096*utf8.UTFMax] + "…"
	}
	text = strings.ToValidUTF8(text, "�")
	if strings.TrimSpace(text) == "" || chatID == 0 {
		return errors.New("telegram: message and destination are required")
	}
	if utf8.RuneCountInString(text) > 4096 {
		for i, count := 0, 0; i < len(text); count++ {
			if count == 4095 {
				text = text[:i] + "…"
				break
			}
			_, size := utf8.DecodeRuneInString(text[i:])
			i += size
		}
	}
	request := struct {
		ChatID      int64  `json:"chat_id"`
		Text        string `json:"text"`
		LinkPreview struct {
			Disabled bool `json:"is_disabled"`
		} `json:"link_preview_options"`
	}{ChatID: chatID, Text: text}
	request.LinkPreview.Disabled = true
	return c.call(ctx, "sendMessage", request, nil)
}

func (c *Client) call(ctx context.Context, method string, input, result any) error {
	// Reject path/query delimiters before including the credential in a URL.
	if c.token == "" || len(c.token) > 256 || strings.IndexFunc(c.token, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == ':' || r == '_' || r == '-')
	}) >= 0 {
		return errors.New("telegram: invalid bot credential")
	}
	body, err := json.Marshal(input)
	if err != nil {
		return errors.New("telegram: encode request")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/bot"+c.token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return errors.New("telegram: create request")
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		// net/http errors include the token-bearing URL. Preserve cancellation
		// identity without exposing the transport error or credential.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("telegram: request failed")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("telegram: read response")
	}
	if len(data) > maxResponseBytes {
		return errors.New("telegram: response too large")
	}
	var envelope struct {
		OK         bool            `json:"ok"`
		Result     json.RawMessage `json:"result"`
		ErrorCode  int             `json:"error_code"`
		Parameters struct {
			RetryAfter int64 `json:"retry_after"`
		} `json:"parameters"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return errors.New("telegram: invalid response")
	}
	if response.StatusCode == http.StatusTooManyRequests || envelope.ErrorCode == http.StatusTooManyRequests {
		// Bound untrusted duration conversion and prevent a tight retry loop.
		seconds := min(max(envelope.Parameters.RetryAfter, 1), 86400)
		return &RetryAfter{Delay: time.Duration(seconds) * time.Second}
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram: HTTP status %d", response.StatusCode)
	}
	if !envelope.OK {
		return errors.New("telegram: API rejected request")
	}
	if result != nil {
		if len(envelope.Result) == 0 || bytes.Equal(envelope.Result, []byte("null")) {
			return errors.New("telegram: missing result")
		}
		if err := json.Unmarshal(envelope.Result, result); err != nil {
			return errors.New("telegram: invalid result")
		}
	}
	return nil
}

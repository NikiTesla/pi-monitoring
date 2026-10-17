// Package httpcheck evaluates one configured HTTP check without notifications.
package httpcheck

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"pi-monitoring/internal/config"
)

// Result describes one completed evaluation, including any sanitized failure.
type Result struct {
	ID         string            `json:"id"`
	StartedAt  time.Time         `json:"time"`
	Success    bool              `json:"success"`
	StatusCode int               `json:"status_code,omitempty"`
	ElapsedMS  int64             `json:"latency_ms"`
	Category   string            `json:"category"`
	Reason     string            `json:"reason"`
	Assertions []AssertionResult `json:"assertions"`
}

// Assertion results identify a configured assertion by target and index. Values
// and pointers are deliberately omitted because they may contain credentials.
type AssertionResult struct {
	Target   string `json:"target"`
	Index    int    `json:"index"`
	Success  bool   `json:"success"`
	Category string `json:"category"`
	Reason   string `json:"reason"`
}

// Run executes a previously validated check. It never includes response bodies,
// request URLs, header values or underlying network errors in diagnostics.
func Run(ctx context.Context, check config.HTTPCheck) (result Result) {
	start := time.Now()
	result = Result{ID: check.ID, StartedAt: start.UTC(), Category: "ok", Reason: "all assertions passed", Assertions: []AssertionResult{}}
	defer func() { result.ElapsedMS = time.Since(start).Milliseconds() }()
	fail := func(category, reason string) Result {
		result.Category = category
		result.Reason = reason
		return result
	}
	timeout := check.Timeout.Value()
	if timeout <= 0 {
		return fail("configuration", "positive timeout required")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	method := check.Method
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead {
		return fail("configuration", "unsupported request method")
	}
	request, err := http.NewRequestWithContext(ctx, method, check.URL, nil)
	if err != nil || (request.URL.Scheme != "http" && request.URL.Scheme != "https") || request.URL.Host == "" || request.URL.User != nil {
		return fail("configuration", "invalid HTTP request")
	}
	for name, header := range check.Headers {
		value := header.Value
		if header.EnvVar != "" {
			var ok bool
			value, ok = os.LookupEnv(header.EnvVar)
			if !ok || value == "" {
				return fail("configuration", "required header environment variable is missing")
			}
		}
		// Environment values bypass YAML validation. Reject invalid field bytes
		// before transport so credential errors remain configuration failures.
		for i := 0; i < len(value); i++ {
			if (value[i] < 0x20 && value[i] != '\t') || value[i] == 0x7f {
				return fail("configuration", "request header contains an invalid value")
			}
		}
		request.Header.Set(name, value)
	}
	client := newHTTPClient(check)
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fail("timeout", "request timed out")
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return fail("canceled", "request canceled")
		}
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() {
			return fail("timeout", "request timed out")
		}
		return fail("connection", "HTTP request failed")
	}
	defer response.Body.Close()
	result.StatusCode = response.StatusCode
	expected := check.Expectations.Status
	if expected == 0 {
		expected = 200
	}
	recordAssertion := func(assertion AssertionResult) {
		if assertion.Success {
			assertion.Category = "ok"
			assertion.Reason = "assertion passed"
		}
		result.Assertions = append(result.Assertions, assertion)
		if !assertion.Success && result.Category == "ok" {
			result.Category = assertion.Category
			result.Reason = assertion.Reason
		}
	}
	recordAssertion(AssertionResult{Target: "status", Success: response.StatusCode == expected, Category: "status", Reason: "unexpected status"})
	if len(check.Expectations.Text) == 0 && len(check.Expectations.JSON) == 0 {
		result.Success = result.Category == "ok"
		return result
	}
	limit := check.MaxBodyBytes
	if limit <= 0 {
		limit = config.DefaultHTTPBodyLimitBytes
	}
	if limit > config.MaxHTTPBodyLimitBytes {
		return fail("configuration", "body limit exceeds maximum")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fail("timeout", "response body timed out")
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return fail("canceled", "request canceled")
		}
		return fail("body", "response body could not be read")
	}
	if int64(len(body)) > limit {
		return fail("body", "response body exceeds configured limit")
	}
	for i, assertion := range check.Expectations.Text {
		pass := false
		switch assertion.Operator {
		case "eq":
			pass = bytes.Equal(body, []byte(assertion.Value))
		case "contains":
			pass = bytes.Contains(body, []byte(assertion.Value))
		}
		recordAssertion(AssertionResult{Target: "text", Index: i, Success: pass, Category: "value", Reason: "text assertion did not match"})
	}
	if len(check.Expectations.JSON) > 0 {
		value, err := parseJSON(body)
		if err != nil {
			return fail("body", "response is not valid unambiguous JSON")
		}
		for i, assertion := range check.Expectations.JSON {
			category, reason := evaluateJSONAssertion(value, assertion)
			recordAssertion(AssertionResult{Target: "json", Index: i, Success: category == "ok", Category: category, Reason: reason})
		}
	}
	result.Success = result.Category == "ok"
	return result
}

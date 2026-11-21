package main

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	crmkit "github.com/sedyukov/sedlab-modules/modules/crm/go/kit"
	notifykit "github.com/sedyukov/sedlab-modules/modules/notify/go/kit"
	paykit "github.com/sedyukov/sedlab-modules/modules/payments/go/kit"
	sheetskit "github.com/sedyukov/sedlab-modules/modules/sheets-storage/go/kit"
)

// ErrInfo is the unified view of a module error (each module vendors its own kit, so the types differ).
type ErrInfo struct {
	Code         string `json:"code"`
	Message      string `json:"message"`
	Retryable    bool   `json:"retryable"`
	HTTPStatus   int    `json:"http_status,omitempty"`
	RetryAfterMs int64  `json:"retry_after_ms,omitempty"`
}

func classify(err error) ErrInfo {
	if err == nil {
		return ErrInfo{}
	}
	var a *crmkit.Error
	var b *paykit.Error
	var c *sheetskit.Error
	var d *notifykit.Error
	switch {
	case errors.As(err, &a):
		return ErrInfo{string(a.Code), a.Message, a.Retryable, a.HTTPStatus, a.RetryAfterMs}
	case errors.As(err, &b):
		return ErrInfo{string(b.Code), b.Message, b.Retryable, b.HTTPStatus, b.RetryAfterMs}
	case errors.As(err, &c):
		return ErrInfo{string(c.Code), c.Message, c.Retryable, c.HTTPStatus, c.RetryAfterMs}
	case errors.As(err, &d):
		return ErrInfo{string(d.Code), d.Message, d.Retryable, d.HTTPStatus, d.RetryAfterMs}
	case errors.Is(err, context.DeadlineExceeded):
		return ErrInfo{Code: "timeout", Message: err.Error(), Retryable: true}
	}
	return ErrInfo{Code: "internal", Message: err.Error()}
}

// RetryPolicy mirrors the modules' kit policy (exponential backoff + full jitter, Retry-After as a floor).
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Sleep       func(ctx context.Context, d time.Duration) error
}

var defaultRetry = RetryPolicy{MaxAttempts: 4, BaseDelay: 250 * time.Millisecond, MaxDelay: 3 * time.Second}

func (p RetryPolicy) delay(attempt int, info ErrInfo) time.Duration {
	backoff := p.BaseDelay << (attempt - 1)
	if backoff > p.MaxDelay {
		backoff = p.MaxDelay
	}
	d := backoff/2 + time.Duration(rand.Int64N(int64(backoff/2)+1)) // half fixed + half jitter: readable in the log
	if ra := time.Duration(info.RetryAfterMs) * time.Millisecond; ra > d {
		d = ra
	}
	return d
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// chaosPlan says which (simulated) failures a step hits on which attempt when "simulate failures" is on.
var chaosPlan = map[string][]ErrInfo{
	"crm.create_deal":    {{Code: "unavailable", Message: "503 Service Unavailable (simulated)", Retryable: true, HTTPStatus: 503}},
	"sheets.append_rows": {{Code: "rate_limited", Message: "429 Too Many Requests (simulated)", Retryable: true, HTTPStatus: 429, RetryAfterMs: 1200}},
	// notify: failures are produced by the notify module's own mock (metadata.mock_fail), not by the glue
	"notify.telegram": {{Code: "unavailable"}, {Code: "timeout"}},
	"crm.update_deal": {{Code: "rate_limited", Message: "429 Too Many Requests (simulated)", Retryable: true, HTTPStatus: 429, RetryAfterMs: 800}},
}

type chaosErr struct{ ErrInfo }

func (c chaosErr) Error() string { return c.Code + ": " + c.Message }

func classifyAny(err error) ErrInfo {
	var c chaosErr
	if errors.As(err, &c) {
		return c.ErrInfo
	}
	return classify(err)
}

type chaosCtxKey struct{}

// chaosCode returns the failure the module mock should simulate on this attempt ("" = none).
func chaosCode(ctx context.Context) string {
	c, _ := ctx.Value(chaosCtxKey{}).(string)
	return c
}

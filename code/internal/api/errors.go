package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// FetchError preserves upstream metadata needed by the worker retry policy.
type FetchError struct {
	HTTPStatus             int
	APICode                int
	APIMessage             string
	RetryAfter             time.Duration
	MayHaveReachedUpstream bool
	Cause                  error
}

func NewFetchError(httpStatus, apiCode int, message string, retryAfter time.Duration, mayHaveReachedUpstream bool) *FetchError {
	return &FetchError{
		HTTPStatus:             httpStatus,
		APICode:                apiCode,
		APIMessage:             message,
		RetryAfter:             retryAfter,
		MayHaveReachedUpstream: mayHaveReachedUpstream,
	}
}

func (e *FetchError) Error() string {
	return fmt.Sprintf("lingxing fetch: http=%d code=%d msg=%q", e.HTTPStatus, e.APICode, e.APIMessage)
}

func (e *FetchError) Unwrap() error { return e.Cause }

// parseRetryAfter accepts the RFC 7231 seconds and HTTP-date forms.
func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	if when.Before(now) {
		return 0, true
	}
	return when.Sub(now), true
}

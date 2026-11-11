package ping

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	// BodyMaxBytes is how much of a ping's body the Ping API keeps.
	BodyMaxBytes   = 10 * 1024
	DefaultTimeout = 30 * time.Second

	attemptTimeout    = 10 * time.Second
	defaultRetryDelay = time.Second
	maxResponseBytes  = 4 << 10
)

type Client struct {
	// HTTPClient sends the pings. Its redirect policy is ignored, since a
	// redirect could carry the body to another host.
	HTTPClient *http.Client
	UserAgent  string
	// Timeout bounds one ping, retries included. Zero is DefaultTimeout.
	Timeout time.Duration

	retryDelay time.Duration
}

// SendError is a ping that got no 2xx answer within its timeout.
type SendError struct {
	// Status is the last answer's status, or 0 when none came.
	Status int
	// Err is the last network error when no answer came. Nil means time
	// ran out while waiting for one.
	Err     error
	Timeout time.Duration
}

func (e *SendError) Error() string {
	switch {
	case e.Status != 0:
		return fmt.Sprintf("the server answered %d", e.Status)
	case e.Err != nil:
		return e.Err.Error()
	}
	return "no response within " + e.Timeout.String()
}

func (e *SendError) Unwrap() error { return e.Err }

func IsNotFound(err error) bool {
	e, ok := errors.AsType[*SendError](err)
	return ok && e.Status == http.StatusNotFound
}

func (c *Client) Start(ctx context.Context, u *URL) error {
	return c.send(ctx, u.endpoint("start"), nil)
}

// ExitCode sends code, which must be 0 to 255, with body as the ping's body.
func (c *Client) ExitCode(ctx context.Context, u *URL, code int, body []byte) error {
	return c.send(ctx, u.endpoint(strconv.Itoa(code)), body)
}

func (c *Client) Fail(ctx context.Context, u *URL, body []byte) error {
	return c.send(ctx, u.endpoint("fail"), body)
}

// send retries network failures, 5xx and 429 until the timeout. It returns
// ctx's error when ctx ends first, and a *SendError otherwise.
func (c *Client) send(ctx context.Context, endpoint string, body []byte) error {
	timeout := cmp.Or(c.Timeout, DefaultTimeout)
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	delay := cmp.Or(c.retryDelay, defaultRetryDelay)
	for {
		a := c.attempt(pingCtx, endpoint, body)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if a.err == nil && a.status >= 200 && a.status < 300 {
			return nil
		}
		failure := &SendError{Status: a.status, Err: a.err, Timeout: timeout}
		if !retryable(a.status) {
			return failure
		}
		wait := cmp.Or(a.retryAfter, delay)
		if until, ok := pingCtx.Deadline(); ok && time.Until(until) < wait {
			return failure
		}
		select {
		case <-time.After(wait):
		case <-pingCtx.Done():
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return failure
		}
		delay *= 2
	}
}

func retryable(status int) bool {
	return status == 0 || status == http.StatusTooManyRequests || status >= 500
}

type attempt struct {
	status     int
	retryAfter time.Duration
	// err is a network error. It is nil with status 0 when the attempt ran
	// out of time.
	err error
}

func (c *Client) attempt(ctx context.Context, endpoint string, body []byte) attempt {
	ctx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return attempt{err: errors.New("the ping URL is not valid")}
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	hc := *c.HTTPClient
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := hc.Do(req)
	if ctx.Err() != nil {
		if err == nil {
			res.Body.Close()
		}
		return attempt{}
	}
	if err != nil {
		if urlErr, ok := errors.AsType[*url.Error](err); ok {
			err = urlErr.Err
		}
		return attempt{err: err}
	}
	defer res.Body.Close()
	io.Copy(io.Discard, io.LimitReader(res.Body, maxResponseBytes))
	return attempt{status: res.StatusCode, retryAfter: retryAfter(res)}
}

// retryAfter reads a 429's Retry-After in seconds. Zero means none was given.
func retryAfter(res *http.Response) time.Duration {
	if res.StatusCode != http.StatusTooManyRequests {
		return 0
	}
	seconds, err := strconv.Atoi(res.Header.Get("Retry-After"))
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

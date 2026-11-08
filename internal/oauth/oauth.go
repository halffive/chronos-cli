// Package oauth runs the browser login against the Chronos API: the
// authorization code flow with PKCE, a loopback listener for the redirect,
// the token exchange and revocation. These protocol requests use form bodies
// and RFC 6749 errors, which the SDK does not send or parse.
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"chronos.sh/cli/internal/config"
	"chronos.sh/cli/internal/exit"
)

const ClientID = "chronos-cli"

const maxResponseBytes = 64 << 10

type Client struct {
	BaseURL string
	// HTTPClient sends the token and revoke requests. Its redirect policy is
	// ignored, since a redirect could carry the form body to another host.
	HTTPClient *http.Client
	UserAgent  string
}

type LoginOptions struct {
	// Scope is the space-separated scopes to ask for.
	Scope string
	// DeviceName names this machine on the consent page. Empty sends none.
	DeviceName string
	Timeout    time.Duration
	// OpenBrowser is called with the authorization URL once the listener is
	// ready.
	OpenBrowser func(authorizeURL string)
	// Ignored is called when the listener ignores a callback: a wrong state,
	// or one after the first.
	Ignored func()
}

type Token struct {
	AccessToken string
	// ExpiresIn is zero when the API sent no expiry.
	ExpiresIn time.Duration
	Scopes    []string
	User      config.User
	Account   config.Account
}

// Login opens the browser for the person to approve the login, waits for the
// redirect to the loopback listener and exchanges the code for a token. It
// returns when ctx ends or opts.Timeout passes, with the listener closed.
func (c *Client) Login(ctx context.Context, opts LoginOptions) (Token, error) {
	verifier := newVerifier()
	code, redirectURI, err := c.authorize(ctx, verifier, opts)
	if err != nil {
		return Token{}, err
	}
	return c.exchange(ctx, exchange{code: code, redirectURI: redirectURI, verifier: verifier, scope: opts.Scope})
}

func newVerifier() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

type authorizeRequest struct {
	redirectURI string
	state       string
	verifier    string
	opts        LoginOptions
}

func (c *Client) authorizeURL(r authorizeRequest) string {
	q := url.Values{
		"client_id":             {ClientID},
		"redirect_uri":          {r.redirectURI},
		"response_type":         {"code"},
		"code_challenge":        {challenge(r.verifier)},
		"code_challenge_method": {"S256"},
		"scope":                 {r.opts.Scope},
		"state":                 {r.state},
	}
	if r.opts.DeviceName != "" {
		q.Set("device_name", r.opts.DeviceName)
	}
	return c.endpoint("/oauth/authorize") + "?" + q.Encode()
}

func (c *Client) issuer() string {
	u, err := url.Parse(strings.TrimSpace(c.BaseURL))
	if err != nil || u.Host == "" {
		return c.BaseURL
	}
	scheme, host, port := strings.ToLower(u.Scheme), strings.ToLower(u.Hostname()), u.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return scheme + "://" + host
}

func (c *Client) endpoint(path string) string {
	return strings.TrimRight(strings.TrimSpace(c.BaseURL), "/") + path
}

type exchange struct {
	code        string
	redirectURI string
	verifier    string
	scope       string
}

type tokenResponse struct {
	AccessToken string         `json:"access_token"`
	TokenType   string         `json:"token_type"`
	ExpiresIn   int64          `json:"expires_in"`
	Scope       string         `json:"scope"`
	User        config.User    `json:"user"`
	Account     config.Account `json:"account"`
}

func (c *Client) exchange(ctx context.Context, e exchange) (Token, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {e.code},
		"redirect_uri":  {e.redirectURI},
		"client_id":     {ClientID},
		"code_verifier": {e.verifier},
	}
	status, body, err := c.post(ctx, "/oauth/token", form)
	if e, ok := errors.AsType[*exit.Error](err); ok && e.Code == exit.CodeNetwork {
		e.Message = fmt.Sprintf("Could not reach the API to finish the login (%v), so nothing was saved. Run chronos auth login again.", e.Cause)
	}
	if err != nil {
		return Token{}, err
	}
	if status != http.StatusOK {
		return Token{}, loginError(errorCode(body), status)
	}
	var res tokenResponse
	if err := json.Unmarshal(body, &res); err != nil || !res.usable() {
		if res.AccessToken != "" {
			c.RevokeBestEffort(ctx, res.AccessToken)
		}
		return Token{}, exit.Failuref("The API sent a token response chronos cannot use, so nothing was saved. Run chronos auth login again.")
	}
	scope := res.Scope
	if scope == "" {
		scope = e.scope
	}
	return Token{
		AccessToken: res.AccessToken,
		ExpiresIn:   time.Duration(res.ExpiresIn) * time.Second,
		Scopes:      strings.Fields(scope),
		User:        res.User,
		Account:     res.Account,
	}, nil
}

// maxExpiresIn keeps a time.Duration of expires_in seconds from overflowing.
const maxExpiresIn = 10 * 365 * 24 * 60 * 60

func (r tokenResponse) usable() bool {
	return strings.EqualFold(r.TokenType, "Bearer") &&
		r.AccessToken != "" && len(r.AccessToken) <= config.MaxSecretLength && printableASCII(r.AccessToken) &&
		r.ExpiresIn >= 0 && r.ExpiresIn <= maxExpiresIn &&
		r.User.Email != "" && r.Account.Name != ""
}

func printableASCII(s string) bool {
	for i := range len(s) {
		if s[i] < 0x21 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

// Revoke revokes the grant behind token (RFC 7009). The API answers 200
// whether or not the token was live, so nil means it no longer works.
func (c *Client) Revoke(ctx context.Context, token string) error {
	res, err := c.send(ctx, "/oauth/revoke", url.Values{"token": {token}, "client_id": {ClientID}})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes))
	if err != nil {
		return networkError(ctx, err)
	}
	return revokeError(errorCode(body), res.StatusCode)
}

const bestEffortTimeout = 5 * time.Second

// RevokeBestEffort revokes token and ignores the outcome. It runs even after
// ctx is cancelled, so a token chronos will not keep does not stay live.
func (c *Client) RevokeBestEffort(ctx context.Context, token string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), bestEffortTimeout)
	defer cancel()
	_ = c.Revoke(ctx, token)
}

func (c *Client) post(ctx context.Context, path string, form url.Values) (status int, body []byte, err error) {
	res, err := c.send(ctx, path, form)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	body, err = io.ReadAll(io.LimitReader(res.Body, maxResponseBytes))
	if err != nil {
		return 0, nil, networkError(ctx, err)
	}
	return res.StatusCode, body, nil
}

func (c *Client) send(ctx context.Context, path string, form url.Values) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(path), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	hc := *c.HTTPClient
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := hc.Do(req)
	if err != nil {
		return nil, networkError(ctx, err)
	}
	return res, nil
}

func networkError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		err = urlErr.Err
	}
	return &exit.Error{Exit: exit.Retryable, Code: exit.CodeNetwork, Message: fmt.Sprintf("Could not reach the API (%v).", err), Cause: err}
}
